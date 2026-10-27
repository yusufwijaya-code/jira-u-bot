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
