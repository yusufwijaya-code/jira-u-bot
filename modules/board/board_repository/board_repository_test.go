package board_repository

import (
	"context"
	"testing"

	"nudge/base/testutil"
	"nudge/modules/board/board_dto"
)

// Far above any real Jira board id, so tests never touch real rows.
const testBoardID = 990000001

func TestUpsertAndFindByID(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewBoardRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_board WHERE boardID = ?`, testBoardID) })

	if _, found, err := repo.FindByID(ctx, testBoardID); err != nil || found {
		t.Fatalf("FindByID before insert = found %v, err %v", found, err)
	}

	in := board_dto.Board{BoardID: testBoardID, Name: "Test Board", ProjectKey: "OMS", AvatarURL: "https://x/a"}
	if err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	in.Name = "Renamed Board"
	if err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, found, err := repo.FindByID(ctx, testBoardID)
	if err != nil || !found || got != in {
		t.Errorf("FindByID = %+v, %v, %v; want %+v", got, found, err, in)
	}
}
