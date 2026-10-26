package testutil

import (
	"database/sql"
	"testing"

	"jira-u-bot/migrations"

	_ "github.com/go-sql-driver/mysql"
)

const testDSN = "root:@(localhost:3308)/jira_db?parseTime=true&loc=Asia%2FJakarta&multiStatements=true"

// OpenTestDB connects to the SAME local MySQL schema (jira_db) the live
// service uses — there is no separate test schema (spec Open Question #2,
// explicit user decision). Callers are responsible for cleaning up their
// own rows via t.Cleanup, using a distinctive key prefix (TESTMYSQL- for
// issue keys, testmysql-*@esb.co.id for emails) to avoid colliding with
// other tests or with real production data in the same tables.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Fatalf("failed to open mysql connection: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping mysql at %s: %v — is the local MySQL server running on port 3308 with an empty-password root user and a jira_db schema already created?", testDSN, err)
	}

	if err := migrations.Apply(db); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}
