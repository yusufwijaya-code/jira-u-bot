package auth_service

import (
	"context"
	"errors"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/registration/registration_dto"
)

var (
	// ErrGoogleVerificationFailed covers both a failed code exchange and a
	// failed ID token verification (spec AC1.2) — callers don't need to
	// distinguish the two, both mean "reject, do not create a session".
	ErrGoogleVerificationFailed = errors.New("failed to verify Google sign-in")
	// ErrDomainNotAllowed is spec AC1.3/AC1.4: the token verified fine, but
	// the email isn't an @esb.co.id address, or isn't verified by Google.
	ErrDomainNotAllowed = errors.New("email domain not allowed")
	// ErrUserDeactivated is spec AC1.4 (flagActive check): the account
	// exists but an admin has deactivated it — must not get a session.
	ErrUserDeactivated = errors.New("user account is deactivated")
)

// AuthService covers spec Chapter 8a (Google Sign-In & Domain
// Verification) and Chapter 8b (session issuance, auto-registration, role
// resolution).
type AuthService interface {
	// AuthCodeURL builds the Google consent redirect URL for the given
	// CSRF state value (spec AC1.2).
	AuthCodeURL(state string) string
	// VerifyGoogleCallback exchanges the authorization code, verifies the
	// resulting ID token, and enforces the domain restriction — returns
	// ErrGoogleVerificationFailed or ErrDomainNotAllowed on rejection,
	// never a partially-trusted claims value.
	VerifyGoogleCallback(ctx context.Context, code string) (*auth_dto.GoogleClaims, error)
	// GetOrCreateUser implements spec AC1.4: looks the email up first and,
	// only if truly new, delegates to registrationService.Register (reused
	// as-is, including its thank-you DM side effect — spec Notes C2).
	// Returns ErrUserDeactivated for an existing-but-deactivated account.
	GetOrCreateUser(ctx context.Context, email, name string) (*registration_dto.User, error)
	// IssueSession creates a new tr_usersession row and returns the opaque
	// token to set as the session cookie (spec AC1.5).
	IssueSession(ctx context.Context, userID int64) (string, error)
	// FindSession resolves a session token to its user, joined with
	// flagActive — used by AuthMiddleware on every request (spec AC1.6/1.7).
	FindSession(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error)
	// Logout deletes a session row (spec AC1.9) — deleting an
	// already-gone/unknown session is not an error (idempotent).
	Logout(ctx context.Context, sessionID string) error
}
