package auth_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/auth/auth_service"
	"nudge/modules/registration/registration_dto"
)

func testCfg() *config.AppConfig {
	return &config.AppConfig{BasePath: "/nudge"}
}

type fakeAuthServiceForHandler struct {
	authCodeURL string

	verifyClaims *auth_dto.GoogleClaims
	verifyErr    error

	getOrCreateUser *registration_dto.User
	getOrCreateErr  error

	issuedToken string
	issueErr    error

	logoutCalledWith string
}

func (f *fakeAuthServiceForHandler) AuthCodeURL(state string) string {
	return f.authCodeURL + "&state=" + state
}

func (f *fakeAuthServiceForHandler) VerifyGoogleCallback(ctx context.Context, code string) (*auth_dto.GoogleClaims, error) {
	return f.verifyClaims, f.verifyErr
}

func (f *fakeAuthServiceForHandler) GetOrCreateUser(ctx context.Context, email, name string) (*registration_dto.User, error) {
	return f.getOrCreateUser, f.getOrCreateErr
}

func (f *fakeAuthServiceForHandler) IssueSession(ctx context.Context, userID int64) (string, error) {
	return f.issuedToken, f.issueErr
}

func (f *fakeAuthServiceForHandler) FindSession(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error) {
	return nil, false, nil
}

func (f *fakeAuthServiceForHandler) Logout(ctx context.Context, sessionID string) error {
	f.logoutCalledWith = sessionID
	return nil
}

func newTestContext() (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return w, c
}

// loginErrorFrom asserts the callback bounced the browser back to the Angular
// login screen (302 to <BasePath>/) and returns the ?error= message it carries.
func loginErrorFrom(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 back to the login screen", w.Code)
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location is not a valid URL: %v", err)
	}
	if loc.Path != "/nudge/" {
		t.Errorf("Location path = %q, want /nudge/ (BasePath-aware)", loc.Path)
	}
	return loc.Query().Get("error")
}

func TestLogin_SetsStateCookieAndRedirectsToGoogle(t *testing.T) {
	svc := &fakeAuthServiceForHandler{authCodeURL: "https://accounts.google.com/o/oauth2/auth?client_id=x"}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/auth/google/login", nil)

	handler.Login(c)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "accounts.google.com") {
		t.Errorf("Location = %q, want it to point at Google's consent screen", location)
	}

	var stateCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == stateCookieName {
			stateCookie = ck
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected a non-empty oauth_state cookie to be set")
	}
	if !strings.Contains(location, "state="+stateCookie.Value) {
		t.Errorf("Location %q does not contain the same state as the cookie %q", location, stateCookie.Value)
	}
	if stateCookie.Path != "/nudge" {
		t.Errorf("state cookie Path = %q, want /nudge — a root-scoped cookie would collide with other apps on the same domain", stateCookie.Path)
	}
}

func TestCallback_RejectsMissingStateCookie(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=xyz", nil)

	handler.Callback(c)

	if got := loginErrorFrom(t, w); !strings.Contains(got, "Sesi login tidak valid") {
		t.Errorf("login error = %q, want it to show the invalid-session error", got)
	}
}

func TestCallback_RejectsStateMismatch(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "correct-state"})
	c.Request = req

	handler.Callback(c)

	if got := loginErrorFrom(t, w); !strings.Contains(got, "Sesi login tidak valid") {
		t.Errorf("login error = %q, want invalid-session error (spec AC security — CSRF state mismatch)", got)
	}
}

func TestCallback_RejectsWrongDomain(t *testing.T) {
	svc := &fakeAuthServiceForHandler{verifyErr: auth_service.ErrDomainNotAllowed}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if got := loginErrorFrom(t, w); !strings.Contains(got, "@esb.co.id") {
		t.Errorf("login error = %q, want it to mention the domain restriction (spec AC1.3)", got)
	}
}

func TestCallback_SucceedsAndIssuesSession(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims:    &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true, Name: "Dev"},
		getOrCreateUser: &registration_dto.User{UserID: 5, Email: "dev@esb.co.id", FlagActive: true},
		issuedToken:     "session-token-xyz",
	}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/nudge/dashboard" {
		t.Errorf("Location = %q, want /nudge/dashboard (spec AC1.5, BasePath-aware)", loc)
	}

	var sessionCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == sessionCookieName {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil || sessionCookie.Value != "session-token-xyz" {
		t.Errorf("session cookie = %v, want value %q", sessionCookie, "session-token-xyz")
	}
}

func TestCallback_RejectsDeactivatedUser(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims:   &auth_dto.GoogleClaims{Email: "off@esb.co.id", EmailVerified: true, Name: "Off"},
		getOrCreateErr: auth_service.ErrUserDeactivated,
	}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if got := loginErrorFrom(t, w); !strings.Contains(got, "dinonaktifkan") {
		t.Errorf("login error = %q, want it to mention deactivation (spec AC1.4 flagActive check)", got)
	}
}

func TestLogout_ClearsCookieAndRedirects(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, testCfg())

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-to-kill"})
	c.Request = req

	handler.Logout(c)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/nudge/" {
		t.Errorf("status=%d location=%q, want 302 to /nudge/ (spec AC1.9, BasePath-aware — the Angular login screen, not straight to Google)", w.Code, w.Header().Get("Location"))
	}
	if svc.logoutCalledWith != "session-to-kill" {
		t.Errorf("Logout called with %q, want %q", svc.logoutCalledWith, "session-to-kill")
	}

	var sessionCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == sessionCookieName {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil || sessionCookie.MaxAge >= 0 {
		t.Error("expected session cookie to be cleared (MaxAge < 0)")
	}
}

func TestMe_ReturnsEmailAndIsAdminFromSession(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, &config.AppConfig{BasePath: "/nudge", AdminEmail: "yusuf.wijaya@esb.co.id"})

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/nudge/api/auth/me", nil)
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{Email: "yusuf.wijaya@esb.co.id", FlagActive: true})

	handler.Me(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"email":"yusuf.wijaya@esb.co.id"`) || !strings.Contains(w.Body.String(), `"isAdmin":true`) {
		t.Errorf("body = %q, want email + isAdmin:true", w.Body.String())
	}
}

func TestMe_ReturnsIsAdminFalseForNonAdmin(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, &config.AppConfig{BasePath: "/nudge", AdminEmail: "yusuf.wijaya@esb.co.id"})

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/nudge/api/auth/me", nil)
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{Email: "dev@esb.co.id", FlagActive: true})

	handler.Me(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"isAdmin":false`) {
		t.Errorf("status=%d body=%q, want 200 and isAdmin:false", w.Code, w.Body.String())
	}
}

func TestCallback_RedirectsToFrontendURLWhenConfigured(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims:    &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true, Name: "Dev"},
		getOrCreateUser: &registration_dto.User{UserID: 5, Email: "dev@esb.co.id", FlagActive: true},
		issuedToken:     "session-token-xyz",
	}
	handler := NewAuthHandler(svc, &config.AppConfig{BasePath: "/nudge", FrontendURL: "http://localhost:4200"})

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if loc := w.Header().Get("Location"); loc != "http://localhost:4200/nudge/dashboard" {
		t.Errorf("Location = %q, want http://localhost:4200/nudge/dashboard", loc)
	}
}
