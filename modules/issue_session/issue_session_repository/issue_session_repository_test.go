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

	err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-1", "TESTMYSQL", 3, "dev@esb.co.id", "Test ticket summary", "")
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

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-2", "TESTMYSQL", 3, "dev@esb.co.id", "Original summary", ""); err != nil {
		t.Fatalf("first OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-2", "TESTMYSQL", 5, "dev2@esb.co.id", "Updated summary", ""); err != nil {
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
	err := repo.OpenOrResumeSessionAt(ctx, "TESTMYSQL-3", "TESTMYSQL", 11, historicalStart, "dev@esb.co.id", "Backfilled ticket", "")
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

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-4", "TESTMYSQL", 3, "dev@esb.co.id", "Closing test", ""); err != nil {
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

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-5", "TESTMYSQL", 3, "original@esb.co.id", "Original summary", ""); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.UpdateStoryPoints(ctx, "TESTMYSQL-5", 5, "reassigned@esb.co.id", "Updated summary", ""); err != nil {
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

func TestFindActiveByAssigneeEmail_ReturnsOnlyThatAssigneesActiveSessions(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-10")
	cleanupSession(t, repo, "TESTMYSQL-11")

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-10", "TESTMYSQL", 2, "dev@esb.co.id", "Ticket A", ""); err != nil {
		t.Fatalf("OpenOrResumeSession TESTMYSQL-10 returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-11", "TESTMYSQL", 3, "other@esb.co.id", "Ticket B", ""); err != nil {
		t.Fatalf("OpenOrResumeSession TESTMYSQL-11 returned error: %v", err)
	}

	got, err := repo.FindActiveByAssigneeEmail(ctx, "dev@esb.co.id")
	if err != nil {
		t.Fatalf("FindActiveByAssigneeEmail returned error: %v", err)
	}

	found := false
	for _, s := range got {
		if s.IssueKey == "TESTMYSQL-11" {
			t.Error("result includes a session belonging to a different assignee")
		}
		if s.IssueKey == "TESTMYSQL-10" {
			found = true
		}
	}
	if !found {
		t.Error("expected TESTMYSQL-10 in the result for dev@esb.co.id")
	}
}

func TestFindActiveByAssigneeEmail_ExcludesClosedSessions(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	cleanupSession(t, repo, "TESTMYSQL-12")

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-12", "TESTMYSQL", 1, "dev@esb.co.id", "Closed ticket", ""); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "TESTMYSQL-12", 100); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	got, err := repo.FindActiveByAssigneeEmail(ctx, "dev@esb.co.id")
	if err != nil {
		t.Fatalf("FindActiveByAssigneeEmail returned error: %v", err)
	}
	for _, s := range got {
		if s.IssueKey == "TESTMYSQL-12" {
			t.Error("result includes a closed (flagActive=0) session")
		}
	}
}

// --- Sprint Report (Tech Spec "Dashboard Jira Manage", Chapter 11a/11b) ---

func TestOpenOrResumeSession_PersistsSprintName(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, "TESTMYSQL-SPRINT1") })

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-SPRINT1", "TESTMYSQL", 3, "dev@esb.co.id", "Sprint test", "Sprint 22"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}

	session, found, err := repo.GetSessionByIssueKey(ctx, "TESTMYSQL-SPRINT1")
	if err != nil || !found {
		t.Fatalf("GetSessionByIssueKey: found=%v err=%v", found, err)
	}
	if session.SprintName != "Sprint 22" {
		t.Errorf("SprintName = %q, want %q", session.SprintName, "Sprint 22")
	}
}

func TestFindBySprintNames_ExcludesSessionsWithNullSprintName(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM tr_issuesession WHERE issueKey IN (?, ?)`, "TESTMYSQL-SPRINT2", "TESTMYSQL-NOSPRINT")
	})

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-SPRINT2", "TESTMYSQL", 3, "dev@esb.co.id", "Has sprint", "Sprint 22"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-NOSPRINT", "TESTMYSQL", 3, "dev@esb.co.id", "No sprint", ""); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}

	sessions, err := repo.FindBySprintNames(ctx, []string{"Sprint 22"})
	if err != nil {
		t.Fatalf("FindBySprintNames returned error: %v", err)
	}
	var foundTarget, foundOther bool
	for _, s := range sessions {
		if s.IssueKey == "TESTMYSQL-SPRINT2" {
			foundTarget = true
		}
		if s.IssueKey == "TESTMYSQL-NOSPRINT" {
			foundOther = true
		}
	}
	if !foundTarget {
		t.Error("expected TESTMYSQL-SPRINT2 (sprintName=Sprint 22) in results")
	}
	if foundOther {
		t.Error("TESTMYSQL-NOSPRINT has no sprintName and must never match any sprint filter (spec AC7.3)")
	}
}

func TestFindBySprintNames_IncludesClosedSessions(t *testing.T) {
	// Unlike GetActiveSessions/FindActiveByAssigneeEmail, the Sprint Report
	// needs completed work too — flagActive=0 sessions must still appear.
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, "TESTMYSQL-SPRINT3") })

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-SPRINT3", "TESTMYSQL", 3, "dev@esb.co.id", "Closed ticket", "Sprint 23"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.CloseSession(ctx, "TESTMYSQL-SPRINT3", 5000); err != nil {
		t.Fatalf("CloseSession returned error: %v", err)
	}

	sessions, err := repo.FindBySprintNames(ctx, []string{"Sprint 23"})
	if err != nil {
		t.Fatalf("FindBySprintNames returned error: %v", err)
	}
	var found bool
	for _, s := range sessions {
		if s.IssueKey == "TESTMYSQL-SPRINT3" {
			found = true
		}
	}
	if !found {
		t.Error("expected a closed session to still appear in Sprint Report results (spec AC7.1)")
	}
}

func TestListDistinctSprintNames_ExcludesNullAndEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewIssueSessionRepository(db)
	ctx := context.Background()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM tr_issuesession WHERE issueKey IN (?, ?)`, "TESTMYSQL-SPRINT4", "TESTMYSQL-NOSPRINT2")
	})

	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-SPRINT4", "TESTMYSQL", 3, "dev@esb.co.id", "Has sprint", "Sprint 24"); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}
	if err := repo.OpenOrResumeSession(ctx, "TESTMYSQL-NOSPRINT2", "TESTMYSQL", 3, "dev@esb.co.id", "No sprint", ""); err != nil {
		t.Fatalf("OpenOrResumeSession returned error: %v", err)
	}

	names, err := repo.ListDistinctSprintNames(ctx)
	if err != nil {
		t.Fatalf("ListDistinctSprintNames returned error: %v", err)
	}
	var foundTarget, foundEmpty bool
	for _, n := range names {
		if n == "Sprint 24" {
			foundTarget = true
		}
		if n == "" {
			foundEmpty = true
		}
	}
	if !foundTarget {
		t.Error("expected Sprint 24 in distinct sprint names")
	}
	if foundEmpty {
		t.Error("empty string must never appear in distinct sprint names")
	}
}
