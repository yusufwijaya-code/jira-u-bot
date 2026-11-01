package auth_session_repository

import (
	"context"
	"time"

	"jira-u-bot/modules/auth/auth_dto"
)

type AuthSessionRepository interface {
	Create(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) error
	// FindByID joins tr_usersession with ms_user so callers get email and
	// flagActive in one query — "not found" covers both a missing row and
	// an expired one (expiresAt <= now), callers don't need to distinguish.
	FindByID(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error)
	Delete(ctx context.Context, sessionID string) error
}
