package reminder_log_service

import "context"

type ReminderLogService interface {
	HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error)
	LogReminderSent(ctx context.Context, issueKey, tier string) error
}
