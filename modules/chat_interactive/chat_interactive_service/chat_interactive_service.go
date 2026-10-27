package chat_interactive_service

import (
	"context"

	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
)

type ChatInteractiveService interface {
	// RouteCommand implements Tech Spec AC2.1/AC2.2/AC7.1/AC7.2: checks the
	// invoking user is registered, then dispatches to the matching command
	// handler (falling back to help for an unrecognized or missing
	// appCommandId). Always returns a user-facing text string — never an
	// error — because every outcome (unregistered, unknown command, ticket
	// not found) is a normal chat reply, not a failure (Tech Spec Error
	// Contract: this endpoint always responds 200 for valid requests).
	RouteCommand(ctx context.Context, event chat_interactive_dto.ChatEvent) string
}
