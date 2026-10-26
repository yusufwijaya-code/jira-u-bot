package registration_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

type fakeJiraClientServiceForRegistration struct {
	found bool
	err   error
}

func (f *fakeJiraClientServiceForRegistration) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	return nil, nil
}

func (f *fakeJiraClientServiceForRegistration) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (f *fakeJiraClientServiceForRegistration) LookupUserByEmail(ctx context.Context, email string) (bool, error) {
	return f.found, f.err
}

type fakeRegistrationRepositoryForService struct {
	upsertCalled bool
	upsertErr    error
}

func (f *fakeRegistrationRepositoryForService) UpsertUser(ctx context.Context, name, email string) error {
	f.upsertCalled = true
	return f.upsertErr
}

func (f *fakeRegistrationRepositoryForService) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}

func (f *fakeRegistrationRepositoryForService) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func TestRegister_UpsertsWhenJiraAccountFound(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	svc := NewRegistrationService(jiraClient, repo, nil, "")

	if err := svc.Register(context.Background(), "Anwar", "anwar@esb.co.id"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if !repo.upsertCalled {
		t.Error("expected UpsertUser to be called")
	}
}

func TestRegister_RejectsWhenJiraAccountNotFound(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: false}
	repo := &fakeRegistrationRepositoryForService{}
	svc := NewRegistrationService(jiraClient, repo, nil, "")

	err := svc.Register(context.Background(), "Ghost", "ghost@esb.co.id")
	if !errors.Is(err, ErrJiraAccountNotFound) {
		t.Errorf("err = %v, want ErrJiraAccountNotFound", err)
	}
	if repo.upsertCalled {
		t.Error("UpsertUser must NOT be called when the Jira account is not found (spec Open Question #2 — hard block)")
	}
}

func TestRegister_RejectsWhenJiraLookupFails(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{err: errors.New("jira api timeout")}
	repo := &fakeRegistrationRepositoryForService{}
	svc := NewRegistrationService(jiraClient, repo, nil, "")

	err := svc.Register(context.Background(), "Anwar", "anwar@esb.co.id")
	if !errors.Is(err, ErrJiraLookupUnavailable) {
		t.Errorf("err = %v, want ErrJiraLookupUnavailable", err)
	}
	if repo.upsertCalled {
		t.Error("UpsertUser must NOT be called when the Jira lookup itself fails (spec Note C5 — availability trade-off)")
	}
}

type fakeChatDMNotifierServiceForRegistration struct {
	sentPlainTextCalls []string
	sentToEmails       []string
	err                error
}

func (f *fakeChatDMNotifierServiceForRegistration) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	return nil
}

func (f *fakeChatDMNotifierServiceForRegistration) SendPlainTextDM(ctx context.Context, email, text string) error {
	f.sentPlainTextCalls = append(f.sentPlainTextCalls, text)
	f.sentToEmails = append(f.sentToEmails, email)
	return f.err
}

func TestRegister_SendsThankYouDMForNonSelfRegistration(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	dmNotifier := &fakeChatDMNotifierServiceForRegistration{}
	svc := NewRegistrationService(jiraClient, repo, dmNotifier, "yusuf.wijaya@esb.co.id")

	if err := svc.Register(context.Background(), "Teammate", "teammate@esb.co.id"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if len(dmNotifier.sentToEmails) != 1 || dmNotifier.sentToEmails[0] != "teammate@esb.co.id" {
		t.Errorf("sentToEmails = %v, want [teammate@esb.co.id] (spec AC3.1)", dmNotifier.sentToEmails)
	}
}

func TestRegister_SkipsThankYouDMForSelfRegistration(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	dmNotifier := &fakeChatDMNotifierServiceForRegistration{}
	svc := NewRegistrationService(jiraClient, repo, dmNotifier, "yusuf.wijaya@esb.co.id")

	if err := svc.Register(context.Background(), "Yusuf Wijaya", "yusuf.wijaya@esb.co.id"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if len(dmNotifier.sentToEmails) != 0 {
		t.Errorf("sentToEmails = %v, want none (spec AC3.2 — self-registration never gets a DM)", dmNotifier.sentToEmails)
	}
}

func TestRegister_SkipsThankYouDMWhenDMNotifierNil(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	svc := NewRegistrationService(jiraClient, repo, nil, "yusuf.wijaya@esb.co.id")

	if err := svc.Register(context.Background(), "Teammate", "teammate@esb.co.id"); err != nil {
		t.Fatalf("Register returned error: %v, want nil — OAuth not configured must not break registration (spec AC3.2)", err)
	}
}

func TestRegister_SucceedsEvenWhenThankYouDMFails(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	dmNotifier := &fakeChatDMNotifierServiceForRegistration{err: errors.New("network error")}
	svc := NewRegistrationService(jiraClient, repo, dmNotifier, "yusuf.wijaya@esb.co.id")

	if err := svc.Register(context.Background(), "Teammate", "teammate@esb.co.id"); err != nil {
		t.Fatalf("Register returned error: %v, want nil — thank-you DM is best-effort, its failure must not fail registration", err)
	}
	if !repo.upsertCalled {
		t.Error("expected UpsertUser to have been called — registration itself must still succeed")
	}
}
