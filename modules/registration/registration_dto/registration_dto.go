package registration_dto

import "time"

type User struct {
	UserID      int64
	UserName    string
	Email       string
	FlagActive  bool
	CreatedBy   string
	CreatedDate time.Time
	// GoogleChatUserID is the numeric Google Workspace user ID (the part
	// after "users/" in a mention tag like <users/123...>), self-reported
	// by the registrant — empty when they haven't supplied one. Used to
	// @mention them in reminders instead of just printing their name.
	GoogleChatUserID string
}
