package chat_interactive_service

import "context"

type ChatInteractiveService interface {
	// RouteCommand checks the sender is registered, then dispatches on the
	// literal /n-* command found at the start of messageText (falling back
	// to help for unregistered-looking text, same as an unrecognized
	// command). Always returns a user-facing text string — never an error —
	// because every outcome (unregistered, unknown command, ticket not
	// found) is a normal chat reply, not a failure. Callers (the DM command
	// poller) always treat the return value as something to send back to
	// the same space.
	RouteCommand(ctx context.Context, senderEmail, messageText string) string

	// Geser moves one of the sender's own tracked tickets (the /n-geser
	// rules) and reports the outcome in a structured form for the Chat
	// App's card buttons (Tech Spec "Nudge sebagai Google Chat App", AC6.3).
	Geser(ctx context.Context, senderEmail, issueKey, targetSlug string) GeserResult
}

// GeserResult is the outcome of Geser. Message is the same user-facing
// text /n-geser replies with.
type GeserResult struct {
	OK         bool
	NotOwner   bool
	StatusName string
	Message    string
}
