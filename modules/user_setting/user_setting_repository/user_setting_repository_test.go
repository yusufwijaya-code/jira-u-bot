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

func TestFindByUserID_NotFoundForUserWithoutRow(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	userID := insertTestUser(t, db, "testmysql-us-1@esb.co.id")

	_, found, err := repo.FindByUserID(context.Background(), userID)
	if err != nil || found {
		t.Errorf("FindByUserID = found %v, err %v; want not found", found, err)
	}
}

func TestUpsertNotification_RoundTripsEveryField(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-2@esb.co.id")

	repeat := 30
	in := user_setting_dto.UserSetting{
		UserID:               userID,
		ReminderTiers:        []string{"2h", "45m"},
		OverageTiers:         []string{"1h", "15m"},
		OverageRepeatMinutes: &repeat,
		NotifyEarlyFinish:    false,
		NotifyReturnedToTodo: true,
		QuietHours:           &user_setting_dto.QuietHours{Start: "22:00", End: "06:00"},
	}
	if err := repo.UpsertNotification(ctx, in); err != nil {
		t.Fatalf("UpsertNotification: %v", err)
	}

	got, found, err := repo.FindByUserID(ctx, userID)
	if err != nil || !found {
		t.Fatalf("FindByUserID = found %v, err %v", found, err)
	}
	if !reflect.DeepEqual(got.ReminderTiers, in.ReminderTiers) || !reflect.DeepEqual(got.OverageTiers, in.OverageTiers) {
		t.Errorf("tiers = %v / %v", got.ReminderTiers, got.OverageTiers)
	}
	if got.OverageRepeatMinutes == nil || *got.OverageRepeatMinutes != 30 || got.NotifyEarlyFinish || !got.NotifyReturnedToTodo {
		t.Errorf("unexpected %+v", got)
	}
	if got.QuietHours == nil || got.QuietHours.Start != "22:00" || got.QuietHours.End != "06:00" {
		t.Errorf("QuietHours = %+v, want 22:00-06:00", got.QuietHours)
	}

	// Second upsert switches repeat off and disables quiet hours.
	in.OverageRepeatMinutes = nil
	in.QuietHours = nil
	if err := repo.UpsertNotification(ctx, in); err != nil {
		t.Fatalf("second UpsertNotification: %v", err)
	}
	got, _, _ = repo.FindByUserID(ctx, userID)
	if got.OverageRepeatMinutes != nil || got.QuietHours != nil {
		t.Errorf("after update got repeat %v quiet %+v, want both nil", got.OverageRepeatMinutes, got.QuietHours)
	}
}

func TestSetMutedUntil_CreatesRowWithDefaultsAndClears(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-3@esb.co.id")

	until := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := repo.SetMutedUntil(ctx, userID, &until); err != nil {
		t.Fatalf("SetMutedUntil: %v", err)
	}
	got, found, err := repo.FindByUserID(ctx, userID)
	if err != nil || !found {
		t.Fatalf("FindByUserID = found %v, err %v", found, err)
	}
	if got.MutedUntil == nil || !got.MutedUntil.Equal(until) {
		t.Errorf("MutedUntil = %v, want %v", got.MutedUntil, until)
	}
	// The row was created by the mute alone, so every other column holds
	// the table defaults (= legacy behavior).
	if !reflect.DeepEqual(got.ReminderTiers, user_setting_dto.DefaultReminderTiers) || got.OverageRepeatMinutes == nil || *got.OverageRepeatMinutes != 5 {
		t.Errorf("defaults not applied: %+v", got)
	}

	if err := repo.SetMutedUntil(ctx, userID, nil); err != nil {
		t.Fatalf("clear SetMutedUntil: %v", err)
	}
	got, _, _ = repo.FindByUserID(ctx, userID)
	if got.MutedUntil != nil {
		t.Errorf("MutedUntil = %v after clearing, want nil", got.MutedUntil)
	}
}

func TestSetMutedUntil_ForeverSentinelRoundTrips(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserSettingRepository(db)
	ctx := context.Background()
	userID := insertTestUser(t, db, "testmysql-us-4@esb.co.id")

	forever := user_setting_dto.MuteForeverUntil
	if err := repo.SetMutedUntil(ctx, userID, &forever); err != nil {
		t.Fatalf("SetMutedUntil(forever): %v", err)
	}
	got, _, err := repo.FindByUserID(ctx, userID)
	if err != nil || !got.IsMutedForever() || !got.IsMuted(time.Now()) {
		t.Errorf("MutedUntil = %v, err %v; want the forever sentinel back", got.MutedUntil, err)
	}
}
