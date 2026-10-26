package google_chat_notifier_service

import (
	"context"

	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
)

type GoogleChatNotifierService interface {
	SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error
}
