package auth_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_service"
	"jira-u-bot/modules/registration/registration_dto"
)

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

func TestLogin_SetsStateCookieAndRedirectsToGoogle(t *testing.T) {
	svc := &fakeAuthServiceForHandler{authCodeURL: "https://accounts.google.com/o/oauth2/auth?client_id=x"}
	handler := NewAuthHandler(svc)

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
}

func TestCallback_RejectsMissingStateCookie(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=xyz", nil)

	handler.Callback(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (inline error page)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Sesi login tidak valid") {
		t.Errorf("body = %q, want it to show the invalid-session error", w.Body.String())
	}
}

func TestCallback_RejectsStateMismatch(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "correct-state"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Sesi login tidak valid") {
		t.Errorf("status=%d body=%q, want 200 with invalid-session error (spec AC security — CSRF state mismatch)", w.Code, w.Body.String())
	}
}

func TestCallback_RejectsWrongDomain(t *testing.T) {
	svc := &fakeAuthServiceForHandler{verifyErr: auth_service.ErrDomainNotAllowed}
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "@esb.co.id") {
		t.Errorf("status=%d body=%q, want 200 mentioning the domain restriction (spec AC1.3)", w.Code, w.Body.String())
	}
}

func TestCallback_SucceedsAndIssuesSession(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims:    &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true, Name: "Dev"},
		getOrCreateUser: &registration_dto.User{UserID: 5, Email: "dev@esb.co.id", FlagActive: true},
		issuedToken:     "session-token-xyz",
	}
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("Location = %q, want /dashboard (spec AC1.5)", loc)
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
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "dinonaktifkan") {
		t.Errorf("status=%d body=%q, want 200 mentioning deactivation (spec AC1.4 flagActive check)", w.Code, w.Body.String())
	}
}

func TestLogout_ClearsCookieAndRedirects(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc)

	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-to-kill"})
	c.Request = req

	handler.Logout(c)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/auth/google/login" {
		t.Errorf("status=%d location=%q, want 302 to /auth/google/login (spec AC1.9)", w.Code, w.Header().Get("Location"))
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
