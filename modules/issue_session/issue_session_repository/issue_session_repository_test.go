package issue_session_repository

import (
	"context"
	"testing"
	"time"

	"jira-u-bot/base/testutil"
)

func cleanupSession(t *testing.T, repo IssueSessionRepository, issueKey string) {
	t.Helper()
	t.Cleanup(func() {
		impl := repo.(*issueSessionRepositoryImpl)
		impl.db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, issueKey)
	})
}

func TestOpenOrResumeSession_CreatesNewSessionWithAssigneeAndSummary(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-1")

	err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-1", "TESTMYSQL", 3, "dev@esb.co.id", "Test ticket summary")
	if err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-1")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found after OpenOrResumeSession")
	}
	if session.AssigneeEmail != "dev@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want %q", session.AssigneeEmail, "dev@esb.co.id")
	}
	if session.IssueSummary != "Test ticket summary" {
		t.Errorf("IssueSummary = %q, want %q", session.IssueSummary, "Test ticket summary")
	}
	if !session.FlagActive {
		t.Error("FlagActive = false, want true for a new session")
	}
}

func TestOpenOrResumeSession_SameIssueKeyUpdatesInsteadOfDuplicating(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-2")

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-2", "TESTMYSQL", 3, "dev@esb.co.id", "Original summary"); err != nil {
		t.Fatalf("first OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-2", "TESTMYSQL", 5, "dev2@esb.co.id", "Updated summary"); err != nil {
		t.Fatalf("second OpenOrResumeSession returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-2")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if session.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5 (updated value)", session.StoryPoints)
	}
	if session.AssigneeEmail != "dev2@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want updated value %q", session.AssigneeEmail, "dev2@esb.co.id")
	}
	if session.IssueSummary != "Updated summary" {
		t.Errorf("IssueSummary = %q, want updated value %q", session.IssueSummary, "Updated summary")
	}
}

func TestOpenOrResumeSessionAt_SetsHistoricalStartTimeAndAssignee(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-3")

	historicalStart := time.Date(2026, 10, 12, 10, 58, 19, 0, time.UTC)
	err := repo.OpenOrResumeSessionAt(ctx, "TESTMYSQL-3", "TESTMYSQL", 11, historicalStart, "dev@esb.co.id", "Backfilled ticket")
	if err != nil {
		t.Fatalf("OpenOrResumeSessionAt returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-3")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if session.AssigneeEmail != "dev@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want %q", session.AssigneeEmail, "dev@esb.co.id")
	}
	if !session.CurrentSessionStartedDate.Equal(historicalStart) {
		t.Errorf("CurrentSessionStartedDate = %v, want %v", session.CurrentSessionStartedDate, historicalStart)
	}
}

func TestCloseSession_PreservesAssigneeEmailAndIssueSummary(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-4")

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-4", "TESTMYSQL", 3, "dev@esb.co.id", "Closing test"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "TESTMYSQL-4", 1800); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-4")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to still exist (closed, not deleted)")
	}
	if session.FlagActive {
		t.Error("FlagActive = true, want false after CloseSession")
	}
	if session.AssigneeEmail != "dev@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want %q to still be readable after close (needed by Chapter 6's early-completion notice)", session.AssigneeEmail, "dev@esb.co.id")
	}
	if session.IssueSummary != "Closing test" {
		t.Errorf("IssueSummary = %q, want %q to still be readable after close", session.IssueSummary, "Closing test")
	}
}

func TestUpdateStoryPoints_AlsoRefreshesAssigneeEmailAndIssueSummary(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-5")

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-5", "TESTMYSQL", 3, "original@esb.co.id", "Original summary"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.UpdateStoryPoints(ctx, "TESTMYSQL-5", 5, "reassigned@esb.co.id", "Updated summary"); err != nil {
		t.Fatalf("UpdateStoryPoints returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-5")
	if err != nil {
		t.Fatalf("GetSessionByIssueKey returned error: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if session.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5", session.StoryPoints)
	}
	if session.AssigneeEmail != "reassigned@esb.co.id" {
		t.Errorf("AssigneeEmail = %q, want updated value %q (steady-state must keep this fresh too)", session.AssigneeEmail, "reassigned@esb.co.id")
	}
	if session.IssueSummary != "Updated summary" {
		t.Errorf("IssueSummary = %q, want updated value %q", session.IssueSummary, "Updated summary")
	}
}
