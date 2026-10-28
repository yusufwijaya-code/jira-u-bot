package migrations

import (
	"database/sql"
	_ "embed"
	"errors"

	"github.com/go-sql-driver/mysql"
)

// mysqlDupColumnErrNo is MySQL's ER_DUP_FIELDNAME ("Duplicate column
// name") — see addColumnIfNotExists.
const mysqlDupColumnErrNo = 1060

//go:embed 0001_init_mysql.sql
var initSQL string

//go:embed 0002_add_jira_account_id.sql
var addJiraAccountIDSQL string

//go:embed 0003_add_chat_command_poll_cursor.sql
var addChatCommandPollCursorSQL string

//go:embed 0004_cursor_subsecond_precision.sql
var cursorSubsecondPrecisionSQL string

func Apply(db *sql.DB) error {
	if _, err := db.Exec(initSQL); err != nil {
		return err
	}
	if err := applyAddJiraAccountID(db); err != nil {
		return err
	}
	// CREATE TABLE IF NOT EXISTS is naturally idempotent (unlike ADD COLUMN
	// on MySQL 8.0.43 — see applyAddJiraAccountID), so this needs no
	// conditional wrapper.
	if _, err := db.Exec(addChatCommandPollCursorSQL); err != nil {
		return err
	}
	// MODIFY COLUMN re-applying the same target type is naturally
	// idempotent too (unlike ADD COLUMN), so no conditional wrapper needed.
	_, err := db.Exec(cursorSubsecondPrecisionSQL)
	return err
}

// applyAddJiraAccountID runs 0002_add_jira_account_id.sql's ALTER TABLE —
// MySQL 8.0.43 (the version this project targets) does not support "ADD
// COLUMN IF NOT EXISTS", so idempotency is handled by addColumnIfNotExists
// instead, the same way v1's incremental migrations did before the
// clean-slate MySQL rewrite.
func applyAddJiraAccountID(db *sql.DB) error {
	return addColumnIfNotExists(db, addJiraAccountIDSQL)
}

// addColumnIfNotExists runs alterSQL and tolerates MySQL reporting the
// column as already present. A separate "does it exist?" pre-check
// (the previous approach) is a TOCTOU race: go test ./... runs each
// package's tests as a separate process, and several packages call
// Apply(db) against the SAME shared jira_db schema (confirmed live in CI —
// see TestAddColumnIfNotExists_ConcurrentCallsNeverFail), so two processes
// can both see "missing" before either's ALTER commits. Attempting the
// ALTER unconditionally and swallowing only the specific duplicate-column
// error removes the race window entirely.
func addColumnIfNotExists(db *sql.DB, alterSQL string) error {
	_, err := db.Exec(alterSQL)
	if err == nil {
		return nil
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDupColumnErrNo {
		return nil
	}
	return err
}
