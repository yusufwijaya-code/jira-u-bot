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
	"jira-u-bot/modules/reminder_engine/reminder_engine_service"
)

const pollInterval = 30 * time.Second

type pollerServiceImpl struct {
	jiraClient      jira_client_service.JiraClientService
	issueSession    issue_session_service.IssueSessionService
	businessHours   business_hours_service.BusinessHoursService
	reminderEngine  reminder_engine_service.ReminderEngineService
	holidayCalendar business_hours_dto.HolidayCalendar
}

func NewPollerService(
	jiraClient jira_client_service.JiraClientService,
	issueSession issue_session_service.IssueSessionService,
	businessHours business_hours_service.BusinessHoursService,
	reminderEngine reminder_engine_service.ReminderEngineService,
	holidayCalendar business_hours_dto.HolidayCalendar,
) PollerService {
	return &pollerServiceImpl{
		jiraClient:      jiraClient,
		issueSession:    issueSession,
		businessHours:   businessHours,
		reminderEngine:  reminderEngine,
		holidayCalendar: holidayCalendar,
	}
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
		if err := p.issueSession.CloseSession(ctx, key, additionalSeconds); err != nil {
			return fmt.Errorf("poller: CloseSession(%s) failed: %w", key, err)
		}
	}

	// Entry events: new to the DB this cycle — open a fresh session.
	for key, issue := range currentByKey {
		if _, alreadyActive := activeByKey[key]; alreadyActive {
			continue
		}
		if err := p.issueSession.OpenOrResumeSession(ctx, key, projectKeyFromIssueKey(key), storyPointsOf(issue)); err != nil {
			return fmt.Errorf("poller: OpenOrResumeSession(%s) failed: %w", key, err)
		}
	}

	// Steady-state: was already active — refresh SP only (spec Note C3),
	// NEVER call OpenOrResumeSession here (it would reset currentSessionStartedDate).
	for key, issue := range currentByKey {
		if _, wasAlreadyActive := activeByKey[key]; !wasAlreadyActive {
			continue
		}
		if err := p.issueSession.UpdateStoryPoints(ctx, key, storyPointsOf(issue)); err != nil {
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

		if err := p.issueSession.OpenOrResumeSessionAt(ctx, issue.Key, projectKeyFromIssueKey(issue.Key), storyPointsOf(issue), entryTime); err != nil {
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

func projectKeyFromIssueKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return issueKey
}
