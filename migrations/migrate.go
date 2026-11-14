package migrations

import (
	"database/sql"
	_ "embed"
	"errors"
	"time"

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

//go:embed 0005_add_dashboard_auth_tables.sql
var addDashboardAuthTablesSQL string

//go:embed 0006_add_reminder_tier_preference.sql
var addReminderTierPreferenceSQL string

//go:embed 0007_add_sprint_name.sql
var addSprintNameSQL string

//go:embed 0008_add_user_setting.sql
var addUserSettingSQL string

//go:embed 0009_add_reminderlog_delivery_status.sql
var addReminderLogDeliveryStatusSQL string

//go:embed 0010_add_board.sql
var addBoardSQL string

//go:embed 0011_issuesession_board.sql
var addIssueSessionBoardSQL string

//go:embed 0011b_issuesession_sprint.sql
var addIssueSessionSprintSQL string

//go:embed 0012_user_setting_board.sql
var addUserSettingBoardSQL string

//go:embed 0012b_user_setting_use_default.sql
var addUserSettingUseDefaultSQL string

//go:embed 0012c_user_setting_board_pk.sql
var userSettingBoardPKSQL string

//go:embed 0013_add_user_picture.sql
var addUserPictureSQL string

// mysqlDeadlockErrNo / mysqlLockWaitTimeoutErrNo are retried: go test runs
// every package in parallel and each one calls Apply against the same
// schema, so concurrent DDL/backfill can collide. Every step is
// idempotent, so re-running Apply is always safe.
const (
	mysqlDeadlockErrNo        = 1213
	mysqlLockWaitTimeoutErrNo = 1205
	applyAttempts             = 5
)

func Apply(db *sql.DB) error {
	var err error
	for attempt := 1; attempt <= applyAttempts; attempt++ {
		if err = apply(db); err == nil || !isRetryable(err) {
			return err
		}
		time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
	}
	return err
}

func isRetryable(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == mysqlDeadlockErrNo || mysqlErr.Number == mysqlLockWaitTimeoutErrNo)
}

func apply(db *sql.DB) error {
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
	if _, err := db.Exec(cursorSubsecondPrecisionSQL); err != nil {
		return err
	}
	// Both CREATE TABLE IF NOT EXISTS statements — naturally idempotent,
	// same as addChatCommandPollCursorSQL above.
	if _, err := db.Exec(addDashboardAuthTablesSQL); err != nil {
		return err
	}
	// ADD COLUMN — needs the same duplicate-column tolerance as
	// applyAddJiraAccountID (MySQL 8.0.43 has no "ADD COLUMN IF NOT EXISTS").
	if err := addColumnIfNotExists(db, addReminderTierPreferenceSQL); err != nil {
		return err
	}
	if err := addColumnIfNotExists(db, addSprintNameSQL); err != nil {
		return err
	}
	// CREATE TABLE IF NOT EXISTS + INSERT IGNORE backfill — both naturally
	// idempotent. Must run after 0006 (reads ms_user.reminderTierPreference).
	if _, err := db.Exec(addUserSettingSQL); err != nil {
		return err
	}
	if err := addColumnIfNotExists(db, addReminderLogDeliveryStatusSQL); err != nil {
		return err
	}
	if err := applyPerBoardSettings(db); err != nil {
		return err
	}
	// CREATE TABLE IF NOT EXISTS — naturally idempotent.
	_, err := db.Exec(addUserPictureSQL)
	return err
}

// applyPerBoardSettings (Tech Spec "Per-Board Settings & Active Sprint
// Dashboard"): board cache, the active board/sprint of each tracked session,
// and ms_user_setting keyed by (userID, boardID) where boardID 0 is the
// user's default for every board.
func applyPerBoardSettings(db *sql.DB) error {
	if _, err := db.Exec(addBoardSQL); err != nil {
		return err
	}
	for _, alter := range []string{addIssueSessionBoardSQL, addIssueSessionSprintSQL, addUserSettingBoardSQL, addUserSettingUseDefaultSQL} {
		if err := addColumnIfNotExists(db, alter); err != nil {
			return err
		}
	}
	// The PK swap is not naturally idempotent — only run it while the
	// primary key doesn't include boardID yet. Existing rows keep
	// boardID = 0, i.e. they become each user's default settings.
	done, err := userSettingPKHasBoard(db)
	if err != nil || done {
		return err
	}
	if _, err := db.Exec(userSettingBoardPKSQL); err != nil {
		// Another process (go test runs packages in parallel against the
		// same schema) may have swapped it between the check and the ALTER.
		if done, checkErr := userSettingPKHasBoard(db); checkErr == nil && done {
			return nil
		}
		return err
	}
	return nil
}

func userSettingPKHasBoard(db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'ms_user_setting'
		  AND CONSTRAINT_NAME = 'PRIMARY'
		  AND COLUMN_NAME = 'boardID'
	`).Scan(&n)
	return n > 0, err
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
