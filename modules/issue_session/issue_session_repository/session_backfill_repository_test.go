package issue_session_repository

import (
	"context"
	"testing"

	"nudge/base/testutil"
	"nudge/modules/issue_session/issue_session_dto"
)

func TestSessionBackfill_InsertsClosedSessionOnceAndNeverOverwrites(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewSessionBackfillRepository(db)
	ctx := context.Background()
	const key = "CORE-990001"
	db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key)
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key) })

	if exists, err := repo.SessionExists(ctx, key); err != nil || exists {
		t.Fatalf("SessionExists before insert = %v, %v", exists, err)
	}
	board, sprint := int64(26), int64(7988)
	s := issue_session_dto.IssueSession{
		IssueKey: key, ProjectKey: "CORE", StoryPoints: 4, AccumulatedSeconds: 112572,
		AssigneeEmail: "dev@esb.co.id", IssueSummary: "Backfilled", SprintName: "Core Sprint 23 2026",
		BoardID: &board, SprintID: &sprint,
	}
	added, err := repo.InsertClosedSession(ctx, s)
	if err != nil || !added {
		t.Fatalf("InsertClosedSession = %v, %v; want added", added, err)
	}

	var active bool
	var acc int64
	var sprintName, createdBy string
	var startedNull bool
	if err := db.QueryRow(`SELECT flagActive, accumulatedSeconds, sprintName, createdBy, currentSessionStartedDate IS NULL FROM tr_issuesession WHERE issueKey = ?`, key).
		Scan(&active, &acc, &sprintName, &createdBy, &startedNull); err != nil {
		t.Fatal(err)
	}
	if active || acc != 112572 || sprintName != "Core Sprint 23 2026" || createdBy != BackfillCreatedBy || !startedNull {
		t.Errorf("row = active=%v acc=%d sprint=%q by=%q startedNull=%v", active, acc, sprintName, createdBy, startedNull)
	}

	s.AccumulatedSeconds = 1
	if added, err := repo.InsertClosedSession(ctx, s); err != nil || added {
		t.Errorf("second insert = %v, %v; want ignored", added, err)
	}
	db.QueryRow(`SELECT accumulatedSeconds FROM tr_issuesession WHERE issueKey = ?`, key).Scan(&acc)
	if acc != 112572 {
		t.Errorf("accumulatedSeconds = %d after a second insert, want the original kept", acc)
	}
}

func TestSessionBackfill_UpdateAccumulatedSecondsKeepsActiveState(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewSessionBackfillRepository(db)
	ctx := context.Background()
	const key = "CORE-990002"
	db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key)
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key) })
	if _, err := db.Exec(`INSERT INTO tr_issuesession (issueKey, projectKey, storyPoints, accumulatedSeconds, currentSessionStartedDate, flagActive, assigneeEmail, createdBy, createdDate)
		VALUES (?, 'CORE', 2, 50000, '2026-11-20 08:30:00', 1, 'dev@esb.co.id', 'poller', CURRENT_TIMESTAMP)`, key); err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateAccumulatedSeconds(ctx, key, 21600); err != nil {
		t.Fatal(err)
	}
	var acc int64
	var active, startedNull bool
	var editedBy string
	if err := db.QueryRow(`SELECT accumulatedSeconds, flagActive, currentSessionStartedDate IS NULL, editedBy FROM tr_issuesession WHERE issueKey = ?`, key).
		Scan(&acc, &active, &startedNull, &editedBy); err != nil {
		t.Fatal(err)
	}
	if acc != 21600 || !active || startedNull || editedBy != LeaveRecalcEditedBy {
		t.Fatalf("got acc=%d active=%v startedNull=%v editedBy=%q", acc, active, startedNull, editedBy)
	}
}
