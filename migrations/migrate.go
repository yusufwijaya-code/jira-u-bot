package migrations

import (
	"database/sql"
	_ "embed"
)

//go:embed 0001_init.sql
var initSQL string

func Apply(db *sql.DB) error {
	_, err := db.Exec(initSQL)
	return err
}
