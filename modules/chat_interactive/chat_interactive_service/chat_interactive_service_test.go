package chat_interactive_service

import (
	"context"
	"strings"
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

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

func newTestService(registered map[string]registration_dto.User) ChatInteractiveService {
	return NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: registered},
		&fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)
}

func TestRouteCommand_UnregisteredUserGetsInviteToRegister(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{})

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "ghost@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "mytickets"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "labcup.tech/jira-notif/register") {
		t.Errorf("RouteCommand() = %q, want it to contain the registration link (AC2.1)", got)
	}
}

func TestRouteCommand_UnknownCommandFallsBackToHelp(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}})

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "not-a-real-command"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "mytickets") || !strings.Contains(got, "budget") || !strings.Contains(got, "overdue") || !strings.Contains(got, "team") {
		t.Errorf("RouteCommand() = %q, want help text listing all 4 commands (AC7.2)", got)
	}
}

func TestRouteCommand_BareMentionFallsBackToHelp(t *testing.T) {
	svc := newTestService(map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}})

	event := chat_interactive_dto.ChatEvent{
		User:              chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: nil,
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "mytickets") {
		t.Errorf("RouteCommand() = %q, want help text for a bare mention (AC7.1)", got)
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
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		repo,
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "mytickets"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "OMS-1") {
		t.Errorf("RouteCommand() = %q, want it to mention OMS-1 (AC3.1)", got)
	}
	if !strings.Contains(got, "Fix bug") {
		t.Errorf("RouteCommand() = %q, want it to mention the issue summary (AC3.1)", got)
	}
}

func TestRouteCommand_MyTicketsNoActiveTicketsIsFriendly(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "mytickets"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if strings.Contains(got, "error") || got == "" {
		t.Errorf("RouteCommand() = %q, want a friendly no-tickets message, not empty/error (AC3.2)", got)
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
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		repo,
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "budget"},
			ArgumentText:       "CORE-99",
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "CORE-99") || !strings.Contains(got, "Audit screen") {
		t.Errorf("RouteCommand() = %q, want ticket detail for CORE-99 (AC4.1)", got)
	}
}

func TestRouteCommand_BudgetTicketNotFound(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "budget"},
			ArgumentText:       "CORE-404",
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "tidak ditemukan") {
		t.Errorf("RouteCommand() = %q, want a not-found message (AC4.2)", got)
	}
}

func TestRouteCommand_BudgetWithoutArgumentShowsUsage(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "budget"},
			ArgumentText:       "",
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "/budget") {
		t.Errorf("RouteCommand() = %q, want usage instructions mentioning /budget (AC4.3)", got)
	}
}

func TestRouteCommand_OverdueListsOnlyOverBudgetTickets(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{
			byAssignee: map[string][]issue_session_dto.IssueSession{},
			byKey:      map[string]issue_session_dto.IssueSession{},
			all: []issue_session_dto.IssueSession{
				// AccumulatedSeconds alone (no CurrentSessionStartedDate) keeps
				// this deterministic regardless of when the test runs — a live
				// session's elapsed time depends on business hours, which this
				// assertion must not be sensitive to.
				{IssueKey: "OVER-1", StoryPoints: 1, AccumulatedSeconds: 7200, CurrentSessionStartedDate: nil, AssigneeEmail: "a@esb.co.id", IssueSummary: "Over budget"},
				{IssueKey: "UNDER-1", StoryPoints: 100, AccumulatedSeconds: 60, CurrentSessionStartedDate: nil, AssigneeEmail: "b@esb.co.id", IssueSummary: "Plenty of budget"},
			},
		},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "overdue"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "OVER-1") {
		t.Errorf("RouteCommand() = %q, want it to list OVER-1 (AC5.1)", got)
	}
	if strings.Contains(got, "UNDER-1") {
		t.Errorf("RouteCommand() = %q, must not list UNDER-1 which is within budget (AC5.1)", got)
	}
}

func TestRouteCommand_OverdueNoneIsPositiveMessage(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{
			byAssignee: map[string][]issue_session_dto.IssueSession{},
			byKey:      map[string]issue_session_dto.IssueSession{},
			all: []issue_session_dto.IssueSession{
				{IssueKey: "UNDER-1", StoryPoints: 100, AccumulatedSeconds: 60, AssigneeEmail: "b@esb.co.id", IssueSummary: "Plenty of budget"},
			},
		},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "overdue"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if strings.Contains(got, "UNDER-1") {
		t.Error("RouteCommand() should not list any tickets when none are overdue")
	}
	if got == "" {
		t.Error("RouteCommand() must return a friendly message, not empty (AC5.2)")
	}
}

func TestRouteCommand_TeamGroupsTicketsByAssignee(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{
			byAssignee: map[string][]issue_session_dto.IssueSession{},
			byKey:      map[string]issue_session_dto.IssueSession{},
			all: []issue_session_dto.IssueSession{
				{IssueKey: "A-1", StoryPoints: 1, AccumulatedSeconds: 10, AssigneeEmail: "alice@esb.co.id", IssueSummary: "Alice ticket 1"},
				{IssueKey: "B-1", StoryPoints: 1, AccumulatedSeconds: 10, AssigneeEmail: "bob@esb.co.id", IssueSummary: "Bob ticket"},
			},
		},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "team"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if !strings.Contains(got, "alice@esb.co.id") || !strings.Contains(got, "bob@esb.co.id") {
		t.Errorf("RouteCommand() = %q, want it to mention both assignees (AC6.1)", got)
	}
	if !strings.Contains(got, "A-1") || !strings.Contains(got, "B-1") {
		t.Errorf("RouteCommand() = %q, want it to mention both ticket keys (AC6.1)", got)
	}
}

func TestRouteCommand_TeamNoActivityMessage(t *testing.T) {
	svc := NewChatInteractiveService(
		&fakeRegistrationRepoForChat{users: map[string]registration_dto.User{"dev@esb.co.id": {Email: "dev@esb.co.id"}}},
		&fakeIssueSessionRepoForChat{byAssignee: map[string][]issue_session_dto.IssueSession{}, byKey: map[string]issue_session_dto.IssueSession{}, all: nil},
		business_hours_service.NewBusinessHoursService(),
		business_hours_dto.HolidayCalendar{},
	)

	event := chat_interactive_dto.ChatEvent{
		User: chat_interactive_dto.ChatEventUser{Email: "dev@esb.co.id"},
		AppCommandPayload: &chat_interactive_dto.AppCommandPayload{
			AppCommandMetadata: chat_interactive_dto.AppCommandMetadata{AppCommandID: "team"},
		},
	}

	got := svc.RouteCommand(context.Background(), event)

	if got == "" {
		t.Error("RouteCommand() must return a friendly no-activity message, not empty (AC6.2)")
	}
}
