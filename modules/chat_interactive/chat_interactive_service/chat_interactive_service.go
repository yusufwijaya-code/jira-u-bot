package chat_interactive_service

import "context"

type ChatInteractiveService interface {
	// RouteCommand checks the sender is registered, then dispatches on the
	// literal /u-* command found at the start of messageText (falling back
	// to help for unregistered-looking text, same as an unrecognized
	// command). Always returns a user-facing text string — never an error —
	// because every outcome (unregistered, unknown command, ticket not
	// found) is a normal chat reply, not a failure. Callers (the DM command
	// poller) always treat the return value as something to send back to
	// the same space.
	RouteCommand(ctx context.Context, senderEmail, messageText string) string
}
