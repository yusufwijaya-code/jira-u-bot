package migrations

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

const testDSN = "root:@(localhost:3308)/jira_db?parseTime=true&loc=Asia%2FJakarta&multiStatements=true"

func openTestConn(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Fatalf("failed to open mysql connection: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping mysql at %s: %v — is the local MySQL server running on port 3308?", testDSN, err)
	}
	return db
}

func TestApply_CreatesAllThreeTables(t *testing.T) {
	db := openTestConn(t)

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	for _, table := range []string{"ms_user", "tr_issuesession", "tr_reminderlog"} {
		var name string
		err := db.QueryRow(`SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = 'jira_db' AND TABLE_NAME = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q was not created: %v", table, err)
		}
	}
}

func TestApply_IsIdempotentAcrossRepeatedCalls(t *testing.T) {
	db := openTestConn(t)

	if err := Apply(db); err != nil {
		t.Fatalf("first Apply() returned error: %v", err)
	}
	// main.go calls Apply(db) on every process start against the same
	// persistent MySQL schema — a second call (simulating a restart) must
	// not fail with "table already exists".
	if err := Apply(db); err != nil {
		t.Fatalf("second Apply() returned error: %v — migrations must be idempotent across restarts", err)
	}
}

func TestApply_NewColumnsExistOnMsUserAndTrIssuesession(t *testing.T) {
	db := openTestConn(t)

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	cases := []struct {
		table, column string
	}{
		{"ms_user", "googleChatDMSpaceID"},
		{"tr_issuesession", "assigneeEmail"},
		{"tr_issuesession", "issueSummary"},
	}
	for _, c := range cases {
		var name string
		err := db.QueryRow(`SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = 'jira_db' AND TABLE_NAME = ? AND COLUMN_NAME = ?`, c.table, c.column).Scan(&name)
		if err != nil {
			t.Errorf("column %s.%s was not created: %v", c.table, c.column, err)
		}
	}
}

func TestApply_DoesNotCreateGoogleChatUserIDColumn(t *testing.T) {
	db := openTestConn(t)

	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = 'jira_db' AND TABLE_NAME = 'ms_user' AND COLUMN_NAME = 'googleChatUserID'`).Scan(&count)
	if err != nil {
		t.Fatalf("query returned error: %v", err)
	}
	if count != 0 {
		t.Error("googleChatUserID column must not exist — that feature was reverted (spec Open Question #5)")
	}
}

func TestApply_EnforcesUniqueEmailOnMsUser(t *testing.T) {
	db := openTestConn(t)
	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM ms_user WHERE email = ?`, "testmysql-migrate@esb.co.id")
	})

	insert := `INSERT INTO ms_user (userName, email, createdBy) VALUES (?, ?, ?)`
	if _, err := db.Exec(insert, "TestMySQL Migrate", "testmysql-migrate@esb.co.id", "testmysql-migrate@esb.co.id"); err != nil {
		t.Fatalf("first insert returned error: %v", err)
	}

	_, err := db.Exec(insert, "TestMySQL Migrate Again", "testmysql-migrate@esb.co.id", "testmysql-migrate@esb.co.id")
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation on duplicate email, got nil error")
	}
}
