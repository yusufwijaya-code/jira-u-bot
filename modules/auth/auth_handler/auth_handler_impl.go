package auth_handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/auth/auth_service"
	"nudge/modules/registration/registration_service"
)

const (
	stateCookieName   = "oauth_state"
	sessionCookieName = "session_token"
	// stateCookieMaxAge needs enough slack for Google's own extra identity
	// challenges ("Verify it's you" 2FA prompts, which Google can trigger
	// mid-flow on a new/unverified OAuth app) on top of the normal consent
	// round-trip — confirmed live: a 5-minute window expired mid-challenge
	// and broke an otherwise-successful login.
	stateCookieMaxAge   = 15 * 60        // 15 minutes
	sessionCookieMaxAge = 30 * 24 * 3600 // 30 days, matches auth_service.sessionLifetime
)

type authHandlerImpl struct {
	authService auth_service.AuthService
	basePath    string
	adminEmail  string
	// appURL is the frontend origin + BasePath — where every post-login /
	// post-logout redirect lands (see config.AppConfig.FrontendURL).
	appURL string
	// onLogin runs after a successful sign-in (e.g. to store the Google
	// profile picture); nil = nothing.
	onLogin LoginHook
}

// LoginHook is told about every successful sign-in. It must not block for
// long and handles its own errors — login never fails because of it.
type LoginHook func(ctx context.Context, userID int64, claims auth_dto.GoogleClaims)

// Option configures optional AuthHandler behaviour.
type Option func(*authHandlerImpl)

// WithLoginHook runs hook after every successful sign-in.
func WithLoginHook(hook LoginHook) Option {
	return func(h *authHandlerImpl) { h.onLogin = hook }
}

func NewAuthHandler(authService auth_service.AuthService, cfg *config.AppConfig, opts ...Option) AuthHandler {
	h := &authHandlerImpl{
		authService: authService,
		basePath:    cfg.BasePath,
		adminEmail:  cfg.AdminEmail,
		appURL:      cfg.FrontendURL + cfg.BasePath,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// cookiePath scopes every cookie this handler sets to cfg.BasePath instead
// of domain root "/" — labcup.tech hosts several repos behind one vhost,
// each under its own prefix; a root-scoped cookie would be sent to every
// other app's backend too and, worse, a same-named cookie set by another
// app at "/" would silently clobber this one in the browser's cookie jar.
func (h *authHandlerImpl) cookiePath() string {
	return h.basePath
}

// Me reports the current session's email and whether it belongs to the
// admin account — consumed by the Angular frontend's session-check on app
// load (Tech Spec "Angular Dashboard Revamp", Repository 1 Chapter 1).
func (h *authHandlerImpl) Me(c *gin.Context) {
	session := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
	c.JSON(http.StatusOK, gin.H{
		"email":   session.Email,
		"isAdmin": session.Email == h.adminEmail,
	})
}

func (h *authHandlerImpl) Login(c *gin.Context) {
	state, err := randomState()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to start login")
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(stateCookieName, state, stateCookieMaxAge, h.cookiePath(), "", true, true)
	c.Redirect(http.StatusFound, h.authService.AuthCodeURL(state))
}

func (h *authHandlerImpl) Callback(c *gin.Context) {
	cookieState, cookieErr := c.Cookie(stateCookieName)
	// Clear the state cookie immediately regardless of outcome — it's
	// single-use, and leaving it around after a failed/successful attempt
	// just extends its (short) window for no benefit.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(stateCookieName, "", -1, h.cookiePath(), "", true, true)

	if cookieErr != nil || cookieState == "" || cookieState != c.Query("state") {
		h.redirectToLoginWithError(c, "Sesi login tidak valid atau kedaluwarsa, coba lagi")
		return
	}

	ctx := c.Request.Context()
	claims, err := h.authService.VerifyGoogleCallback(ctx, c.Query("code"))
	if err != nil {
		if errors.Is(err, auth_service.ErrDomainNotAllowed) {
			h.redirectToLoginWithError(c, "Login hanya untuk email @esb.co.id yang terverifikasi Google")
			return
		}
		log.Printf("auth_handler: VerifyGoogleCallback failed: %v", err)
		h.redirectToLoginWithError(c, "Gagal login dengan Google, coba lagi sebentar lagi")
		return
	}

	user, err := h.authService.GetOrCreateUser(ctx, claims.Email, claims.Name)
	if err != nil {
		switch {
		case errors.Is(err, auth_service.ErrUserDeactivated):
			h.redirectToLoginWithError(c, "Akun kamu sudah dinonaktifkan — hubungi admin")
		case errors.Is(err, registration_service.ErrJiraAccountNotFound):
			h.redirectToLoginWithError(c, "Email ini belum terdaftar sebagai akun Jira esb-id")
		case errors.Is(err, registration_service.ErrJiraLookupUnavailable):
			h.redirectToLoginWithError(c, "Gagal memverifikasi akun ke Jira, coba lagi sebentar lagi")
		default:
			h.redirectToLoginWithError(c, "Gagal memproses akun, coba lagi")
		}
		return
	}

	token, err := h.authService.IssueSession(ctx, user.UserID)
	if err != nil {
		h.redirectToLoginWithError(c, "Gagal membuat sesi, coba lagi")
		return
	}

	if h.onLogin != nil {
		h.onLogin(ctx, user.UserID, *claims)
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, sessionCookieMaxAge, h.cookiePath(), "", true, true)
	c.Redirect(http.StatusFound, h.appURL+"/dashboard")
}

func (h *authHandlerImpl) Logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookieName); err == nil && token != "" {
		h.authService.Logout(c.Request.Context(), token)
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, h.cookiePath(), "", true, true)
	c.Redirect(http.StatusFound, h.appURL+"/")
}

// redirectToLoginWithError sends the browser back to the Angular login
// screen (the app root), which shows the message from the ?error= query.
func (h *authHandlerImpl) redirectToLoginWithError(c *gin.Context, message string) {
	c.Redirect(http.StatusFound, h.appURL+"/?error="+url.QueryEscape(message))
}

// randomState generates the CSRF state parameter (spec AC1.2/security) —
// 32 random bytes, base64url-encoded, same shape/strength as the session
// token in auth_service.newSessionToken but intentionally not shared code
// (different lifetime/purpose, no benefit to coupling them).
func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
