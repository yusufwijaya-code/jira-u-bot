package issue_session_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
)

type fakeIssueSessionRepository struct {
	openOrResumeCalled  bool
	updateOverageCalled bool
	err                 error
}

func (f *fakeIssueSessionRepository) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64, assigneeEmail, issueSummary string) error {
	f.openOrResumeCalled = true
	return f.err
}

func (f *fakeIssueSessionRepository) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time, assigneeEmail, issueSummary string) error {
	return f.err
}

func (f *fakeIssueSessionRepository) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64, assigneeEmail, issueSummary string) error {
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

	if err := svc.OpenOrResumeSession(context.Background(), "OMS-1", "OMS", 3, "dev@esb.co.id", "Test ticket"); err != nil {
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

func TestCalculateElapsedSeconds_NoActiveSession(t *testing.T) {
	session := issue_session_dto.IssueSession{
		AccumulatedSeconds:        1800,
		CurrentSessionStartedDate: nil,
	}
	bh := business_hours_service.NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	got := CalculateElapsedSeconds(session, bh, cal)

	if got != 1800 {
		t.Errorf("CalculateElapsedSeconds() = %d, want 1800 (AccumulatedSeconds only, no active session)", got)
	}
}

func TestCalculateElapsedSeconds_WithActiveSession(t *testing.T) {
	started := time.Now().Add(-10 * time.Minute)
	bh := business_hours_service.NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	session := issue_session_dto.IssueSession{
		AccumulatedSeconds:        600,
		CurrentSessionStartedDate: &started,
	}

	got := CalculateElapsedSeconds(session, bh, cal)

	// Computed independently here (not hardcoded) because this call and the
	// one inside CalculateElapsedSeconds resolve time.Now() a few
	// milliseconds apart — small tolerance absorbs that drift. If this test
	// runs outside business hours (nights/weekends), ElapsedBusinessSeconds
	// legitimately returns 0 for both calls and want == session.AccumulatedSeconds,
	// which still passes.
	wantLive := bh.ElapsedBusinessSeconds(started, time.Now(), cal)
	want := session.AccumulatedSeconds + wantLive
	if got < want-2 || got > want+2 {
		t.Errorf("CalculateElapsedSeconds() = %d, want approximately %d (AccumulatedSeconds + live business seconds since CurrentSessionStartedDate)", got, want)
	}
}
