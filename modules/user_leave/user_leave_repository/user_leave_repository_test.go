package user_leave_repository

import (
	"context"
	"testing"

	"nudge/base/testutil"
)

func TestUserLeaveRepository_InsertListDelete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserLeaveRepository(db)
	ctx := context.Background()
	const email = "leave-test@esb.co.id"
	db.Exec(`DELETE FROM tr_user_leave WHERE email = ?`, email)
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_user_leave WHERE email = ?`, email) })

	added, err := repo.Insert(ctx, email, []string{"2026-11-18", "2026-11-19"}, "Cuti")
	if err != nil || len(added) != 2 {
		t.Fatalf("Insert = %v, %v", added, err)
	}
	again, err := repo.Insert(ctx, email, []string{"2026-11-19", "2026-11-20"}, "")
	if err != nil || len(again) != 1 || again[0] != "2026-11-20" {
		t.Fatalf("second Insert = %v, %v; want only 2026-11-20", again, err)
	}

	list, err := repo.ListByEmail(ctx, email)
	if err != nil || len(list) != 3 || list[0].Date != "2026-11-20" || list[2].Note != "Cuti" {
		t.Fatalf("ListByEmail = %+v, %v", list, err)
	}
	dates, err := repo.DatesByEmail(ctx, email)
	if err != nil || len(dates) != 3 {
		t.Fatalf("DatesByEmail = %v, %v", dates, err)
	}

	if ok, err := repo.Delete(ctx, email, "2026-11-18"); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if ok, _ := repo.Delete(ctx, email, "2026-11-18"); ok {
		t.Fatal("deleting twice must report false")
	}
	if ok, _ := repo.Delete(ctx, "other@esb.co.id", "2026-11-19"); ok {
		t.Fatal("must not delete another user's leave")
	}
}
