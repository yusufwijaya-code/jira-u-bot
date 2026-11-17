package auth_service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"

	"nudge/config"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_session_repository"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/registration/registration_service"
)

// sessionLifetime is how long a session stays valid after login (spec
// Database Changes) — 3 days, so an idle login doesn't linger a whole month.
const sessionLifetime = 3 * 24 * time.Hour

// signInScopes is the minimum OIDC scope set needed to get email + name
// claims on the ID token — this is a separate OAuth Client (type "Web
// application") from the one chat_oauth_service uses (type "Desktop app",
// scoped to Chat API access) even though both live in the same GCP project
// (spec Open Question #5).
var signInScopes = []string{"openid", "email", "profile"}

// GoogleVerifier abstracts the actual network/cryptographic calls to
// Google (code exchange + ID token verification) so authServiceImpl's
// orchestration (domain check, error mapping) can be unit tested without
// hitting real Google endpoints — mirrors how JiraClientService is faked in
// other services' tests, since idtoken.Validate itself (real signature
// verification against Google's public certs) isn't something a fake HTTP
// server can stand in for.
type GoogleVerifier interface {
	// Exchange trades an authorization code for the raw (still-encoded) ID
	// token string.
	Exchange(ctx context.Context, code string) (rawIDToken string, err error)
	// Verify validates a raw ID token's signature, expiry, and audience,
	// returning its claims.
	Verify(ctx context.Context, rawIDToken string) (*auth_dto.GoogleClaims, error)
}

type authServiceImpl struct {
	oauthConfig      *oauth2.Config
	verifier         GoogleVerifier
	registrationSvc  registration_service.RegistrationService
	registrationRepo registration_repository.RegistrationRepository
	sessionRepo      auth_session_repository.AuthSessionRepository
}

// NewAuthService wires the real Google verifier (network calls) — tests
// construct authServiceImpl directly with fakes instead of calling this.
func NewAuthService(
	cfg *config.AppConfig,
	registrationSvc registration_service.RegistrationService,
	registrationRepo registration_repository.RegistrationRepository,
	sessionRepo auth_session_repository.AuthSessionRepository,
) AuthService {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleSignInClientID,
		ClientSecret: cfg.GoogleSignInClientSecret,
		RedirectURL:  cfg.GoogleSignInRedirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       signInScopes,
	}
	return &authServiceImpl{
		oauthConfig:      oauthConfig,
		verifier:         &googleVerifierImpl{oauthConfig: oauthConfig},
		registrationSvc:  registrationSvc,
		registrationRepo: registrationRepo,
		sessionRepo:      sessionRepo,
	}
}

func (s *authServiceImpl) AuthCodeURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state)
}

func (s *authServiceImpl) VerifyGoogleCallback(ctx context.Context, code string) (*auth_dto.GoogleClaims, error) {
	rawIDToken, err := s.verifier.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGoogleVerificationFailed, err)
	}

	claims, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGoogleVerificationFailed, err)
	}

	if !auth_dto.IsAllowedDomain(*claims) {
		return nil, ErrDomainNotAllowed
	}

	return claims, nil
}

func (s *authServiceImpl) GetOrCreateUser(ctx context.Context, email, name string) (*registration_dto.User, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if !found {
		if err := s.registrationSvc.Register(ctx, name, email); err != nil {
			return nil, err
		}
		user, found, err = s.registrationRepo.FindUserByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("auth_service: user %s not found immediately after registration", email)
		}
	}

	if !user.FlagActive {
		return nil, ErrUserDeactivated
	}
	return user, nil
}

func (s *authServiceImpl) IssueSession(ctx context.Context, userID int64) (string, error) {
	sessionID, err := newSessionToken()
	if err != nil {
		return "", err
	}
	if err := s.sessionRepo.Create(ctx, sessionID, userID, time.Now().Add(sessionLifetime)); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *authServiceImpl) FindSession(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error) {
	return s.sessionRepo.FindByID(ctx, sessionID)
}

func (s *authServiceImpl) Logout(ctx context.Context, sessionID string) error {
	return s.sessionRepo.Delete(ctx, sessionID)
}

// newSessionToken generates the opaque, unguessable session identifier
// stored as the session cookie's value (spec Database Changes) — 32 random
// bytes, base64url-encoded (no padding, cookie/URL-safe without escaping).
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// googleVerifierImpl is the real adapter — not unit tested directly
// (same precedent as chat_oauth_service: live-tested per the Deployment
// Checklist rather than mocked, since idtoken.Validate's signature check
// against Google's public certs can't be faithfully faked).
type googleVerifierImpl struct {
	oauthConfig *oauth2.Config
}

func (g *googleVerifierImpl) Exchange(ctx context.Context, code string) (string, error) {
	token, err := g.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "", fmt.Errorf("token response did not include an id_token")
	}
	return rawIDToken, nil
}

func (g *googleVerifierImpl) Verify(ctx context.Context, rawIDToken string) (*auth_dto.GoogleClaims, error) {
	payload, err := idtoken.Validate(ctx, rawIDToken, g.oauthConfig.ClientID)
	if err != nil {
		return nil, err
	}

	email, _ := payload.Claims["email"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	return &auth_dto.GoogleClaims{
		Email:         email,
		EmailVerified: emailVerified,
		Name:          name,
		Picture:       picture,
	}, nil
}
