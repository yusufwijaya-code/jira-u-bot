package poller_service

import (
	"context"
	"encoding/json"
	"nudge/modules/board/board_dto"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
)

// --- fakes ---

type fakeJiraClientService struct {
	issues             []jira_client_dto.Issue
	searchErr          error
	changelogEntryTime time.Time
	changelogFound     bool
	changelogErr       error
	issueStatus        string
	issueStatusErr     error
	// runStart / lastEntry, when set, drive GetTrackedRunTimes directly.
	runStart, lastEntry time.Time
}

func (f *fakeJiraClientService) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return f.issues, f.searchErr
}

func (f *fakeJiraClientService) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return f.changelogEntryTime, f.changelogFound, f.changelogErr
}

func (f *fakeJiraClientService) GetTrackedRunTimes(ctx context.Context, issueKey string) (time.Time, time.Time, bool, error) {
	if !f.runStart.IsZero() {
		return f.runStart, f.lastEntry, true, nil
	}
	t, found, err := f.GetLatestCodeProcessEntryTime(ctx, issueKey)
	return t, t, found, err
}

func (f *fakeJiraClientService) LookupUserByEmail(ctx context.Context, email string) (string, bool, error) {
	return "", false, nil
}

func (f *fakeJiraClientService) GetIssueStatus(ctx context.Context, issueKey string) (string, error) {
	return f.issueStatus, f.issueStatusErr
}

func (f *fakeJiraClientService) GetTransitions(ctx context.Context, issueKey string) ([]jira_client_dto.Transition, error) {
	return nil, nil
}

func (f *fakeJiraClientService) TransitionIssue(ctx context.Context, issueKey, transitionID string) error {
	return nil
}

func (f *fakeJiraClientService) SearchAllIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error) {
	return nil, nil
}

func (f *fakeJiraClientService) GetTransitionsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.Transition, error) {
	return nil, nil
}

func (f *fakeJiraClientService) TransitionIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string) error {
	return nil
}

type fakeIssueSessionServiceForPoller struct {
	sessions      map[string]*issue_session_dto.IssueSession
	openCalls     []string
	openAtCalls   []string
	updateSPCalls map[string]float64
	closeCalls    map[string]int64

	activeSprintCalls int
}

func newFakeIssueSessionServiceForPoller() *fakeIssueSessionServiceForPoller {
	return &fakeIssueSessionServiceForPoller{
		sessions:      map[string]*issue_session_dto.IssueSession{},
		updateSPCalls: map[string]float64{},
		closeCalls:    map[string]int64{},
	}
}

func (f *fakeIssueSessionServiceForPoller) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	f.openCalls = append(f.openCalls, issueKey)
	now := time.Now()
	f.sessions[issueKey] = &issue_session_dto.IssueSession{
		IssueKey: issueKey, ProjectKey: projectKey, StoryPoints: sp,
		CurrentSessionStartedDate: &now, FlagActive: true,
		AssigneeEmail: assigneeEmail, IssueSummary: issueSummary,
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary, sprintName string) error {
	f.openAtCalls = append(f.openAtCalls, issueKey)
	f.sessions[issueKey] = &issue_session_dto.IssueSession{
		IssueKey: issueKey, ProjectKey: projectKey, StoryPoints: sp,
		CurrentSessionStartedDate: &startedAt, FlagActive: true,
		AssigneeEmail: assigneeEmail, IssueSummary: issueSummary,
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	f.updateSPCalls[issueKey] = sp
	if s, ok := f.sessions[issueKey]; ok {
		s.StoryPoints = sp
		s.AssigneeEmail = assigneeEmail
		s.IssueSummary = issueSummary
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	f.closeCalls[issueKey] = additionalSeconds
	if s, ok := f.sessions[issueKey]; ok {
		s.FlagActive = false
		s.CurrentSessionStartedDate = nil
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	var active []issue_session_dto.IssueSession
	for _, s := range f.sessions {
		if s.FlagActive {
			active = append(active, *s)
		}
	}
	return active, nil
}

func (f *fakeIssueSessionServiceForPoller) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	s, ok := f.sessions[issueKey]
	if !ok {
		return nil, false, nil
	}
	cp := *s
	return &cp, true, nil
}

func (f *fakeIssueSessionServiceForPoller) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	return nil
}

type fakeReminderEngineService struct {
	evaluateCalls []string
}

func (f *fakeReminderEngineService) ApplicableTiers(budgetSeconds int64) []string { return nil }

func (f *fakeReminderEngineService) EvaluateAndSend(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession) error {
	f.evaluateCalls = append(f.evaluateCalls, issue.Key)
	return nil
}

// fakeRegistrationRepositoryForPoller backs the poller's eligibility filter
// (only tickets whose assignee has registered via /register are tracked —
// team decision made after seeing the bot flood Google Chat with reminders
// for every CORE/OMS ticket regardless of assignee).
type fakeRegistrationRepositoryForPoller struct {
	registeredEmails  map[string]bool
	deactivatedEmails map[string]bool
}

func newFakeRegistrationRepositoryForPoller(registeredEmails ...string) *fakeRegistrationRepositoryForPoller {
	m := make(map[string]bool, len(registeredEmails))
	for _, e := range registeredEmails {
		m[e] = true
	}
	return &fakeRegistrationRepositoryForPoller{registeredEmails: m}
}

func (f *fakeRegistrationRepositoryForPoller) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	if !f.registeredEmails[email] {
		return nil, false, nil
	}
	return &registration_dto.User{Email: email}, true, nil
}

// FindUserByJiraAccountID: test convention is that an issue's
// Assignee.AccountID is set to the same string used to register the user
// (see assignedIssue/assignedCoreIssue) — a realistic Jira accountId
// format doesn't matter here, only that matching correctly routes through
// accountId rather than the (often blank in production) assignee email.
func (f *fakeRegistrationRepositoryForPoller) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	if !f.registeredEmails[jiraAccountID] {
		return nil, false, nil
	}
	return &registration_dto.User{Email: jiraAccountID, FlagActive: !f.deactivatedEmails[jiraAccountID]}, true, nil
}

func (f *fakeRegistrationRepositoryForPoller) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func (f *fakeRegistrationRepositoryForPoller) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) DeleteByID(ctx context.Context, userID int64) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

// --- helper ---

func newTestPoller(jiraClient *fakeJiraClientService, issueSession *fakeIssueSessionServiceForPoller, reminderEngine *fakeReminderEngineService, registrationRepo *fakeRegistrationRepositoryForPoller) *pollerServiceImpl {
	return &pollerServiceImpl{
		jiraClient:       jiraClient,
		issueSession:     issueSession,
		businessHours:    business_hours_service.NewBusinessHoursService(),
		reminderEngine:   reminderEngine,
		registrationRepo: registrationRepo,
		holidayCalendar:  business_hours_dto.HolidayCalendar{},
	}
}

func storyPointsPtr(v float64) *float64 { return &v }

// spNumbers puts sp on the first Story Point field of key's project
// (OMS: customfield_10016, CORE: customfield_10125).
func spNumbers(key string, sp float64) map[string]*float64 {
	return map[string]*float64{jira_client_dto.RuleForIssue(key).StoryPointFields[0]: storyPointsPtr(sp)}
}

// activeSprintRaw puts a test ticket in an active sprint — only those are
// tracked (Tech Spec "Active Sprint Tracking & Changelog Backfill").
var activeSprintRaw = json.RawMessage(`[{"id": 8294, "name": "OMS Sprint 23 - 2026", "state": "active", "boardId": 28}]`)

func assignedIssue(key, email string, sp float64) jira_client_dto.Issue {
	return jira_client_dto.Issue{
		Key: key,
		Fields: jira_client_dto.IssueFields{
			SprintRaw: activeSprintRaw,
			Numbers:   spNumbers(key, sp),
			// AccountID reuses the email string as a stand-in identifier —
			// see fakeRegistrationRepositoryForPoller.FindUserByJiraAccountID.
			Assignee: &jira_client_dto.Assignee{AccountID: email, EmailAddress: email, DisplayName: email},
		},
	}
}

// --- tests ---

func TestPollInterval_Is10Seconds(t *testing.T) {
	if pollInterval != 10*time.Second {
		t.Errorf("pollInterval = %v, want 10s", pollInterval)
	}
}

func TestPollOnce_OpensNewSessionForNewlySeenIssue(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "OMS-1" {
		t.Errorf("openAtCalls = %v, want [OMS-1]", issueSession.openAtCalls)
	}
}

func TestPollOnce_AlreadyActiveIssueUpdatesSPWithoutReopening(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 5)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	originalStart := time.Now().Add(-45 * time.Minute)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, FlagActive: true,
		CurrentSessionStartedDate: &originalStart,
	}
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openCalls) != 0 {
		t.Errorf("openCalls = %v, want none — OMS-1 was already active, must not be re-opened (this is the exact bug UpdateStoryPoints was added to prevent)", issueSession.openCalls)
	}
	if got, ok := issueSession.updateSPCalls["OMS-1"]; !ok || got != 5 {
		t.Errorf("updateSPCalls[OMS-1] = %v, want 5", issueSession.updateSPCalls["OMS-1"])
	}
	if !issueSession.sessions["OMS-1"].CurrentSessionStartedDate.Equal(originalStart) {
		t.Error("CurrentSessionStartedDate changed — steady-state poll must never touch it")
	}
}

func TestPollOnce_ClosesSessionForIssueNoLongerInCodeProcess(t *testing.T) {
	jiraClient := &fakeJiraClientService{issues: nil} // Jira no longer reports OMS-1
	issueSession := newFakeIssueSessionServiceForPoller()
	start := time.Now().Add(-1 * time.Hour)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, FlagActive: true,
		CurrentSessionStartedDate: &start,
	}
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller()
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	additional, closed := issueSession.closeCalls["OMS-1"]
	if !closed {
		t.Fatal("expected CloseSession to be called for OMS-1")
	}
	if additional < 0 {
		t.Errorf("additionalSeconds = %d, want >= 0 (never negative) — exact business-hours math is covered by Task 1b.1's dedicated tests", additional)
	}
	if issueSession.sessions["OMS-1"].FlagActive {
		t.Error("FlagActive = true, want false after close")
	}
}

func TestPollOnce_EvaluatesReminderForEveryCurrentlyActiveIssue(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{
			assignedIssue("OMS-1", "dev1@esb.co.id", 3),
			assignedIssue("CORE-1", "dev2@esb.co.id", 2),
		},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev1@esb.co.id", "dev2@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(reminderEngine.evaluateCalls) != 2 {
		t.Fatalf("evaluateCalls = %v, want 2 calls (one per active issue)", reminderEngine.evaluateCalls)
	}
}

func TestPollOnce_SkipsIssueWithUnregisteredAssignee(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "nobody@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller() // nobody registered
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openCalls) != 0 {
		t.Errorf("openCalls = %v, want none — assignee never registered via /register, ticket must not be tracked", issueSession.openCalls)
	}
	if len(reminderEngine.evaluateCalls) != 0 {
		t.Errorf("evaluateCalls = %v, want none", reminderEngine.evaluateCalls)
	}
}

func TestPollOnce_SkipsUnassignedIssue(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{
			{Key: "OMS-1", Fields: jira_client_dto.IssueFields{Numbers: spNumbers("OMS-1", 3)}}, // no Assignee
		},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller()
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openCalls) != 0 {
		t.Errorf("openCalls = %v, want none — unassigned ticket cannot be matched against the registration list", issueSession.openCalls)
	}
}

func TestPollOnce_SkipsIssueWithNilStoryPoints(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{
			{Key: "OMS-1", Fields: jira_client_dto.IssueFields{
				Assignee: &jira_client_dto.Assignee{EmailAddress: "dev@esb.co.id", DisplayName: "Dev"},
			}}, // StoryPoints is nil
		},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openCalls) != 0 {
		t.Errorf("openCalls = %v, want none — no Story Points means no budget to track against", issueSession.openCalls)
	}
}

func TestPollOnce_SkipsIssueWithZeroStoryPoints(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 0)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openCalls) != 0 {
		t.Errorf("openCalls = %v, want none — SP of exactly 0 is treated the same as no SP at all", issueSession.openCalls)
	}
}

func TestPollOnce_ClosesActiveSessionWhenTicketBecomesIneligible(t *testing.T) {
	// OMS-1 was being tracked (registered assignee), but this poll reports it
	// reassigned to someone who never registered — tracking must stop exactly
	// like an exit event, closing out the session rather than leaving it
	// active with a stale timer.
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "nobody@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	start := time.Now().Add(-30 * time.Minute)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, FlagActive: true,
		CurrentSessionStartedDate: &start,
	}
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller() // nobody registered (not even the original assignee anymore)
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if _, closed := issueSession.closeCalls["OMS-1"]; !closed {
		t.Error("expected CloseSession to be called for OMS-1 once it becomes ineligible")
	}
}

func TestBackfillActiveSessions_UsesChangelogTimeForUntrackedIssue(t *testing.T) {
	historicalStart := time.Date(2026, 10, 12, 10, 58, 19, 0, time.UTC)
	jiraClient := &fakeJiraClientService{
		issues:             []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 11)},
		changelogEntryTime: historicalStart,
		changelogFound:     true,
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "OMS-1" {
		t.Fatalf("openAtCalls = %v, want [OMS-1]", issueSession.openAtCalls)
	}
	if !issueSession.sessions["OMS-1"].CurrentSessionStartedDate.Equal(historicalStart) {
		t.Errorf("CurrentSessionStartedDate = %v, want the changelog time %v (spec AC1.4)", issueSession.sessions["OMS-1"].CurrentSessionStartedDate, historicalStart)
	}
}

func TestBackfillActiveSessions_SkipsIssueAlreadyTracked(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	existingStart := time.Now().Add(-10 * time.Minute)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", FlagActive: true, CurrentSessionStartedDate: &existingStart,
	}
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 0 {
		t.Errorf("openAtCalls = %v, want none — OMS-1 is already tracked, backfill must not touch it", issueSession.openAtCalls)
	}
}

func TestBackfillActiveSessions_FallsBackToNowWhenChangelogHasNoEntry(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues:         []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 3)},
		changelogFound: false, // no matching changelog entry found
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 {
		t.Fatalf("openAtCalls = %v, want exactly 1 call even without a changelog match", issueSession.openAtCalls)
	}
	if time.Since(*issueSession.sessions["OMS-1"].CurrentSessionStartedDate) > time.Minute {
		t.Error("CurrentSessionStartedDate should fall back to approximately now when no changelog entry is found")
	}
}

func TestBackfillActiveSessions_SkipsIneligibleIssue(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "nobody@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller() // nobody registered
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 0 {
		t.Errorf("openAtCalls = %v, want none — unregistered assignee must not be backfilled either", issueSession.openAtCalls)
	}
}

func TestBackfillActiveSessions_SkipsDeactivatedUser(t *testing.T) {
	// Tech Spec "Dashboard Jira Manage", Chapter 10a bug fix: a user
	// deactivated via the Admin Dashboard must stop being tracked, not just
	// lose login access. registeredEmails alone used to be sufficient for
	// eligibility (FindUserByJiraAccountID never checked flagActive).
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedIssue("OMS-1", "off@esb.co.id", 3)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("off@esb.co.id")
	registrationRepo.deactivatedEmails = map[string]bool{"off@esb.co.id": true}
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 0 {
		t.Errorf("openAtCalls = %v, want none — a deactivated user's ticket must not be tracked (spec AC6.2)", issueSession.openAtCalls)
	}
}

func TestPollOnce_SteadyStateRefreshesAssigneeEmailAndIssueSummary(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{
			{Key: "OMS-1", Fields: jira_client_dto.IssueFields{
				SprintRaw: activeSprintRaw,
				Numbers:   spNumbers("OMS-1", 3),
				Assignee:  &jira_client_dto.Assignee{AccountID: "reassigned@esb.co.id", EmailAddress: "reassigned@esb.co.id", DisplayName: "Reassigned"},
				Summary:   "Updated summary",
			}},
		},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	start := time.Now()
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", StoryPoints: 3, FlagActive: true,
		CurrentSessionStartedDate: &start, AssigneeEmail: "original@esb.co.id", IssueSummary: "Original summary",
	}
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("reassigned@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if issueSession.sessions["OMS-1"].AssigneeEmail != "reassigned@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want refreshed value %q", issueSession.sessions["OMS-1"].AssigneeEmail, "reassigned@esb.co.id")
	}
	if issueSession.sessions["OMS-1"].IssueSummary != "Updated summary" {
		t.Errorf("IssueSummary = %q, want refreshed value %q", issueSession.sessions["OMS-1"].IssueSummary, "Updated summary")
	}
}

func TestPollOnce_OpensNewSessionAtRealEntryTimeWhenAssigneeRegistersAfterTicketAlreadyInCodeProcess(t *testing.T) {
	// Reproduces: a ticket has been in Code Process for hours, but its
	// assignee only just registered — filterEligible excluded it from every
	// prior cycle, so this is its first appearance in currentByKey. The
	// session must still start at the REAL changelog entry time, not "now",
	// otherwise elapsed/budget tracking silently resets.
	historicalStart := time.Now().Add(-3 * time.Hour)
	jiraClient := &fakeJiraClientService{
		issues:             []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 3)},
		changelogEntryTime: historicalStart,
		changelogFound:     true,
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "OMS-1" {
		t.Fatalf("openAtCalls = %v, want [OMS-1] — entry events must use the real changelog entry time, not CURRENT_TIMESTAMP", issueSession.openAtCalls)
	}
	if !issueSession.sessions["OMS-1"].CurrentSessionStartedDate.Equal(historicalStart) {
		t.Errorf("CurrentSessionStartedDate = %v, want the changelog entry time %v — got reset to \"now\" instead of backfilled", issueSession.sessions["OMS-1"].CurrentSessionStartedDate, historicalStart)
	}
}

func TestPollOnce_OpensNewSessionAtNowWhenNoChangelogEntryFound(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues:         []jira_client_dto.Issue{assignedIssue("OMS-1", "dev@esb.co.id", 3)},
		changelogFound: false,
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("dev@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "OMS-1" {
		t.Fatalf("openAtCalls = %v, want [OMS-1]", issueSession.openAtCalls)
	}
	if time.Since(*issueSession.sessions["OMS-1"].CurrentSessionStartedDate) > time.Minute {
		t.Error("CurrentSessionStartedDate should fall back to approximately now when no changelog entry is found")
	}
}

func assignedCoreIssue(key, email string, sp float64) jira_client_dto.Issue {
	return jira_client_dto.Issue{
		Key: key,
		Fields: jira_client_dto.IssueFields{
			SprintRaw: activeSprintRaw,
			Numbers:   map[string]*float64{"customfield_11053": storyPointsPtr(sp)}, // CORE's "Story Points" field
			Assignee:  &jira_client_dto.Assignee{AccountID: email, EmailAddress: email, DisplayName: email},
		},
	}
}

func TestPollOnce_TracksCoreTicketUsingCoreStoryPointField(t *testing.T) {
	// CORE tickets carry Story Points on customfield_11053, not
	// customfield_10016 — isEligible must not treat them as "no SP".
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{assignedCoreIssue("CORE-10782", "rio.pratama@esb.co.id", 1.5)},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("rio.pratama@esb.co.id")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "CORE-10782" {
		t.Errorf("openAtCalls = %v, want [CORE-10782] — a CORE ticket's Story Points (customfield_11053) must count toward eligibility", issueSession.openAtCalls)
	}
	if issueSession.sessions["CORE-10782"].StoryPoints != 1.5 {
		t.Errorf("StoryPoints = %v, want 1.5", issueSession.sessions["CORE-10782"].StoryPoints)
	}
}

func TestPollOnce_TracksIssueWhenAssigneeEmailIsBlankButAccountIDMatchesRegisteredUser(t *testing.T) {
	// Reproduces the real production shape: Jira hides assignee.emailAddress
	// org-wide (confirmed live for Rio/Benni) but always populates
	// accountId. Eligibility must not depend on EmailAddress being set.
	jiraClient := &fakeJiraClientService{
		issues: []jira_client_dto.Issue{
			{
				Key: "CORE-10782",
				Fields: jira_client_dto.IssueFields{
					SprintRaw: activeSprintRaw,
					Numbers:   map[string]*float64{"customfield_11053": storyPointsPtr(1.5)},
					Assignee:  &jira_client_dto.Assignee{AccountID: "acc-rio-123", EmailAddress: "", DisplayName: "Rio Adhi Pratama"},
				},
			},
		},
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	registrationRepo := newFakeRegistrationRepositoryForPoller("acc-rio-123")
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, registrationRepo)

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "CORE-10782" {
		t.Fatalf("openAtCalls = %v, want [CORE-10782] — eligibility must match on accountId even when EmailAddress is blank", issueSession.openAtCalls)
	}
	if issueSession.sessions["CORE-10782"].AssigneeEmail != "acc-rio-123" {
		t.Errorf("AssigneeEmail = %q, want the REGISTERED email (%q in this fake), not the blank Jira-reported one", issueSession.sessions["CORE-10782"].AssigneeEmail, "acc-rio-123")
	}
}

func (f *fakeIssueSessionServiceForPoller) UpdateActiveSprint(ctx context.Context, issueKey string, boardID, sprintID *int64) error {
	f.activeSprintCalls++
	if s, ok := f.sessions[issueKey]; ok {
		s.BoardID, s.SprintID = boardID, sprintID
	}
	return nil
}

type fakeBoardRegistry struct{ ensured []int64 }

func (f *fakeBoardRegistry) EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error) {
	f.ensured = append(f.ensured, boardID)
	return board_dto.Board{BoardID: boardID}, nil
}

func TestPollOnce_RecordsActiveSprintBoardOnlyWhenItChanges(t *testing.T) {
	issue := assignedIssue("OMS-1", "dev@esb.co.id", 2)
	issue.Fields.SprintRaw = json.RawMessage(`[{"id": 8294, "name": "OMS Sprint 23", "state": "active", "boardId": 28}]`)
	jiraClient := &fakeJiraClientService{issues: []jira_client_dto.Issue{issue}}
	issueSession := newFakeIssueSessionServiceForPoller()
	start := time.Now().Add(-time.Minute)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{IssueKey: "OMS-1", StoryPoints: 2, FlagActive: true, CurrentSessionStartedDate: &start}
	poller := newTestPoller(jiraClient, issueSession, &fakeReminderEngineService{}, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))
	boards := &fakeBoardRegistry{}
	poller.boards = boards

	for i := 0; i < 2; i++ {
		if err := poller.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce %d returned error: %v", i, err)
		}
	}

	s := issueSession.sessions["OMS-1"]
	if s.BoardID == nil || *s.BoardID != 28 || s.SprintID == nil || *s.SprintID != 8294 {
		t.Errorf("session board/sprint = %v/%v, want 28/8294", s.BoardID, s.SprintID)
	}
	if issueSession.activeSprintCalls != 1 || len(boards.ensured) != 1 {
		t.Errorf("UpdateActiveSprint calls = %d, EnsureKnown calls = %d; want 1 each (the second poll is unchanged)", issueSession.activeSprintCalls, len(boards.ensured))
	}
}

// --- per-project tracking rules (Tech Spec "CORE: Waktu Code Review Ikut Dihitung") ---

func activeSession(key string) *issue_session_dto.IssueSession {
	start := time.Now().Add(-10 * time.Minute)
	return &issue_session_dto.IssueSession{
		IssueKey: key, StoryPoints: 3, FlagActive: true,
		CurrentSessionStartedDate: &start, AssigneeEmail: "teammate@esb.co.id", IssueSummary: "Test ticket",
	}
}

func TestPollOnce_CoreExitToFeedbackStopsSilently(t *testing.T) {
	issueSession := newFakeIssueSessionServiceForPoller()
	issueSession.sessions["CORE-1"] = activeSession("CORE-1")
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(&fakeJiraClientService{issueStatus: "Feedback"}, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller())

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if issueSession.closeCalls["CORE-1"] == 0 && issueSession.sessions["CORE-1"].FlagActive {
		t.Error("the session must be closed")
	}
	if len(reminderEngine.evaluateCalls) != 0 {
		t.Error("leaving to Feedback must not evaluate reminders")
	}
}

func TestPollOnce_CoreTicketInReviewKeepsItsSessionOpen(t *testing.T) {
	issue := assignedIssue("CORE-1", "dev@esb.co.id", 3)
	issue.Fields.Status = jira_client_dto.Status{Name: "CODE REVIEW"}
	issueSession := newFakeIssueSessionServiceForPoller()
	issueSession.sessions["CORE-1"] = activeSession("CORE-1")
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(&fakeJiraClientService{issues: []jira_client_dto.Issue{issue}}, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if issueSession.closeCalls["CORE-1"] != 0 || !issueSession.sessions["CORE-1"].FlagActive {
		t.Error("a CORE ticket still in CODE REVIEW must keep its session open")
	}
	if len(reminderEngine.evaluateCalls) != 1 {
		t.Errorf("evaluateCalls = %v, want reminders to keep running during review", reminderEngine.evaluateCalls)
	}
}

func TestPollOnce_NewSessionStartsAtRunStartReopenedAtLastEntry(t *testing.T) {
	runStart := time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)
	lastEntry := time.Date(2026, 11, 11, 14, 0, 0, 0, time.Local)

	for name, existed := range map[string]bool{"new": false, "reopened": true} {
		t.Run(name, func(t *testing.T) {
			issue := assignedIssue("CORE-1", "dev@esb.co.id", 3)
			issueSession := newFakeIssueSessionServiceForPoller()
			if existed {
				closed := activeSession("CORE-1")
				closed.FlagActive = false
				closed.CurrentSessionStartedDate = nil
				closed.AccumulatedSeconds = 3600
				issueSession.sessions["CORE-1"] = closed
			}
			jiraClient := &fakeJiraClientService{issues: []jira_client_dto.Issue{issue}, runStart: runStart, lastEntry: lastEntry}
			poller := newTestPoller(jiraClient, issueSession, &fakeReminderEngineService{}, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

			if err := poller.pollOnce(context.Background()); err != nil {
				t.Fatalf("pollOnce: %v", err)
			}
			got := issueSession.sessions["CORE-1"].CurrentSessionStartedDate
			want := runStart
			if existed {
				want = lastEntry
			}
			if got == nil || !got.Equal(want) {
				t.Errorf("session started at %v, want %v", got, want)
			}
		})
	}
}

// --- active sprint only (Tech Spec "Active Sprint Tracking & Changelog Backfill") ---

func staleIssue(key, email string, sprints string) jira_client_dto.Issue {
	issue := assignedCoreIssue(key, email, 2)
	issue.Fields.SprintRaw = json.RawMessage(sprints)
	return issue
}

func TestPollOnce_IgnoresTicketsOutsideAnActiveSprint(t *testing.T) {
	jiraClient := &fakeJiraClientService{issues: []jira_client_dto.Issue{
		staleIssue("CORE-8033", "dev@esb.co.id", `[{"id": 2393, "name": "Core Sprint 13", "state": "closed", "boardId": 26}]`),
		staleIssue("CORE-9000", "dev@esb.co.id", `null`), // never put in a sprint
		assignedCoreIssue("CORE-10955", "dev@esb.co.id", 4),
	}}
	issueSession := newFakeIssueSessionServiceForPoller()
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "CORE-10955" {
		t.Errorf("opened = %v, want only the active-sprint ticket CORE-10955", issueSession.openAtCalls)
	}
	if len(reminderEngine.evaluateCalls) != 1 {
		t.Errorf("evaluated = %v, want only CORE-10955", reminderEngine.evaluateCalls)
	}

	// Startup backfill applies the same rule.
	issueSession = newFakeIssueSessionServiceForPoller()
	poller = newTestPoller(jiraClient, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))
	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions: %v", err)
	}
	if len(issueSession.openAtCalls) != 1 || issueSession.openAtCalls[0] != "CORE-10955" {
		t.Errorf("startup backfill opened = %v, want only CORE-10955", issueSession.openAtCalls)
	}
}

func TestPollOnce_ClosesStaleSessionWithoutTimeOrNotice(t *testing.T) {
	jiraClient := &fakeJiraClientService{
		issues:      []jira_client_dto.Issue{staleIssue("CORE-8033", "dev@esb.co.id", `[{"id": 2393, "name": "Core Sprint 13", "state": "closed", "boardId": 26}]`)},
		issueStatus: "Code Completed",
	}
	issueSession := newFakeIssueSessionServiceForPoller()
	started := time.Date(2025, 6, 23, 8, 21, 0, 0, time.UTC)
	issueSession.sessions["CORE-8033"] = &issue_session_dto.IssueSession{
		IssueKey: "CORE-8033", StoryPoints: 2, FlagActive: true, CurrentSessionStartedDate: &started, AssigneeEmail: "dev@esb.co.id",
	}
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(jiraClient, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	added, closed := issueSession.closeCalls["CORE-8033"]
	if !closed || added != 0 {
		t.Errorf("CloseSession(CORE-8033) closed=%v added=%d, want closed with 0 seconds", closed, added)
	}
	if len(reminderEngine.evaluateCalls) != 0 {
		t.Errorf("reminder engine called (%v), want nothing for a stale ticket", reminderEngine.evaluateCalls)
	}
}

// Tech Spec "Notifikasi Perubahan Status Tiket": leaving the tracked
// statuses only closes the session — the notice is status_watch's job.
func TestPollOnce_ExitClosesSessionWithoutNotice(t *testing.T) {
	issueSession := newFakeIssueSessionServiceForPoller()
	issueSession.sessions["OMS-1"] = activeSession("OMS-1")
	jira := &fakeJiraClientService{issueStatus: "Code Review"}
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(jira, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller())

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if _, closed := issueSession.closeCalls["OMS-1"]; !closed {
		t.Error("the session must be closed")
	}
	if len(reminderEngine.evaluateCalls) != 0 {
		t.Errorf("reminder engine called %v, want nothing", reminderEngine.evaluateCalls)
	}
}
