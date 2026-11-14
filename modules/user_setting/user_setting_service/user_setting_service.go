package user_setting_service

import (
	"context"
	"time"

	"nudge/modules/user_setting/user_setting_dto"
)

type UserSettingService interface {
	// GetForEmail is the reminder engine's read path: it never fails — an
	// unregistered assignee, a user without a settings row, or a DB error
	// all yield user_setting_dto.Default (AC6.1), so settings can never stop
	// reminders.
	GetForEmail(ctx context.Context, email string) user_setting_dto.UserSetting
	// GetForUser is the dashboard's read path (errors are surfaced).
	GetForUser(ctx context.Context, userID int64) (user_setting_dto.UserSetting, error)
	// SaveNotification validates and stores everything except mute. A
	// validation failure is a *user_setting_dto.ValidationError.
	SaveNotification(ctx context.Context, userID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error)
	// MuteUntil starts (or extends/shortens) a temporary mute.
	MuteUntil(ctx context.Context, userID int64, until time.Time) error
	// Unmute clears a temporary mute.
	Unmute(ctx context.Context, userID int64) error
}
