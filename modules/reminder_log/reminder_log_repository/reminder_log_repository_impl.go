package reminder_log_repository

import (
	"context"
	"database/sql"
)

type reminderLogRepositoryImpl struct {
	db *sql.DB
}

func NewReminderLogRepository(db *sql.DB) ReminderLogRepository {
	return &reminderLogRepositoryImpl{db: db}
}

func (r *reminderLogRepositoryImpl) HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error) {
	const query = `
		SELECT 1
		FROM tr_reminderlog
		WHERE issueKey = ?
		  AND tier = ?
		LIMIT 1
	`
	var exists int
	err := r.db.QueryRowContext(ctx, query, issueKey, tier).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *reminderLogRepositoryImpl) LogReminderSent(ctx context.Context, issueKey, tier string) error {
	const query = `
		INSERT INTO tr_reminderlog (issueKey, tier, sentDate)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`
	_, err := r.db.ExecContext(ctx, query, issueKey, tier)
	return err
}
