package poller_service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/issue_session/issue_session_service"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/jira_client/jira_client_service"
	"jira-u-bot/modules/registration/registration_repository"
	"jira-u-bot/modules/reminder_engine/reminder_engine_service"
)

const pollInterval = 30 * time.Second

type pollerServiceImpl struct {
	jiraClient       jira_client_service.JiraClientService
	issueSession     issue_session_service.IssueSessionService
	businessHours    business_hours_service.BusinessHoursService
	reminderEngine   reminder_engine_service.ReminderEngineService
	registrationRepo registration_repository.RegistrationRepository
	holidayCalendar  business_hours_dto.HolidayCalendar
}

func NewPollerService(
	jiraClient jira_client_service.JiraClientService,
	issueSession issue_session_service.IssueSessionService,
	businessHours business_hours_service.BusinessHoursService,
	reminderEngine reminder_engine_service.ReminderEngineService,
	registrationRepo registration_repository.RegistrationRepository,
	holidayCalendar business_hours_dto.HolidayCalendar,
) PollerService {
	return &pollerServiceImpl{
		jiraClient:       jiraClient,
		issueSession:     issueSession,
		businessHours:    businessHours,
		reminderEngine:   reminderEngine,
		registrationRepo: registrationRepo,
		holidayCalendar:  holidayCalendar,
	}
}

// isEligible implements the team's decision to scope the bot down from
// "every CORE/OMS ticket in Code Process" to only tickets assigned to a
// registered user with a real Story Points budget — an unassigned ticket,
// an assignee who never used /register, or a ticket with no (or zero) SP
// carries nothing meaningful to track and must never reach the session/
// reminder pipeline at all (not even to show up as an instant overage).
func (p *pollerServiceImpl) isEligible(ctx context.Context, issue jira_client_dto.Issue) bool {
	if issue.Fields.StoryPoints == nil || *issue.Fields.StoryPoints <= 0 {
		return false
	}
	if issue.Fields.Assignee == nil {
		return false
	}
	_, found, err := p.registrationRepo.FindUserByEmail(ctx, issue.Fields.Assignee.EmailAddress)
	if err != nil {
		return false
	}
	return found
}

func (p *pollerServiceImpl) filterEligible(ctx context.Context, issues []jira_client_dto.Issue) []jira_client_dto.Issue {
	eligible := make([]jira_client_dto.Issue, 0, len(issues))
	for _, issue := range issues {
		if p.isEligible(ctx, issue) {
			eligible = append(eligible, issue)
		}
	}
	return eligible
}

func (p *pollerServiceImpl) Run(ctx context.Context) {
	if err := p.backfillActiveSessions(ctx); err != nil {
		log.Printf("poller: startup backfill failed: %v", err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(ctx); err != nil {
				log.Printf("poller: cycle failed: %v", err)
			}
		}
	}
}

func (p *pollerServiceImpl) pollOnce(ctx context.Context) error {
	issues, err := p.jiraClient.SearchCodeProcessIssues(ctx)
	if err != nil {
		return fmt.Errorf("poller: SearchCodeProcessIssues failed: %w", err)
	}
	issues = p.filterEligible(ctx, issues)

	currentByKey := make(map[string]jira_client_dto.Issue, len(issues))
	for _, issue := range issues {
		currentByKey[issue.Key] = issue
	}

	activeSessions, err := p.issueSession.GetActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("poller: GetActiveSessions failed: %w", err)
	}
	activeByKey := make(map[string]issue_session_dto.IssueSession, len(activeSessions))
	for _, s := range activeSessions {
		activeByKey[s.IssueKey] = s
	}

	// Exit events: DB still has it active, Jira no longer reports it as
	// "Code Process" this cycle.
	for key, session := range activeByKey {
		if _, stillThere := currentByKey[key]; stillThere {
			continue
		}
		additionalSeconds := int64(0)
		if session.CurrentSessionStartedDate != nil {
			additionalSeconds = p.businessHours.ElapsedBusinessSeconds(*session.CurrentSessionStartedDate, time.Now(), p.holidayCalendar)
		}
		totalElapsed := session.AccumulatedSeconds + additionalSeconds
		budgetSeconds := int64(session.StoryPoints * 3600)

		if err := p.issueSession.CloseSession(ctx, key, additionalSeconds); err != nil {
			return fmt.Errorf("poller: CloseSession(%s) failed: %w", key, err)
		}

		// Early-completion notice (spec AC4.1/AC4.4/AC4.5) is best-effort:
		// the session above is already closed by the time we get here, so
		// there is no natural retry hook if either step below fails — log
		// and move on to the next ticket rather than failing the cycle.
		status, err := p.jiraClient.GetIssueStatus(ctx, key)
		if err != nil {
			log.Printf("poller: GetIssueStatus(%s) failed, skipping early-completion notice: %v", key, err)
			continue
		}
		if status != jira_client_dto.StatusCodeReview {
			continue
		}
		if err := p.reminderEngine.NotifyIfEarlyCompletion(ctx, key, session.AssigneeEmail, session.IssueSummary, totalElapsed, budgetSeconds); err != nil {
			log.Printf("poller: NotifyIfEarlyCompletion(%s) failed: %v", key, err)
		}
	}

	// Entry events: new to the DB this cycle — open a fresh session at the
	// REAL Code Process entry time, not now. A ticket may have already been
	// in Code Process for a while before this is its first eligible cycle
	// (e.g. the assignee only just registered) — using CURRENT_TIMESTAMP
	// here would silently reset elapsed-time tracking to zero.
	for key, issue := range currentByKey {
		if _, alreadyActive := activeByKey[key]; alreadyActive {
			continue
		}
		entryTime, entryFound, err := p.jiraClient.GetLatestCodeProcessEntryTime(ctx, key)
		if err != nil {
			return fmt.Errorf("poller: GetLatestCodeProcessEntryTime(%s) failed: %w", key, err)
		}
		if !entryFound {
			entryTime = time.Now()
		}
		if err := p.issueSession.OpenOrResumeSessionAt(ctx, key, projectKeyFromIssueKey(key), storyPointsOf(issue), entryTime, assigneeEmailOf(issue), issue.Fields.Summary); err != nil {
			return fmt.Errorf("poller: OpenOrResumeSessionAt(%s) failed: %w", key, err)
		}
	}

	// Steady-state: was already active — refresh SP/assignee/summary (spec
	// Note C3, and spec Business Logic for Early Completion Notice), NEVER
	// call OpenOrResumeSession here (it would reset currentSessionStartedDate).
	for key, issue := range currentByKey {
		if _, wasAlreadyActive := activeByKey[key]; !wasAlreadyActive {
			continue
		}
		if err := p.issueSession.UpdateStoryPoints(ctx, key, storyPointsOf(issue), assigneeEmailOf(issue), issue.Fields.Summary); err != nil {
			return fmt.Errorf("poller: UpdateStoryPoints(%s) failed: %w", key, err)
		}
	}

	// Evaluate reminders for every ticket currently in Code Process.
	for key, issue := range currentByKey {
		session, found, err := p.issueSession.GetSessionByIssueKey(ctx, key)
		if err != nil {
			return fmt.Errorf("poller: GetSessionByIssueKey(%s) failed: %w", key, err)
		}
		if !found {
			continue
		}
		if err := p.reminderEngine.EvaluateAndSend(ctx, issue, *session); err != nil {
			return fmt.Errorf("poller: EvaluateAndSend(%s) failed: %w", key, err)
		}
	}

	return nil
}

// backfillActiveSessions runs once at startup (spec AC1.4): any ticket Jira
// already reports as "Code Process" that this process has no DB row for yet
// (fresh deploy, or DB was reset) gets its session opened at the REAL
// changelog transition time, not at process-start time — otherwise a ticket
// that has been in Code Process for days would appear to have just started.
func (p *pollerServiceImpl) backfillActiveSessions(ctx context.Context) error {
	issues, err := p.jiraClient.SearchCodeProcessIssues(ctx)
	if err != nil {
		return fmt.Errorf("poller: backfill SearchCodeProcessIssues failed: %w", err)
	}
	issues = p.filterEligible(ctx, issues)

	for _, issue := range issues {
		_, found, err := p.issueSession.GetSessionByIssueKey(ctx, issue.Key)
		if err != nil {
			return fmt.Errorf("poller: backfill GetSessionByIssueKey(%s) failed: %w", issue.Key, err)
		}
		if found {
			continue
		}

		entryTime, entryFound, err := p.jiraClient.GetLatestCodeProcessEntryTime(ctx, issue.Key)
		if err != nil {
			return fmt.Errorf("poller: backfill GetLatestCodeProcessEntryTime(%s) failed: %w", issue.Key, err)
		}
		if !entryFound {
			entryTime = time.Now()
		}

		if err := p.issueSession.OpenOrResumeSessionAt(ctx, issue.Key, projectKeyFromIssueKey(issue.Key), storyPointsOf(issue), entryTime, assigneeEmailOf(issue), issue.Fields.Summary); err != nil {
			return fmt.Errorf("poller: backfill OpenOrResumeSessionAt(%s) failed: %w", issue.Key, err)
		}
	}

	return nil
}

func storyPointsOf(issue jira_client_dto.Issue) float64 {
	if issue.Fields.StoryPoints == nil {
		return 0
	}
	return *issue.Fields.StoryPoints
}

func assigneeEmailOf(issue jira_client_dto.Issue) string {
	if issue.Fields.Assignee == nil {
		return ""
	}
	return issue.Fields.Assignee.EmailAddress
}

func projectKeyFromIssueKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return issueKey
}
