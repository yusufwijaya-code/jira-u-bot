package chat_app_repository

import (
	"context"
	"testing"
	"time"

	"nudge/base/testutil"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/registration/registration_repository"
)

const testEmail = "testmysql-chatapp@esb.co.id"

func setup(t *testing.T) (Repository, func(q string, args ...any)) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	if err := registration_repository.NewRegistrationRepository(db).UpsertUser(ctx, "TestMySQL ChatApp", testEmail, "acc-chatapp-1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, testEmail) })
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	return NewRepository(db), exec
}

func TestFindByEmail_NewUserIsNotActivated(t *testing.T) {
	repo, _ := setup(t)
	s, found, err := repo.FindByEmail(context.Background(), testEmail)
	if err != nil || !found {
		t.Fatalf("FindByEmail = found %v, err %v", found, err)
	}
	if s.Activated() || s.Sender != nil || s.ActivatedDate != nil {
		t.Errorf("state = %+v, want not activated, automatic sender", s)
	}
}

func TestFindByEmail_ReadsGroupJoinState(t *testing.T) {
	repo, exec := setup(t)
	ctx := context.Background()
	s, _, err := repo.FindByEmail(ctx, testEmail)
	if err != nil || s.GroupAddedDate != nil || s.GroupError != "" {
		t.Fatalf("fresh user = %+v, %v; want no group state", s, err)
	}
	exec(`UPDATE ms_user SET chatGroupAddedDate = '2026-11-16 09:00:00', chatGroupError = 'boom' WHERE email = ?`, testEmail)
	s, _, err = repo.FindByEmail(ctx, testEmail)
	if err != nil || s.GroupAddedDate == nil || s.GroupError != "boom" {
		t.Fatalf("after update = %+v, %v; want added date and error", s, err)
	}
}

func TestFindByEmail_UnknownUser(t *testing.T) {
	repo, _ := setup(t)
	s, found, err := repo.FindByEmail(context.Background(), "testmysql-nobody@esb.co.id")
	if err != nil || found {
		t.Fatalf("found = %v, err = %v; want not found", found, err)
	}
	if s.Email != "testmysql-nobody@esb.co.id" {
		t.Errorf("Email = %q, want the queried email", s.Email)
	}
}

func TestSetSpace_StoresAndKeepsActivationDateForSameSpace(t *testing.T) {
	repo, exec := setup(t)
	ctx := context.Background()
	if err := repo.SetSpace(ctx, testEmail, "spaces/AAA", "users/111"); err != nil {
		t.Fatal(err)
	}
	s, _, _ := repo.FindByEmail(ctx, testEmail)
	if s.SpaceID != "spaces/AAA" || s.ChatUserID != "users/111" || s.ActivatedDate == nil {
		t.Fatalf("after SetSpace: %+v", s)
	}

	exec(`UPDATE ms_user SET chatAppActivatedDate = '2026-01-02 03:04:05' WHERE email = ?`, testEmail)
	if err := repo.SetSpace(ctx, testEmail, "spaces/AAA", ""); err != nil {
		t.Fatal(err)
	}
	s, _, _ = repo.FindByEmail(ctx, testEmail)
	if s.ActivatedDate.Year() != 2026 || s.ActivatedDate.Month() != time.January {
		t.Errorf("ActivatedDate = %v, want kept for the same space", s.ActivatedDate)
	}
	if s.ChatUserID != "users/111" {
		t.Errorf("ChatUserID = %q, want kept when the new one is empty", s.ChatUserID)
	}

	if err := repo.SetSpace(ctx, testEmail, "spaces/BBB", "users/111"); err != nil {
		t.Fatal(err)
	}
	s, _, _ = repo.FindByEmail(ctx, testEmail)
	if s.SpaceID != "spaces/BBB" || s.ActivatedDate.Year() == 2026 && s.ActivatedDate.Month() == time.January && s.ActivatedDate.Day() == 2 {
		t.Errorf("after new space: %+v, want new space and a fresh activation date", s)
	}
}

func TestSetSender_AndClearResetsEverything(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	nudge := chat_app_dto.SenderNudge
	if err := repo.SetSpace(ctx, testEmail, "spaces/CCC", "users/2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSender(ctx, testEmail, &nudge); err != nil {
		t.Fatal(err)
	}
	s, _, _ := repo.FindByEmail(ctx, testEmail)
	if s.Sender == nil || *s.Sender != "nudge" {
		t.Fatalf("Sender = %v, want nudge", s.Sender)
	}

	if err := repo.ClearBySpace(ctx, "spaces/CCC"); err != nil {
		t.Fatal(err)
	}
	s, _, _ = repo.FindByEmail(ctx, testEmail)
	if s.Activated() || s.Sender != nil || s.ActivatedDate != nil || s.ChatUserID != "" {
		t.Errorf("after ClearBySpace: %+v, want everything reset", s)
	}

	_ = repo.SetSpace(ctx, testEmail, "spaces/DDD", "users/2")
	_ = repo.SetSender(ctx, testEmail, &nudge)
	if err := repo.ClearByEmail(ctx, testEmail); err != nil {
		t.Fatal(err)
	}
	s, _, _ = repo.FindByEmail(ctx, testEmail)
	if s.Activated() || s.Sender != nil {
		t.Errorf("after ClearByEmail: %+v, want everything reset", s)
	}

	if err := repo.SetSender(ctx, testEmail, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListAll_KeysByLowerCaseEmail(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	_ = repo.SetSpace(ctx, testEmail, "spaces/EEE", "users/3")
	all, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all[testEmail].SpaceID != "spaces/EEE" {
		t.Errorf("ListAll[%s] = %+v", testEmail, all[testEmail])
	}
}
