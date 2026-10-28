package registration_repository

import (
	"context"
	"testing"

	"jira-u-bot/base/testutil"
	"jira-u-bot/modules/registration/registration_dto"
)

func TestUpsertUser_CreatesNewUser(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, "testmysql-anwar@esb.co.id") })

	if err := repo.UpsertUser(ctx, "TestMySQL Anwar", "testmysql-anwar@esb.co.id", "acc-anwar-123"); err != nil {
		t.Fatalf("UpsertUser returned error: %v", err)
	}

	user, found, err := repo.FindUserByEmail(ctx, "testmysql-anwar@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found after UpsertUser")
	}
	if user.UserName != "TestMySQL Anwar" {
		t.Errorf("UserName = %q, want %q", user.UserName, "TestMySQL Anwar")
	}
	if !user.FlagActive {
		t.Error("FlagActive = false, want true for a new registration")
	}
	if user.GoogleChatDMSpaceID != "" {
		t.Errorf("GoogleChatDMSpaceID = %q, want empty for a brand-new registration", user.GoogleChatDMSpaceID)
	}
	if user.JiraAccountID != "acc-anwar-123" {
		t.Errorf("JiraAccountID = %q, want %q", user.JiraAccountID, "acc-anwar-123")
	}
}

func TestUpsertUser_SameEmailUpdatesInsteadOfDuplicating(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, "testmysql-anwar2@esb.co.id") })

	if err := repo.UpsertUser(ctx, "TestMySQL Anwar", "testmysql-anwar2@esb.co.id", "acc-anwar2-1"); err != nil {
		t.Fatalf("first UpsertUser returned error: %v", err)
	}
	if err := repo.UpsertUser(ctx, "TestMySQL Anwar Setiawan", "testmysql-anwar2@esb.co.id", "acc-anwar2-2"); err != nil {
		t.Fatalf("second UpsertUser returned error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ms_user WHERE email = ?`, "testmysql-anwar2@esb.co.id").Scan(&count); err != nil {
		t.Fatalf("count query returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (spec AC1.3 — resubmit updates, not duplicates)", count)
	}

	user, _, err := repo.FindUserByEmail(ctx, "testmysql-anwar2@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if user.UserName != "TestMySQL Anwar Setiawan" {
		t.Errorf("UserName = %q, want updated name %q", user.UserName, "TestMySQL Anwar Setiawan")
	}
	if user.JiraAccountID != "acc-anwar2-2" {
		t.Errorf("JiraAccountID = %q, want updated value %q", user.JiraAccountID, "acc-anwar2-2")
	}
}

func TestUpdateGoogleChatDMSpaceID_PersistsAndIsReadable(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, "testmysql-dmspace@esb.co.id") })

	if err := repo.UpsertUser(ctx, "TestMySQL DMSpace", "testmysql-dmspace@esb.co.id", "acc-dmspace"); err != nil {
		t.Fatalf("UpsertUser returned error: %v", err)
	}
	if err := repo.UpdateGoogleChatDMSpaceID(ctx, "testmysql-dmspace@esb.co.id", "spaces/ABC123"); err != nil {
		t.Fatalf("UpdateGoogleChatDMSpaceID returned error: %v", err)
	}

	user, found, err := repo.FindUserByEmail(ctx, "testmysql-dmspace@esb.co.id")
	if err != nil {
		t.Fatalf("FindUserByEmail returned error: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found")
	}
	if user.GoogleChatDMSpaceID != "spaces/ABC123" {
		t.Errorf("GoogleChatDMSpaceID = %q, want %q", user.GoogleChatDMSpaceID, "spaces/ABC123")
	}
}

func TestFindUserByEmail_NotFoundReturnsFalseNoError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)

	user, found, err := repo.FindUserByEmail(context.Background(), "testmysql-nobody@esb.co.id")
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

func TestFindUserByJiraAccountID_ReturnsMatchingUser(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, "testmysql-accid@esb.co.id") })

	if err := repo.UpsertUser(ctx, "TestMySQL AccID", "testmysql-accid@esb.co.id", "acc-id-xyz"); err != nil {
		t.Fatalf("UpsertUser returned error: %v", err)
	}

	user, found, err := repo.FindUserByJiraAccountID(ctx, "acc-id-xyz")
	if err != nil {
		t.Fatalf("FindUserByJiraAccountID returned error: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found by jiraAccountId")
	}
	if user.Email != "testmysql-accid@esb.co.id" {
		t.Errorf("Email = %q, want %q", user.Email, "testmysql-accid@esb.co.id")
	}
}

func TestFindAllWithGoogleChatDMSpaceID_ReturnsOnlyUsersWithCachedSpace(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)
	ctx := context.Background()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM ms_user WHERE email IN (?, ?)`, "testmysql-withspace@esb.co.id", "testmysql-nospace@esb.co.id")
	})

	if err := repo.UpsertUser(ctx, "TestMySQL WithSpace", "testmysql-withspace@esb.co.id", "acc-withspace"); err != nil {
		t.Fatalf("UpsertUser(withspace) returned error: %v", err)
	}
	if err := repo.UpdateGoogleChatDMSpaceID(ctx, "testmysql-withspace@esb.co.id", "spaces/HASSPACE"); err != nil {
		t.Fatalf("UpdateGoogleChatDMSpaceID returned error: %v", err)
	}
	if err := repo.UpsertUser(ctx, "TestMySQL NoSpace", "testmysql-nospace@esb.co.id", "acc-nospace"); err != nil {
		t.Fatalf("UpsertUser(nospace) returned error: %v", err)
	}

	users, err := repo.FindAllWithGoogleChatDMSpaceID(ctx)
	if err != nil {
		t.Fatalf("FindAllWithGoogleChatDMSpaceID returned error: %v", err)
	}

	var found *registration_dto.User
	for i := range users {
		if users[i].Email == "testmysql-withspace@esb.co.id" {
			found = &users[i]
		}
		if users[i].Email == "testmysql-nospace@esb.co.id" {
			t.Errorf("result includes %q which has no cached DM space ID", users[i].Email)
		}
	}
	if found == nil {
		t.Fatal("expected testmysql-withspace@esb.co.id in results")
	}
	if found.GoogleChatDMSpaceID != "spaces/HASSPACE" {
		t.Errorf("GoogleChatDMSpaceID = %q, want %q", found.GoogleChatDMSpaceID, "spaces/HASSPACE")
	}
}

func TestFindUserByJiraAccountID_NotFoundReturnsFalseNoError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRegistrationRepository(db)

	user, found, err := repo.FindUserByJiraAccountID(context.Background(), "acc-does-not-exist")
	if err != nil {
		t.Fatalf("FindUserByJiraAccountID returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false for an unregistered accountId")
	}
	if user != nil {
		t.Error("user should be nil when not found")
	}
}
