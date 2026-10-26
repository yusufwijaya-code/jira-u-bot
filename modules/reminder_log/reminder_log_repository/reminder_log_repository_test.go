package reminder_log_repository

import (
	"context"
	"testing"

	"jira-u-bot/base/testutil"
)

func TestHasTierBeenSent_FalseWhenNoRowExists(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewReminderLogRepository(db)

	got, err := repo.HasTierBeenSent(context.Background(), "OMS-1", "1h")
	if err != nil {
		t.Fatalf("HasTierBeenSent returned error: %v", err)
	}
	if got {
		t.Error("HasTierBeenSent = true, want false for a tier never logged")
	}
}

func TestLogReminderSent_ThenHasTierBeenSentIsTrue(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewReminderLogRepository(db)
	ctx := context.Background()

	if err := repo.LogReminderSent(ctx, "OMS-1", "1h"); err != nil {
		t.Fatalf("LogReminderSent returned error: %v", err)
	}

	got, err := repo.HasTierBeenSent(ctx, "OMS-1", "1h")
	if err != nil {
		t.Fatalf("HasTierBeenSent returned error: %v", err)
	}
	if !got {
		t.Error("HasTierBeenSent = false, want true after LogReminderSent")
	}
}

func TestLogReminderSent_AllowsMultipleOverageEntriesForSameIssue(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewReminderLogRepository(db)
	ctx := context.Background()

	if err := repo.LogReminderSent(ctx, "OMS-1", "overage"); err != nil {
		t.Fatalf("first LogReminderSent returned error: %v", err)
	}
	if err := repo.LogReminderSent(ctx, "OMS-1", "overage"); err != nil {
		t.Fatalf("second LogReminderSent returned error: %v", err)
	}

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM tr_reminderlog WHERE issueKey = ? AND tier = ?`, "OMS-1", "overage").Scan(&count)
	if err != nil {
		t.Fatalf("count query returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (overage tier must allow repeated inserts for the 5-minute repeat, spec AC2.4)", count)
	}
}

func TestHasTierBeenSent_DifferentTiersAreIndependent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewReminderLogRepository(db)
	ctx := context.Background()

	if err := repo.LogReminderSent(ctx, "OMS-1", "1h"); err != nil {
		t.Fatalf("LogReminderSent returned error: %v", err)
	}

	got, err := repo.HasTierBeenSent(ctx, "OMS-1", "30m")
	if err != nil {
		t.Fatalf("HasTierBeenSent returned error: %v", err)
	}
	if got {
		t.Error("HasTierBeenSent(30m) = true, want false — logging 1h must not affect 30m")
	}
}
