package registration_repository

import (
	"context"

	"jira-u-bot/modules/registration/registration_dto"
)

type RegistrationRepository interface {
	UpsertUser(ctx context.Context, name, email, googleChatUserID string) error
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}
