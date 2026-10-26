package registration_dto

import "time"

type User struct {
	UserID      int64
	UserName    string
	Email       string
	FlagActive  bool
	CreatedBy   string
	CreatedDate time.Time
	// GoogleChatDMSpaceID caches the Google Chat DM space resource name
	// (from spaces:setup) so chat_dm_notifier doesn't need to re-resolve it
	// on every send — empty until the first successful DM to this user.
	GoogleChatDMSpaceID string
}
