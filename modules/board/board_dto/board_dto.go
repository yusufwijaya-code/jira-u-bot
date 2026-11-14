// Package board_dto holds the Jira Agile boards Nudge knows about (Tech Spec
// "Per-Board Settings & Active Sprint Dashboard").
package board_dto

// Board is one cached Jira Agile board (ms_board).
type Board struct {
	BoardID    int64
	Name       string
	ProjectKey string
	// AvatarURL is the board's project avatar (Jira boards have none of
	// their own) — served to the browser only through the avatar proxy.
	AvatarURL string
}
