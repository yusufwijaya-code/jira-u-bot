package reminder_engine_service

import (
	"context"
	"fmt"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/issue_session/issue_session_service"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_repository"
	"jira-u-bot/modules/reminder_log/reminder_log_dto"
	"jira-u-bot/modules/reminder_log/reminder_log_service"
)

const overageRepeatInterval = 5 * time.Minute

var tierDurations = []struct {
	Tier     string
	Duration time.Duration
}{
	{reminder_log_dto.Tier2h, 2 * time.Hour},
	{reminder_log_dto.Tier1h, 1 * time.Hour},
	{reminder_log_dto.Tier30m, 30 * time.Minute},
	{reminder_log_dto.Tier15m, 15 * time.Minute},
	{reminder_log_dto.Tier1m, 1 * time.Minute},
}

type reminderEngineServiceImpl struct {
	jiraBaseURL      string
	holidayCalendar  business_hours_dto.HolidayCalendar
	businessHours    business_hours_service.BusinessHoursService
	reminderLog      reminder_log_service.ReminderLogService
	issueSession     issue_session_service.IssueSessionService
	registrationRepo registration_repository.RegistrationRepository
	webhookNotifier  google_chat_notifier_service.GoogleChatNotifierService
	dmNotifier       google_chat_notifier_service.GoogleChatNotifierService
	selfEmail        string
}

func NewReminderEngineService(
	jiraBaseURL string,
	holidayCalendar business_hours_dto.HolidayCalendar,
	businessHours business_hours_service.BusinessHoursService,
	reminderLog reminder_log_service.ReminderLogService,
	issueSession issue_session_service.IssueSessionService,
	registrationRepo registration_repository.RegistrationRepository,
	webhookNotifier google_chat_notifier_service.GoogleChatNotifierService,
	dmNotifier google_chat_notifier_service.GoogleChatNotifierService,
	selfEmail string,
) ReminderEngineService {
	return &reminderEngineServiceImpl{
		jiraBaseURL:      jiraBaseURL,
		holidayCalendar:  holidayCalendar,
		businessHours:    businessHours,
		reminderLog:      reminderLog,
		issueSession:     issueSession,
		registrationRepo: registrationRepo,
		webhookNotifier:  webhookNotifier,
		dmNotifier:       dmNotifier,
		selfEmail:        selfEmail,
	}
}

// pickNotifier implements spec AC2.1/AC2.2/AC2.3: the assignee's own
// tickets always stay on the existing webhook (unchanged v1 behavior);
// everyone else's tickets go to a personal DM — UNLESS OAuth was never
// configured (dmNotifier is nil, decided once at startup in main.go), in
// which case every reminder falls back to the webhook, matching pre-this-
// enhancement behavior exactly.
func (e *reminderEngineServiceImpl) pickNotifier(assigneeEmail string) google_chat_notifier_service.GoogleChatNotifierService {
	if assigneeEmail == e.selfEmail || e.dmNotifier == nil {
		return e.webhookNotifier
	}
	return e.dmNotifier
}

func (e *reminderEngineServiceImpl) ApplicableTiers(budgetSeconds int64) []string {
	budget := time.Duration(budgetSeconds) * time.Second
	var tiers []string
	for _, td := range tierDurations {
		if budget > td.Duration {
			tiers = append(tiers, td.Tier)
		}
	}
	return tiers
}

func tierDurationSeconds(tier string) int64 {
	for _, td := range tierDurations {
		if td.Tier == tier {
			return int64(td.Duration.Seconds())
		}
	}
	return 0
}

func (e *reminderEngineServiceImpl) EvaluateAndSend(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession) error {
	elapsedSeconds := session.AccumulatedSeconds
	if session.CurrentSessionStartedDate != nil {
		elapsedSeconds += e.businessHours.ElapsedBusinessSeconds(*session.CurrentSessionStartedDate, time.Now(), e.holidayCalendar)
	}

	budgetSeconds := int64(session.StoryPoints * 3600)
	assigneeName := e.resolveAssigneeDisplayName(ctx, issue.Fields.Assignee)
	issueURL := fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issue.Key)

	if elapsedSeconds > budgetSeconds {
		return e.sendOverageIfDue(ctx, issue, session, elapsedSeconds, budgetSeconds, assigneeName, issueURL)
	}
	return e.sendTierIfDue(ctx, issue, session.StoryPoints, elapsedSeconds, budgetSeconds, assigneeName, issueURL)
}

// resolveAssigneeDisplayName implements spec AC3.3: an unregistered assignee
// still gets reminded (not skipped), just with the raw Jira display name
// instead of the name they registered. An unassigned ticket (nil Assignee)
// is an edge case the spec doesn't explicitly address; "(unassigned)" is
// this plan's defensive choice so EvaluateAndSend never panics on it.
func (e *reminderEngineServiceImpl) resolveAssigneeDisplayName(ctx context.Context, assignee *jira_client_dto.Assignee) string {
	if assignee == nil {
		return "(unassigned)"
	}
	user, found, err := e.registrationRepo.FindUserByEmail(ctx, assignee.EmailAddress)
	if err == nil && found {
		return user.UserName
	}
	return assignee.DisplayName
}

func (e *reminderEngineServiceImpl) sendOverageIfDue(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL string) error {
	due := session.LastOverageReminderDate == nil || time.Since(*session.LastOverageReminderDate) >= overageRepeatInterval
	if !due {
		return nil
	}

	var assigneeEmail string
	if issue.Fields.Assignee != nil {
		assigneeEmail = issue.Fields.Assignee.EmailAddress
	}

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       issue.Key,
		IssueURL:       issueURL,
		IssueSummary:   issue.Fields.Summary,
		AssigneeName:   assigneeName,
		AssigneeEmail:  assigneeEmail,
		StoryPoints:    session.StoryPoints,
		BudgetSeconds:  budgetSeconds,
		ElapsedSeconds: elapsedSeconds,
		Tier:           reminder_log_dto.TierOverage,
		IsOverage:      true,
	}

	// Order matters: only record the send (log + timestamp) AFTER the
	// notifier confirms success, so a failed POST can be retried on the
	// next poll cycle instead of being silently marked as sent (spec
	// Reliability, Component Poller). This pre-existing mechanism is
	// exactly what satisfies spec AC2.5 for the DM path too — pickNotifier
	// only changes WHICH notifier gets called, not this retry contract, so
	// no new test is needed here beyond the ones v1 already had for it.
	if err := e.pickNotifier(assigneeEmail).SendReminder(ctx, msg); err != nil {
		return err
	}
	if err := e.reminderLog.LogReminderSent(ctx, issue.Key, reminder_log_dto.TierOverage); err != nil {
		return err
	}
	return e.issueSession.UpdateLastOverageReminder(ctx, issue.Key)
}

func (e *reminderEngineServiceImpl) sendTierIfDue(ctx context.Context, issue jira_client_dto.Issue, storyPoints float64, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL string) error {
	remaining := budgetSeconds - elapsedSeconds
	deadline := e.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remaining, e.holidayCalendar)

	var assigneeEmail string
	if issue.Fields.Assignee != nil {
		assigneeEmail = issue.Fields.Assignee.EmailAddress
	}

	for _, tier := range e.ApplicableTiers(budgetSeconds) {
		if remaining > tierDurationSeconds(tier) {
			continue // threshold not reached yet for this tier
		}

		alreadySent, err := e.reminderLog.HasTierBeenSent(ctx, issue.Key, tier)
		if err != nil {
			return err
		}
		if alreadySent {
			continue
		}

		msg := google_chat_notifier_dto.ReminderMessage{
			IssueKey:       issue.Key,
			IssueURL:       issueURL,
			IssueSummary:   issue.Fields.Summary,
			AssigneeName:   assigneeName,
			AssigneeEmail:  assigneeEmail,
			StoryPoints:    storyPoints,
			BudgetSeconds:  budgetSeconds,
			ElapsedSeconds: elapsedSeconds,
			Tier:           tier,
			IsOverage:      false,
			Deadline:       deadline,
		}

		if err := e.pickNotifier(assigneeEmail).SendReminder(ctx, msg); err != nil {
			return err
		}
		return e.reminderLog.LogReminderSent(ctx, issue.Key, tier)
	}

	return nil
}

func (e *reminderEngineServiceImpl) NotifyIfEarlyCompletion(ctx context.Context, issueKey, assigneeEmail, issueSummary string, elapsedSeconds, budgetSeconds int64) error {
	if elapsedSeconds >= budgetSeconds {
		return nil
	}

	remaining := budgetSeconds - elapsedSeconds
	deadline := e.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remaining, e.holidayCalendar)

	assigneeName := assigneeEmail
	if user, found, err := e.registrationRepo.FindUserByEmail(ctx, assigneeEmail); err == nil && found {
		assigneeName = user.UserName
	}

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:          issueKey,
		IssueURL:          fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issueKey),
		IssueSummary:      issueSummary,
		AssigneeName:      assigneeName,
		AssigneeEmail:     assigneeEmail,
		StoryPoints:       float64(budgetSeconds) / 3600,
		BudgetSeconds:     budgetSeconds,
		ElapsedSeconds:    elapsedSeconds,
		IsEarlyCompletion: true,
		Deadline:          deadline,
	}

	return e.pickNotifier(assigneeEmail).SendReminder(ctx, msg)
}
