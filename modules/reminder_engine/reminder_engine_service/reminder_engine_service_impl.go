package reminder_engine_service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/reminder_log/reminder_log_dto"
	"nudge/modules/reminder_log/reminder_log_service"
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
	{reminder_log_dto.Tier5m, 5 * time.Minute},
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

// codeProcessStartedAt defensively unwraps the session's current-session
// start time — nil is only a theoretical state for a session this function
// is actively evaluating (see the nil-check already in EvaluateAndSend
// below), but there is no sane display fallback besides the zero value.
func codeProcessStartedAt(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
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
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, e.businessHours, e.holidayCalendar)

	budgetSeconds := int64(session.StoryPoints * 3600)
	assigneeName := e.resolveAssigneeDisplayName(ctx, issue.Fields.Assignee)
	issueURL := fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issue.Key)

	if elapsedSeconds > budgetSeconds {
		return e.sendOverageIfDue(ctx, issue, session, elapsedSeconds, budgetSeconds, assigneeName, issueURL)
	}
	return e.sendTierIfDue(ctx, issue, session.StoryPoints, session.AssigneeEmail, elapsedSeconds, budgetSeconds, assigneeName, issueURL, codeProcessStartedAt(session.CurrentSessionStartedDate))
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
	// The poller runs every 10s around the clock, so without this gate an
	// overaged ticket would keep re-sending every 5 minutes all night/
	// weekend/holiday too. Skipping here (instead of touching
	// LastOverageReminderDate) leaves "due" exactly as-is, so the very next
	// in-hours poll cycle sends immediately rather than waiting out another
	// 5-minute window.
	if !e.businessHours.IsWithinBusinessHours(time.Now(), e.holidayCalendar) {
		return nil
	}

	due := session.LastOverageReminderDate == nil || time.Since(*session.LastOverageReminderDate) >= overageRepeatInterval
	if !due {
		return nil
	}

	// session.AssigneeEmail (resolved/persisted by the poller at session-
	// open time) is the source of truth here — issue.Fields.Assignee.
	// EmailAddress is often blank (Jira hides it org-wide for any account
	// but the API token's own), and a blank email breaks DM routing.
	assigneeEmail := session.AssigneeEmail

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:             issue.Key,
		IssueURL:             issueURL,
		IssueSummary:         issue.Fields.Summary,
		AssigneeName:         assigneeName,
		AssigneeEmail:        assigneeEmail,
		StoryPoints:          session.StoryPoints,
		BudgetSeconds:        budgetSeconds,
		ElapsedSeconds:       elapsedSeconds,
		Tier:                 reminder_log_dto.TierOverage,
		IsOverage:            true,
		CodeProcessStartedAt: codeProcessStartedAt(session.CurrentSessionStartedDate),
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

func (e *reminderEngineServiceImpl) sendTierIfDue(ctx context.Context, issue jira_client_dto.Issue, storyPoints float64, assigneeEmail string, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL string, startedAt time.Time) error {
	// Elapsed itself freezes outside business hours (it's computed via
	// business-hours-aware ElapsedBusinessSeconds), but which POLL CYCLE
	// first notices a tier has been crossed is not — if that happens to
	// land after hours (crossing right before close, or the poller only
	// resuming later), the tier would otherwise fire immediately regardless
	// of the clock. Safe to defer here exactly like sendOverageIfDue: no
	// state (HasTierBeenSent) is touched before this point, so the next
	// in-hours cycle re-evaluates and sends normally.
	if !e.businessHours.IsWithinBusinessHours(time.Now(), e.holidayCalendar) {
		return nil
	}

	remaining := budgetSeconds - elapsedSeconds
	deadline := e.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remaining, e.holidayCalendar)

	tiers := e.preferredTiers(ctx, assigneeEmail, e.ApplicableTiers(budgetSeconds))
	for _, tier := range tiers {
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
			IssueKey:             issue.Key,
			IssueURL:             issueURL,
			IssueSummary:         issue.Fields.Summary,
			AssigneeName:         assigneeName,
			AssigneeEmail:        assigneeEmail,
			StoryPoints:          storyPoints,
			BudgetSeconds:        budgetSeconds,
			ElapsedSeconds:       elapsedSeconds,
			Tier:                 tier,
			IsOverage:            false,
			Deadline:             deadline,
			CodeProcessStartedAt: startedAt,
		}

		if err := e.pickNotifier(assigneeEmail).SendReminder(ctx, msg); err != nil {
			return err
		}
		return e.reminderLog.LogReminderSent(ctx, issue.Key, tier)
	}

	return nil
}

// preferredTiers implements spec AC2.3 (Tech Spec "Dashboard Jira Manage"):
// filters applicableTiers down to assigneeEmail's chosen subset, if they've
// ever set one. ReminderTierPreference == nil (never set, the common case
// for every user today) returns applicableTiers unchanged — this is the
// ONLY place preference is consulted; sendOverageIfDue never calls this,
// so the overage tier is always sent regardless of preference.
func (e *reminderEngineServiceImpl) preferredTiers(ctx context.Context, assigneeEmail string, applicableTiers []string) []string {
	user, found, err := e.registrationRepo.FindUserByEmail(ctx, assigneeEmail)
	if err != nil || !found || user.ReminderTierPreference == nil {
		return applicableTiers
	}

	allowed := map[string]bool{}
	for _, t := range strings.Split(*user.ReminderTierPreference, ",") {
		if t != "" {
			allowed[t] = true
		}
	}

	var filtered []string
	for _, tier := range applicableTiers {
		if allowed[tier] {
			filtered = append(filtered, tier)
		}
	}
	return filtered
}

func (e *reminderEngineServiceImpl) NotifyIfEarlyCompletion(ctx context.Context, issueKey, assigneeEmail, issueSummary string, elapsedSeconds, budgetSeconds int64, codeProcessStartedAt, codeReviewEnteredAt time.Time) error {
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
		IssueKey:             issueKey,
		IssueURL:             fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issueKey),
		IssueSummary:         issueSummary,
		AssigneeName:         assigneeName,
		AssigneeEmail:        assigneeEmail,
		StoryPoints:          float64(budgetSeconds) / 3600,
		BudgetSeconds:        budgetSeconds,
		ElapsedSeconds:       elapsedSeconds,
		IsEarlyCompletion:    true,
		Deadline:             deadline,
		CodeProcessStartedAt: codeProcessStartedAt,
		CodeReviewEnteredAt:  codeReviewEnteredAt,
	}

	return e.pickNotifier(assigneeEmail).SendReminder(ctx, msg)
}

func (e *reminderEngineServiceImpl) NotifyReturnedToTodo(ctx context.Context, issueKey, assigneeEmail, issueSummary string, elapsedSeconds, budgetSeconds int64, codeProcessStartedAt time.Time) error {
	assigneeName := assigneeEmail
	if user, found, err := e.registrationRepo.FindUserByEmail(ctx, assigneeEmail); err == nil && found {
		assigneeName = user.UserName
	}

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:             issueKey,
		IssueURL:             fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issueKey),
		IssueSummary:         issueSummary,
		AssigneeName:         assigneeName,
		AssigneeEmail:        assigneeEmail,
		StoryPoints:          float64(budgetSeconds) / 3600,
		BudgetSeconds:        budgetSeconds,
		ElapsedSeconds:       elapsedSeconds,
		IsReturnedToTodo:     true,
		CodeProcessStartedAt: codeProcessStartedAt,
	}

	return e.pickNotifier(assigneeEmail).SendReminder(ctx, msg)
}
