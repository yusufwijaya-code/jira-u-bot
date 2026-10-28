package chat_command_poll_cursor_repository

import (
	"context"
	"database/sql"
	"time"
)

type chatCommandPollCursorRepositoryImpl struct {
	db *sql.DB
}

func NewChatCommandPollCursorRepository(db *sql.DB) ChatCommandPollCursorRepository {
	return &chatCommandPollCursorRepositoryImpl{db: db}
}

func (r *chatCommandPollCursorRepositoryImpl) GetCursor(ctx context.Context, spaceID string) (time.Time, bool, error) {
	const query = `SELECT lastProcessedMessageCreateTime FROM chat_command_poll_cursor WHERE googleChatDMSpaceID = ?`
	var lastProcessed time.Time
	err := r.db.QueryRowContext(ctx, query, spaceID).Scan(&lastProcessed)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return lastProcessed, true, nil
}

func (r *chatCommandPollCursorRepositoryImpl) UpdateCursor(ctx context.Context, spaceID string, lastProcessed time.Time) error {
	const query = `
		INSERT INTO chat_command_poll_cursor (googleChatDMSpaceID, lastProcessedMessageCreateTime)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE lastProcessedMessageCreateTime = VALUES(lastProcessedMessageCreateTime)
	`
	_, err := r.db.ExecContext(ctx, query, spaceID, lastProcessed)
	return err
}
