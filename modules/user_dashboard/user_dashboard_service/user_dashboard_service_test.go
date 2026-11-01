package user_dashboard_service

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

type fakeJiraClientForDashboard struct {
	issues        []jira_client_dto.Issue
	searchErr     error
	lastCredUsed  *jira_client_dto.Credential
	lastAccountID string
}

func (f *fakeJiraClientForDashboard) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return nil, nil
}
func (f *fakeJiraClientForDashboard) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (f *fakeJiraClientForDashboard) LookupUserByEmail(ctx context.Context, email string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeJiraClientForDashboard) GetIssueStatus(ctx context.Context, issueKey string) (string, error) {
	return "", nil
}
func (f *fakeJiraClientForDashboard) GetTransitions(ctx context.Context, issueKey string) ([]jira_client_dto.Transition, error) {
	return nil, nil
}
func (f *fakeJiraClientForDashboard) TransitionIssue(ctx context.Context, issueKey, transitionID string) error {
	return nil
}
func (f *fakeJiraClientForDashboard) SearchAllIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error) {
	f.lastCredUsed = cred
	f.lastAccountID = accountID
	return f.issues, f.searchErr
}
func (f *fakeJiraClientForDashboard) GetTransitionsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.Transition, error) {
	return nil, nil
}
func (f *fakeJiraClientForDashboard) TransitionIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string) error {
	return nil
}

type fakeJiraCredentialForDashboard struct {
	cred *jira_client_dto.Credential
	err  error
}

func (f *fakeJiraCredentialForDashboard) ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error) {
	return f.cred, f.err
}
func (f *fakeJiraCredentialForDashboard) UpsertCredential(ctx context.Context, userID int64, jiraEmail, apiToken string) error {
	return nil
}
func (f *fakeJiraCredentialForDashboard) DeleteCredential(ctx context.Context, userID int64) error {
	return nil
}
func (f *fakeJiraCredentialForDashboard) MaskedPreview(ctx context.Context, userID int64) (string, bool, error) {
	return "", false, nil
}

type fakeRegistrationRepoForDashboard struct {
	usersByEmail        map[string]registration_dto.User
	updatedPreferenceCSV string
}

func (f *fakeRegistrationRepoForDashboard) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForDashboard) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}
func (f *fakeRegistrationRepoForDashboard) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForDashboard) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}
func (f *fakeRegistrationRepoForDashboard) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	f.updatedPreferenceCSV = csv
	return nil
}

func (f *fakeRegistrationRepoForDashboard) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func (f *fakeRegistrationRepoForDashboard) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	return nil
}

func (f *fakeRegistrationRepoForDashboard) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepoForDashboard) DeleteByID(ctx context.Context, userID int64) error {
	return nil
}
func (f *fakeRegistrationRepoForDashboard) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

type fakeIssueSessionRepoForDashboard struct {
	byIssueKey   map[string]issue_session_dto.IssueSession
	activeAll    []issue_session_dto.IssueSession
}

func (f *fakeIssueSessionRepoForDashboard) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	return f.activeAll, nil
}
func (f *fakeIssueSessionRepoForDashboard) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	s, ok := f.byIssueKey[issueKey]
	if !ok {
		return nil, false, nil
	}
	return &s, true, nil
}
func (f *fakeIssueSessionRepoForDashboard) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) FindActiveByAssigneeEmail(ctx context.Context, email string) ([]issue_session_dto.IssueSession, error) {
	return nil, nil
}

func newTestService(jiraClient *fakeJiraClientForDashboard, jiraCred *fakeJiraCredentialForDashboard, regRepo *fakeRegistrationRepoForDashboard, sessionRepo *fakeIssueSessionRepoForDashboard) UserDashboardService {
	return NewUserDashboardService(jiraClient, jiraCred, regRepo, sessionRepo, business_hours_service.NewBusinessHoursService(), business_hours_dto.HolidayCalendar{})
}

// --- GetMyBoard ---

func TestGetMyBoard_GroupsCardsByStatus(t *testing.T) {
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		{Key: "OMS-1", Fields: jira_client_dto.IssueFields{Summary: "A", Status: jira_client_dto.Status{Name: "To Do"}}},
		{Key: "OMS-2", Fields: jira_client_dto.IssueFields{Summary: "B", Status: jira_client_dto.Status{Name: "Done"}}},
		{Key: "OMS-3", Fields: jira_client_dto.IssueFields{Summary: "C", Status: jira_client_dto.Status{Name: "To Do"}}},
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}
	if len(board.Columns) != 2 {
		t.Fatalf("len(Columns) = %d, want 2 (To Do, Done)", len(board.Columns))
	}
	if board.Columns[0].Status != "To Do" || len(board.Columns[0].Cards) != 2 {
		t.Errorf("Columns[0] = %+v, want status=To Do with 2 cards", board.Columns[0])
	}
	if jiraClient.lastAccountID != "acc-1" {
		t.Errorf("SearchAllIssuesByAssignee called with accountID=%q, want acc-1", jiraClient.lastAccountID)
	}
}

func TestGetMyBoard_AttachesBudgetDetailOnlyForCodeProcessWithSession(t *testing.T) {
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		{Key: "OMS-1", Fields: jira_client_dto.IssueFields{Summary: "Tracked", Status: jira_client_dto.Status{Name: "Code Process"}}},
		{Key: "OMS-2", Fields: jira_client_dto.IssueFields{Summary: "Not Tracked Yet", Status: jira_client_dto.Status{Name: "Code Process"}}},
		{Key: "OMS-3", Fields: jira_client_dto.IssueFields{Summary: "Todo", Status: jira_client_dto.Status{Name: "To Do"}}},
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{byIssueKey: map[string]issue_session_dto.IssueSession{
		"OMS-1": {IssueKey: "OMS-1", StoryPoints: 2, AccumulatedSeconds: 1000, FlagActive: true},
	}}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}

	for _, col := range board.Columns {
		for i := range col.Cards {
			c := col.Cards[i]
			switch c.IssueKey {
			case "OMS-1":
				if !c.HasBudgetDetail {
					t.Error("OMS-1 (Code Process, has session) should have HasBudgetDetail=true")
				}
			case "OMS-2":
				if c.HasBudgetDetail {
					t.Error("OMS-2 (Code Process, no session yet) should have HasBudgetDetail=false, not error")
				}
			case "OMS-3":
				if c.HasBudgetDetail {
					t.Error("OMS-3 (To Do) should never have HasBudgetDetail")
				}
			}
		}
	}
}

func TestGetMyBoard_UsesPersonalCredentialWhenSet(t *testing.T) {
	jiraClient := &fakeJiraClientForDashboard{}
	cred := &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "personal-token"}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{cred: cred}, regRepo, &fakeIssueSessionRepoForDashboard{})

	if _, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id"); err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}
	if jiraClient.lastCredUsed != cred {
		t.Errorf("SearchAllIssuesByAssignee called with cred=%v, want the resolved personal credential", jiraClient.lastCredUsed)
	}
}

func TestGetMyBoard_ReturnsEmptyBoardForUnknownEmail(t *testing.T) {
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, &fakeIssueSessionRepoForDashboard{})

	board, err := svc.GetMyBoard(context.Background(), "ghost@esb.co.id")
	if err != nil || len(board.Columns) != 0 {
		t.Errorf("GetMyBoard = %+v, %v, want empty board, nil error", board, err)
	}
}

// --- GetTeamView / GetOverdueView ---

func TestGetTeamView_GroupsActiveSessionsByAssignee(t *testing.T) {
	sessionRepo := &fakeIssueSessionRepoForDashboard{activeAll: []issue_session_dto.IssueSession{
		{IssueKey: "OMS-1", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
		{IssueKey: "OMS-2", AssigneeEmail: "b@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
		{IssueKey: "OMS-3", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, sessionRepo)

	view, err := svc.GetTeamView(context.Background())
	if err != nil {
		t.Fatalf("GetTeamView returned error: %v", err)
	}
	if len(view.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2", len(view.Groups))
	}
}

func TestGetOverdueView_OnlyReturnsSessionsPastBudget(t *testing.T) {
	// AccumulatedSeconds (already-banked time), not CurrentSessionStartedDate
	// + wall-clock business-hours math — deterministic regardless of when
	// this test actually runs (weekend/after-hours would otherwise compute
	// near-zero elapsed business seconds for a live session).
	sessionRepo := &fakeIssueSessionRepoForDashboard{activeAll: []issue_session_dto.IssueSession{
		{IssueKey: "OMS-1", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 7200}, // 1 SP = 1h budget, 2h elapsed -> overdue
		{IssueKey: "OMS-2", AssigneeEmail: "b@esb.co.id", StoryPoints: 10, AccumulatedSeconds: 3600}, // 10h budget, well within
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, sessionRepo)

	overdue, err := svc.GetOverdueView(context.Background())
	if err != nil {
		t.Fatalf("GetOverdueView returned error: %v", err)
	}
	if len(overdue) != 1 || overdue[0].IssueKey != "OMS-1" {
		t.Errorf("overdue = %+v, want only OMS-1", overdue)
	}
}

// --- SetReminderTierPreference ---

func TestGetReminderTierPreference_ReturnsSavedValue(t *testing.T) {
	pref := "1h,15m"
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {Email: "dev@esb.co.id", ReminderTierPreference: &pref},
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	got, err := svc.GetReminderTierPreference(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("GetReminderTierPreference returned error: %v", err)
	}
	if got == nil || *got != "1h,15m" {
		t.Errorf("GetReminderTierPreference = %v, want \"1h,15m\"", got)
	}
}

func TestGetReminderTierPreference_NilForUserWhoNeverSetOne(t *testing.T) {
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {Email: "dev@esb.co.id"},
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	got, err := svc.GetReminderTierPreference(context.Background(), "dev@esb.co.id")
	if err != nil || got != nil {
		t.Errorf("GetReminderTierPreference = %v, %v, want nil, nil (spec AC2.2)", got, err)
	}
}

func TestSetReminderTierPreference_PersistsCSV(t *testing.T) {
	regRepo := &fakeRegistrationRepoForDashboard{}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	if err := svc.SetReminderTierPreference(context.Background(), "dev@esb.co.id", "1h,15m"); err != nil {
		t.Fatalf("SetReminderTierPreference returned error: %v", err)
	}
	if regRepo.updatedPreferenceCSV != "1h,15m" {
		t.Errorf("updatedPreferenceCSV = %q, want %q", regRepo.updatedPreferenceCSV, "1h,15m")
	}
}
