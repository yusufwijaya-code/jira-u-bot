package chat_command_poller_service

import "context"

type ChatCommandPollerService interface {
	// Run blocks until ctx is cancelled, polling every 10 seconds. Callers
	// launch it as a goroutine (see main.go) — it never returns until
	// shutdown.
	Run(ctx context.Context)
}

// DMSender is the narrow slice of chat_dm_notifier_service.ChatDMNotifierService
// this poller actually needs — satisfied structurally by
// *chat_dm_notifier_service.chatDMNotifierServiceImpl, no adapter required.
type DMSender interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}
