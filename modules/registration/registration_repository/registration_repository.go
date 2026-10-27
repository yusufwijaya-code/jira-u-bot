package registration_repository

import (
	"context"

	"jira-u-bot/modules/registration/registration_dto"
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
}
