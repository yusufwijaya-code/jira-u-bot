package migrations

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApply_CreatesAllThreeTables(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	for _, table := range []string{"ms_user", "tr_issuesession", "tr_reminderlog"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q was not created: %v", table, err)
		}
	}
}

func TestApply_EnforcesUniqueEmailOnMsUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	insert := `INSERT INTO ms_user (userName, email, createdBy) VALUES (?, ?, ?)`
	if _, err := db.Exec(insert, "Anwar", "anwar@esb.co.id", "anwar@esb.co.id"); err != nil {
		t.Fatalf("first insert returned error: %v", err)
	}

	_, err = db.Exec(insert, "Anwar Again", "anwar@esb.co.id", "anwar@esb.co.id")
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation on duplicate email, got nil error")
	}
}

func TestApply_AddsGoogleChatUserIDColumnToMsUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	exists, err := columnExists(db, "ms_user", "googleChatUserID")
	if err != nil {
		t.Fatalf("columnExists returned error: %v", err)
	}
	if !exists {
		t.Error("expected ms_user.googleChatUserID column to exist after Apply")
	}
}

func TestApply_AddsColumnToDatabaseThatPredatesTheMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	defer db.Close()

	// Simulate a real production database created before googleChatUserID
	// existed — exactly the state the user's actual data/jira-u-bot.db is
	// in right now.
	if _, err := db.Exec(initSQL); err != nil {
		t.Fatalf("failed to apply the pre-0002 schema: %v", err)
	}

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error against a pre-existing database: %v", err)
	}

	exists, err := columnExists(db, "ms_user", "googleChatUserID")
	if err != nil {
		t.Fatalf("columnExists returned error: %v", err)
	}
	if !exists {
		t.Error("expected ms_user.googleChatUserID column to be added by Apply even on an already-migrated-to-0001 database")
	}
}

func TestApply_IsIdempotentAcrossRepeatedCalls(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("first Apply() returned error: %v", err)
	}

	// main.go calls Apply(db) on every process start against the same
	// on-disk file — a second call (simulating a restart) must not fail
	// with "table already exists".
	if err := Apply(db); err != nil {
		t.Fatalf("second Apply() returned error: %v — migrations must be idempotent across restarts", err)
	}
}
