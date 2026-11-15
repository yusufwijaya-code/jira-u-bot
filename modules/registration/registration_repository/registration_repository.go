package registration_repository

import (
	"context"

	"nudge/modules/registration/registration_dto"
)

type RegistrationRepository interface {
	UpsertUser(ctx context.Context, name, email, jiraAccountID string) error
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
	// FindUserByJiraAccountID looks up a registered user by their Jira
	// accountId — the identifier used to match a registered user against
	// an issue's assignee (Jira hides assignee.emailAddress org-wide, but
	// accountId is always present).
	FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error)
	UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error
	// FindAllWithGoogleChatDMSpaceID returns every registered user that
	// already has a cached DM space (i.e. has received at least one DM from
	// the bot) — chat_command_poller uses this as its poll scope, so a
	// sender is only ever someone the registration + reminder flow already
	// vetted.
	FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error)
	// FindAll lists every registered user — Admin Dashboard (spec AC6.1).
	FindAll(ctx context.Context) ([]registration_dto.User, error)
	UpdateFlagActive(ctx context.Context, userID int64, active bool) error
	UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error
	// DeleteByID permanently removes a user (spec AC6.5) — tr_usersession
	// and tr_usercredential cascade via ON DELETE CASCADE.
	DeleteByID(ctx context.Context, userID int64) error
}
