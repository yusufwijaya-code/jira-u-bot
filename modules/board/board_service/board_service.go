package board_service

import (
	"context"
	"errors"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
)

// ErrAvatarNotFound means the board is unknown or has no avatar.
var ErrAvatarNotFound = errors.New("board avatar not found")

type BoardService interface {
	// EnsureKnown caches a board's name/project/avatar from the Jira Agile
	// API the first time it is seen; already-known boards cost one DB read.
	EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error)
	// Get returns a cached board (found=false if never seen).
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
	// ActiveSprint returns the board's current active sprint (with dates),
	// cached briefly in memory.
	ActiveSprint(ctx context.Context, boardID int64) (jira_client_dto.SprintRef, bool, error)
	// Avatar returns the board's project avatar image via the bot
	// credentials, cached in memory for 24h. Unknown boards →
	// ErrAvatarNotFound (the proxy never fetches arbitrary URLs).
	Avatar(ctx context.Context, boardID int64) (body []byte, contentType string, err error)
}
