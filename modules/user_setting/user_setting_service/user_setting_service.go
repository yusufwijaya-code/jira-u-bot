package user_setting_service

import (
	"context"
	"errors"
	"time"

	"nudge/modules/user_setting/user_setting_dto"
)

// ErrDefaultScope is returned when a board-only action (reset to default) is
// asked for the default scope itself.
var ErrDefaultScope = errors.New("the default scope cannot be reset to default")

// BoardState is what the board picker needs about one board row.
type BoardState struct {
	BoardID int64
	// Custom: the board has its own notification settings.
	Custom     bool
	MutedUntil *time.Time
}

// UserSettingService resolves per-board settings (Tech Spec "Per-Board
// Settings & Active Sprint Dashboard"): boardID 0 is the user's default for
// every board; a board without custom settings follows the default; the
// global mute (default row) and the board mute both silence a board.
type UserSettingService interface {
	// GetEffectiveForEmail is the reminder engine's read path: it never
	// fails — unregistered assignee, missing rows, or a DB error all yield
	// defaults (AC6.1 of the previous spec still holds).
	GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting
	// GetEffective is the dashboard's read path (errors are surfaced).
	GetEffective(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, error)
	// SaveNotification validates and stores everything except mute for the
	// scope; for a board it also makes the board custom. A validation
	// failure is a *user_setting_dto.ValidationError.
	SaveNotification(ctx context.Context, userID, boardID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error)
	// ResetToDefault makes a board follow the default again (keeps its mute).
	ResetToDefault(ctx context.Context, userID, boardID int64) error
	// MuteUntil / Unmute: boardID 0 = global mute, otherwise that board.
	MuteUntil(ctx context.Context, userID, boardID int64, until time.Time) error
	Unmute(ctx context.Context, userID, boardID int64) error
	// BoardStates returns custom/mute state of every board row of the user.
	BoardStates(ctx context.Context, userID int64) (map[int64]BoardState, error)
	// NotificationSwitch / SetNotificationsEnabled: the master switch for
	// automatic notifications (Tech Spec "Notification On-Off Consent").
	// Setting it also records that the user made the choice.
	NotificationSwitch(ctx context.Context, userID int64) (user_setting_dto.NotificationSwitch, error)
	SetNotificationsEnabled(ctx context.Context, userID int64, enabled bool) (user_setting_dto.NotificationSwitch, error)
}

// ErrUserNotFound is returned for a switch read of a missing user.
var ErrUserNotFound = errors.New("user not found")
