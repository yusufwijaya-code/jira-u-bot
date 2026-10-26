package migrations

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed 0001_init.sql
var initSQL string

//go:embed 0002_add_google_chat_user_id.sql
var addGoogleChatUserIDSQL string

func Apply(db *sql.DB) error {
	if _, err := db.Exec(initSQL); err != nil {
		return err
	}
	return applyAddGoogleChatUserID(db)
}

// applyAddGoogleChatUserID runs the 0002 migration exactly once. Unlike
// CREATE TABLE/INDEX, SQLite's ALTER TABLE ADD COLUMN has no IF NOT EXISTS
// guard, so re-running it on every startup (like 0001) would crash the same
// way the original CREATE TABLE bug did — check column existence first.
func applyAddGoogleChatUserID(db *sql.DB) error {
	exists, err := columnExists(db, "ms_user", "googleChatUserID")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.Exec(addGoogleChatUserIDSQL)
	return err
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
