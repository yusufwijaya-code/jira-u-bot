package registration_service

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestRegister_UpsertsWhenJiraAccountFound(t *testing.T) {
	jiraClient := &fakeJiraClientServiceForRegistration{found: true}
	repo := &fakeRegistrationRepositoryForService{}
	svc := NewRegistrationService(jiraClient, repo)

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
	svc := NewRegistrationService(jiraClient, repo)

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
	svc := NewRegistrationService(jiraClient, repo)

	err := svc.Register(context.Background(), "Anwar", "anwar@esb.co.id")
	if !errors.Is(err, ErrJiraLookupUnavailable) {
		t.Errorf("err = %v, want ErrJiraLookupUnavailable", err)
	}
	if repo.upsertCalled {
		t.Error("UpsertUser must NOT be called when the Jira lookup itself fails (spec Note C5 — availability trade-off)")
	}
}
