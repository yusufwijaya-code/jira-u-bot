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

// resolveEligibility implements the team's decision to scope the bot down
// from "every CORE/OMS ticket in Code Process" to only tickets assigned to
// a registered user with a real Story Points budget — an unassigned
// ticket, an assignee who never used /register, or a ticket with no (or
// zero) SP carries nothing meaningful to track and must never reach the
// session/reminder pipeline at all (not even to show up as an instant
// overage).
//
// Matching is done by the assignee's Jira accountId, not their email —
// this Jira org hides assignee.emailAddress for any account but the API
// token's own (confirmed live), so accountId is the only assignee
// identifier that's ever reliably populated. On a match, the REGISTERED
// email (typed at registration, already verified against Jira) is
// returned for session tracking/DM routing — never the issue's own
// (often blank) emailAddress field.
func (p *pollerServiceImpl) resolveEligibility(ctx context.Context, issue jira_client_dto.Issue) (assigneeEmail string, eligible bool) {
	sp := issue.Fields.EffectiveStoryPoints()
	if sp == nil || *sp <= 0 {
		return "", false
	}
	if issue.Fields.Assignee == nil {
		return "", false
	}
	user, found, err := p.registrationRepo.FindUserByJiraAccountID(ctx, issue.Fields.Assignee.AccountID)
	if err != nil || !found {
		return "", false
	}
	return user.Email, true
}

// filterEligible returns the eligible subset of issues alongside a
// issueKey -> registered-email map (see resolveEligibility) for callers
// that need the resolved email when opening/updating sessions.
func (p *pollerServiceImpl) filterEligible(ctx context.Context, issues []jira_client_dto.Issue) ([]jira_client_dto.Issue, map[string]string) {
	eligible := make([]jira_client_dto.Issue, 0, len(issues))
	resolvedEmails := make(map[string]string, len(issues))
	for _, issue := range issues {
		email, ok := p.resolveEligibility(ctx, issue)
		if !ok {
			continue
		}
		eligible = append(eligible, issue)
		resolvedEmails[issue.Key] = email
	}
	return eligible, resolvedEmails
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
	issues, resolvedEmails := p.filterEligible(ctx, issues)

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
		if err := p.issueSession.OpenOrResumeSessionAt(ctx, key, projectKeyFromIssueKey(key), storyPointsOf(issue), entryTime, resolvedEmails[key], issue.Fields.Summary); err != nil {
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
		if err := p.issueSession.UpdateStoryPoints(ctx, key, storyPointsOf(issue), resolvedEmails[key], issue.Fields.Summary); err != nil {
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
	issues, resolvedEmails := p.filterEligible(ctx, issues)

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

		if err := p.issueSession.OpenOrResumeSessionAt(ctx, issue.Key, projectKeyFromIssueKey(issue.Key), storyPointsOf(issue), entryTime, resolvedEmails[issue.Key], issue.Fields.Summary); err != nil {
			return fmt.Errorf("poller: backfill OpenOrResumeSessionAt(%s) failed: %w", issue.Key, err)
		}
	}

	return nil
}

func storyPointsOf(issue jira_client_dto.Issue) float64 {
	sp := issue.Fields.EffectiveStoryPoints()
	if sp == nil {
		return 0
	}
	return *sp
}

func projectKeyFromIssueKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return issueKey
}
