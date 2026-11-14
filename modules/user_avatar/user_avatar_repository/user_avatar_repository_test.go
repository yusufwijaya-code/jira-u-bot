package user_avatar_repository

import (
	"context"
	"testing"

	"nudge/base/testutil"
)

const testEmail = "testmysql-picture@esb.co.id"

func TestSaveAndFind(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserAvatarRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, testEmail) })

	if _, err := db.Exec(`INSERT INTO ms_user (userName, email, createdBy) VALUES ('Picture Test', ?, 'test')`, testEmail); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var userID int64
	if err := db.QueryRow(`SELECT userID FROM ms_user WHERE email = ?`, testEmail).Scan(&userID); err != nil {
		t.Fatalf("select user: %v", err)
	}

	if got, err := repo.FindByUserID(ctx, userID); err != nil || got != "" {
		t.Fatalf("before save = %q, %v; want empty", got, err)
	}
	if err := repo.Save(ctx, userID, "https://lh3.googleusercontent.com/a/one=s96-c"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Save(ctx, userID, "https://lh3.googleusercontent.com/a/two=s96-c"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got, err := repo.FindByUserID(ctx, userID); err != nil || got != "https://lh3.googleusercontent.com/a/two=s96-c" {
		t.Errorf("FindByUserID = %q, %v", got, err)
	}
	if got, err := repo.FindByEmail(ctx, testEmail); err != nil || got != "https://lh3.googleusercontent.com/a/two=s96-c" {
		t.Errorf("FindByEmail = %q, %v", got, err)
	}
	if got, err := repo.FindByEmail(ctx, "nobody-picture@esb.co.id"); err != nil || got != "" {
		t.Errorf("unknown email = %q, %v; want empty", got, err)
	}
}
