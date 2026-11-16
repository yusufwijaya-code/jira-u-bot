package chat_group

import (
	"context"
	"database/sql"
)

// Store is the join state on ms_user.
type Store interface {
	// NeedsJoin: active, not yet added, and has not activated the app.
	NeedsJoin(ctx context.Context, email string) (bool, error)
	// Pending lists up to limit such users (oldest first).
	Pending(ctx context.Context, limit int) ([]string, error)
	MarkAdded(ctx context.Context, email string) error
	MarkFailed(ctx context.Context, email, msg string) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Store { return &repository{db: db} }

const needsJoin = `flagActive = 1 AND chatGroupAddedDate IS NULL AND (chatAppSpaceID IS NULL OR chatAppSpaceID = '')`

func (r *repository) NeedsJoin(ctx context.Context, email string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ms_user WHERE email = ? AND `+needsJoin, email).Scan(&n)
	return n > 0, err
}

func (r *repository) Pending(ctx context.Context, limit int) ([]string, error) {
	// Never-tried users first, so users that keep failing can't fill every run.
	rows, err := r.db.QueryContext(ctx, `SELECT email FROM ms_user WHERE `+needsJoin+` ORDER BY chatGroupError IS NOT NULL, userID LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *repository) MarkAdded(ctx context.Context, email string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET chatGroupAddedDate = CURRENT_TIMESTAMP, chatGroupError = NULL WHERE email = ?`, email)
	return err
}

func (r *repository) MarkFailed(ctx context.Context, email, msg string) error {
	// A failure racing a success (login hook vs sweep) must not overwrite it.
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET chatGroupError = ? WHERE email = ? AND chatGroupAddedDate IS NULL`, msg, email)
	return err
}
