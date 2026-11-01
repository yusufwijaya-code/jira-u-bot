package auth_dto

import (
	"strings"
	"time"
)

// Session is a tr_usersession row joined with the owning user's email and
// flagActive — this is exactly what AuthMiddleware needs per request
// without a second query (Tech Spec "Dashboard Jira Manage", Chapter 8b).
type Session struct {
	SessionID  string
	UserID     int64
	Email      string
	FlagActive bool
	ExpiresAt  time.Time
}

// GoogleClaims is the subset of Google's ID token claims this service
// cares about after exchange + verification (Tech Spec "Dashboard Jira
// Manage", Chapter 8a).
type GoogleClaims struct {
	Email         string
	EmailVerified bool
	Name          string
}

// allowedEmailDomain is the only domain permitted to sign in (spec AC1.3) —
// a constant, not configurable, matching the spec's explicit requirement
// that this is a hard restriction, not a setting.
const allowedEmailDomain = "@esb.co.id"

// IsAllowedDomain implements spec AC1.3/AC1.4: the email must be verified
// by Google AND end in the ESB domain. Checked by suffix on the claim's
// own email, not the OIDC "hd" (hosted domain) claim — hd isn't guaranteed
// present for an "External" consent screen project (spec Notes/P1), while
// email_verified + suffix is always available on a standard openid/email
// scope grant.
func IsAllowedDomain(claims GoogleClaims) bool {
	if !claims.EmailVerified {
		return false
	}
	return strings.HasSuffix(strings.ToLower(claims.Email), allowedEmailDomain)
}
