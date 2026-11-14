package user_setting_repository

import (
	"context"
	"time"

	"nudge/modules/user_setting/user_setting_dto"
)

// UserSettingRepository stores ms_user_setting rows keyed by (userID,
// boardID); boardID 0 is the user's default for every board.
type UserSettingRepository interface {
	// FindRow returns one stored row; found=false if it doesn't exist.
	FindRow(ctx context.Context, userID, boardID int64) (setting user_setting_dto.UserSetting, found bool, err error)
	// ListRows returns every row of the user (default + boards).
	ListRows(ctx context.Context, userID int64) ([]user_setting_dto.UserSetting, error)
	// UpsertNotification writes every notification field of
	// setting.BoardID's row (not the mute) and marks it as custom
	// (useDefault = 0).
	UpsertNotification(ctx context.Context, setting user_setting_dto.UserSetting) error
	// SetUseDefault switches a board row back to following the default
	// (keeps its mute). No-op if the row doesn't exist.
	SetUseDefault(ctx context.Context, userID, boardID int64) error
	// SetMutedUntil sets (or with nil, clears) the mute of (userID,
	// boardID), creating the row if needed — a new board row follows the
	// default (useDefault = 1).
	SetMutedUntil(ctx context.Context, userID, boardID int64, until *time.Time) error
	// FindNotificationSwitch reads the master switch from ms_user;
	// found=false if the user doesn't exist.
	FindNotificationSwitch(ctx context.Context, userID int64) (sw user_setting_dto.NotificationSwitch, found bool, err error)
	// SaveNotificationSwitch stores the user's choice and marks it as made.
	SaveNotificationSwitch(ctx context.Context, userID int64, enabled bool) error
}
