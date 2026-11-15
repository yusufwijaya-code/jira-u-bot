package user_dashboard_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nudge/modules/board/board_dto"
	"reflect"
	"sort"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

// --- fakes ---

type fakeJiraClientForDashboard struct {
	issues        []jira_client_dto.Issue
	searchErr     error
	lastCredUsed  *jira_client_dto.Credential
	lastAccountID string
	// related backs SearchIssuesByKeysForAssignee.
	related          []jira_client_dto.Issue
	relatedErr       error
	relatedKeysAsked []string
	// transitions backs GetTransitionsAs; transitioned records
	// TransitionIssueAs as "KEY:ID".
	transitions  []jira_client_dto.Transition
	transitioned string
}

func (f *fakeJiraClientForDashboard) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return nil, nil
}
func (f *fakeJiraClientForDashboard) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (f *fakeJiraClientForDashboard) GetTrackedRunTimes(ctx context.Context, issueKey string) (time.Time, time.Time, bool, error) {
	t, found, err := f.GetLatestCodeProcessEntryTime(ctx, issueKey)
	return t, t, found, err
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
	return f.transitions, nil
}
func (f *fakeJiraClientForDashboard) TransitionIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string) error {
	f.transitioned = issueKey + ":" + transitionID
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
	usersByEmail         map[string]registration_dto.User
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
	byIssueKey map[string]issue_session_dto.IssueSession
	activeAll  []issue_session_dto.IssueSession
}

func (f *fakeIssueSessionRepoForDashboard) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionRepoForDashboard) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
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
	var own []issue_session_dto.IssueSession
	for _, session := range f.activeAll {
		if session.AssigneeEmail == email {
			own = append(own, session)
		}
	}
	return own, nil
}

func (f *fakeIssueSessionRepoForDashboard) FindBySprintNames(ctx context.Context, sprintNames []string) ([]issue_session_dto.IssueSession, error) {
	return nil, nil
}

func (f *fakeIssueSessionRepoForDashboard) ListDistinctSprintNames(ctx context.Context) ([]string, error) {
	return nil, nil
}

func newTestService(jiraClient *fakeJiraClientForDashboard, jiraCred *fakeJiraCredentialForDashboard, regRepo *fakeRegistrationRepoForDashboard, sessionRepo *fakeIssueSessionRepoForDashboard) UserDashboardService {
	return NewUserDashboardService(jiraClient, jiraCred, regRepo, sessionRepo, business_hours_service.NewBusinessHoursService(), business_hours_dto.HolidayCalendar{}, "https://esb-id.atlassian.net/", jiraClient, &fakeBoardsForDashboard{})
}

// SearchActiveSprintIssuesByAssignee lets the same fake serve the Agile
// search (the JQL itself is covered in jira_client tests).
func (f *fakeJiraClientForDashboard) SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error) {
	return f.SearchAllIssuesByAssignee(ctx, cred, accountID)
}

// SearchIssuesByKeysForAssignee returns the fake's related issues whose key
// was asked for (assignee filtering is the JQL's job, covered in
// jira_client tests — related may include someone else's row on purpose).
func (f *fakeJiraClientForDashboard) SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error) {
	f.relatedKeysAsked = keys
	if f.relatedErr != nil {
		return nil, f.relatedErr
	}
	asked := map[string]bool{}
	for _, k := range keys {
		asked[k] = true
	}
	var out []jira_client_dto.Issue
	for _, issue := range f.related {
		if asked[issue.Key] {
			out = append(out, issue)
		}
	}
	return out, nil
}

type fakeBoardsForDashboard struct{ failFor map[int64]bool }

func (f *fakeBoardsForDashboard) EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error) {
	if f.failFor[boardID] {
		return board_dto.Board{}, errors.New("jira down")
	}
	names := map[int64]string{28: "ESB OMS Board", 26: "ESB Core Board"}
	return board_dto.Board{BoardID: boardID, Name: names[boardID], ProjectKey: map[int64]string{28: "OMS", 26: "CORE"}[boardID]}, nil
}

func (f *fakeBoardsForDashboard) ActiveSprint(ctx context.Context, boardID int64) (jira_client_dto.SprintRef, bool, error) {
	return jira_client_dto.SprintRef{ID: 8294, StartDate: "2026-11-09", EndDate: "2026-11-22"}, true, nil
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

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
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

func TestGetMyBoard_ExposesSessionStartAndDeadline(t *testing.T) {
	started := time.Date(2026, 11, 14, 9, 10, 0, 0, time.FixedZone("WIB", 7*3600))
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		{Key: "OMS-1", Fields: jira_client_dto.IssueFields{Summary: "Tracked", Status: jira_client_dto.Status{Name: "Code Process"}}},
		{Key: "OMS-2", Fields: jira_client_dto.IssueFields{Summary: "Banked only", Status: jira_client_dto.Status{Name: "Code Process"}}},
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{byIssueKey: map[string]issue_session_dto.IssueSession{
		"OMS-1": {IssueKey: "OMS-1", StoryPoints: 2, FlagActive: true, CurrentSessionStartedDate: &started},
		"OMS-2": {IssueKey: "OMS-2", StoryPoints: 1, AccumulatedSeconds: 600, FlagActive: true},
	}}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			if _, err := time.Parse(time.RFC3339, c.DeadlineAt); err != nil {
				t.Errorf("%s DeadlineAt = %q, want RFC3339: %v", c.IssueKey, c.DeadlineAt, err)
			}
			switch c.IssueKey {
			case "OMS-1":
				if c.StartedAt != started.Format(time.RFC3339) {
					t.Errorf("OMS-1 StartedAt = %q, want %q", c.StartedAt, started.Format(time.RFC3339))
				}
			case "OMS-2":
				if c.StartedAt != "" {
					t.Errorf("OMS-2 StartedAt = %q, want empty when the session has no start", c.StartedAt)
				}
			}
		}
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

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
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
				if c.BudgetSeconds != 7200 || c.ElapsedSeconds != 1000 {
					t.Errorf("OMS-1 budget/elapsed = %d/%d, want 7200/1000 (numbers for the progress bar)", c.BudgetSeconds, c.ElapsedSeconds)
				}
			case "OMS-2":
				if c.HasBudgetDetail {
					t.Error("OMS-2 (Code Process, no session yet) should have HasBudgetDetail=false, not error")
				}
			case "OMS-3":
				if c.HasBudgetDetail {
					t.Error("OMS-3 (To Do) should never have HasBudgetDetail")
				}
				if c.IssueURL != "https://esb-id.atlassian.net/browse/OMS-3" {
					t.Errorf("OMS-3 IssueURL = %q, want the Jira browse link (trailing slash trimmed)", c.IssueURL)
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

	if _, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0); err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}
	if jiraClient.lastCredUsed != cred {
		t.Errorf("SearchAllIssuesByAssignee called with cred=%v, want the resolved personal credential", jiraClient.lastCredUsed)
	}
}

func TestGetMyBoard_ReturnsEmptyBoardForUnknownEmail(t *testing.T) {
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, &fakeIssueSessionRepoForDashboard{})

	board, err := svc.GetMyBoard(context.Background(), "ghost@esb.co.id", 0)
	if err != nil || len(board.Columns) != 0 {
		t.Errorf("GetMyBoard = %+v, %v, want empty board, nil error", board, err)
	}
}

// --- GetTeamView / GetOverdueView ---

func TestGetTeamView_ReturnsOnlyCallersOwnActiveSessions(t *testing.T) {
	sessionRepo := &fakeIssueSessionRepoForDashboard{activeAll: []issue_session_dto.IssueSession{
		{IssueKey: "OMS-1", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
		{IssueKey: "OMS-2", AssigneeEmail: "b@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
		{IssueKey: "OMS-3", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 100},
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, sessionRepo)

	view, err := svc.GetTeamView(context.Background(), "a@esb.co.id")
	if err != nil {
		t.Fatalf("GetTeamView returned error: %v", err)
	}
	if len(view.Groups) != 1 || view.Groups[0].AssigneeEmail != "a@esb.co.id" || len(view.Groups[0].Cards) != 2 {
		t.Fatalf("Groups = %+v, want a single group for a@esb.co.id with its 2 cards and nothing of b@esb.co.id", view.Groups)
	}
}

func TestGetTeamView_EmptyWhenCallerHasNoActiveSessions(t *testing.T) {
	sessionRepo := &fakeIssueSessionRepoForDashboard{activeAll: []issue_session_dto.IssueSession{
		{IssueKey: "OMS-2", AssigneeEmail: "b@esb.co.id", StoryPoints: 1},
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, sessionRepo)

	view, err := svc.GetTeamView(context.Background(), "a@esb.co.id")
	if err != nil || len(view.Groups) != 0 {
		t.Errorf("GetTeamView = %+v, %v, want no groups", view, err)
	}
}

func TestGetOverdueView_OnlyReturnsCallersOwnSessionsPastBudget(t *testing.T) {
	// AccumulatedSeconds (already-banked time), not CurrentSessionStartedDate
	// + wall-clock business-hours math — deterministic regardless of when
	// this test actually runs (weekend/after-hours would otherwise compute
	// near-zero elapsed business seconds for a live session).
	sessionRepo := &fakeIssueSessionRepoForDashboard{activeAll: []issue_session_dto.IssueSession{
		{IssueKey: "OMS-1", AssigneeEmail: "a@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 7200},  // 1 SP = 1h budget, 2h elapsed -> overdue
		{IssueKey: "OMS-2", AssigneeEmail: "a@esb.co.id", StoryPoints: 10, AccumulatedSeconds: 3600}, // 10h budget, well within
		{IssueKey: "OMS-3", AssigneeEmail: "b@esb.co.id", StoryPoints: 1, AccumulatedSeconds: 7200},  // overdue, but someone else's
	}}
	svc := newTestService(&fakeJiraClientForDashboard{}, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{}, sessionRepo)

	overdue, err := svc.GetOverdueView(context.Background(), "a@esb.co.id")
	if err != nil {
		t.Fatalf("GetOverdueView returned error: %v", err)
	}
	if len(overdue) != 1 || overdue[0].IssueKey != "OMS-1" {
		t.Errorf("overdue = %+v, want only OMS-1 (OMS-3 belongs to another user)", overdue)
	}
}

func (f *fakeIssueSessionRepoForDashboard) UpdateActiveSprint(ctx context.Context, issueKey string, boardID, sprintID *int64) error {
	return nil
}

// --- per board (Tech Spec "Per-Board Settings & Active Sprint Dashboard") ---

func sprintIssue(key, status string, boardID, sprintID int64, sprintName string) jira_client_dto.Issue {
	raw := fmt.Sprintf(`[{"id": %d, "name": %q, "state": "active", "boardId": %d}]`, sprintID, sprintName, boardID)
	return jira_client_dto.Issue{Key: key, Fields: jira_client_dto.IssueFields{
		Summary: key, Status: jira_client_dto.Status{Name: status}, SprintRaw: json.RawMessage(raw),
	}}
}

func boardFixture() (*fakeJiraClientForDashboard, *fakeRegistrationRepoForDashboard) {
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		sprintIssue("OMS-1", "To Do", 28, 8294, "OMS Sprint 23 - 2026"),
		sprintIssue("OMS-2", "Code Review", 28, 8294, "OMS Sprint 23 - 2026"),
		sprintIssue("CORE-1", "To Do", 26, 7988, "Core Sprint 23 2026"),
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	return jiraClient, regRepo
}

func cardKeys(board user_dashboard_dto.Board) []string {
	var keys []string
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			keys = append(keys, fmt.Sprintf("%s@%d", c.IssueKey, c.BoardID))
		}
	}
	sort.Strings(keys)
	return keys
}

func TestGetMyBoard_FiltersToTheSelectedBoardsActiveSprint(t *testing.T) {
	jiraClient, regRepo := boardFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 28)
	if err != nil {
		t.Fatalf("GetMyBoard: %v", err)
	}
	if got := cardKeys(board); !reflect.DeepEqual(got, []string{"OMS-1@28", "OMS-2@28"}) {
		t.Errorf("cards = %v, want only board 28", got)
	}

	all, _ := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if got := cardKeys(all); len(got) != 3 {
		t.Errorf("all boards = %v, want every active-sprint ticket", got)
	}
}

func TestListBoards_GroupsByActiveSprintBoardWithNamesAndCounts(t *testing.T) {
	jiraClient, regRepo := boardFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	boards, err := svc.ListBoards(context.Background(), "dev@esb.co.id")
	if err != nil || len(boards) != 2 {
		t.Fatalf("ListBoards = %+v, %v", boards, err)
	}
	// Sorted by name: Core before OMS.
	if boards[0].BoardID != 26 || boards[0].Name != "ESB Core Board" || boards[0].TicketCount != 1 || boards[0].ProjectKey != "CORE" || !boards[0].ReviewTracked {
		t.Errorf("boards[0] = %+v", boards[0])
	}
	if boards[1].BoardID != 28 || boards[1].TicketCount != 2 || boards[1].ActiveSprint.Name != "OMS Sprint 23 - 2026" || boards[1].ActiveSprint.ID != 8294 || boards[1].ReviewTracked {
		t.Errorf("boards[1] = %+v", boards[1])
	}
	if boards[1].ActiveSprint.StartDate != "2026-11-09" {
		t.Errorf("sprint dates not filled from the Agile API: %+v", boards[1].ActiveSprint)
	}
}

func TestListBoards_UnknownBoardNameFallsBack(t *testing.T) {
	jiraClient, regRepo := boardFixture()
	svc := NewUserDashboardService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{},
		business_hours_service.NewBusinessHoursService(), business_hours_dto.HolidayCalendar{}, "", jiraClient,
		&fakeBoardsForDashboard{failFor: map[int64]bool{28: true}})

	boards, err := svc.ListBoards(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ListBoards: %v", err)
	}
	for _, b := range boards {
		if b.BoardID == 28 && b.Name != "Board #28" {
			t.Errorf("name = %q, want the Board #28 fallback", b.Name)
		}
	}
}

func TestListBoards_UnregisteredUserHasNoBoards(t *testing.T) {
	jiraClient, _ := boardFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{}}, &fakeIssueSessionRepoForDashboard{})
	boards, err := svc.ListBoards(context.Background(), "nobody@esb.co.id")
	if err != nil || len(boards) != 0 {
		t.Errorf("ListBoards = %+v, %v; want none", boards, err)
	}
}

func TestGetMyBoard_CoreReviewTicketGetsBudgetDetailButOmsReviewDoesNot(t *testing.T) {
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		{Key: "CORE-1", Fields: jira_client_dto.IssueFields{Summary: "In review", Status: jira_client_dto.Status{Name: "CODE REVIEW"}}},
		{Key: "OMS-1", Fields: jira_client_dto.IssueFields{Summary: "In review", Status: jira_client_dto.Status{Name: "Code Review"}}},
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{byIssueKey: map[string]issue_session_dto.IssueSession{
		"CORE-1": {IssueKey: "CORE-1", StoryPoints: 2, AccumulatedSeconds: 5000, FlagActive: true},
		"OMS-1":  {IssueKey: "OMS-1", StoryPoints: 2, AccumulatedSeconds: 5000, FlagActive: true},
	}}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if err != nil {
		t.Fatalf("GetMyBoard: %v", err)
	}
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			if c.IssueKey == "CORE-1" && (!c.HasBudgetDetail || c.ElapsedSeconds != 5000) {
				t.Errorf("CORE-1 in CODE REVIEW must carry budget detail, got %+v", c)
			}
			if c.IssueKey == "OMS-1" && c.HasBudgetDetail {
				t.Errorf("OMS-1 in Code Review is not tracked, got budget detail %+v", c)
			}
		}
	}
}

// --- branches (Tech Spec "Subtask & Linked Item Branches") ---

// withRelations adds subtasks and outward links in the given order; links
// are key/label pairs (a slice, not a map, so the order is deterministic).
func withRelations(issue jira_client_dto.Issue, subtasks []string, links [][2]string) jira_client_dto.Issue {
	for _, key := range subtasks {
		issue.Fields.Subtasks = append(issue.Fields.Subtasks, jira_client_dto.IssueRef{Key: key})
	}
	for _, kv := range links {
		link := jira_client_dto.IssueLink{OutwardIssue: &jira_client_dto.IssueRef{Key: kv[0]}}
		link.Type.Outward = kv[1]
		issue.Fields.IssueLinks = append(issue.Fields.IssueLinks, link)
	}
	return issue
}

func plainIssue(key, status, assignee string) jira_client_dto.Issue {
	return jira_client_dto.Issue{Key: key, Fields: jira_client_dto.IssueFields{
		Summary: key + " summary", Status: jira_client_dto.Status{Name: status},
		Assignee: &jira_client_dto.Assignee{AccountID: assignee},
	}}
}

func branchFixture() (*fakeJiraClientForDashboard, *fakeRegistrationRepoForDashboard, *fakeIssueSessionRepoForDashboard) {
	parentOMS := sprintIssue("OMS-11727", "Code Review", 28, 8294, "OMS Sprint 23 - 2026")
	parentOMS = withRelations(parentOMS, []string{"OMS-11938", "OMS-11912"}, [][2]string{{"OMS-11905", "relates to"}, {"CORE-5", "relates to"}})
	sprintSubtask := sprintIssue("OMS-11939", "Code Process", 28, 8294, "OMS Sprint 23 - 2026")
	sprintSubtask.Fields.Parent = &jira_client_dto.IssueRef{Key: "OMS-11727"}
	parentCORE := withRelations(sprintIssue("CORE-1", "To Do", 26, 7988, "Core Sprint 23 2026"), []string{"CORE-2", "OMS-11938"}, nil)

	jiraClient := &fakeJiraClientForDashboard{
		issues: []jira_client_dto.Issue{parentOMS, sprintSubtask, parentCORE},
		related: []jira_client_dto.Issue{
			plainIssue("OMS-11938", "Code Process", "acc-1"), // subtask, mine, tracked
			plainIssue("OMS-11905", "Code Process", "acc-1"), // linked, mine, tracked
			plainIssue("OMS-11912", "Code Process", "acc-9"), // someone else's
			plainIssue("CORE-5", "To Do", "acc-1"),           // any status: kept for "Tiket saya per status"
			plainIssue("CORE-2", "CODE REVIEW", "acc-1"),     // CORE review is tracked
		},
	}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{byIssueKey: map[string]issue_session_dto.IssueSession{
		"OMS-11905": {IssueKey: "OMS-11905", StoryPoints: 2, AccumulatedSeconds: 3600, FlagActive: true},
	}}
	return jiraClient, regRepo, sessionRepo
}

func relatedByKey(board user_dashboard_dto.Board) map[string]user_dashboard_dto.Card {
	out := map[string]user_dashboard_dto.Card{}
	for _, c := range board.Related {
		out[c.IssueKey] = c
	}
	return out
}

func TestGetMyBoard_RelatedBranchesOfSprintTickets(t *testing.T) {
	jiraClient, regRepo, sessionRepo := branchFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if err != nil {
		t.Fatalf("GetMyBoard: %v", err)
	}

	wantAsked := []string{"OMS-11938", "OMS-11912", "OMS-11905", "CORE-5", "CORE-2"}
	if !reflect.DeepEqual(jiraClient.relatedKeysAsked, wantAsked) {
		t.Errorf("keys asked = %v, want %v (sprint tickets excluded, OMS-11938 once)", jiraClient.relatedKeysAsked, wantAsked)
	}

	related := relatedByKey(board)
	if len(related) != 4 || related["OMS-11912"].IssueKey != "" {
		t.Fatalf("related = %+v, want OMS-11938, OMS-11905, CORE-5, CORE-2 (not someone else's OMS-11912)", board.Related)
	}
	if to := related["CORE-5"]; to.HasBudgetDetail || to.ParentKey != "OMS-11727" {
		t.Errorf("CORE-5 = %+v, want an untracked branch without budget", to)
	}
	sub := related["OMS-11938"]
	if sub.ParentKey != "OMS-11727" || sub.Relation != "subtask" || sub.ParentStatus != "Code Review" || sub.ParentSummary != "OMS-11727" || sub.BoardID != 28 {
		t.Errorf("OMS-11938 = %+v, want a subtask branch of OMS-11727 on board 28 (first parent wins)", sub)
	}
	link := related["OMS-11905"]
	if link.Relation != "relates to" || link.ParentKey != "OMS-11727" || !link.HasBudgetDetail || link.BudgetSeconds != 7200 {
		t.Errorf("OMS-11905 = %+v, want a 'relates to' branch with its own budget", link)
	}
	if core := related["CORE-2"]; core.ParentKey != "CORE-1" || core.BoardID != 26 {
		t.Errorf("CORE-2 = %+v, want a branch of CORE-1 on board 26", core)
	}

	// Related items never land in the status columns.
	total := 0
	for _, col := range board.Columns {
		total += len(col.Cards)
		for _, c := range col.Cards {
			if c.IssueKey == "OMS-11939" && (c.ParentKey != "OMS-11727" || c.Relation != "subtask") {
				t.Errorf("sprint subtask OMS-11939 = %+v, want it hung under OMS-11727", c)
			}
			if c.IssueKey == "OMS-11727" && c.ParentKey != "" {
				t.Errorf("OMS-11727 must stay a top-level card, got parent %q", c.ParentKey)
			}
		}
	}
	if total != 3 {
		t.Errorf("columns hold %d cards, want the 3 sprint tickets", total)
	}
}

func TestGetMyBoard_RelatedFollowsParentBoardFilter(t *testing.T) {
	jiraClient, regRepo, sessionRepo := branchFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 26)
	if err != nil {
		t.Fatalf("GetMyBoard: %v", err)
	}
	related := relatedByKey(board)
	if len(related) != 1 || related["CORE-2"].IssueKey == "" {
		t.Errorf("board 26 related = %+v, want only CORE-2", board.Related)
	}
}

func TestGetMyBoard_RelatedLookupFailureKeepsTheBoard(t *testing.T) {
	jiraClient, regRepo, sessionRepo := branchFixture()
	jiraClient.relatedErr = errors.New("jql error")
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if err != nil {
		t.Fatalf("a related lookup failure must not fail the board: %v", err)
	}
	if len(board.Related) != 0 || len(board.Columns) == 0 {
		t.Errorf("board = %+v, want the columns without branches", board)
	}
}

func TestGetMyBoard_NoRelationsSkipsTheLookup(t *testing.T) {
	jiraClient, regRepo := boardFixture()
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{})

	if _, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0); err != nil {
		t.Fatalf("GetMyBoard: %v", err)
	}
	if jiraClient.relatedKeysAsked != nil {
		t.Errorf("keys asked = %v, want no lookup when no ticket has relations", jiraClient.relatedKeysAsked)
	}
}

// --- Tech Spec "Papan Tiket & Cuti" (US-1) ---

func newTransitionFixture() (UserDashboardService, *fakeJiraClientForDashboard) {
	jira := &fakeJiraClientForDashboard{
		issues: []jira_client_dto.Issue{{Key: "OMS-1", Fields: jira_client_dto.IssueFields{
			Summary: "Lock payments", Status: jira_client_dto.Status{Name: "To Do"},
		}}},
		transitions: []jira_client_dto.Transition{{ID: "11", Name: "Start Development", To: jira_client_dto.Status{Name: "Code Process"}}},
	}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {Email: "dev@esb.co.id", JiraAccountID: "acc-dev"},
	}}
	return newTestService(jira, &fakeJiraCredentialForDashboard{}, regRepo, &fakeIssueSessionRepoForDashboard{}), jira
}

func TestListTransitions_ReturnsJiraWorkflowOptionsForOwnSprintTicket(t *testing.T) {
	svc, _ := newTransitionFixture()
	got, err := svc.ListTransitions(context.Background(), "dev@esb.co.id", "OMS-1")
	if err != nil {
		t.Fatal(err)
	}
	want := user_dashboard_dto.TransitionOption{ID: "11", Name: "Start Development", ToStatus: "Code Process"}
	if got.IssueKey != "OMS-1" || got.CurrentStatus != "To Do" || len(got.Transitions) != 1 || got.Transitions[0] != want {
		t.Fatalf("got %+v", got)
	}
}

func TestListTransitions_RejectsTicketOutsideOwnActiveSprint(t *testing.T) {
	svc, _ := newTransitionFixture()
	if _, err := svc.ListTransitions(context.Background(), "dev@esb.co.id", "OMS-999"); !errors.Is(err, ErrNotOwnTicket) {
		t.Fatalf("err = %v, want ErrNotOwnTicket", err)
	}
	if _, err := svc.ListTransitions(context.Background(), "stranger@esb.co.id", "OMS-1"); !errors.Is(err, ErrNotOwnTicket) {
		t.Fatalf("unregistered user: err = %v, want ErrNotOwnTicket", err)
	}
}

func TestTransition_RejectsIDNotOfferedNow(t *testing.T) {
	svc, jira := newTransitionFixture()
	if _, err := svc.Transition(context.Background(), "dev@esb.co.id", "OMS-1", "31"); !errors.Is(err, ErrTransitionUnavailable) {
		t.Fatalf("err = %v, want ErrTransitionUnavailable", err)
	}
	if jira.transitioned != "" {
		t.Fatalf("nothing may be sent to Jira, got %q", jira.transitioned)
	}
}

func TestTransition_ExecutesAndReturnsNewStatus(t *testing.T) {
	svc, jira := newTransitionFixture()
	status, err := svc.Transition(context.Background(), "dev@esb.co.id", "OMS-1", "11")
	if err != nil || status != "Code Process" || jira.transitioned != "OMS-1:11" {
		t.Fatalf("status=%q err=%v transitioned=%q", status, err, jira.transitioned)
	}
}

// Papan Tiket: SP always comes from Jira, and a ticket whose clock stopped
// (finished / back to To Do) still shows the time its closed session
// recorded — without HasBudgetDetail, which the dashboard keeps for running
// clocks.
func TestGetMyBoard_StoppedTicketShowsSPAndRecordedTime(t *testing.T) {
	three, two := 3.0, 2.0
	jiraClient := &fakeJiraClientForDashboard{issues: []jira_client_dto.Issue{
		{Key: "CORE-1", Fields: jira_client_dto.IssueFields{Summary: "Done", Status: jira_client_dto.Status{Name: "Code Completed"}, StoryPointsCoreEstimate: &three}},
		{Key: "OMS-2", Fields: jira_client_dto.IssueFields{Summary: "Never tracked", Status: jira_client_dto.Status{Name: "To Do"}, StoryPoints: &two}},
	}}
	regRepo := &fakeRegistrationRepoForDashboard{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
	}}
	sessionRepo := &fakeIssueSessionRepoForDashboard{byIssueKey: map[string]issue_session_dto.IssueSession{
		"CORE-1": {IssueKey: "CORE-1", StoryPoints: 3, AccumulatedSeconds: 9000, FlagActive: false},
	}}
	svc := newTestService(jiraClient, &fakeJiraCredentialForDashboard{}, regRepo, sessionRepo)

	board, err := svc.GetMyBoard(context.Background(), "dev@esb.co.id", 0)
	if err != nil {
		t.Fatalf("GetMyBoard returned error: %v", err)
	}
	cards := map[string]user_dashboard_dto.Card{}
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			cards[c.IssueKey] = c
		}
	}
	done := cards["CORE-1"]
	if done.StoryPoints != 3 || !done.TimeRecorded || done.HasBudgetDetail {
		t.Errorf("CORE-1 sp=%v timeRecorded=%v hasBudgetDetail=%v, want 3/true/false", done.StoryPoints, done.TimeRecorded, done.HasBudgetDetail)
	}
	if done.ElapsedSeconds != 9000 || done.BudgetSeconds != 10800 {
		t.Errorf("CORE-1 elapsed/budget = %d/%d, want 9000/10800", done.ElapsedSeconds, done.BudgetSeconds)
	}
	todo := cards["OMS-2"]
	if todo.StoryPoints != 2 || todo.TimeRecorded {
		t.Errorf("OMS-2 sp=%v timeRecorded=%v, want 2/false (no session, nothing recorded)", todo.StoryPoints, todo.TimeRecorded)
	}
}

func TestTransition_AllowsOwnSubtaskOutsideTheSprint(t *testing.T) {
	svc, jira := newTransitionFixture()
	jira.issues[0] = withRelations(jira.issues[0], []string{"OMS-2", "OMS-3"}, nil)
	jira.related = []jira_client_dto.Issue{
		plainIssue("OMS-2", "To Do", "acc-dev"),
		plainIssue("OMS-3", "To Do", "acc-other"), // someone else's subtask
	}

	got, err := svc.ListTransitions(context.Background(), "dev@esb.co.id", "oms-2")
	if err != nil || got.IssueKey != "OMS-2" || got.CurrentStatus != "To Do" {
		t.Fatalf("own subtask: got %+v, err %v", got, err)
	}
	if status, err := svc.Transition(context.Background(), "dev@esb.co.id", "OMS-2", "11"); err != nil || status != "Code Process" || jira.transitioned != "OMS-2:11" {
		t.Fatalf("move own subtask: status %q err %v sent %q", status, err, jira.transitioned)
	}
	if _, err := svc.ListTransitions(context.Background(), "dev@esb.co.id", "OMS-3"); !errors.Is(err, ErrNotOwnTicket) {
		t.Errorf("someone else's subtask: err = %v, want ErrNotOwnTicket", err)
	}
	if _, err := svc.ListTransitions(context.Background(), "dev@esb.co.id", "OMS-77"); !errors.Is(err, ErrNotOwnTicket) {
		t.Errorf("unrelated ticket: err = %v, want ErrNotOwnTicket", err)
	}
}
