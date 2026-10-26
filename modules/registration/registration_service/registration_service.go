package registration_service

import (
	"context"
	"errors"
)

var (
	ErrJiraAccountNotFound   = errors.New("email not found as a Jira account")
	ErrJiraLookupUnavailable = errors.New("failed to verify email against Jira")
)

type RegistrationService interface {
	Register(ctx context.Context, name, email, googleChatUserID string) error
}
