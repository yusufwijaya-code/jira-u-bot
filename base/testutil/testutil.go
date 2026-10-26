package testutil

import (
	"database/sql"
	"testing"

	"jira-u-bot/migrations"

	_ "modernc.org/sqlite"
)

func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := migrations.Apply(db); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}
