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
