package reminder_engine_service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

// --- fakes ---

type fakeReminderLogService struct {
	sentTiers map[string]bool
	// statuses records the deliveryStatus of every LogReminder call, keyed
	// like sentTiers ("ISSUE|tier").
	statuses map[string]string
}

func newFakeReminderLogService() *fakeReminderLogService {
	return &fakeReminderLogService{sentTiers: map[string]bool{}, statuses: map[string]string{}}
}

func (f *fakeReminderLogService) LogReminder(ctx context.Context, issueKey, tier, status string) error {
	f.sentTiers[issueKey+"|"+tier] = true
	f.statuses[issueKey+"|"+tier] = status
	return nil
}

// fakeUserSettings returns a fixed setting per email (defaults otherwise)
// and records the board each lookup was for.
type fakeUserSettings struct {
	byEmail       map[string]user_setting_dto.UserSetting
	byEmailBoard  map[string]map[int64]user_setting_dto.UserSetting
	requestedFrom []int64
}

func (f *fakeUserSettings) GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting {
	f.requestedFrom = append(f.requestedFrom, boardID)
	if boards, ok := f.byEmailBoard[email]; ok {
		if s, ok := boards[boardID]; ok {
			return s
		}
	}
	if s, ok := f.byEmail[email]; ok {
		return s
	}
	return user_setting_dto.Default()
}

func (f *fakeReminderLogService) HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error) {
	return f.sentTiers[issueKey+"|"+tier], nil
}

func (f *fakeReminderLogService) LogReminderSent(ctx context.Context, issueKey, tier string) error {
	f.sentTiers[issueKey+"|"+tier] = true
	return nil
}

type fakeIssueSessionService struct {
	updateOverageCalls []string
}

func (f *fakeIssueSessionService) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionService) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionService) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionService) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return nil
}
func (f *fakeIssueSessionService) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	return nil, nil
}
func (f *fakeIssueSessionService) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	return nil, false, nil
}
func (f *fakeIssueSessionService) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	f.updateOverageCalls = append(f.updateOverageCalls, issueKey)
	return nil
}

type fakeRegistrationRepository struct {
	users map[string]registration_dto.User
}

func (f *fakeRegistrationRepository) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepository) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}
func (f *fakeRegistrationRepository) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepository) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepository) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	return nil
}

func (f *fakeRegistrationRepository) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func (f *fakeRegistrationRepository) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	return nil
}

func (f *fakeRegistrationRepository) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepository) DeleteByID(ctx context.Context, userID int64) error {
	return nil
}

func (f *fakeRegistrationRepository) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

type fakeGoogleChatNotifierService struct {
	sentMessages []google_chat_notifier_dto.ReminderMessage
}

func (f *fakeGoogleChatNotifierService) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	f.sentMessages = append(f.sentMessages, msg)
	return nil
}

// fakeBusinessHoursGate wraps the real BusinessHoursService (for accurate
// ElapsedBusinessSeconds/DeadlineAfterBusinessSeconds math) but pins
// IsWithinBusinessHours to a fixed value, so tests unrelated to the
// off-hours overage gate don't become flaky depending on the wall-clock
// time the suite happens to run at.
type fakeBusinessHoursGate struct {
	business_hours_service.BusinessHoursService
	within bool
}

func (f *fakeBusinessHoursGate) IsWithinBusinessHours(t time.Time, cal business_hours_dto.HolidayCalendar) bool {
	return f.within
}

func newAlwaysOpenBusinessHours() business_hours_service.BusinessHoursService {
	return &fakeBusinessHoursGate{BusinessHoursService: business_hours_service.NewBusinessHoursService(), within: true}
}

// --- helpers ---

func newTestEngine(reminderLog *fakeReminderLogService, issueSession *fakeIssueSessionService, registrationRepo *fakeRegistrationRepository, notifier *fakeGoogleChatNotifierService) ReminderEngineService {
	return NewReminderEngineService(
		"https://esb-id.atlassian.net",
		business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(),
		reminderLog,
		issueSession,
		registrationRepo,
		notifier,
		nil,
		"",
		nil,
	)
}

func testIssue(email, displayName string) jira_client_dto.Issue {
	var assignee *jira_client_dto.Assignee
	if email != "" {
		assignee = &jira_client_dto.Assignee{EmailAddress: email, DisplayName: displayName}
	}
	return jira_client_dto.Issue{
		Key: "OMS-1",
		Fields: jira_client_dto.IssueFields{
			Summary:  "Book Integration",
			Assignee: assignee,
		},
	}
}

// --- ApplicableTiers ---

func TestApplicableTiers_SkipsTiersNotShorterThanBudget(t *testing.T) {
	engine := newTestEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}}, &fakeGoogleChatNotifierService{})

	got := engine.ApplicableTiers(3600) // SP = 1, budget = exactly 1 hour

	want := []string{"30m", "15m", "5m", "1m"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplicableTiers(3600) = %v, want %v (2h and 1h must be skipped — budget must be STRICTLY greater than tier duration, spec AC2.2)", got, want)
	}
}

func TestApplicableTiers_AllTiersApplyForLargeBudget(t *testing.T) {
	engine := newTestEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}}, &fakeGoogleChatNotifierService{})

	got := engine.ApplicableTiers(10800) // SP = 3, budget = 3 hours

	want := []string{"2h", "1h", "30m", "15m", "5m", "1m"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplicableTiers(10800) = %v, want %v", got, want)
	}
}

// --- EvaluateAndSend: tier reminders ---
//
// Every test below uses CurrentSessionStartedDate = nil and sets
// AccumulatedSeconds directly, so elapsed time is a deterministic input
// instead of depending on time.Now() — EvaluateAndSend adds live elapsed
// time on top of AccumulatedSeconds only when a session is still active
// (CurrentSessionStartedDate != nil); with it nil, elapsed == AccumulatedSeconds
// exactly, which is what these tests need to assert precise tier boundaries.

func TestEvaluateAndSend_SendsFirstDueUnsentTier(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	reminderLog.sentTiers["OMS-1|2h"] = true
	reminderLog.sentTiers["OMS-1|1h"] = true
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:                  "OMS-1",
		StoryPoints:               3,    // budget = 10800s
		AccumulatedSeconds:        9100, // remaining = 1700s -> due for 30m (1800s) and 15m is not yet (900s), but 2h/1h already sent
		CurrentSessionStartedDate: nil,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Fatalf("len(sentMessages) = %d, want 1", len(notifier.sentMessages))
	}
	if notifier.sentMessages[0].Tier != "30m" {
		t.Errorf("Tier = %q, want %q", notifier.sentMessages[0].Tier, "30m")
	}
	if !reminderLog.sentTiers["OMS-1|30m"] {
		t.Error("expected tr_reminderlog to record OMS-1|30m as sent")
	}
}

func TestEvaluateAndSend_TierSkipsSendOutsideBusinessHours(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	reminderLog.sentTiers["OMS-1|2h"] = true
	reminderLog.sentTiers["OMS-1|1h"] = true
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		&fakeBusinessHoursGate{BusinessHoursService: business_hours_service.NewBusinessHoursService(), within: false},
		reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier, nil, "",
		nil,
	)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,    // budget = 10800s
		AccumulatedSeconds: 9100, // remaining = 1700s -> 30m tier is due
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — outside business hours, tier reminders must not fire (e.g. a threshold crossed right before closing time, only noticed by a later off-hours poll cycle)", len(notifier.sentMessages))
	}
	if reminderLog.sentTiers["OMS-1|30m"] {
		t.Error("HasTierBeenSent must not be recorded when the send was skipped — otherwise the tier would never fire once business hours resume")
	}
}

func TestEvaluateAndSend_DoesNotResendAlreadyLoggedTier(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	reminderLog.sentTiers["OMS-1|2h"] = true
	reminderLog.sentTiers["OMS-1|1h"] = true
	reminderLog.sentTiers["OMS-1|30m"] = true
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 9100, // same remaining as above, but 30m already sent too
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 (spec AC2.3 — no duplicate send for an already-logged tier)", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_NoTierDueYetSendsNothing(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,    // budget = 10800s
		AccumulatedSeconds: 1000, // remaining = 9800s, well above the 2h (7200s) tier
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — no tier threshold has been crossed yet", len(notifier.sentMessages))
	}
}

// --- EvaluateAndSend: per-user settings (Tech Spec "Dynamic User Notification Settings") ---

func settingsEngine(reminderLog *fakeReminderLogService, issueSession *fakeIssueSessionService, notifier *fakeGoogleChatNotifierService, setting user_setting_dto.UserSetting) ReminderEngineService {
	return NewReminderEngineService(
		"https://esb-id.atlassian.net",
		business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(),
		reminderLog,
		issueSession,
		&fakeRegistrationRepository{users: map[string]registration_dto.User{}},
		notifier,
		nil,
		"",
		&fakeUserSettings{byEmail: map[string]user_setting_dto.UserSetting{"anwar@esb.co.id": setting}},
	)
}

func withTiers(tiers ...string) user_setting_dto.UserSetting {
	s := user_setting_dto.Default()
	s.ReminderTiers = tiers
	return s
}

func TestEvaluateAndSend_FiltersOutTierNotInUserTiers(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withTiers("15m"))

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 9100, // remaining = 1700s -> 30m would be due, 15m not yet
		AssigneeEmail:      "anwar@esb.co.id",
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — 30m isn't one of the user's tiers and 15m isn't due yet", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_SendsCustomTier(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withTiers("45m"))

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,    // budget 10800s
		AccumulatedSeconds: 8200, // remaining 2600s <= 45m (2700s)
		AssigneeEmail:      "anwar@esb.co.id",
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "45m" {
		t.Fatalf("sentMessages = %+v, want the custom 45m tier", notifier.sentMessages)
	}
	if reminderLog.statuses["OMS-1|45m"] != "sent" {
		t.Errorf("status = %q, want sent", reminderLog.statuses["OMS-1|45m"])
	}
}

func TestEvaluateAndSend_SendsOnlyMostRelevantTierAndSkipsOlderOnes(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, user_setting_dto.Default())

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,     // budget 10800s
		AccumulatedSeconds: 10000, // remaining 800s -> 2h, 1h, 30m, 15m all crossed at once
		AssigneeEmail:      "anwar@esb.co.id",
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "15m" {
		t.Fatalf("sentMessages = %+v, want exactly one message for 15m (closest to the deadline)", notifier.sentMessages)
	}
	for _, tier := range []string{"2h", "1h", "30m"} {
		if reminderLog.statuses["OMS-1|"+tier] != "skipped" {
			t.Errorf("tier %s status = %q, want skipped (superseded)", tier, reminderLog.statuses["OMS-1|"+tier])
		}
	}

	// Next cycle: nothing new is due, so nothing is sent.
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("second EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 {
		t.Errorf("len(sentMessages) = %d after a second cycle, want still 1", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_MutedSkipsDueTierForGood(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	setting := user_setting_dto.Default()
	until := time.Now().Add(time.Hour)
	setting.MutedUntil = &until
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, setting)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 9100, // 30m due
		AssigneeEmail:      "anwar@esb.co.id",
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Fatalf("len(sentMessages) = %d, want 0 while muted", len(notifier.sentMessages))
	}
	if reminderLog.statuses["OMS-1|30m"] != "skipped" {
		t.Errorf("30m status = %q, want skipped so it never fires after the mute", reminderLog.statuses["OMS-1|30m"])
	}
}

func TestEvaluateAndSend_OverageSendsEvenWithNoPreBudgetTiers(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withTiers())

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        1,    // budget = 3600s
		AccumulatedSeconds: 4000, // already over budget
		AssigneeEmail:      "anwar@esb.co.id",
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || !notifier.sentMessages[0].IsOverage {
		t.Errorf("sentMessages = %+v, want the T+0 overage notice regardless of pre-budget tiers", notifier.sentMessages)
	}
}

// --- EvaluateAndSend: post-budget tiers & repeat ---

func overageSession(overSeconds int64, last *time.Time) issue_session_dto.IssueSession {
	return issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             1, // budget 3600s
		AccumulatedSeconds:      3600 + overSeconds,
		AssigneeEmail:           "anwar@esb.co.id",
		LastOverageReminderDate: last,
	}
}

func withOverage(repeat *int, tiers ...string) user_setting_dto.UserSetting {
	s := user_setting_dto.Default()
	s.OverageTiers = tiers
	s.OverageRepeatMinutes = repeat
	return s
}

func intPtr(v int) *int { return &v }

func TestEvaluateAndSend_OverageTierSentOnceWhenCrossed(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, issueSession, notifier, withOverage(intPtr(5), "1h", "15m"))

	last := time.Now().Add(-time.Minute) // T+0 already sent a minute ago
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(16*60, &last)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "+15m" || !notifier.sentMessages[0].IsOverage {
		t.Fatalf("sentMessages = %+v, want one overage message for +15m", notifier.sentMessages)
	}
	if len(issueSession.updateOverageCalls) != 1 {
		t.Errorf("UpdateLastOverageReminder calls = %d, want 1", len(issueSession.updateOverageCalls))
	}
}

func TestEvaluateAndSend_NoRepeatUntilEveryOverageTierCrossed(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	reminderLog.sentTiers["OMS-1|+15m"] = true
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withOverage(intPtr(5), "1h", "15m"))

	last := time.Now().Add(-30 * time.Minute) // well past the 5-minute repeat
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(40*60, &last)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — the +1h tier is still pending, so the repeat hasn't started", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_RepeatsAfterLastOverageTierAtUserInterval(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	reminderLog.sentTiers["OMS-1|+15m"] = true
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withOverage(intPtr(30), "15m"))

	recent := time.Now().Add(-10 * time.Minute)
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(60*60, &recent)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Fatalf("len(sentMessages) = %d, want 0 — only 10 of the 30 repeat minutes have passed", len(notifier.sentMessages))
	}

	old := time.Now().Add(-31 * time.Minute)
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(60*60, &old)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "overage" {
		t.Errorf("sentMessages = %+v, want one repeat (Tier=overage)", notifier.sentMessages)
	}
}

func TestEvaluateAndSend_RepeatOffStopsAfterLastOverageTier(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withOverage(nil))

	old := time.Now().Add(-2 * time.Hour)
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(3*60*60, &old)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — repeat is switched off", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_FirstOverageSupersedesAlreadyCrossedOverageTiers(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, withOverage(intPtr(5), "15m", "30m"))

	// First noticed already 40 minutes over budget: one T+0 notice only.
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(40*60, nil)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "overage" {
		t.Fatalf("sentMessages = %+v, want a single T+0 notice", notifier.sentMessages)
	}
	if reminderLog.statuses["OMS-1|+15m"] != "skipped" || reminderLog.statuses["OMS-1|+30m"] != "skipped" {
		t.Errorf("crossed post-budget tiers must be skipped, got %v", reminderLog.statuses)
	}
}

func TestEvaluateAndSend_MutedOverageNeitherSendsNorAdvancesSchedule(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	setting := withOverage(intPtr(5), "15m")
	until := time.Now().Add(time.Hour)
	setting.MutedUntil = &until
	engine := settingsEngine(reminderLog, issueSession, notifier, setting)

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(20*60, nil)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 || len(issueSession.updateOverageCalls) != 0 {
		t.Fatalf("muted: sent %d, schedule updates %d — want 0/0", len(notifier.sentMessages), len(issueSession.updateOverageCalls))
	}
	if reminderLog.statuses["OMS-1|+15m"] != "skipped" {
		t.Errorf("+15m crossed while muted must be skipped, got %q", reminderLog.statuses["OMS-1|+15m"])
	}
	if reminderLog.sentTiers["OMS-1|overage"] {
		t.Error("the T+0 notice must stay pending while muted")
	}

	// Mute over: the T+0 notice goes out right away.
	engine = settingsEngine(reminderLog, issueSession, notifier, withOverage(intPtr(5), "15m"))
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(25*60, nil)); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "overage" {
		t.Errorf("after the mute: sentMessages = %+v, want the pending T+0 notice", notifier.sentMessages)
	}
}

func TestEvaluateAndSend_QuietHoursSilenceLikeMute(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	setting := user_setting_dto.Default()
	// A 23h59m window starting one minute ago always covers "now".
	now := time.Now().In(user_setting_dto.Jakarta)
	start := now.Add(-time.Minute)
	end := now.Add(-2 * time.Minute)
	setting.QuietHours = &user_setting_dto.QuietHours{Start: start.Format("15:04"), End: end.Format("15:04")}
	engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, setting)

	session := issue_session_dto.IssueSession{IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 9100, AssigneeEmail: "anwar@esb.co.id"}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 || reminderLog.statuses["OMS-1|30m"] != "skipped" {
		t.Errorf("quiet hours: sent %d, 30m status %q — want 0 / skipped", len(notifier.sentMessages), reminderLog.statuses["OMS-1|30m"])
	}
}

// --- event notifications toggles ---

func TestNotifyIfEarlyCompletion_RespectsToggleAndMute(t *testing.T) {
	off := user_setting_dto.Default()
	off.NotifyEarlyFinish = false
	muted := user_setting_dto.Default()
	until := time.Now().Add(time.Hour)
	muted.MutedUntil = &until

	for name, setting := range map[string]user_setting_dto.UserSetting{"toggle off": off, "muted": muted} {
		t.Run(name, func(t *testing.T) {
			notifier := &fakeGoogleChatNotifierService{}
			engine := settingsEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, notifier, setting)
			if err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-1", "anwar@esb.co.id", "x", 1000, 3600, time.Now(), time.Now()); err != nil {
				t.Fatalf("NotifyIfEarlyCompletion returned error: %v", err)
			}
			if len(notifier.sentMessages) != 0 {
				t.Errorf("len(sentMessages) = %d, want 0", len(notifier.sentMessages))
			}
		})
	}
}

func TestNotifyReturnedToTodo_RespectsToggle(t *testing.T) {
	off := user_setting_dto.Default()
	off.NotifyReturnedToTodo = false
	notifier := &fakeGoogleChatNotifierService{}
	engine := settingsEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, notifier, off)

	if err := engine.NotifyReturnedToTodo(context.Background(), "OMS-1", "anwar@esb.co.id", "x", 1000, 3600, time.Now()); err != nil {
		t.Fatalf("NotifyReturnedToTodo returned error: %v", err)
	}
	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 with the toggle off", len(notifier.sentMessages))
	}
}

// --- EvaluateAndSend: overage ---

func TestEvaluateAndSend_FirstOverageSendsAndRecordsTimestamp(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, issueSession, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             3,     // budget = 10800s
		AccumulatedSeconds:      12000, // elapsed > budget by 1200s
		LastOverageReminderDate: nil,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Fatalf("len(sentMessages) = %d, want 1", len(notifier.sentMessages))
	}
	if !notifier.sentMessages[0].IsOverage {
		t.Error("IsOverage = false, want true")
	}
	if len(issueSession.updateOverageCalls) != 1 {
		t.Errorf("UpdateLastOverageReminder call count = %d, want 1", len(issueSession.updateOverageCalls))
	}
}

func TestEvaluateAndSend_OverageWithinFiveMinutesOfLastSendDoesNotResend(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, issueSession, registrationRepo, notifier)

	recent := time.Now().Add(-2 * time.Minute)
	session := issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             3,
		AccumulatedSeconds:      12000,
		LastOverageReminderDate: &recent,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — last overage reminder was only 2 minutes ago, spec AC2.4 requires a 5-minute gap", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_OverageAfterFiveMinutesResends(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, issueSession, registrationRepo, notifier)

	old := time.Now().Add(-6 * time.Minute)
	session := issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             3,
		AccumulatedSeconds:      12000,
		LastOverageReminderDate: &old,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Errorf("len(sentMessages) = %d, want 1 — 6 minutes have passed, spec AC2.4 requires resending every 5 minutes", len(notifier.sentMessages))
	}
}

func TestEvaluateAndSend_OverageSkipsSendOutsideBusinessHours(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		&fakeBusinessHoursGate{BusinessHoursService: business_hours_service.NewBusinessHoursService(), within: false},
		reminderLog, issueSession, registrationRepo, notifier, nil, "",
		nil,
	)

	old := time.Now().Add(-6 * time.Minute)
	session := issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             3,
		AccumulatedSeconds:      12000,
		LastOverageReminderDate: &old, // well past the 5-minute repeat interval
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 0 {
		t.Errorf("len(sentMessages) = %d, want 0 — outside business hours, overage reminders must not fire (would otherwise spam all night/weekend)", len(notifier.sentMessages))
	}
	if len(issueSession.updateOverageCalls) != 0 {
		t.Errorf("UpdateLastOverageReminder call count = %d, want 0 — skipping outside business hours must not touch the timestamp, so the next in-hours cycle sends immediately", len(issueSession.updateOverageCalls))
	}
}

func TestEvaluateAndSend_OverageResumesAsSoonAsBusinessHoursReturn(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(),
		reminderLog, issueSession, registrationRepo, notifier, nil, "",
		nil,
	)

	// LastOverageReminderDate stays nil because the off-hours gate above
	// never let a send reach the point of recording one — simulates a
	// ticket that sat overaged through an entire night/weekend with zero
	// sends, now evaluated on the first in-hours poll cycle.
	session := issue_session_dto.IssueSession{
		IssueKey:                "OMS-1",
		StoryPoints:             3,
		AccumulatedSeconds:      12000,
		LastOverageReminderDate: nil,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Errorf("len(sentMessages) = %d, want 1 — business hours have returned, the overdue overage reminder must send immediately", len(notifier.sentMessages))
	}
}

// --- EvaluateAndSend: assignee name resolution (AC3.3) ---

func TestEvaluateAndSend_UsesRegisteredNameWhenAssigneeHasRegistered(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{
		users: map[string]registration_dto.User{
			"anwar@esb.co.id": {UserName: "Anwar (Registered)", Email: "anwar@esb.co.id"},
		},
	}

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 12000, // forces overage so exactly one message is sent
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Fatalf("len(sentMessages) = %d, want 1", len(notifier.sentMessages))
	}
	if notifier.sentMessages[0].AssigneeName != "Anwar (Registered)" {
		t.Errorf("AssigneeName = %q, want the registered name %q", notifier.sentMessages[0].AssigneeName, "Anwar (Registered)")
	}
}

func TestEvaluateAndSend_FallsBackToRawJiraNameWhenUnregistered(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}} // nobody registered

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 12000,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("ghost@esb.co.id", "Ghost Developer"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Fatalf("len(sentMessages) = %d, want 1", len(notifier.sentMessages))
	}
	if notifier.sentMessages[0].AssigneeName != "Ghost Developer" {
		t.Errorf("AssigneeName = %q, want raw Jira displayName %q (spec AC3.3 — unregistered assignee still gets reminded, just unpersonalized)", notifier.sentMessages[0].AssigneeName, "Ghost Developer")
	}
}

func TestEvaluateAndSend_UnassignedTicketUsesPlaceholderWithoutError(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := newTestEngine(reminderLog, &fakeIssueSessionService{}, registrationRepo, notifier)

	session := issue_session_dto.IssueSession{
		IssueKey:           "OMS-1",
		StoryPoints:        3,
		AccumulatedSeconds: 12000,
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("", ""), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(notifier.sentMessages) != 1 {
		t.Fatalf("len(sentMessages) = %d, want 1", len(notifier.sentMessages))
	}
	if notifier.sentMessages[0].AssigneeName != "(unassigned)" {
		t.Errorf("AssigneeName = %q, want %q for a ticket with no assignee", notifier.sentMessages[0].AssigneeName, "(unassigned)")
	}
}

func TestEvaluateAndSend_SelfAssigneeAlwaysUsesWebhookNotifier(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	dmNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, dmNotifier, "yusuf.wijaya@esb.co.id",
		nil,
	)

	session := issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 12000, // forces overage so exactly one message is sent
		AssigneeEmail: "yusuf.wijaya@esb.co.id",
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("yusuf.wijaya@esb.co.id", "Yusuf Wijaya"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(webhookNotifier.sentMessages) != 1 {
		t.Errorf("webhookNotifier.sentMessages = %d, want 1 (self assignee must always use the webhook, AC2.1)", len(webhookNotifier.sentMessages))
	}
	if len(dmNotifier.sentMessages) != 0 {
		t.Errorf("dmNotifier.sentMessages = %d, want 0", len(dmNotifier.sentMessages))
	}
}

func TestEvaluateAndSend_TeammateAssigneeUsesDMNotifierWhenConfigured(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	dmNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, dmNotifier, "yusuf.wijaya@esb.co.id",
		nil,
	)

	session := issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 12000,
		AssigneeEmail: "teammate@esb.co.id",
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("teammate@esb.co.id", "Teammate"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(dmNotifier.sentMessages) != 1 {
		t.Errorf("dmNotifier.sentMessages = %d, want 1 (non-self assignee with OAuth configured must use DM, AC2.2)", len(dmNotifier.sentMessages))
	}
	if len(webhookNotifier.sentMessages) != 0 {
		t.Errorf("webhookNotifier.sentMessages = %d, want 0", len(webhookNotifier.sentMessages))
	}
}

func TestEvaluateAndSend_TeammateAssigneeFallsBackToWebhookWhenDMNotifierNil(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, nil, "yusuf.wijaya@esb.co.id",
		nil,
	)

	session := issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 12000,
		AssigneeEmail: "teammate@esb.co.id",
	}

	if err := engine.EvaluateAndSend(context.Background(), testIssue("teammate@esb.co.id", "Teammate"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(webhookNotifier.sentMessages) != 1 {
		t.Errorf("webhookNotifier.sentMessages = %d, want 1 (OAuth not configured — ALL reminders fall back to webhook, AC2.3)", len(webhookNotifier.sentMessages))
	}
}

func TestNotifyIfEarlyCompletion_SendsNoticeWhenElapsedBelowBudget(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{
		"teammate@esb.co.id": {UserName: "Teammate Name", Email: "teammate@esb.co.id"},
	}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, nil, "yusuf.wijaya@esb.co.id",
		nil,
	)

	startedAt := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	enteredCodeReviewAt := time.Date(2026, 10, 27, 9, 23, 20, 0, time.UTC)

	err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-1", "teammate@esb.co.id", "Test summary", 5000, 10800, startedAt, enteredCodeReviewAt)
	if err != nil {
		t.Fatalf("NotifyIfEarlyCompletion returned error: %v", err)
	}

	if len(webhookNotifier.sentMessages) != 1 {
		t.Fatalf("sentMessages = %d, want 1 (AC4.1)", len(webhookNotifier.sentMessages))
	}
	msg := webhookNotifier.sentMessages[0]
	if !msg.IsEarlyCompletion {
		t.Error("IsEarlyCompletion = false, want true")
	}
	if msg.AssigneeName != "Teammate Name" {
		t.Errorf("AssigneeName = %q, want %q (resolved via registration lookup)", msg.AssigneeName, "Teammate Name")
	}
	if !msg.CodeProcessStartedAt.Equal(startedAt) {
		t.Errorf("CodeProcessStartedAt = %v, want %v", msg.CodeProcessStartedAt, startedAt)
	}
	if !msg.CodeReviewEnteredAt.Equal(enteredCodeReviewAt) {
		t.Errorf("CodeReviewEnteredAt = %v, want %v", msg.CodeReviewEnteredAt, enteredCodeReviewAt)
	}
}

func TestNotifyIfEarlyCompletion_SkipsWhenElapsedAtOrAboveBudget(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, nil, "yusuf.wijaya@esb.co.id",
		nil,
	)

	err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-1", "teammate@esb.co.id", "Test summary", 10800, 10800, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("NotifyIfEarlyCompletion returned error: %v", err)
	}

	if len(webhookNotifier.sentMessages) != 0 {
		t.Errorf("sentMessages = %d, want 0 (AC4.2 — elapsed equals budget, not early)", len(webhookNotifier.sentMessages))
	}
}

func TestNotifyIfEarlyCompletion_RoutesSelfToWebhookAndTeammateToDM(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	dmNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, dmNotifier, "yusuf.wijaya@esb.co.id",
		nil,
	)

	if err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-1", "yusuf.wijaya@esb.co.id", "Self ticket", 1000, 10800, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("NotifyIfEarlyCompletion returned error: %v", err)
	}
	if len(webhookNotifier.sentMessages) != 1 || len(dmNotifier.sentMessages) != 0 {
		t.Errorf("self assignee must use webhook (spec AC4.3) — webhook=%d dm=%d", len(webhookNotifier.sentMessages), len(dmNotifier.sentMessages))
	}

	if err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-2", "teammate@esb.co.id", "Teammate ticket", 1000, 10800, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("NotifyIfEarlyCompletion returned error: %v", err)
	}
	if len(dmNotifier.sentMessages) != 1 {
		t.Errorf("teammate assignee must use DM (spec AC4.3) — dm=%d", len(dmNotifier.sentMessages))
	}
}

func TestNotifyReturnedToTodo_AlwaysSendsRegardlessOfElapsedVsBudget(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{
		"teammate@esb.co.id": {UserName: "Teammate Name", Email: "teammate@esb.co.id"},
	}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, nil, "yusuf.wijaya@esb.co.id",
		nil,
	)

	startedAt := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)

	// elapsedSeconds > budgetSeconds here — unlike NotifyIfEarlyCompletion,
	// this must still send: there's no "early"/"late" framing for being
	// pulled back to To Do.
	if err := engine.NotifyReturnedToTodo(context.Background(), "OMS-1", "teammate@esb.co.id", "Test summary", 99999, 10800, startedAt); err != nil {
		t.Fatalf("NotifyReturnedToTodo returned error: %v", err)
	}

	if len(webhookNotifier.sentMessages) != 1 {
		t.Fatalf("sentMessages = %d, want 1", len(webhookNotifier.sentMessages))
	}
	msg := webhookNotifier.sentMessages[0]
	if !msg.IsReturnedToTodo {
		t.Error("IsReturnedToTodo = false, want true")
	}
	if msg.AssigneeName != "Teammate Name" {
		t.Errorf("AssigneeName = %q, want %q (resolved via registration lookup)", msg.AssigneeName, "Teammate Name")
	}
	if !msg.CodeProcessStartedAt.Equal(startedAt) {
		t.Errorf("CodeProcessStartedAt = %v, want %v", msg.CodeProcessStartedAt, startedAt)
	}
}

func TestNotifyReturnedToTodo_RoutesSelfToWebhookAndTeammateToDM(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	dmNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, dmNotifier, "yusuf.wijaya@esb.co.id",
		nil,
	)

	if err := engine.NotifyReturnedToTodo(context.Background(), "OMS-1", "yusuf.wijaya@esb.co.id", "Self ticket", 1000, 10800, time.Time{}); err != nil {
		t.Fatalf("NotifyReturnedToTodo returned error: %v", err)
	}
	if len(webhookNotifier.sentMessages) != 1 || len(dmNotifier.sentMessages) != 0 {
		t.Errorf("self assignee must use webhook — webhook=%d dm=%d", len(webhookNotifier.sentMessages), len(dmNotifier.sentMessages))
	}

	if err := engine.NotifyReturnedToTodo(context.Background(), "OMS-2", "teammate@esb.co.id", "Teammate ticket", 1000, 10800, time.Time{}); err != nil {
		t.Fatalf("NotifyReturnedToTodo returned error: %v", err)
	}
	if len(dmNotifier.sentMessages) != 1 {
		t.Errorf("teammate assignee must use DM — dm=%d", len(dmNotifier.sentMessages))
	}
}

func TestEvaluateAndSend_UsesSessionAssigneeEmailNotBlankJiraEmail(t *testing.T) {
	// Reproduces the real production bug: Jira hides assignee.emailAddress
	// org-wide for non-self accounts (confirmed live for Rio/Benni), but
	// tr_issuesession.assigneeEmail was already correctly resolved and
	// stored by the poller at session-open time. sendOverageIfDue/
	// sendTierIfDue must use session.AssigneeEmail, not re-derive (and get
	// a blank string) from issue.Fields.Assignee.EmailAddress — a blank
	// string there breaks DM routing (Google Chat API rejects "users/"
	// with no identifier).
	reminderLog := newFakeReminderLogService()
	issueSession := &fakeIssueSessionService{}
	webhookNotifier := &fakeGoogleChatNotifierService{}
	dmNotifier := &fakeGoogleChatNotifierService{}
	registrationRepo := &fakeRegistrationRepository{users: map[string]registration_dto.User{}}

	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{},
		newAlwaysOpenBusinessHours(), reminderLog, issueSession,
		registrationRepo, webhookNotifier, dmNotifier, "yusuf.wijaya@esb.co.id",
		nil,
	)

	session := issue_session_dto.IssueSession{
		IssueKey: "CORE-10783", StoryPoints: 1.5, AccumulatedSeconds: 10000,
		AssigneeEmail: "rio.pratama@esb.co.id", // correctly resolved/stored by the poller
	}
	// issue.Fields.Assignee.EmailAddress is blank — the real Jira shape for
	// a non-self assignee on this org.
	issueWithBlankEmail := jira_client_dto.Issue{
		Key: "CORE-10783",
		Fields: jira_client_dto.IssueFields{
			Assignee: &jira_client_dto.Assignee{AccountID: "acc-rio", EmailAddress: "", DisplayName: "Rio Adhi Pratama"},
			Summary:  "Test ticket",
		},
	}

	if err := engine.EvaluateAndSend(context.Background(), issueWithBlankEmail, session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}

	if len(dmNotifier.sentMessages) != 1 {
		t.Fatalf("dmNotifier.sentMessages = %d, want 1 — routing must use session.AssigneeEmail, not the blank issue field", len(dmNotifier.sentMessages))
	}
	if dmNotifier.sentMessages[0].AssigneeEmail != "rio.pratama@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want %q (from session, not the blank Jira field)", dmNotifier.sentMessages[0].AssigneeEmail, "rio.pratama@esb.co.id")
	}
	if len(webhookNotifier.sentMessages) != 0 {
		t.Errorf("webhookNotifier.sentMessages = %d, want 0", len(webhookNotifier.sentMessages))
	}
}

func (f *fakeIssueSessionService) UpdateActiveSprint(ctx context.Context, issueKey string, boardID, sprintID *int64) error {
	return nil
}

// --- per-board settings (Tech Spec "Per-Board Settings & Active Sprint Dashboard") ---

func TestEvaluateAndSend_UsesTheSettingsOfTheTicketsBoard(t *testing.T) {
	reminderLog := newFakeReminderLogService()
	notifier := &fakeGoogleChatNotifierService{}
	boardSetting := user_setting_dto.Default()
	boardSetting.ReminderTiers = []string{"45m"}
	settings := &fakeUserSettings{byEmailBoard: map[string]map[int64]user_setting_dto.UserSetting{
		"anwar@esb.co.id": {28: boardSetting},
	}}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{}, newAlwaysOpenBusinessHours(),
		reminderLog, &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}},
		notifier, nil, "", settings,
	)

	board := int64(28)
	session := issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 8200, // remaining 2600s <= 45m
		AssigneeEmail: "anwar@esb.co.id", BoardID: &board,
	}
	if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
		t.Fatalf("EvaluateAndSend returned error: %v", err)
	}
	if len(settings.requestedFrom) == 0 || settings.requestedFrom[0] != 28 {
		t.Errorf("settings requested for boards %v, want 28", settings.requestedFrom)
	}
	if len(notifier.sentMessages) != 1 || notifier.sentMessages[0].Tier != "45m" {
		t.Errorf("sentMessages = %+v, want board 28's 45m tier", notifier.sentMessages)
	}
}

func TestEvaluateAndSend_TicketWithoutActiveSprintUsesDefaultScope(t *testing.T) {
	settings := &fakeUserSettings{}
	engine := NewReminderEngineService(
		"https://esb-id.atlassian.net", business_hours_dto.HolidayCalendar{}, newAlwaysOpenBusinessHours(),
		newFakeReminderLogService(), &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}},
		&fakeGoogleChatNotifierService{}, nil, "", settings,
	)
	session := issue_session_dto.IssueSession{IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 1000, AssigneeEmail: "anwar@esb.co.id"}
	engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session)

	if len(settings.requestedFrom) == 0 || settings.requestedFrom[0] != 0 {
		t.Errorf("settings requested for boards %v, want 0 (default)", settings.requestedFrom)
	}
}

func TestEvaluateAndSend_MarksCoreReviewTickets(t *testing.T) {
	notifier := &fakeGoogleChatNotifierService{}
	engine := newTestEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}}, notifier)

	issue := testIssue("anwar@esb.co.id", "Anwar")
	issue.Key = "CORE-1"
	issue.Fields.Status = jira_client_dto.Status{Name: "CODE REVIEW"}
	session := issue_session_dto.IssueSession{IssueKey: "CORE-1", StoryPoints: 3, AccumulatedSeconds: 9100}

	if err := engine.EvaluateAndSend(context.Background(), issue, session); err != nil {
		t.Fatalf("EvaluateAndSend: %v", err)
	}
	if len(notifier.sentMessages) != 1 || !notifier.sentMessages[0].InReview {
		t.Errorf("sentMessages = %+v, want one reminder flagged InReview", notifier.sentMessages)
	}
}

// --- master switch (Tech Spec "Notification On-Off Consent") ---

func switchedOff(s user_setting_dto.UserSetting) user_setting_dto.UserSetting {
	s.NotificationsOff = true
	return s
}

func TestNotificationsOff_SilencesEveryAutomaticNotification(t *testing.T) {
	t.Run("pre-budget tier: skipped, never sent", func(t *testing.T) {
		reminderLog := newFakeReminderLogService()
		notifier := &fakeGoogleChatNotifierService{}
		engine := settingsEngine(reminderLog, &fakeIssueSessionService{}, notifier, switchedOff(user_setting_dto.Default()))
		session := issue_session_dto.IssueSession{IssueKey: "OMS-1", StoryPoints: 3, AccumulatedSeconds: 9100, AssigneeEmail: "anwar@esb.co.id"}
		if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), session); err != nil {
			t.Fatal(err)
		}
		if len(notifier.sentMessages) != 0 || reminderLog.statuses["OMS-1|30m"] != "skipped" {
			t.Errorf("sent %d, 30m status %q — want 0 / skipped", len(notifier.sentMessages), reminderLog.statuses["OMS-1|30m"])
		}
	})

	t.Run("overage: not sent, schedule not advanced", func(t *testing.T) {
		issueSession := &fakeIssueSessionService{}
		notifier := &fakeGoogleChatNotifierService{}
		engine := settingsEngine(newFakeReminderLogService(), issueSession, notifier, switchedOff(withOverage(intPtr(5), "15m")))
		if err := engine.EvaluateAndSend(context.Background(), testIssue("anwar@esb.co.id", "Anwar"), overageSession(20*60, nil)); err != nil {
			t.Fatal(err)
		}
		if len(notifier.sentMessages) != 0 || len(issueSession.updateOverageCalls) != 0 {
			t.Errorf("sent %d, schedule updates %d — want 0/0", len(notifier.sentMessages), len(issueSession.updateOverageCalls))
		}
	})

	t.Run("early completion and returned to To Do", func(t *testing.T) {
		notifier := &fakeGoogleChatNotifierService{}
		engine := settingsEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, notifier, switchedOff(user_setting_dto.Default()))
		if err := engine.NotifyIfEarlyCompletion(context.Background(), "OMS-1", "anwar@esb.co.id", "x", 1000, 3600, time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := engine.NotifyReturnedToTodo(context.Background(), "OMS-1", "anwar@esb.co.id", "x", 1000, 3600, time.Now()); err != nil {
			t.Fatal(err)
		}
		if len(notifier.sentMessages) != 0 {
			t.Errorf("sent %d, want 0", len(notifier.sentMessages))
		}
	})
}
