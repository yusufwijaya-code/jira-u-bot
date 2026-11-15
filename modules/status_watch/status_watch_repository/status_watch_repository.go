package status_watch_repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"nudge/modules/status_watch/status_watch_dto"
)

// StatusWatchRepository stores the status snapshot (tr_issuestatus).
type StatusWatchRepository interface {
	// Find returns the snapshots of keys that have one.
	Find(ctx context.Context, keys []string) (map[string]status_watch_dto.Snapshot, error)
	Upsert(ctx context.Context, s status_watch_dto.Snapshot) error
	// DeleteNotSeenSince removes tickets the watcher hasn't seen since t.
	DeleteNotSeenSince(ctx context.Context, t time.Time) (int64, error)
}

type statusWatchRepositoryImpl struct{ db *sql.DB }

func NewStatusWatchRepository(db *sql.DB) StatusWatchRepository {
	return &statusWatchRepositoryImpl{db: db}
}

func (r *statusWatchRepositoryImpl) Find(ctx context.Context, keys []string) (map[string]status_watch_dto.Snapshot, error) {
	out := map[string]status_watch_dto.Snapshot{}
	if len(keys) == 0 {
		return out, nil
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	query := `SELECT issueKey, status, assigneeEmail, lastSeenDate FROM tr_issuestatus WHERE issueKey IN (?` +
		strings.Repeat(",?", len(keys)-1) + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s status_watch_dto.Snapshot
		if err := rows.Scan(&s.IssueKey, &s.Status, &s.AssigneeEmail, &s.LastSeen); err != nil {
			return nil, err
		}
		out[s.IssueKey] = s
	}
	return out, rows.Err()
}

func (r *statusWatchRepositoryImpl) Upsert(ctx context.Context, s status_watch_dto.Snapshot) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tr_issuestatus (issueKey, status, assigneeEmail, lastSeenDate)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE status = VALUES(status), assigneeEmail = VALUES(assigneeEmail), lastSeenDate = VALUES(lastSeenDate)`,
		s.IssueKey, s.Status, s.AssigneeEmail, s.LastSeen)
	return err
}

func (r *statusWatchRepositoryImpl) DeleteNotSeenSince(ctx context.Context, t time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tr_issuestatus WHERE lastSeenDate < ?`, t)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
