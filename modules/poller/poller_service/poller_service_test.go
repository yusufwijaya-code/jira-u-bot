package poller_service

import (
	"context"
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

// --- fakes ---

type fakeJiraClientService struct {
	issues             []jira_client_dto.Issue
	searchErr          error
	changelogEntryTime time.Time
	changelogFound     bool
	changelogErr       error
}

func (f *fakeJiraClientService) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return f.issues, f.searchErr
}

func (f *fakeJiraClientService) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return f.changelogEntryTime, f.changelogFound, f.changelogErr
}

func (f *fakeJiraClientService) LookupUserByEmail(ctx context.Context, email string) (bool, error) {
	return false, nil
}

type fakeIssueSessionServiceForPoller struct {
	sessions      map[string]*issue_session_dto.IssueSession
	openCalls     []string
	openAtCalls   []string
	updateSPCalls map[string]float64
	closeCalls    map[string]int64
}

func newFakeIssueSessionServiceForPoller() *fakeIssueSessionServiceForPoller {
	return &fakeIssueSessionServiceForPoller{
		sessions:      map[string]*issue_session_dto.IssueSession{},
		updateSPCalls: map[string]float64{},
		closeCalls:    map[string]int64{},
	}
}

func (f *fakeIssueSessionServiceForPoller) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64) error {
	f.openCalls = append(f.openCalls, issueKey)
	now := time.Now()
	f.sessions[issueKey] = &issue_session_dto.IssueSession{
		IssueKey: issueKey, ProjectKey: projectKey, StoryPoints: sp,
		CurrentSessionStartedDate: &now, FlagActive: true,
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time) error {
	f.openAtCalls = append(f.openAtCalls, issueKey)
	f.sessions[issueKey] = &issue_session_dto.IssueSession{
		IssueKey: issueKey, ProjectKey: projectKey, StoryPoints: sp,
		CurrentSessionStartedDate: &startedAt, FlagActive: true,
	}
	return nil
}

func (f *fakeIssueSessionServiceForPoller) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64) error {
	f.updateSPCalls[issueKey] = sp
	if s, ok := f.sessions[issueKey]; ok {
		s.StoryPoints = sp
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
	registeredEmails map[string]bool
}

func newFakeRegistrationRepositoryForPoller(registeredEmails ...string) *fakeRegistrationRepositoryForPoller {
	m := make(map[string]bool, len(registeredEmails))
	for _, e := range registeredEmails {
		m[e] = true
	}
	return &fakeRegistrationRepositoryForPoller{registeredEmails: m}
}

func (f *fakeRegistrationRepositoryForPoller) UpsertUser(ctx context.Context, name, email string) error {
	return nil
}

func (f *fakeRegistrationRepositoryForPoller) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	if !f.registeredEmails[email] {
		return nil, false, nil
	}
	return &registration_dto.User{Email: email}, true, nil
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

func assignedIssue(key, email string, sp float64) jira_client_dto.Issue {
	return jira_client_dto.Issue{
		Key: key,
		Fields: jira_client_dto.IssueFields{
			StoryPoints: storyPointsPtr(sp),
			Assignee:    &jira_client_dto.Assignee{EmailAddress: email, DisplayName: email},
		},
	}
}

// --- tests ---

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

	if len(issueSession.openCalls) != 1 || issueSession.openCalls[0] != "OMS-1" {
		t.Errorf("openCalls = %v, want [OMS-1]", issueSession.openCalls)
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
			{Key: "OMS-1", Fields: jira_client_dto.IssueFields{StoryPoints: storyPointsPtr(3)}}, // no Assignee
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
