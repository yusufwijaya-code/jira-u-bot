package issue_session_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
)

type fakeIssueSessionRepository struct {
	openOrResumeCalled  bool
	updateOverageCalled bool
	err                 error
}

func (f *fakeIssueSessionRepository) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64) error {
	f.openOrResumeCalled = true
	return f.err
}

func (f *fakeIssueSessionRepository) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time) error {
	return f.err
}

func (f *fakeIssueSessionRepository) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64) error {
	return f.err
}

func (f *fakeIssueSessionRepository) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return f.err
}

func (f *fakeIssueSessionRepository) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []issue_session_dto.IssueSession{{IssueKey: "OMS-1"}}, nil
}

func (f *fakeIssueSessionRepository) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	return nil, false, f.err
}

func (f *fakeIssueSessionRepository) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	f.updateOverageCalled = true
	return f.err
}

func TestOpenOrResumeSession_DelegatesToRepository(t *testing.T) {
	repo := &fakeIssueSessionRepository{}
	svc := NewIssueSessionService(repo)

	if err := svc.OpenOrResumeSession(context.Background(), "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if !repo.openOrResumeCalled {
		t.Error("expected repository.OpenOrResumeSession to be called")
	}
}

func TestGetActiveSessions_ReturnsRepositoryResult(t *testing.T) {
	repo := &fakeIssueSessionRepository{}
	svc := NewIssueSessionService(repo)

	got, err := svc.GetActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("GetActiveSessions returned error: %v", err)
	}
	if len(got) != 1 || got[0].IssueKey != "OMS-1" {
		t.Errorf("GetActiveSessions = %+v, want one session with IssueKey OMS-1", got)
	}
}

func TestUpdateLastOverageReminder_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeIssueSessionRepository{err: errors.New("db unavailable")}
	svc := NewIssueSessionService(repo)

	err := svc.UpdateLastOverageReminder(context.Background(), "OMS-1")
	if err == nil {
		t.Fatal("expected error to propagate from repository, got nil")
	}
	if !repo.updateOverageCalled {
		t.Error("expected repository.UpdateLastOverageReminder to be called")
	}
}
