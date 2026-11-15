package status_watch_repository

import (
	"context"
	"testing"
	"time"

	"nudge/base/testutil"
	"nudge/modules/status_watch/status_watch_dto"
)

func TestStatusWatchRepository_UpsertFindAndCleanup(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewStatusWatchRepository(db)
	ctx := context.Background()
	keys := []string{"OMS-990001", "OMS-990002"}
	cleanup := func() { db.Exec(`DELETE FROM tr_issuestatus WHERE issueKey IN (?, ?)`, keys[0], keys[1]) }
	cleanup()
	t.Cleanup(cleanup)

	old := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	if err := repo.Upsert(ctx, status_watch_dto.Snapshot{IssueKey: keys[0], Status: "To Do", AssigneeEmail: "a@esb.co.id", LastSeen: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, status_watch_dto.Snapshot{IssueKey: keys[1], Status: "Done", AssigneeEmail: "a@esb.co.id", LastSeen: old}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, status_watch_dto.Snapshot{IssueKey: keys[0], Status: "Code Process", AssigneeEmail: "b@esb.co.id", LastSeen: now}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Find(ctx, append(keys, "OMS-990099"))
	if err != nil || len(got) != 2 {
		t.Fatalf("Find = %v, %v", got, err)
	}
	if s := got[keys[0]]; s.Status != "Code Process" || s.AssigneeEmail != "b@esb.co.id" {
		t.Errorf("snapshot = %+v, want the update", s)
	}

	n, err := repo.DeleteNotSeenSince(ctx, now.AddDate(0, 0, -30))
	if err != nil || n < 1 {
		t.Fatalf("DeleteNotSeenSince = %d, %v", n, err)
	}
	if got, _ := repo.Find(ctx, keys); len(got) != 1 {
		t.Errorf("after cleanup = %v, want only the recent one", got)
	}
	if got, err := repo.Find(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("Find(nil) = %v, %v", got, err)
	}
}
