package user_setting_repository

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"nudge/base/testutil"
	"nudge/modules/user_setting/user_setting_dto"
)

// insertTestUser creates a throwaway ms_user row (ms_user_setting has a FK
// to it) and returns its userID; cleanup cascades to ms_user_setting.
func insertTestUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	db.Exec(`DELETE FROM ms_user WHERE email = ?`, email)
	res, err := db.Exec(`INSERT INTO ms_user (userName, email, createdBy) VALUES (?, ?, 'test')`, "Test User", email)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE userID = ?`, id) })
	return id
}

func TestFindRow_NotFoundForUserWithoutRow(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	userID := insertTestUser(t, db, "testmysql-us-1@esb.co.id")

	_, found, err := repo.FindRow(context.Background(), userID, 0)
	if err != nil || found {
		t.Errorf("FindRow = found %v, err %v; want not found", found, err)
	}
}

func TestUpsertNotification_RoundTripsEveryFieldPerBoard(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-2@esb.co.id")

	repeat := 30
	def := user_setting_dto.UserSetting{
		UserID:               userID,
		BoardID:              0,
		ReminderTiers:        []string{"2h", "45m"},
		OverageTiers:         []string{"1h", "15m"},
		OverageRepeatMinutes: &repeat,
		NotifyEarlyFinish:    false,
		NotifyReturnedToTodo: true,
		QuietHours:           &user_setting_dto.QuietHours{Start: "22:00", End: "06:00"},
	}
	if err := repo.UpsertNotification(ctx, def); err != nil {
		t.Fatalf("UpsertNotification(default): %v", err)
	}
	board := def
	board.BoardID = 28
	board.ReminderTiers = []string{"1h"}
	board.OverageRepeatMinutes = nil
	board.QuietHours = nil
	if err := repo.UpsertNotification(ctx, board); err != nil {
		t.Fatalf("UpsertNotification(board): %v", err)
	}

	got, found, err := repo.FindRow(ctx, userID, 0)
	if err != nil || !found {
		t.Fatalf("FindRow(default) = found %v, err %v", found, err)
	}
	if !reflect.DeepEqual(got.ReminderTiers, def.ReminderTiers) || !reflect.DeepEqual(got.OverageTiers, def.OverageTiers) ||
		got.OverageRepeatMinutes == nil || *got.OverageRepeatMinutes != 30 || got.NotifyEarlyFinish || !got.NotifyReturnedToTodo ||
		got.QuietHours == nil || got.QuietHours.Start != "22:00" || got.UseDefault {
		t.Errorf("default row = %+v", got)
	}

	gotBoard, found, err := repo.FindRow(ctx, userID, 28)
	if err != nil || !found {
		t.Fatalf("FindRow(board) = found %v, err %v", found, err)
	}
	if gotBoard.BoardID != 28 || gotBoard.UseDefault || !reflect.DeepEqual(gotBoard.ReminderTiers, []string{"1h"}) || gotBoard.OverageRepeatMinutes != nil || gotBoard.QuietHours != nil {
		t.Errorf("board row = %+v, want its own custom values", gotBoard)
	}

	rows, err := repo.ListRows(ctx, userID)
	if err != nil || len(rows) != 2 || rows[0].BoardID != 0 || rows[1].BoardID != 28 {
		t.Errorf("ListRows = %+v, %v; want default then board 28", rows, err)
	}
}

func TestSetUseDefault_KeepsTheBoardMute(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-5@esb.co.id")

	custom := user_setting_dto.Default()
	custom.UserID, custom.BoardID = userID, 28
	if err := repo.UpsertNotification(ctx, custom); err != nil {
		t.Fatalf("UpsertNotification: %v", err)
	}
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := repo.SetMutedUntil(ctx, userID, 28, &until); err != nil {
		t.Fatalf("SetMutedUntil: %v", err)
	}
	if err := repo.SetUseDefault(ctx, userID, 28); err != nil {
		t.Fatalf("SetUseDefault: %v", err)
	}

	got, _, _ := repo.FindRow(ctx, userID, 28)
	if !got.UseDefault || got.MutedUntil == nil || !got.MutedUntil.Equal(until) {
		t.Errorf("row = useDefault %v, muted %v; want following default with the mute kept", got.UseDefault, got.MutedUntil)
	}
}

func TestSetMutedUntil_CreatesRowsAndClears(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-3@esb.co.id")

	until := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := repo.SetMutedUntil(ctx, userID, 0, &until); err != nil {
		t.Fatalf("SetMutedUntil(global): %v", err)
	}
	if err := repo.SetMutedUntil(ctx, userID, 26, &until); err != nil {
		t.Fatalf("SetMutedUntil(board): %v", err)
	}

	def, found, err := repo.FindRow(ctx, userID, 0)
	if err != nil || !found || def.MutedUntil == nil || !def.MutedUntil.Equal(until) || def.UseDefault {
		t.Fatalf("default row = %+v, found %v, err %v", def, found, err)
	}
	if !reflect.DeepEqual(def.ReminderTiers, user_setting_dto.DefaultReminderTiers) || def.OverageRepeatMinutes == nil || *def.OverageRepeatMinutes != 5 {
		t.Errorf("a row created by a mute must hold the table defaults, got %+v", def)
	}
	board, _, _ := repo.FindRow(ctx, userID, 26)
	if !board.UseDefault || board.MutedUntil == nil {
		t.Errorf("a board row created by a mute must follow the default, got %+v", board)
	}

	if err := repo.SetMutedUntil(ctx, userID, 26, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	board, _, _ = repo.FindRow(ctx, userID, 26)
	if board.MutedUntil != nil {
		t.Errorf("board mute = %v after clearing, want nil", board.MutedUntil)
	}
	def, _, _ = repo.FindRow(ctx, userID, 0)
	if def.MutedUntil == nil {
		t.Error("clearing the board mute must not touch the global mute")
	}
}

func TestSetMutedUntil_ForeverSentinelRoundTrips(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-4@esb.co.id")

	forever := user_setting_dto.MuteForeverUntil
	if err := repo.SetMutedUntil(ctx, userID, 0, &forever); err != nil {
		t.Fatalf("SetMutedUntil(forever): %v", err)
	}
	got, _, err := repo.FindRow(ctx, userID, 0)
	if err != nil || !got.IsMutedForever() || !got.IsMuted(time.Now()) {
		t.Errorf("MutedUntil = %v, err %v; want the forever sentinel back", got.MutedUntil, err)
	}
}
