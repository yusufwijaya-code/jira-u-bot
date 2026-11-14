package auth_middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/registration/registration_dto"
)

type fakeAuthServiceForMiddleware struct {
	session *auth_dto.Session
	found   bool
	err     error
}

func (f *fakeAuthServiceForMiddleware) AuthCodeURL(state string) string { return "" }
func (f *fakeAuthServiceForMiddleware) VerifyGoogleCallback(ctx context.Context, code string) (*auth_dto.GoogleClaims, error) {
	return nil, nil
}
func (f *fakeAuthServiceForMiddleware) GetOrCreateUser(ctx context.Context, email, name string) (*registration_dto.User, error) {
	return nil, nil
}
func (f *fakeAuthServiceForMiddleware) IssueSession(ctx context.Context, userID int64) (string, error) {
	return "", nil
}
func (f *fakeAuthServiceForMiddleware) FindSession(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error) {
	return f.session, f.found, f.err
}
func (f *fakeAuthServiceForMiddleware) Logout(ctx context.Context, sessionID string) error {
	return nil
}

func runMiddleware(t *testing.T, mw gin.HandlerFunc, req *http.Request) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	mw(c)
	return w, c
}

func testCfg() *config.AppConfig {
	return &config.AppConfig{AdminEmail: "yusuf.wijaya@esb.co.id", BasePath: "/nudge"}
}

func TestAdminMiddleware_ForbidsNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(CurrentUserKey, auth_dto.Session{Email: "someone@esb.co.id"})

	AdminMiddleware(testCfg())(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (spec AC1.8)", w.Code, http.StatusForbidden)
	}
}

func TestAdminMiddleware_FailsClosedWhenRunWithoutAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// CurrentUserKey deliberately not set — simulates AdminMiddleware being
	// mounted without AuthMiddleware running first.

	AdminMiddleware(testCfg())(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d — must fail closed, not panic", w.Code, http.StatusInternalServerError)
	}
}

func TestAdminMiddleware_AllowsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(CurrentUserKey, auth_dto.Session{Email: "yusuf.wijaya@esb.co.id"})

	AdminMiddleware(testCfg())(c)

	if c.IsAborted() {
		t.Error("expected the chain to continue for the admin email")
	}
}

func TestAuthMiddlewareAPI_Returns401JSONWhenNoCookie(t *testing.T) {
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{}, testCfg())
	w, c := runMiddleware(t, mw, httptest.NewRequest("GET", "/nudge/api/dashboard/settings", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want JSON error body, not a redirect", w.Body.String())
	}
	if !c.IsAborted() {
		t.Error("expected the middleware chain to be aborted")
	}
}

func TestAuthMiddlewareAPI_Returns401JSONWhenSessionNotFound(t *testing.T) {
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{found: false}, testCfg())
	req := httptest.NewRequest("GET", "/nudge/api/dashboard/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "unknown-token"})

	w, _ := runMiddleware(t, mw, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddlewareAPI_Returns401JSONWhenUserDeactivated(t *testing.T) {
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{
		found:   true,
		session: &auth_dto.Session{SessionID: "s1", UserID: 1, Email: "off@esb.co.id", FlagActive: false, ExpiresAt: time.Now().Add(time.Hour)},
	}, testCfg())
	req := httptest.NewRequest("GET", "/nudge/api/dashboard/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "s1"})

	w, _ := runMiddleware(t, mw, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddlewareAPI_AllowsValidSession(t *testing.T) {
	session := &auth_dto.Session{SessionID: "s1", UserID: 1, Email: "dev@esb.co.id", FlagActive: true, ExpiresAt: time.Now().Add(time.Hour)}
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{found: true, session: session}, testCfg())
	req := httptest.NewRequest("GET", "/nudge/api/dashboard/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "s1"})

	_, c := runMiddleware(t, mw, req)

	got, ok := c.Get(CurrentUserKey)
	if !ok {
		t.Fatal("expected CurrentUserKey to be set")
	}
	if got.(auth_dto.Session).Email != "dev@esb.co.id" {
		t.Errorf("CurrentUserKey email = %q, want dev@esb.co.id", got.(auth_dto.Session).Email)
	}
}
