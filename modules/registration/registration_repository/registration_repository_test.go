package registration_repository

import (
	"context"
	"testing"

	"jira-u-bot/base/testutil"
)

func TestUpsertUser_CreatesNewUser(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()

	if err := repo.UpsertUser(ctx, "Anwar", "anwar@esb.co.id", ""); err != nil {
		t.Fatalf("UpsertUser returned error: %v", err)
	}

	user, found, err := repo.FindUserByEmail(ctx, "anwar@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found after UpsertUser")
	}
	if user.UserName != "Anwar" {
		t.Errorf("UserName = %q, want %q", user.UserName, "Anwar")
	}
	if !user.FlagActive {
		t.Error("FlagActive = false, want true for a new registration")
	}
	if user.GoogleChatUserID != "" {
		t.Errorf("GoogleChatUserID = %q, want empty when not supplied", user.GoogleChatUserID)
	}
}

func TestUpsertUser_PersistsGoogleChatUserIDWhenSupplied(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()

	if err := repo.UpsertUser(ctx, "Anwar", "anwar@esb.co.id", "123456789012345678901"); err != nil {
		t.Fatalf("UpsertUser returned error: %v", err)
	}

	user, found, err := repo.FindUserByEmail(ctx, "anwar@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found after UpsertUser")
	}
	if user.GoogleChatUserID != "123456789012345678901" {
		t.Errorf("GoogleChatUserID = %q, want %q", user.GoogleChatUserID, "123456789012345678901")
	}
}

func TestUpsertUser_SameEmailUpdatesInsteadOfDuplicating(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()

	if err := repo.UpsertUser(ctx, "Anwar", "anwar@esb.co.id", ""); err != nil {
		t.Fatalf("first UpsertUser returned error: %v", err)
	}
	if err := repo.UpsertUser(ctx, "Anwar Setiawan", "anwar@esb.co.id", "999"); err != nil {
		t.Fatalf("second UpsertUser returned error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ms_user WHERE email = ?`, "anwar@esb.co.id").Scan(&count); err != nil {
		t.Fatalf("count query returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (spec AC3.2 — resubmit updates, not duplicates)", count)
	}

	user, _, err := repo.FindUserByEmail(ctx, "anwar@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if user.UserName != "Anwar Setiawan" {
		t.Errorf("UserName = %q, want updated name %q", user.UserName, "Anwar Setiawan")
	}
	if user.GoogleChatUserID != "999" {
		t.Errorf("GoogleChatUserID = %q, want updated value %q", user.GoogleChatUserID, "999")
	}
}

func TestFindUserByEmail_NotFoundReturnsFalseNoError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)

	user, found, err := repo.FindUserByEmail(context.Background(), "nobody@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false for an unregistered email")
	}
	if user != nil {
		t.Error("user should be nil when not found")
	}
}
