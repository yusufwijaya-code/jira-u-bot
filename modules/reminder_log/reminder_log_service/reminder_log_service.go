package reminder_log_service

import "context"

type ReminderLogService interface {
	HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error)
	LogReminderSent(ctx context.Context, issueKey, tier string) error
	// LogReminder records a tier as handled with the given
	// reminder_log_dto.DeliverySent / DeliverySkipped status.
	LogReminder(ctx context.Context, issueKey, tier, status string) error
}
