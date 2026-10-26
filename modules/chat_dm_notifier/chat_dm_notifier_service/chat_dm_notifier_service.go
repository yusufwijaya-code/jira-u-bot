package chat_dm_notifier_service

import (
	"context"

	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
)

// ChatDMNotifierService sends Google Chat messages as a personal DM via
// OAuth (Yusuf's own account), not a bot/webhook identity. SendReminder has
// the exact same signature as
// google_chat_notifier_service.GoogleChatNotifierService, so a
// *chatDMNotifierServiceImpl satisfies that interface too — Go's
// structural typing means reminder_engine_service can hold both the
// webhook and DM notifiers as that one interface type and pick between
// them per message (spec Note C3), with no adapter type needed.
type ChatDMNotifierService interface {
	SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error
	// SendPlainTextDM sends a plain-text message with no ReminderMessage
	// formatting — used for the registration thank-you note (Chapter 5b),
	// which has no "self" webhook counterpart.
	SendPlainTextDM(ctx context.Context, email, text string) error
}
