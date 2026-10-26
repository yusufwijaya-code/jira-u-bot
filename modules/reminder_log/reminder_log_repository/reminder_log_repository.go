package reminder_log_repository

import "context"

type ReminderLogRepository interface {
	HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error)
	LogReminderSent(ctx context.Context, issueKey, tier string) error
}
