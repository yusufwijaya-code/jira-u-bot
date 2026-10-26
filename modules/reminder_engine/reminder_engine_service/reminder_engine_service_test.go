package reminder_engine_service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

// --- fakes ---

type fakeReminderLogService struct {
	sentTiers map[string]bool
}

func newFakeReminderLogService() *fakeReminderLogService {
	return &fakeReminderLogService{sentTiers: map[string]bool{}}
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

func (f *fakeIssueSessionService) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64) error {
	return nil
}
func (f *fakeIssueSessionService) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time) error {
	return nil
}
func (f *fakeIssueSessionService) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64) error {
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

func (f *fakeRegistrationRepository) UpsertUser(ctx context.Context, name, email, googleChatUserID string) error {
	return nil
}
func (f *fakeRegistrationRepository) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

type fakeGoogleChatNotifierService struct {
	sentMessages []google_chat_notifier_dto.ReminderMessage
}

func (f *fakeGoogleChatNotifierService) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	f.sentMessages = append(f.sentMessages, msg)
	return nil
}

// --- helpers ---

func newTestEngine(reminderLog *fakeReminderLogService, issueSession *fakeIssueSessionService, registrationRepo *fakeRegistrationRepository, notifier *fakeGoogleChatNotifierService) ReminderEngineService {
	return NewReminderEngineService(
		"https://esb-id.atlassian.net",
		business_hours_dto.HolidayCalendar{},
		business_hours_service.NewBusinessHoursService(),
		reminderLog,
		issueSession,
		registrationRepo,
		notifier,
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

	want := []string{"30m", "15m", "1m"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplicableTiers(3600) = %v, want %v (2h and 1h must be skipped — budget must be STRICTLY greater than tier duration, spec AC2.2)", got, want)
	}
}

func TestApplicableTiers_AllTiersApplyForLargeBudget(t *testing.T) {
	engine := newTestEngine(newFakeReminderLogService(), &fakeIssueSessionService{}, &fakeRegistrationRepository{users: map[string]registration_dto.User{}}, &fakeGoogleChatNotifierService{})

	got := engine.ApplicableTiers(10800) // SP = 3, budget = 3 hours

	want := []string{"2h", "1h", "30m", "15m", "1m"}
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
