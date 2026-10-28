package chat_command_poll_cursor_repository

import (
	"context"
	"time"
)

// ChatCommandPollCursorRepository persists, per DM space, the createTime of
// the last message chat_command_poller has already handled — so a restart
// or a slow poll cycle never reprocesses (and re-replies to) a command that
// already got an answer.
type ChatCommandPollCursorRepository interface {
	GetCursor(ctx context.Context, spaceID string) (time.Time, bool, error)
	UpdateCursor(ctx context.Context, spaceID string, lastProcessed time.Time) error
}
