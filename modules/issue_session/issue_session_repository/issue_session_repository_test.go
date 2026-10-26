package issue_session_repository

import (
	"context"
	"testing"
	"time"

	"jira-u-bot/base/testutil"
)

func TestOpenOrResumeSession_CreatesNewActiveSession(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found after OpenOrResumeSession")
	}
	if !session.FlagActive {
		t.Error("FlagActive = false, want true for a freshly opened session")
	}
	if session.CurrentSessionStartedDate == nil {
		t.Error("CurrentSessionStartedDate is nil, want it set")
	}
	if session.StoryPoints != 3 {
		t.Errorf("StoryPoints = %v, want 3", session.StoryPoints)
	}
	if session.AccumulatedSeconds != 0 {
		t.Errorf("AccumulatedSeconds = %d, want 0 for a new session", session.AccumulatedSeconds)
	}
}

func TestOpenOrResumeSession_IsIdempotentAndUpdatesStoryPoints(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("first OpenOrResumeSession returned error: %v", err)
	}
	// Simulate SP being edited mid-flight (spec Note C3 — OMS-12198 went 7 -> 11).
	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 5); err != nil {
		t.Fatalf("second OpenOrResumeSession returned error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tr_issuesession WHERE issueKey = ?`, "OMS-1").Scan(&count); err != nil {
		t.Fatalf("count query returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (ON CONFLICT must update, not duplicate)", count)
	}

	session, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if session.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5 after re-entry with updated SP", session.StoryPoints)
	}
}

func TestUpdateStoryPoints_UpdatesValueWithoutTouchingSessionStartDate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	before, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey (before) returned error: %v", err)
	}

	if err := repo.UpdateStoryPoints(ctx, "OMS-1", 5); err != nil {
		t.Fatalf("UpdateStoryPoints returned error: %v", err)
	}

	after, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey (after) returned error: %v", err)
	}
	if after.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5", after.StoryPoints)
	}
	if !after.CurrentSessionStartedDate.Equal(*before.CurrentSessionStartedDate) {
		t.Errorf("CurrentSessionStartedDate changed from %v to %v — UpdateStoryPoints must NOT reset it (this is exactly the bug that breaks elapsed-time tracking if OpenOrResumeSession is called instead on every poll cycle)", before.CurrentSessionStartedDate, after.CurrentSessionStartedDate)
	}
}

func TestCloseSession_AccumulatesSecondsAndDeactivates(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "OMS-1", 1800); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	session, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if session.FlagActive {
		t.Error("FlagActive = true, want false after CloseSession")
	}
	if session.AccumulatedSeconds != 1800 {
		t.Errorf("AccumulatedSeconds = %d, want 1800", session.AccumulatedSeconds)
	}
	if session.CurrentSessionStartedDate != nil {
		t.Error("CurrentSessionStartedDate should be nil after CloseSession")
	}
}

func TestCloseSession_ThenReopen_AccumulatesAcrossVisits(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("first OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "OMS-1", 1800); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("second OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "OMS-1", 900); err != nil {
		t.Fatalf("second CloseSession returned error: %v", err)
	}

	session, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if session.AccumulatedSeconds != 2700 {
		t.Errorf("AccumulatedSeconds = %d, want 2700 (1800 + 900 across two visits, spec AC1.3)", session.AccumulatedSeconds)
	}
}

func TestGetActiveSessions_OnlyReturnsFlagActiveRows(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession(OMS-1) returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "OMS-2", "OMS", 2); err != nil {
		t.Fatalf("OpenOrResumeSession(OMS-2) returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "OMS-2", 100); err != nil {
		t.Fatalf("CloseSession(OMS-2) returned error: %v", err)
	}

	active, err := repo.GetActiveSessions(ctx)
	if err != nil {
		t.Fatalf("GetActiveSessions returned error: %v", err)
	}
	if len(active) != 1 || active[0].IssueKey != "OMS-1" {
		t.Errorf("GetActiveSessions = %+v, want exactly [OMS-1]", active)
	}
}

func TestUpdateLastOverageReminder_SetsTimestamp(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	if err := repo.OpenOrResumeSession(ctx, "OMS-1", "OMS", 3); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.UpdateLastOverageReminder(ctx, "OMS-1"); err != nil {
		t.Fatalf("UpdateLastOverageReminder returned error: %v", err)
	}

	session, _, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if session.LastOverageReminderDate == nil {
		t.Error("LastOverageReminderDate is nil, want it set after UpdateLastOverageReminder")
	}
}

func TestOpenOrResumeSessionAt_SetsExplicitStartTime(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()

	historicalStart := time.Date(2026, 10, 12, 10, 58, 19, 0, time.UTC)

	if err := repo.OpenOrResumeSessionAt(ctx, "OMS-1", "OMS", 11, historicalStart); err != nil {
		t.Fatalf("OpenOrResumeSessionAt returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "OMS-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if session.CurrentSessionStartedDate == nil || !session.CurrentSessionStartedDate.Equal(historicalStart) {
		t.Errorf("CurrentSessionStartedDate = %v, want %v (spec AC1.4 — backfill must anchor to the real changelog timestamp, not time.Now())", session.CurrentSessionStartedDate, historicalStart)
	}
	if session.StoryPoints != 11 {
		t.Errorf("StoryPoints = %v, want 11", session.StoryPoints)
	}
}

func TestGetSessionByIssueKey_NotFoundReturnsFalseNoError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)

	session, found, err := repo.GetSessionByIssueKey(context.Background(), "OMS-999")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false for a nonexistent issueKey")
	}
	if session != nil {
		t.Error("session should be nil when not found")
	}
}
