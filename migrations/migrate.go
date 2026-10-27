package migrations

import (
	"database/sql"
	_ "embed"
)

//go:embed 0001_init_mysql.sql
var initSQL string

//go:embed 0002_add_jira_account_id.sql
var addJiraAccountIDSQL string

func Apply(db *sql.DB) error {
	if _, err := db.Exec(initSQL); err != nil {
		return err
	}
	return applyAddJiraAccountID(db)
}

// applyAddJiraAccountID runs 0002_add_jira_account_id.sql's ALTER TABLE
// conditionally — MySQL 8.0.43 (the version this project targets) does not
// support "ADD COLUMN IF NOT EXISTS", so idempotency across repeated Apply
// calls is handled here instead, the same way v1's incremental migrations
// did before the clean-slate MySQL rewrite.
func applyAddJiraAccountID(db *sql.DB) error {
	exists, err := columnExists(db, "ms_user", "jiraAccountId")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.Exec(addJiraAccountIDSQL)
	return err
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	const query = `
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
	`
	var count int
	if err := db.QueryRow(query, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
