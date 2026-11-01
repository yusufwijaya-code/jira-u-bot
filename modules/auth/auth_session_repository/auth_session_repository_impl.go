package auth_session_repository

import (
	"context"
	"database/sql"
	"time"

	"jira-u-bot/modules/auth/auth_dto"
)

type authSessionRepositoryImpl struct {
	db *sql.DB
}

func NewAuthSessionRepository(db *sql.DB) AuthSessionRepository {
	return &authSessionRepositoryImpl{db: db}
}

func (r *authSessionRepositoryImpl) Create(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) error {
	const query = `
		INSERT INTO tr_usersession (sessionID, userID, expiresAt, createdDate)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`
	_, err := r.db.ExecContext(ctx, query, sessionID, userID, expiresAt)
	return err
}

func (r *authSessionRepositoryImpl) FindByID(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error) {
	const query = `
		SELECT s.sessionID, s.userID, u.email, u.flagActive, s.expiresAt
		FROM tr_usersession s
		JOIN ms_user u ON u.userID = s.userID
		WHERE s.sessionID = ? AND s.expiresAt > CURRENT_TIMESTAMP
	`
	row := r.db.QueryRowContext(ctx, query, sessionID)

	var s auth_dto.Session
	err := row.Scan(&s.SessionID, &s.UserID, &s.Email, &s.FlagActive, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &s, true, nil
}

func (r *authSessionRepositoryImpl) Delete(ctx context.Context, sessionID string) error {
	const query = `DELETE FROM tr_usersession WHERE sessionID = ?`
	_, err := r.db.ExecContext(ctx, query, sessionID)
	return err
}
