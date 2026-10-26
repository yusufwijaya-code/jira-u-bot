package reminder_log_service

import (
	"context"
	"testing"
)

type fakeReminderLogRepository struct {
	hasTierBeenSentResult bool
	logReminderSentCalled bool
}

func (f *fakeReminderLogRepository) HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error) {
	return f.hasTierBeenSentResult, nil
}

func (f *fakeReminderLogRepository) LogReminderSent(ctx context.Context, issueKey, tier string) error {
	f.logReminderSentCalled = true
	return nil
}

func TestHasTierBeenSent_DelegatesToRepository(t *testing.T) {
	repo := &fakeReminderLogRepository{hasTierBeenSentResult: true}
	svc := NewReminderLogService(repo)

	got, err := svc.HasTierBeenSent(context.Background(), "OMS-1", "1h")
	if err != nil {
		t.Fatalf("HasTierBeenSent returned error: %v", err)
	}
	if !got {
		t.Error("HasTierBeenSent = false, want true (repository fake returns true)")
	}
}

func TestLogReminderSent_DelegatesToRepository(t *testing.T) {
	repo := &fakeReminderLogRepository{}
	svc := NewReminderLogService(repo)

	if err := svc.LogReminderSent(context.Background(), "OMS-1", "1h"); err != nil {
		t.Fatalf("LogReminderSent returned error: %v", err)
	}
	if !repo.logReminderSentCalled {
		t.Error("expected repository.LogReminderSent to be called")
	}
}
