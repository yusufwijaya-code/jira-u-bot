package auth_handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_service"
	authtemplates "jira-u-bot/modules/auth/templates"
	"jira-u-bot/modules/registration/registration_service"
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

type loginPageData struct {
	BasePath string
	Error    string
}

type authHandlerImpl struct {
	authService   auth_service.AuthService
	basePath      string
	loginTemplate *template.Template
}

func NewAuthHandler(authService auth_service.AuthService, cfg *config.AppConfig) AuthHandler {
	loginTmpl := template.Must(template.ParseFS(authtemplates.FS, "login.html"))
	return &authHandlerImpl{authService: authService, basePath: cfg.BasePath, loginTemplate: loginTmpl}
}

// cookiePath scopes every cookie this handler sets to cfg.BasePath instead
// of domain root "/" — labcup.tech hosts several repos behind one vhost,
// each under its own prefix; a root-scoped cookie would be sent to every
// other app's backend too and, worse, a same-named cookie set by another
// app at "/" would silently clobber this one in the browser's cookie jar.
func (h *authHandlerImpl) cookiePath() string {
	return h.basePath
}

// LoginPage shows the app's own landing page — no cookies, no redirect, no
// contact with Google. This is what unauthenticated visitors should land
// on first (AuthMiddleware points here), so they see the app before being
// sent into Google's consent flow by clicking the button themselves.
func (h *authHandlerImpl) LoginPage(c *gin.Context) {
	h.renderLoginPage(c, "")
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
		h.renderLoginPage(c, "Sesi login tidak valid atau kedaluwarsa, coba lagi")
		return
	}

	ctx := c.Request.Context()
	claims, err := h.authService.VerifyGoogleCallback(ctx, c.Query("code"))
	if err != nil {
		if errors.Is(err, auth_service.ErrDomainNotAllowed) {
			h.renderLoginPage(c, "Login hanya untuk email @esb.co.id yang terverifikasi Google")
			return
		}
		log.Printf("auth_handler: VerifyGoogleCallback failed: %v", err)
		h.renderLoginPage(c, "Gagal login dengan Google, coba lagi sebentar lagi")
		return
	}

	user, err := h.authService.GetOrCreateUser(ctx, claims.Email, claims.Name)
	if err != nil {
		switch {
		case errors.Is(err, auth_service.ErrUserDeactivated):
			h.renderLoginPage(c, "Akun kamu sudah dinonaktifkan — hubungi admin")
		case errors.Is(err, registration_service.ErrJiraAccountNotFound):
			h.renderLoginPage(c, "Email ini belum terdaftar sebagai akun Jira esb-id")
		case errors.Is(err, registration_service.ErrJiraLookupUnavailable):
			h.renderLoginPage(c, "Gagal memverifikasi akun ke Jira, coba lagi sebentar lagi")
		default:
			h.renderLoginPage(c, "Gagal memproses akun, coba lagi")
		}
		return
	}

	token, err := h.authService.IssueSession(ctx, user.UserID)
	if err != nil {
		h.renderLoginPage(c, "Gagal membuat sesi, coba lagi")
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, sessionCookieMaxAge, h.cookiePath(), "", true, true)
	c.Redirect(http.StatusFound, h.basePath+"/dashboard")
}

func (h *authHandlerImpl) Logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookieName); err == nil && token != "" {
		h.authService.Logout(c.Request.Context(), token)
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, h.cookiePath(), "", true, true)
	c.Redirect(http.StatusFound, h.basePath+"/auth/login")
}

func (h *authHandlerImpl) renderLoginPage(c *gin.Context, message string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	h.loginTemplate.Execute(c.Writer, loginPageData{BasePath: h.basePath, Error: message})
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
