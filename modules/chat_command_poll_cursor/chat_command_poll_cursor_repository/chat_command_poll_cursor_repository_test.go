package chat_command_poll_cursor_repository

import (
	"context"
	"testing"
	"time"

	"jira-u-bot/base/testutil"
)

func TestGetCursor_NotFoundReturnsFalseNoError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewChatCommandPollCursorRepository(db)

	_, found, err := repo.GetCursor(context.Background(), "spaces/TESTMYSQL-NOCURSOR")
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false for a space with no stored cursor")
	}
}

func TestUpdateCursor_ThenGetCursor_RoundTrips(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewChatCommandPollCursorRepository(db)
	ctx := context.Background()
	space := "spaces/TESTMYSQL-ROUNDTRIP"
	t.Cleanup(func() { db.Exec(`DELETE FROM chat_command_poll_cursor WHERE googleChatDMSpaceID = ?`, space) })

	want := time.Date(2026, 10, 27, 10, 10, 16, 0, time.UTC)
	if err := repo.UpdateCursor(ctx, space, want); err != nil {
		t.Fatalf("UpdateCursor returned error: %v", err)
	}

	got, found, err := repo.GetCursor(ctx, space)
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if !found {
		t.Fatal("expected cursor to be found after UpdateCursor")
	}
	if !got.Equal(want) {
		t.Errorf("GetCursor = %v, want %v", got, want)
	}
}

// TestUpdateCursor_PreservesSubSecondPrecision reproduces a real production
// bug: Google Chat message createTime carries microsecond precision (e.g.
// "2026-10-27T12:32:52.552687Z"). If the stored column truncates that away,
// the ORIGINAL message's timestamp looks newer than the truncated cursor on
// the next poll cycle, so chat_command_poller reprocesses it and sends a
// duplicate reply — confirmed live (Benni got /u-mytickets answered twice,
// exactly one poll interval apart).
func TestUpdateCursor_PreservesSubSecondPrecision(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewChatCommandPollCursorRepository(db)
	ctx := context.Background()
	space := "spaces/TESTMYSQL-SUBSECOND"
	t.Cleanup(func() { db.Exec(`DELETE FROM chat_command_poll_cursor WHERE googleChatDMSpaceID = ?`, space) })

	want, err := time.Parse(time.RFC3339Nano, "2026-10-27T12:32:52.552687Z")
	if err != nil {
		t.Fatalf("time.Parse failed: %v", err)
	}
	if err := repo.UpdateCursor(ctx, space, want); err != nil {
		t.Fatalf("UpdateCursor returned error: %v", err)
	}

	got, found, err := repo.GetCursor(ctx, space)
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if !found {
		t.Fatal("expected cursor to be found after UpdateCursor")
	}
	if !got.Equal(want) {
		t.Errorf("GetCursor = %v, want %v (sub-second precision lost in storage)", got, want)
	}
	// The exact condition chat_command_poller checks on the next cycle —
	// if this is true, the original message gets reprocessed forever.
	if want.After(got) {
		t.Errorf("want.After(got) = true — the original message's timestamp still looks newer than the stored cursor, so it would be reprocessed on every future poll cycle")
	}
}

func TestUpdateCursor_SecondCallOverwritesInsteadOfDuplicating(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewChatCommandPollCursorRepository(db)
	ctx := context.Background()
	space := "spaces/TESTMYSQL-OVERWRITE"
	t.Cleanup(func() { db.Exec(`DELETE FROM chat_command_poll_cursor WHERE googleChatDMSpaceID = ?`, space) })

	first := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	second := time.Date(2026, 10, 27, 11, 0, 0, 0, time.UTC)
	if err := repo.UpdateCursor(ctx, space, first); err != nil {
		t.Fatalf("first UpdateCursor returned error: %v", err)
	}
	if err := repo.UpdateCursor(ctx, space, second); err != nil {
		t.Fatalf("second UpdateCursor returned error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chat_command_poll_cursor WHERE googleChatDMSpaceID = ?`, space).Scan(&count); err != nil {
		t.Fatalf("count query returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 — second UpdateCursor must overwrite, not duplicate", count)
	}

	got, found, err := repo.GetCursor(ctx, space)
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if !found || !got.Equal(second) {
		t.Errorf("GetCursor = %v (found=%v), want %v", got, found, second)
	}
}
