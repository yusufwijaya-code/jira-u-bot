package reminder_log_service

import (
	"context"

	"jira-u-bot/modules/reminder_log/reminder_log_repository"
)

type reminderLogServiceImpl struct {
	repo reminder_log_repository.ReminderLogRepository
}

func NewReminderLogService(repo reminder_log_repository.ReminderLogRepository) ReminderLogService {
	return &reminderLogServiceImpl{repo: repo}
}

func (s *reminderLogServiceImpl) HasTierBeenSent(ctx context.Context, issueKey, tier string) (bool, error) {
	return s.repo.HasTierBeenSent(ctx, issueKey, tier)
}

func (s *reminderLogServiceImpl) LogReminderSent(ctx context.Context, issueKey, tier string) error {
	return s.repo.LogReminderSent(ctx, issueKey, tier)
}
