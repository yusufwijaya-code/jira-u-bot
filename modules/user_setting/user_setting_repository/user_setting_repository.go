package user_setting_repository

import (
	"context"
	"time"

	"nudge/modules/user_setting/user_setting_dto"
)

type UserSettingRepository interface {
	// FindByUserID returns the stored row; found=false means the user has
	// never saved settings (callers fall back to user_setting_dto.Default).
	FindByUserID(ctx context.Context, userID int64) (setting user_setting_dto.UserSetting, found bool, err error)
	// UpsertNotification writes every field except mutedUntil.
	UpsertNotification(ctx context.Context, setting user_setting_dto.UserSetting) error
	// SetMutedUntil sets (or with nil, clears) the temporary mute, creating
	// the row with defaults if it doesn't exist yet.
	SetMutedUntil(ctx context.Context, userID int64, until *time.Time) error
}
