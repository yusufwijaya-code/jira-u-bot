package jira_credential_repository

import (
	"context"
	"database/sql"

	"nudge/modules/jira_credential/jira_credential_dto"
)

type jiraCredentialRepositoryImpl struct {
	db *sql.DB
}

func NewJiraCredentialRepository(db *sql.DB) JiraCredentialRepository {
	return &jiraCredentialRepositoryImpl{db: db}
}

func (r *jiraCredentialRepositoryImpl) Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error {
	const query = `
		INSERT INTO tr_usercredential (userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdBy, createdDate)
		VALUES (?, ?, ?, 1, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			jiraEmail = VALUES(jiraEmail),
			jiraAPITokenEncrypted = VALUES(jiraAPITokenEncrypted),
			flagActive = 1,
			editedBy = VALUES(createdBy),
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy record the Jira email the token is FOR, not an
	// admin actor — same self-service convention as registration_repository.
	_, err := r.db.ExecContext(ctx, query, userID, jiraEmail, encrypted, jiraEmail)
	return err
}

func (r *jiraCredentialRepositoryImpl) FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error) {
	const query = `
		SELECT userCredentialID, userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdDate
		FROM tr_usercredential
		WHERE userID = ? AND flagActive = 1
	`
	row := r.db.QueryRowContext(ctx, query, userID)
	return scanRecord(row)
}

func scanRecord(row *sql.Row) (*jira_credential_dto.Record, bool, error) {
	var rec jira_credential_dto.Record
	err := row.Scan(&rec.UserCredentialID, &rec.UserID, &rec.JiraEmail, &rec.JiraAPITokenEncrypted, &rec.FlagActive, &rec.CreatedDate)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &rec, true, nil
}

func (r *jiraCredentialRepositoryImpl) DeleteByUserID(ctx context.Context, userID int64) error {
	const query = `DELETE FROM tr_usercredential WHERE userID = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *jiraCredentialRepositoryImpl) FindAll(ctx context.Context) ([]jira_credential_dto.Record, error) {
	const query = `
		SELECT userCredentialID, userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdDate
		FROM tr_usercredential
		WHERE flagActive = 1
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []jira_credential_dto.Record
	for rows.Next() {
		var rec jira_credential_dto.Record
		if err := rows.Scan(&rec.UserCredentialID, &rec.UserID, &rec.JiraEmail, &rec.JiraAPITokenEncrypted, &rec.FlagActive, &rec.CreatedDate); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
