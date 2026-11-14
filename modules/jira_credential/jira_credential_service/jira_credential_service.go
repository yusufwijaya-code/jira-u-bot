package jira_credential_service

import (
	"context"

	"nudge/modules/jira_client/jira_client_dto"
)

// JiraCredentialService implements Tech Spec "Dashboard Jira Manage",
// Chapter 9e (personal Jira API key setup) and Chapter 10b (admin masked
// view/reset).
type JiraCredentialService interface {
	// ResolveCredential returns email's personal credential, or (nil, nil)
	// if they haven't set one up (or it's been deactivated) — this is NOT
	// an error case, callers (Chapter 9b, 9d) fall back to the central
	// credential on a nil result (spec AC5.4, Open Question #2).
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
	UpsertCredential(ctx context.Context, userID int64, jiraEmail, apiToken string) error
	DeleteCredential(ctx context.Context, userID int64) error
	// MaskedPreview returns a display-safe masked form of userID's token
	// (e.g. "••••••••aB3f") — the full decrypted token never leaves this
	// service (spec AC6.4).
	MaskedPreview(ctx context.Context, userID int64) (preview string, found bool, err error)
}
