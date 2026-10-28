package chat_interactive_service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

var errGeserTest = errors.New("jira api error")

type fakeJiraClientServiceForChat struct {
	transitions     []jira_client_dto.Transition
	transitionsErr  error
	transitionErr   error
	transitionCalls []string // "issueKey|transitionID"
}

func (f *fakeJiraClientServiceForChat) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return nil, nil
}
func (f *fakeJiraClientServiceForChat) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (f *fakeJiraClientServiceForChat) LookupUserByEmail(ctx context.Context, email string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeJiraClientServiceForChat) GetIssueStatus(ctx context.Context, issueKey string) (string, error) {
	return "", nil
}
func (f *fakeJiraClientServiceForChat) GetTransitions(ctx context.Context, issueKey string) ([]jira_client_dto.Transition, error) {
	return f.transitions, f.transitionsErr
}
func (f *fakeJiraClientServiceForChat) TransitionIssue(ctx context.Context, issueKey, transitionID string) error {
	f.transitionCalls = append(f.transitionCalls, issueKey+"|"+transitionID)
	return f.transitionErr
}

type fakeRegistrationRepoForChat struct {
	users map[string]registration_dto.User
}

func (f *fakeRegistrationRepoForChat) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForChat) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}
func (f *fakeRegistrationRepoForChat) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForChat) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}
func (f *fakeRegistrationRepoForChat) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

type fakeIssueSessionRepoForChat struct {
	byAssignee map[string][]issue_session_dto.IssueSession
	all        []issue_session_dto.IssueSession
	byKey      map[string]issue_session_dto.IssueSession
}

func (f *fakeIssueSessionRepoForChat) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForChat) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForChat) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64, assigneeEmail, issueSummary string) error {
	return nil
}
func (f *fakeIssueSessionRepoForChat) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return nil
}
func (f *fakeIssueSessionRepoForChat) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	return f.all, nil
}
func (f *fakeIssueSessionRepoForChat) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	s, ok := f.byKey[issueKey]
	if !ok {
		return nil, false, nil
	}
	return &s, true, nil
}
func (f *fakeIssueSessionRepoForChat) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	return nil
}
func (f *fakeIssueSessionRepoForChat) FindActiveByAssigneeEmail(ctx context.Context, email string) ([]issue_session_dto.IssueSession, error) {
	return f.byAssignee[email], nil
}

func newTestService(registered map[string]registration_dto.User, issueRepo *fakeIssueSessionRepoForChat) ChatInteractiveService {
	return newTestServiceWithJira(registered, issueRepo, &fakeJiraClientServiceForChat{})
}

func newTestServiceWithJira(registered map[string]registration_dto.User, issueRepo *fakeIssueSessionRepoForChat, jiraClient *fakeJiraClientServiceForChat) ChatInteractiveService {
	if issueRepo == nil {
		issueRepo = &fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}}
	}
	return NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: registered},
		issueRepo,
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
		jiraClient,
	)
}

func TestRouteCommand_UnregisteredUserGetsInviteToRegister(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{}, nil)

	got := svc.RouteCommand(context.Background(), "ghost@esb.co.id", "/u-mytickets")

	if !strings.Contains(got, "labcup.tech/jira-notif/register") {
		t.Errorf("RouteCommand() = %q, want it to contain the registration link", got)
	}
}

func TestRouteCommand_UnknownTextFallsBackToHelp(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "woi kapan deploy")

	if !strings.Contains(got, "/u-mytickets") || !strings.Contains(got, "/u-budget") || !strings.Contains(got, "/u-overdue") || !strings.Contains(got, "/u-team") || !strings.Contains(got, "/u-geser") {
		t.Errorf("RouteCommand() = %q, want help text listing all 5 commands including /u-geser", got)
	}
}

func TestRouteCommand_UHelpShowsHelp(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-help")

	if !strings.Contains(got, "/u-mytickets") {
		t.Errorf("RouteCommand() = %q, want help text for /u-help", got)
	}
}

func TestRouteCommand_MyTicketsListsActiveTicketsForInvoker(t *testing.T) {
	started := time.Now().Add(-30 * time.Minute)
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{
			"dev@esb.co.id": {
				{IssueKey: "OMS-1", StoryPoints: 2, AccumulatedSeconds: 600, CurrentSessionStartedDate: &started, IssueSummary: "Fix bug"},
			},
		},
		byKey: map[string]issue_session_dto.IssueSession{},
	}
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-mytickets")

	if !strings.Contains(got, "OMS-1") {
		t.Errorf("RouteCommand() = %q, want it to mention OMS-1", got)
	}
	if !strings.Contains(got, "Fix bug") {
		t.Errorf("RouteCommand() = %q, want it to mention the issue summary", got)
	}
}

func TestRouteCommand_MyTicketsNoActiveTicketsIsFriendly(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-mytickets")

	if strings.Contains(got, "error") || got == "" {
		t.Errorf("RouteCommand() = %q, want a friendly no-tickets message, not empty/error", got)
	}
}

func TestRouteCommand_BudgetFoundTicketShowsDetail(t *testing.T) {
	started := time.Now().Add(-15 * time.Minute)
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"CORE-99": {IssueKey: "CORE-99", StoryPoints: 2, AccumulatedSeconds: 1800, CurrentSessionStartedDate: &started, IssueSummary: "Audit screen", FlagActive: true},
		},
	}
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-budget CORE-99")

	if !strings.Contains(got, "CORE-99") || !strings.Contains(got, "Audit screen") {
		t.Errorf("RouteCommand() = %q, want ticket detail for CORE-99", got)
	}
}

func TestRouteCommand_BudgetTicketNotFound(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-budget CORE-404")

	if !strings.Contains(got, "tidak ditemukan") {
		t.Errorf("RouteCommand() = %q, want a not-found message", got)
	}
}

func TestRouteCommand_BudgetWithoutArgumentShowsUsage(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-budget")

	if !strings.Contains(got, "/u-budget") {
		t.Errorf("RouteCommand() = %q, want usage instructions mentioning /u-budget", got)
	}
}

func TestRouteCommand_OverdueListsOnlyOverBudgetTickets(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey:      map[string]issue_session_dto.IssueSession{},
		all: []issue_session_dto.IssueSession{
			{IssueKey: "OVER-1", StoryPoints: 1, AccumulatedSeconds: 7200, CurrentSessionStartedDate: nil, AssigneeEmail: "a@esb.co.id", IssueSummary: "Over budget"},
			{IssueKey: "UNDER-1", StoryPoints: 100, AccumulatedSeconds: 60, CurrentSessionStartedDate: nil, AssigneeEmail: "b@esb.co.id", IssueSummary: "Plenty of budget"},
		},
	}
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-overdue")

	if !strings.Contains(got, "OVER-1") {
		t.Errorf("RouteCommand() = %q, want it to list OVER-1", got)
	}
	if strings.Contains(got, "UNDER-1") {
		t.Errorf("RouteCommand() = %q, must not list UNDER-1 which is within budget", got)
	}
}

func TestRouteCommand_OverdueNoneIsPositiveMessage(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey:      map[string]issue_session_dto.IssueSession{},
		all: []issue_session_dto.IssueSession{
			{IssueKey: "UNDER-1", StoryPoints: 100, AccumulatedSeconds: 60, AssigneeEmail: "b@esb.co.id", IssueSummary: "Plenty of budget"},
		},
	}
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-overdue")

	if strings.Contains(got, "UNDER-1") {
		t.Error("RouteCommand() should not list any tickets when none are overdue")
	}
	if got == "" {
		t.Error("RouteCommand() must return a friendly message, not empty")
	}
}

func TestRouteCommand_TeamGroupsTicketsByAssignee(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey:      map[string]issue_session_dto.IssueSession{},
		all: []issue_session_dto.IssueSession{
			{IssueKey: "A-1", StoryPoints: 1, AccumulatedSeconds: 10, AssigneeEmail: "alice@esb.co.id", IssueSummary: "Alice ticket 1"},
			{IssueKey: "B-1", StoryPoints: 1, AccumulatedSeconds: 10, AssigneeEmail: "bob@esb.co.id", IssueSummary: "Bob ticket"},
		},
	}
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-team")

	if !strings.Contains(got, "alice@esb.co.id") || !strings.Contains(got, "bob@esb.co.id") {
		t.Errorf("RouteCommand() = %q, want it to mention both assignees", got)
	}
	if !strings.Contains(got, "A-1") || !strings.Contains(got, "B-1") {
		t.Errorf("RouteCommand() = %q, want it to mention both ticket keys", got)
	}
}

func TestRouteCommand_TeamNoActivityMessage(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-team")

	if got == "" {
		t.Error("RouteCommand() must return a friendly no-activity message, not empty")
	}
}

func TestRouteCommand_GeserTransitionsToMatchingStatus(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12561": {IssueKey: "OMS-12561", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{
		transitions: []jira_client_dto.Transition{
			{ID: "5", To: jira_client_dto.Status{Name: "To Do"}},
			{ID: "9", To: jira_client_dto.Status{Name: "Code Review"}},
		},
	}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12561>:todo")

	if len(jiraClient.transitionCalls) != 1 || jiraClient.transitionCalls[0] != "OMS-12561|5" {
		t.Fatalf("TransitionIssue calls = %v, want [OMS-12561|5]", jiraClient.transitionCalls)
	}
	if !strings.Contains(got, "OMS-12561") || !strings.Contains(got, "To Do") {
		t.Errorf("RouteCommand() = %q, want confirmation mentioning OMS-12561 and To Do", got)
	}
}

func TestRouteCommand_GeserMatchesCaseInsensitiveTargetStatus(t *testing.T) {
	// CORE uses "CODE REVIEW" (all caps) while OMS uses "Code Review" — the
	// command must match regardless of casing.
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"CORE-10841": {IssueKey: "CORE-10841", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{
		transitions: []jira_client_dto.Transition{
			{ID: "22", To: jira_client_dto.Status{Name: "CODE REVIEW"}},
		},
	}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<CORE-10841>:code-review")

	if len(jiraClient.transitionCalls) != 1 || jiraClient.transitionCalls[0] != "CORE-10841|22" {
		t.Fatalf("TransitionIssue calls = %v, want [CORE-10841|22]", jiraClient.transitionCalls)
	}
	if strings.Contains(got, "tidak dikenali") {
		t.Errorf("RouteCommand() = %q, want it to recognize CODE REVIEW as a case-insensitive match for code-review", got)
	}
}

func TestRouteCommand_GeserRejectsTicketNotOwnedBySender(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12561": {IssueKey: "OMS-12561", AssigneeEmail: "someoneelse@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12561>:todo")

	if len(jiraClient.transitionCalls) != 0 {
		t.Errorf("TransitionIssue calls = %v, want none — ticket is not the sender's own", jiraClient.transitionCalls)
	}
	if got == "" {
		t.Error("RouteCommand() must return a rejection message, not empty")
	}
}

func TestRouteCommand_GeserRejectsUnknownOrUntrackedTicket(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForChat{}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, nil, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-99999>:todo")

	if len(jiraClient.transitionCalls) != 0 {
		t.Errorf("TransitionIssue calls = %v, want none", jiraClient.transitionCalls)
	}
	if !strings.Contains(got, "OMS-99999") {
		t.Errorf("RouteCommand() = %q, want it to mention the issue key in the not-found message", got)
	}
}

func TestRouteCommand_GeserRejectsUnrecognizedTarget(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12561": {IssueKey: "OMS-12561", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12561>:bukan-target-valid")

	if len(jiraClient.transitionCalls) != 0 {
		t.Errorf("TransitionIssue calls = %v, want none", jiraClient.transitionCalls)
	}
	if got == "" {
		t.Error("RouteCommand() must return a message explaining the valid targets, not empty")
	}
}

func TestRouteCommand_GeserReturnsFriendlyMessageWhenWorkflowHasNoSuchTransition(t *testing.T) {
	// Reproduces OMS-12546: from Code Review, only Done/Feedback transitions
	// exist — no path back to To Do.
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12546": {IssueKey: "OMS-12546", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{
		transitions: []jira_client_dto.Transition{
			{ID: "2", To: jira_client_dto.Status{Name: "Done"}},
			{ID: "13", To: jira_client_dto.Status{Name: "Feedback"}},
		},
	}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12546>:todo")

	if len(jiraClient.transitionCalls) != 0 {
		t.Errorf("TransitionIssue calls = %v, want none — no matching transition available", jiraClient.transitionCalls)
	}
	if !strings.Contains(got, "OMS-12546") {
		t.Errorf("RouteCommand() = %q, want it to mention the issue key", got)
	}
}

func TestRouteCommand_GeserReturnsFriendlyMessageWhenGetTransitionsFails(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12561": {IssueKey: "OMS-12561", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{transitionsErr: errGeserTest}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12561>:todo")

	if got == "" {
		t.Error("RouteCommand() must return a friendly error message, not empty")
	}
}

func TestRouteCommand_GeserReturnsFriendlyMessageWhenTransitionIssueFails(t *testing.T) {
	repo := &fakeIssueSessionRepoForChat{
		byAssignee: map[string][]issue_session_dto.IssueSession{},
		byKey: map[string]issue_session_dto.IssueSession{
			"OMS-12561": {IssueKey: "OMS-12561", AssigneeEmail: "dev@esb.co.id", FlagActive: true},
		},
	}
	jiraClient := &fakeJiraClientServiceForChat{
		transitions:   []jira_client_dto.Transition{{ID: "5", To: jira_client_dto.Status{Name: "To Do"}}},
		transitionErr: errGeserTest,
	}
	svc := newTestServiceWithJira(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}, repo, jiraClient)

	got := svc.RouteCommand(context.Background(), "dev@esb.co.id", "/u-geser<OMS-12561>:todo")

	if got == "" {
		t.Error("RouteCommand() must return a friendly error message, not empty")
	}
}
