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
	// JiraAccountID is the registrant's Jira accountId, captured at
	// registration time (LookupUserByEmail) — used to match this user
	// against an issue's assignee, since Jira hides emailAddress on
	// assignee objects org-wide but accountId is always present.
	JiraAccountID string
}
