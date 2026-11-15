package reminder_engine_service

import (
	"context"
	"fmt"
	"sort"
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
	"nudge/modules/user_setting/user_setting_dto"
)

// UserSettingReader is the one user_setting capability the engine needs:
// the EFFECTIVE settings of an assignee for the board of the ticket's active
// sprint (0 = no active sprint → the user's default). Never fails — it
// falls back to defaults (Tech Spec "Per-Board Settings & Active Sprint
// Dashboard", AC5.2).
type UserSettingReader interface {
	GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting
}

type reminderEngineServiceImpl struct {
	jiraBaseURL      string
	holidayCalendar  business_hours_dto.CalendarSource
	businessHours    business_hours_service.BusinessHoursService
	reminderLog      reminder_log_service.ReminderLogService
	issueSession     issue_session_service.IssueSessionService
	registrationRepo registration_repository.RegistrationRepository
	webhookNotifier  google_chat_notifier_service.GoogleChatNotifierService
	dmNotifier       google_chat_notifier_service.GoogleChatNotifierService
	selfEmail        string
	userSettings     UserSettingReader
}

// NewReminderEngineService wires the engine. userSettings may be nil, in
// which case every assignee gets user_setting_dto.Default (= the legacy
// fixed behavior).
func NewReminderEngineService(
	jiraBaseURL string,
	holidayCalendar business_hours_dto.CalendarSource,
	businessHours business_hours_service.BusinessHoursService,
	reminderLog reminder_log_service.ReminderLogService,
	issueSession issue_session_service.IssueSessionService,
	registrationRepo registration_repository.RegistrationRepository,
	webhookNotifier google_chat_notifier_service.GoogleChatNotifierService,
	dmNotifier google_chat_notifier_service.GoogleChatNotifierService,
	selfEmail string,
	userSettings UserSettingReader,
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
		userSettings:     userSettings,
	}
}

// pickNotifier implements spec AC2.1/AC2.2/AC2.3: the assignee's own
// tickets always stay on the existing webhook (unchanged v1 behavior);
// everyone else's tickets go to a personal DM — UNLESS OAuth was never
// configured (dmNotifier is nil, decided once at startup in main.go), in
// which case every reminder falls back to the webhook, matching pre-this-
// enhancement behavior exactly.
func (e *reminderEngineServiceImpl) pickNotifier(assigneeEmail string) google_chat_notifier_service.GoogleChatNotifierService {
	return google_chat_notifier_service.PickNotifier(assigneeEmail, e.selfEmail, e.webhookNotifier, e.dmNotifier)
}

// settingsFor returns the assignee's effective settings for boardID. The
// settings of the ticket's ASSIGNEE always apply, whichever channel
// delivers it.
func (e *reminderEngineServiceImpl) settingsFor(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting {
	if e.userSettings == nil {
		return user_setting_dto.Default()
	}
	return e.userSettings.GetEffectiveForEmail(ctx, email, boardID)
}

// inReview reports whether the ticket sits in its project's tracked review
// status (CORE "CODE REVIEW") — the message then says review time counts.
func inReview(issue jira_client_dto.Issue) bool {
	return jira_client_dto.RuleForIssue(issue.Key).IsReview(issue.Fields.Status.Name)
}

func boardOf(session issue_session_dto.IssueSession) int64 {
	if session.BoardID == nil {
		return user_setting_dto.DefaultBoardID
	}
	return *session.BoardID
}

// ApplicableTiers returns, largest-first, the DEFAULT tiers strictly shorter
// than budgetSeconds (spec AC2.2). Per-user tiers go through
// applicableTiers instead.
func (e *reminderEngineServiceImpl) ApplicableTiers(budgetSeconds int64) []string {
	return applicableTiers(user_setting_dto.DefaultReminderTiers, budgetSeconds)
}

func applicableTiers(codes []string, budgetSeconds int64) []string {
	budget := time.Duration(budgetSeconds) * time.Second
	var tiers []string
	for _, code := range sortedLargestFirst(codes) {
		d := user_setting_dto.TierDuration(code)
		if d > 0 && budget > d {
			tiers = append(tiers, code)
		}
	}
	return tiers
}

func sortedLargestFirst(codes []string) []string {
	out := append([]string(nil), codes...)
	sort.SliceStable(out, func(i, j int) bool {
		return user_setting_dto.TierDuration(out[i]) > user_setting_dto.TierDuration(out[j])
	})
	return out
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

func (e *reminderEngineServiceImpl) EvaluateAndSend(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession) error {
	// The assignee's own calendar: national holidays + their leave days
	// (Tech Spec "Papan Tiket & Cuti", AC3.2/AC3.3).
	cal := e.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, e.businessHours, cal)

	budgetSeconds := int64(session.StoryPoints * 3600)
	assigneeName := e.resolveAssigneeDisplayName(ctx, issue.Fields.Assignee)
	issueURL := fmt.Sprintf("%s/browse/%s", e.jiraBaseURL, issue.Key)
	setting := e.settingsFor(ctx, session.AssigneeEmail, boardOf(session))

	if elapsedSeconds > budgetSeconds {
		return e.sendOverageIfDue(ctx, issue, session, setting, cal, elapsedSeconds, budgetSeconds, assigneeName, issueURL)
	}
	return e.sendTierIfDue(ctx, issue, session, setting, cal, elapsedSeconds, budgetSeconds, assigneeName, issueURL)
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

// unhandled filters codes (as logged, i.e. already prefixed) down to the ones
// tr_reminderlog has no row for yet — sent and skipped both count as handled.
func (e *reminderEngineServiceImpl) unhandled(ctx context.Context, issueKey string, codes []string) ([]string, error) {
	var out []string
	for _, code := range codes {
		handled, err := e.reminderLog.HasTierBeenSent(ctx, issueKey, code)
		if err != nil {
			return nil, err
		}
		if !handled {
			out = append(out, code)
		}
	}
	return out, nil
}

// newestIsDue reports whether the most relevant crossed tier (the last of
// crossed) is still unhandled — due keeps crossed's order, so it is then
// due's last entry too.
func newestIsDue(crossed, due []string) bool {
	return len(crossed) > 0 && len(due) > 0 && due[len(due)-1] == crossed[len(crossed)-1]
}

func (e *reminderEngineServiceImpl) logAll(ctx context.Context, issueKey string, codes []string, status string) error {
	for _, code := range codes {
		if err := e.reminderLog.LogReminder(ctx, issueKey, code, status); err != nil {
			return err
		}
	}
	return nil
}

// sendTierIfDue handles the pre-budget tiers (AC1.1–AC1.3, AC4.1).
func (e *reminderEngineServiceImpl) sendTierIfDue(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession, setting user_setting_dto.UserSetting, cal business_hours_dto.HolidayCalendar, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL string) error {
	// Elapsed itself freezes outside business hours (it's computed via
	// business-hours-aware ElapsedBusinessSeconds), but which POLL CYCLE
	// first notices a tier has been crossed is not — if that happens to
	// land after hours (crossing right before close, or the poller only
	// resuming later), the tier would otherwise fire immediately regardless
	// of the clock. No state is touched before this point, so the next
	// in-hours cycle re-evaluates normally.
	if !e.businessHours.IsWithinBusinessHours(time.Now(), cal) {
		return nil
	}

	remaining := budgetSeconds - elapsedSeconds

	// Crossed = the remaining time has dropped to/below the tier. Ordered
	// largest-first, so the LAST crossed tier is the one closest to the
	// deadline — the only one still worth sending.
	var crossed []string
	for _, tier := range applicableTiers(setting.ReminderTiers, budgetSeconds) {
		if remaining <= int64(user_setting_dto.TierDuration(tier).Seconds()) {
			crossed = append(crossed, tier)
		}
	}
	due, err := e.unhandled(ctx, issue.Key, crossed)
	if err != nil || len(due) == 0 {
		return err
	}

	if setting.IsSilenced(time.Now()) {
		// Muted / quiet hours: these tiers are stale by the time the mute
		// ends, so they are recorded as skipped and never sent (AC4.1).
		return e.logAll(ctx, issue.Key, due, reminder_log_dto.DeliverySkipped)
	}

	// A tier closer to the deadline was already handled (the user added a
	// larger tier mid-ticket): what's left is stale, never sent late.
	if !newestIsDue(crossed, due) {
		return e.logAll(ctx, issue.Key, due, reminder_log_dto.DeliverySkipped)
	}

	target := due[len(due)-1]
	stale := due[:len(due)-1]

	deadline := e.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remaining, cal)
	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:             issue.Key,
		IssueURL:             issueURL,
		IssueSummary:         issue.Fields.Summary,
		AssigneeName:         assigneeName,
		AssigneeEmail:        session.AssigneeEmail,
		StoryPoints:          session.StoryPoints,
		BudgetSeconds:        budgetSeconds,
		ElapsedSeconds:       elapsedSeconds,
		Tier:                 target,
		IsOverage:            false,
		InReview:             inReview(issue),
		Deadline:             deadline,
		CodeProcessStartedAt: codeProcessStartedAt(session.CurrentSessionStartedDate),
	}

	// Only record the send AFTER the notifier confirms success, so a failed
	// POST is retried on the next poll cycle (spec Reliability).
	if err := e.pickNotifier(session.AssigneeEmail).SendReminder(ctx, msg); err != nil {
		return err
	}
	if err := e.reminderLog.LogReminder(ctx, issue.Key, target, reminder_log_dto.DeliverySent); err != nil {
		return err
	}
	// Older crossed tiers were superseded by the one just sent (AC1.3).
	return e.logAll(ctx, issue.Key, stale, reminder_log_dto.DeliverySkipped)
}

// sendOverageIfDue handles everything past the budget (AC2.1–AC2.4, AC4.1,
// AC4.2): the one-off "budget habis" notice (T+0), the user's post-budget
// tiers ("+15m"), then the repeat interval (or silence when it's off).
func (e *reminderEngineServiceImpl) sendOverageIfDue(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession, setting user_setting_dto.UserSetting, cal business_hours_dto.HolidayCalendar, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL string) error {
	// The poller runs every 10s around the clock, so without this gate an
	// overaged ticket would keep re-sending all night/weekend/holiday too.
	// Skipping here (instead of touching LastOverageReminderDate) leaves
	// "due" exactly as-is, so the next in-hours cycle sends immediately.
	if !e.businessHours.IsWithinBusinessHours(time.Now(), cal) {
		return nil
	}

	over := time.Duration(elapsedSeconds-budgetSeconds) * time.Second

	// Post-budget tiers already crossed, smallest-first so the LAST one is
	// the most recent — the only one still worth sending.
	var crossed []string
	overageTiers := sortedLargestFirst(setting.OverageTiers)
	allCrossed := true
	for i := len(overageTiers) - 1; i >= 0; i-- {
		tier := overageTiers[i]
		if over >= user_setting_dto.TierDuration(tier) {
			crossed = append(crossed, reminder_log_dto.OverageTierPrefix+tier)
		} else {
			allCrossed = false
		}
	}
	due, err := e.unhandled(ctx, issue.Key, crossed)
	if err != nil {
		return err
	}

	if setting.IsSilenced(time.Now()) {
		// Post-budget tiers crossed during a mute are stale → skipped. The
		// T+0 notice and the repeat are left untouched (not sent, schedule
		// not advanced), so they go out the moment the mute ends (AC4.2).
		return e.logAll(ctx, issue.Key, due, reminder_log_dto.DeliverySkipped)
	}

	// 1. T+0: the first time a session goes over budget. LastOverageReminderDate
	// is the marker — it's set by every overage send and predates this
	// feature, so tickets already overaged before the upgrade aren't
	// re-announced. Post-budget tiers already crossed by then (a ticket first
	// noticed well past its budget) are superseded by this notice → skipped.
	if session.LastOverageReminderDate == nil {
		return e.sendOverage(ctx, issue, session, elapsedSeconds, budgetSeconds, assigneeName, issueURL, reminder_log_dto.TierOverage, due)
	}

	// 2. A post-budget tier is due — only when it's the most recent crossed
	// one. Otherwise a later tier was already handled (the user added a
	// smaller tier mid-ticket): the leftovers are stale → skipped, and the
	// repeat schedule below carries on untouched.
	if len(due) > 0 {
		if newestIsDue(crossed, due) {
			return e.sendOverage(ctx, issue, session, elapsedSeconds, budgetSeconds, assigneeName, issueURL, due[len(due)-1], due[:len(due)-1])
		}
		if err := e.logAll(ctx, issue.Key, due, reminder_log_dto.DeliverySkipped); err != nil {
			return err
		}
	}

	// 3. Repeat — only once every post-budget tier has been crossed, and
	// only if the user hasn't switched repeating off.
	if !allCrossed || setting.OverageRepeatMinutes == nil {
		return nil
	}
	interval := time.Duration(*setting.OverageRepeatMinutes) * time.Minute
	if time.Since(*session.LastOverageReminderDate) < interval {
		return nil
	}
	return e.sendOverage(ctx, issue, session, elapsedSeconds, budgetSeconds, assigneeName, issueURL, reminder_log_dto.TierOverage, nil)
}

func (e *reminderEngineServiceImpl) sendOverage(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession, elapsedSeconds, budgetSeconds int64, assigneeName, issueURL, tier string, stale []string) error {
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
		Tier:                 tier,
		IsOverage:            true,
		InReview:             inReview(issue),
		CodeProcessStartedAt: codeProcessStartedAt(session.CurrentSessionStartedDate),
	}

	// Order matters: only record the send (log + timestamp) AFTER the
	// notifier confirms success, so a failed POST can be retried on the
	// next poll cycle instead of being silently marked as sent.
	if err := e.pickNotifier(assigneeEmail).SendReminder(ctx, msg); err != nil {
		return err
	}
	if err := e.reminderLog.LogReminder(ctx, issue.Key, tier, reminder_log_dto.DeliverySent); err != nil {
		return err
	}
	if err := e.logAll(ctx, issue.Key, stale, reminder_log_dto.DeliverySkipped); err != nil {
		return err
	}
	// Every overage send (T+0, tier, repeat) restarts the repeat interval.
	return e.issueSession.UpdateLastOverageReminder(ctx, issue.Key)
}
