package admin_dashboard_service

import (
	"context"
	"errors"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
)

// ErrCannotModifySelf is spec's self-lockout guard: an admin can't
// deactivate or delete their own account through this panel.
var ErrCannotModifySelf = errors.New("admin cannot deactivate or delete their own account")

// ErrDMNotConfigured means personal DM (Google Chat OAuth) was never set
// up on this server — BroadcastMessage refuses to run rather than silently
// no-op, since there's no fallback channel that reaches every registered
// user individually.
var ErrDMNotConfigured = errors.New("personal DM notifier is not configured")

// AdminDashboardService implements Tech Spec "Dashboard Jira Manage",
// Chapter 10a (user list/lifecycle) and 10b (credential controls).
type AdminDashboardService interface {
	ListUsers(ctx context.Context) ([]admin_dashboard_dto.UserRow, error)
	// SetActive toggles a user's flagActive — actingUserID must not equal
	// targetUserID (AC: self-lockout guard).
	SetActive(ctx context.Context, actingUserID, targetUserID int64, active bool) error
	UpdateJiraAccountID(ctx context.Context, targetUserID int64, newAccountID string) error
	// DeleteUser permanently removes targetUserID — actingUserID must not
	// equal targetUserID.
	DeleteUser(ctx context.Context, actingUserID, targetUserID int64) error
	// DeleteUserCredential resets targetUserID's personal Jira credential
	// (spec AC6.4) — reuses jira_credential_service.DeleteCredential as-is.
	DeleteUserCredential(ctx context.Context, targetUserID int64) error
	// BroadcastMessage DMs message to every active registered user except
	// the bot operator's own account (DMing yourself is rejected by the
	// Google Chat API — same constraint as the registration thank-you
	// note). Returns ErrDMNotConfigured if DM was never set up.
	BroadcastMessage(ctx context.Context, message string) (admin_dashboard_dto.BroadcastResult, error)
}
