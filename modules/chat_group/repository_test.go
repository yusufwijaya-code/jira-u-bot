package chat_group

import (
	"context"
	"testing"

	"nudge/base/testutil"
	"nudge/modules/registration/registration_repository"
)

const repoTestEmail = "testmysql-chatgroup@esb.co.id"

func TestRepository_JoinLifecycle(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	if err := registration_repository.NewRegistrationRepository(db).UpsertUser(ctx, "TestMySQL ChatGroup", repoTestEmail, "acc-chatgroup-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, repoTestEmail) })
	repo := NewRepository(db)

	need, err := repo.NeedsJoin(ctx, repoTestEmail)
	if err != nil || !need {
		t.Fatalf("new user NeedsJoin = %v, %v; want true", need, err)
	}
	pending, err := repo.Pending(ctx, 1000)
	if err != nil || !contains(pending, repoTestEmail) {
		t.Fatalf("Pending = %v, %v; want it to list the new user", pending, err)
	}

	if err := repo.MarkFailed(ctx, repoTestEmail, "HTTP 403"); err != nil {
		t.Fatal(err)
	}
	if need, _ := repo.NeedsJoin(ctx, repoTestEmail); !need {
		t.Error("a failed user must stay pending so the sweep retries it")
	}

	if err := repo.MarkAdded(ctx, repoTestEmail); err != nil {
		t.Fatal(err)
	}
	// A late failure (login hook racing the sweep) must not undo the success.
	if err := repo.MarkFailed(ctx, repoTestEmail, "late timeout"); err != nil {
		t.Fatal(err)
	}
	var errText *string
	if err := db.QueryRow(`SELECT chatGroupError FROM ms_user WHERE email = ?`, repoTestEmail).Scan(&errText); err != nil || errText != nil {
		t.Errorf("chatGroupError = %v, %v; want cleared after success", errText, err)
	}
	if need, _ := repo.NeedsJoin(ctx, repoTestEmail); need {
		t.Error("an added user must not need joining again")
	}

	// Activated users and deactivated users are never pending.
	db.Exec(`UPDATE ms_user SET chatGroupAddedDate = NULL, chatAppSpaceID = 'spaces/x' WHERE email = ?`, repoTestEmail)
	if need, _ := repo.NeedsJoin(ctx, repoTestEmail); need {
		t.Error("a user who activated the app must be skipped")
	}
	db.Exec(`UPDATE ms_user SET chatAppSpaceID = NULL, flagActive = 0 WHERE email = ?`, repoTestEmail)
	if need, _ := repo.NeedsJoin(ctx, repoTestEmail); need {
		t.Error("a deactivated user must be skipped")
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

