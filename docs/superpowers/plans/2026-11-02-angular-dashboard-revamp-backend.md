# Angular Dashboard Revamp — Backend API (Plan A of 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 6 new JSON GET endpoints to jira-u-bot's Go backend (session-check, board, team, overdue, admin user list, sprint report) so an Angular frontend can consume them — without touching or removing any existing route yet.

**Architecture:** Each endpoint is a thin wrapper around an existing, already-tested service method (`AuthService`, `UserDashboardService`, `AdminDashboardService`, `SprintReportService`) — swap `template.Execute` for `c.JSON`. A new `AuthMiddlewareAPI` variant (401 JSON instead of 302 redirect) is introduced for the `/api/dashboard/*` route group; the existing redirect-based `AuthMiddleware` is left completely untouched and keeps protecting the old HTML page routes until they're deleted in Plan C.

**Tech Stack:** Go, Gin, `go test` (table-driven, hand-written fakes — no mocking library, matching existing convention in this repo).

**Spec:** `docs/tech-spec/angular-dashboard-revamp/Tech Spec - Angular Dashboard Revamp.md` — this plan implements Repository 1, Chapters 1-6 only (NOT Chapter 7a/7b — those are Plan C, executed last, after this plan AND the frontend plan are both done).

## Global Constraints

- No schema changes — every endpoint calls an existing service method unchanged (see spec Database Changes).
- Existing POST/DELETE endpoints (`/api/dashboard/preference`, `/api/dashboard/tickets/:issueKey/transition`, `/api/dashboard/credential`, all `/dashboard/admin/*`) are **not touched** in this plan — no behavior change to their handlers.
- The existing HTML page routes (`/dashboard`, `/dashboard/team`, `/dashboard/overdue`, `/dashboard/admin`, `/dashboard/report`) and their templates **stay exactly as they are** — they are only removed in Plan C, once Angular fully replaces them.
- `gofmt -w` every file touched before committing (project convention, confirmed in CLAUDE.md-adjacent session history).
- Run `go build ./...`, `go vet ./...`, `go test ./...` after every task — zero tolerance for regressions, matching this project's established discipline.
- Error response shape for all new endpoints: `{"error": "<message>"}`, matching every existing JSON handler in this codebase.

---

### Task 1: `AuthMiddlewareAPI` — JSON 401 variant + `GET /api/auth/me`

**Files:**
- Modify: `modules/auth/auth_middleware/auth_middleware.go`
- Modify: `modules/auth/auth_middleware/auth_middleware_test.go`
- Modify: `modules/auth/auth_handler/auth_handler.go`
- Modify: `modules/auth/auth_handler/auth_handler_impl.go`
- Modify: `modules/auth/auth_handler/auth_handler_test.go`
- Modify: `router.go`

**Interfaces:**
- Produces: `auth_middleware.AuthMiddlewareAPI(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc` — same session validation as `AuthMiddleware`, but responds `401 {"error": "unauthorized"}` + `c.Abort()` instead of redirecting. Sets `auth_middleware.CurrentUserKey` in context on success, identical to `AuthMiddleware`.
- Produces: `AuthHandler.Me(c *gin.Context)` — reads `auth_middleware.CurrentUserKey` from context (set by `AuthMiddlewareAPI`), responds `200 {"email": string, "isAdmin": bool}`.
- Consumes: `auth_dto.Session` (existing, `modules/auth/auth_dto/auth_dto.go`) — fields `Email string`, `FlagActive bool`.
- Consumes: `config.AppConfig.AdminEmail string` (existing).

- [ ] **Step 1: Write the failing test for `AuthMiddlewareAPI`**

Add to `modules/auth/auth_middleware/auth_middleware_test.go` (same file already has `fakeAuthServiceForMiddleware` and `testCfg()` — reuse both):

```go
func TestAuthMiddlewareAPI_Returns401JSONWhenNoCookie(t *testing.T) {
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{}, testCfg())
	w, c := runMiddleware(t, mw, httptest.NewRequest("GET", "/jira-notif/api/dashboard/board", nil))

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
	req := httptest.NewRequest("GET", "/jira-notif/api/dashboard/board", nil)
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
	req := httptest.NewRequest("GET", "/jira-notif/api/dashboard/board", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "s1"})

	w, _ := runMiddleware(t, mw, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddlewareAPI_AllowsValidSession(t *testing.T) {
	session := &auth_dto.Session{SessionID: "s1", UserID: 1, Email: "dev@esb.co.id", FlagActive: true, ExpiresAt: time.Now().Add(time.Hour)}
	mw := AuthMiddlewareAPI(&fakeAuthServiceForMiddleware{found: true, session: session}, testCfg())
	req := httptest.NewRequest("GET", "/jira-notif/api/dashboard/board", nil)
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
```

Check the top of the file already imports `"strings"` — if not, add it to the import block.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/auth/auth_middleware/... -run TestAuthMiddlewareAPI -v`
Expected: FAIL with `undefined: AuthMiddlewareAPI`

- [ ] **Step 3: Implement `AuthMiddlewareAPI` by extracting shared logic from `AuthMiddleware`**

Replace the full contents of `modules/auth/auth_middleware/auth_middleware.go` with:

```go
package auth_middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_service"
)

// CurrentUserKey is the gin.Context key AuthMiddleware/AuthMiddlewareAPI
// stores the resolved auth_dto.Session under — handlers read it via
// c.MustGet(CurrentUserKey).
const CurrentUserKey = "currentUser"

const sessionCookieName = "session_token"

// AuthMiddleware implements spec AC1.6/AC1.7: every request under the old
// server-rendered page routes must have a valid, non-expired session for a
// flagActive user — resolved fresh from the DB every request (not cached in
// the cookie). Unauthenticated requests are redirected to the app's own
// landing page. This variant stays in place only for as long as the old
// HTML page routes exist — once they're deleted (Angular Dashboard Revamp
// Plan C), only AuthMiddlewareAPI remains.
func AuthMiddleware(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc {
	loginURL := cfg.BasePath + "/auth/login"
	return authMiddleware(authService, func(c *gin.Context) {
		c.Redirect(http.StatusFound, loginURL)
	})
}

// AuthMiddlewareAPI is the JSON-API counterpart to AuthMiddleware — used
// for routes consumed by fetch()/HttpClient (Angular), where a 302 redirect
// to an HTML page is useless (the caller can't meaningfully follow it and
// the response body wouldn't be JSON). Same session validation, same
// CurrentUserKey contract, only the failure response differs: 401 JSON
// instead of a redirect.
func AuthMiddlewareAPI(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc {
	return authMiddleware(authService, func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})
}

func authMiddleware(authService auth_service.AuthService, onUnauthorized gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			onUnauthorized(c)
			c.Abort()
			return
		}

		session, found, err := authService.FindSession(c.Request.Context(), token)
		if err != nil || !found || !session.FlagActive {
			onUnauthorized(c)
			c.Abort()
			return
		}

		c.Set(CurrentUserKey, *session)
		c.Next()
	}
}

// AdminMiddleware implements spec AC1.8 — must run AFTER AuthMiddleware or
// AuthMiddlewareAPI (reads CurrentUserKey either one sets). Role is
// re-checked by email comparison every request, never cached.
func AdminMiddleware(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := c.Get(CurrentUserKey)
		if !ok {
			// Misconfiguration (mounted without an auth middleware first) —
			// fail closed with 500 rather than panic through gin.Recovery.
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if session.(auth_dto.Session).Email != cfg.AdminEmail {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
```

Note: `AuthMiddleware`'s redirect target changed from the earlier `/auth/google/login` straight-to-Google redirect to `/auth/login` (the app's own landing page) in an earlier session — this plan preserves that, it's unrelated to this task.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./modules/auth/auth_middleware/... -v`
Expected: PASS — all `TestAuthMiddlewareAPI_*` tests pass, AND every pre-existing `TestAuthMiddleware_*`/`TestAdminMiddleware_*` test still passes unmodified (confirms the refactor didn't change `AuthMiddleware`'s own behavior).

- [ ] **Step 5: Write the failing test for `AuthHandler.Me`**

Add to `modules/auth/auth_handler/auth_handler_test.go` (the file already has `testCfg()`, `newTestContext()`, and `fakeAuthServiceForHandler`):

```go
func TestMe_ReturnsEmailAndIsAdminFromSession(t *testing.T) {
	svc := &fakeAuthServiceForHandler{}
	handler := NewAuthHandler(svc, &config.AppConfig{BasePath: "/jira-notif", AdminEmail: "yusuf.wijaya@esb.co.id"})

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/jira-notif/api/auth/me", nil)
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
	handler := NewAuthHandler(svc, &config.AppConfig{BasePath: "/jira-notif", AdminEmail: "yusuf.wijaya@esb.co.id"})

	w, c := newTestContext()
	c.Request = httptest.NewRequest("GET", "/jira-notif/api/auth/me", nil)
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{Email: "dev@esb.co.id", FlagActive: true})

	handler.Me(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"isAdmin":false`) {
		t.Errorf("status=%d body=%q, want 200 and isAdmin:false", w.Code, w.Body.String())
	}
}
```

Add `"jira-u-bot/modules/auth/auth_middleware"` to the test file's import block if not already present (check first — it likely isn't, since this test file currently constructs handlers directly without going through the middleware).

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test ./modules/auth/auth_handler/... -run TestMe -v`
Expected: FAIL with `handler.Me undefined`

- [ ] **Step 7: Add `Me` to the `AuthHandler` interface**

In `modules/auth/auth_handler/auth_handler.go`, add one line to the interface:

```go
package auth_handler

import "github.com/gin-gonic/gin"

type AuthHandler interface {
	// Me returns the current session's email and admin status as JSON —
	// used by the Angular frontend to decide whether to show the Login
	// Screen or the Dashboard Shell. Mounted behind AuthMiddlewareAPI.
	Me(c *gin.Context)
	// LoginPage renders the app's own landing page (Sign in with Google
	// button) without touching Google at all — distinct from Login, which
	// actually kicks off the OAuth round-trip once the button is clicked.
	LoginPage(c *gin.Context)
	Login(c *gin.Context)
	Callback(c *gin.Context)
	Logout(c *gin.Context)
}
```

- [ ] **Step 8: Implement `Me` in `auth_handler_impl.go`**

Add this method anywhere in `modules/auth/auth_handler/auth_handler_impl.go` (after `LoginPage` is a natural spot):

```go
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
```

This needs `h.adminEmail` to exist on `authHandlerImpl` and `"jira-u-bot/modules/auth/auth_middleware"` imported. Check the current struct — `auth_handler_impl.go` doesn't currently store `cfg.AdminEmail` (`authHandlerImpl` only has `authService`, `basePath`, `loginTemplate`). Add the field:

```go
type authHandlerImpl struct {
	authService   auth_service.AuthService
	basePath      string
	adminEmail    string
	loginTemplate *template.Template
}

func NewAuthHandler(authService auth_service.AuthService, cfg *config.AppConfig) AuthHandler {
	loginTmpl := template.Must(template.ParseFS(authtemplates.FS, "login.html"))
	return &authHandlerImpl{
		authService:   authService,
		basePath:      cfg.BasePath,
		adminEmail:    cfg.AdminEmail,
		loginTemplate: loginTmpl,
	}
}
```

Add the import `"jira-u-bot/modules/auth/auth_middleware"` to `auth_handler_impl.go`'s import block.

- [ ] **Step 9: Run the test to verify it passes**

Run: `go test ./modules/auth/auth_handler/... -v`
Expected: PASS — both new `TestMe_*` tests pass, all pre-existing tests in this file still pass unmodified.

- [ ] **Step 10: Wire the route in `router.go`**

In `router.go`, inside `SetupRouter`, add a new route group right after the existing `auth.Router(base, authHandler)` line:

```go
	auth.Router(base, authHandler)

	apiAuth := base.Group("/api/auth")
	apiAuth.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	apiAuth.GET("/me", authHandler.Me)

	dashboardPages := base.Group("/dashboard")
```

(The `dashboardPages := base.Group("/dashboard")` line already exists immediately after — just inserting the 3 new lines above it, not replacing anything.)

- [ ] **Step 11: Full regression**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green, no FAIL lines anywhere in the output.

- [ ] **Step 12: gofmt and commit**

```bash
gofmt -w modules/auth/auth_middleware/auth_middleware.go modules/auth/auth_middleware/auth_middleware_test.go modules/auth/auth_handler/auth_handler.go modules/auth/auth_handler/auth_handler_impl.go modules/auth/auth_handler/auth_handler_test.go router.go
go build ./... && go test ./...
git add modules/auth/auth_middleware/auth_middleware.go modules/auth/auth_middleware/auth_middleware_test.go modules/auth/auth_handler/auth_handler.go modules/auth/auth_handler/auth_handler_impl.go modules/auth/auth_handler/auth_handler_test.go router.go
git commit -m "feat: add GET /api/auth/me session-check endpoint

Introduces AuthMiddlewareAPI (401 JSON) alongside the existing
redirect-based AuthMiddleware, which stays untouched for the old HTML
page routes. The Angular frontend (Tech Spec Angular Dashboard
Revamp) will call this on app load to decide whether to show the
Login Screen or the Dashboard Shell."
```

---

### Task 2: `GET /api/dashboard/board` — My Board

**Files:**
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go`
- Modify: `modules/user_dashboard/router.go`
- Modify: `router.go`

**Interfaces:**
- Consumes: `UserDashboardService.GetMyBoard(ctx, email) (user_dashboard_dto.Board, error)` (existing, unchanged)
- Consumes: `UserDashboardService.GetReminderTierPreference(ctx, email) (*string, error)` (existing, unchanged)
- Consumes: `JiraCredentialService.MaskedPreview(ctx, userID) (string, bool, error)` (existing, unchanged)
- Consumes: `tierOptions` (existing package-level var in `user_dashboard_handler_impl.go`) — the 6 fixed tier code/label pairs
- Produces: `UserDashboardHandler.GetBoard(c *gin.Context)` — `200 {"columns": [...], "tierPreference": {"tiers": [...]}, "credential": {"hasCredential": bool, "maskedPreview": string}}`

- [ ] **Step 1: Write the failing test**

Add to `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go` (the file already has `fakeUserDashboardService`, `fakeJiraCredentialForHandler`, `newTestContextWithUser`, `testCfg`):

```go
func TestGetBoard_ReturnsBoardPreferenceAndCredentialAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{board: user_dashboard_dto.Board{Columns: []user_dashboard_dto.BoardColumn{
		{Status: "Code Process", Cards: []user_dashboard_dto.Card{
			{IssueKey: "OMS-1", Summary: "Test ticket", Status: "Code Process", HasBudgetDetail: true, StoryPoints: 3, ElapsedLabel: "1 jam", RemainingLabel: "2 jam"},
		}},
	}}}
	cred := &fakeJiraCredentialForHandler{maskedPreview: "••••ab3f", maskedFound: true}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"issueKey":"OMS-1"`) {
		t.Errorf("body missing board ticket: %s", body)
	}
	if !strings.Contains(body, `"tiers":[`) {
		t.Errorf("body missing tierPreference.tiers: %s", body)
	}
	if !strings.Contains(body, `"hasCredential":true`) || !strings.Contains(body, `"maskedPreview":"••••ab3f"`) {
		t.Errorf("body missing credential info: %s", body)
	}
}

func TestGetBoard_DefaultsAllTiersWhenPreferenceNeverSet(t *testing.T) {
	svc := &fakeUserDashboardService{getPreference: nil}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	for _, code := range []string{"2h", "1h", "30m", "15m", "5m", "1m"} {
		if !strings.Contains(w.Body.String(), `"`+code+`"`) {
			t.Errorf("body missing default tier %q: %s", code, w.Body.String())
		}
	}
}

func TestGetBoard_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{boardErr: errors.New("jira down")}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

Add `"errors"` to the test file's import block if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -run TestGetBoard -v`
Expected: FAIL with `handler.GetBoard undefined`

- [ ] **Step 3: Rename `Dashboard` to `GetBoard` in the interface**

In `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`:

```go
package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	// GetBoard returns My Board as JSON — consumed by the Angular
	// frontend's board.service.ts (Tech Spec "Angular Dashboard Revamp",
	// Repository 1 Chapter 2).
	GetBoard(c *gin.Context)
	TeamView(c *gin.Context)
	OverdueView(c *gin.Context)
	UpdatePreference(c *gin.Context)
	TransitionTicket(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
```

- [ ] **Step 4: Replace `Dashboard` with `GetBoard` in `user_dashboard_handler_impl.go`**

Replace the existing `Dashboard` method (and its now-unused `dashboardTemplate` field/usage) with:

```go
func (h *userDashboardHandlerImpl) GetBoard(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()

	board, err := h.dashboardSvc.GetMyBoard(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
		return
	}

	savedPreference, err := h.dashboardSvc.GetReminderTierPreference(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
		return
	}
	// nil (never set) means every tier is checked by default (spec AC5.2);
	// a non-nil CSV (possibly "") means only the listed codes are checked.
	preferredSet := map[string]bool{}
	if savedPreference != nil {
		for _, code := range strings.Split(*savedPreference, ",") {
			preferredSet[code] = true
		}
	}
	tiers := make([]string, 0, len(tierOptions))
	for _, opt := range tierOptions {
		if savedPreference == nil || preferredSet[opt.Code] {
			tiers = append(tiers, opt.Code)
		}
	}

	credentialMasked, hasCredential, _ := h.jiraCredential.MaskedPreview(ctx, user.UserID)

	c.JSON(http.StatusOK, gin.H{
		"columns": board.Columns,
		"tierPreference": gin.H{
			"tiers": tiers,
		},
		"credential": gin.H{
			"hasCredential": hasCredential,
			"maskedPreview": credentialMasked,
		},
	})
}
```

This handler no longer needs `dashboardTemplate` — remove the field from `userDashboardHandlerImpl` and its initialization in `NewUserDashboardHandler` (leave `teamTemplate`/`overdueTemplate` alone, those are still used by `TeamView`/`OverdueView` until Task 3/Task 4 of this plan replace them too). Remove the now-unused `dashboardPageData`, `tierCheckbox` types and the `tierCheckbox` struct literal construction — but **keep** the `tierOptions` package-level var, it's still needed by the new `GetBoard` above.

Check: after this edit, is `"html/template"` still used elsewhere in this file (yes — `teamTemplate`/`overdueTemplate` still reference `*template.Template`)? Keep the import. Is `userdashboardtemplates "jira-u-bot/modules/user_dashboard/templates"` still used (yes, for `teamTemplate`/`overdueTemplate` parsing)? Keep it too. Only `dashboardPageData` and the `dashboard.html` parse call (`template.Must(template.ParseFS(userdashboardtemplates.FS, "dashboard.html"))`) should be removed from `NewUserDashboardHandler`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -v`
Expected: PASS — all 3 new tests pass, and `TestDashboard_RendersBoardAndPreference` (the old test for the now-deleted `Dashboard` method) fails to compile since it calls `handler.Dashboard(c)` — **delete that old test** (`TestDashboard_RendersBoardAndPreference`) from the test file as part of this step, since the method it tests no longer exists. Re-run and confirm PASS.

- [ ] **Step 6: Update the module router**

In `modules/user_dashboard/router.go`, change:

```go
func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler user_dashboard_handler.UserDashboardHandler) {
	dashboard.GET("/team", handler.TeamView)
	dashboard.GET("/overdue", handler.OverdueView)

	api.GET("/board", handler.GetBoard)
	api.POST("/preference", handler.UpdatePreference)
	api.POST("/tickets/:issueKey/transition", handler.TransitionTicket)
	api.POST("/credential", handler.SaveCredential)
	api.DELETE("/credential", handler.DeleteCredential)
}
```

(Removed `dashboard.GET("", handler.Dashboard)` — the old HTML board route. `dashboard.GET("/team", ...)` and `dashboard.GET("/overdue", ...)` stay for now, replaced in Task 3/4. Added `api.GET("/board", handler.GetBoard)`.)

- [ ] **Step 7: Point `/api/dashboard` at the JSON-401 middleware**

In `router.go`, find:

```go
	dashboardAPI := base.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddleware(authSvc, cfg))
```

Change to:

```go
	dashboardAPI := base.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
```

This affects every route already mounted on `dashboardAPI` (`preference`, `transition`, `credential`) too — from this point on, hitting any of them without a valid session returns `401 {"error":"unauthorized"}` instead of a redirect. This is intentional (see spec Background): Angular calls all of these via `fetch()`, so they all need the same JSON-401 contract, not just the brand-new `GET /api/dashboard/board`.

- [ ] **Step 8: Full regression**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 9: gofmt and commit**

```bash
gofmt -w modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go router.go
go build ./... && go test ./...
git add modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go router.go
git commit -m "feat: add GET /api/dashboard/board JSON endpoint for My Board

Replaces the HTML-rendering Dashboard handler with GetBoard, which
returns the same data (board columns, tier preference, credential
status) as JSON for the upcoming Angular frontend. Also switches
/api/dashboard/* to the JSON-401 auth middleware — the existing
preference/transition/credential POST/DELETE endpoints are unchanged
in behavior otherwise."
```

---

### Task 3: `GET /api/dashboard/team` — Team View

**Files:**
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go`
- Modify: `modules/user_dashboard/router.go`

**Interfaces:**
- Consumes: `UserDashboardService.GetTeamView(ctx) (user_dashboard_dto.TeamView, error)` (existing, unchanged)
- Produces: `UserDashboardHandler.GetTeam(c *gin.Context)` — `200 {"groups": [...]}`

- [ ] **Step 1: Write the failing test**

Add to `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go`:

```go
func TestGetTeam_ReturnsGroupsAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{teamView: user_dashboard_dto.TeamView{Groups: []user_dashboard_dto.TeamGroup{
		{AssigneeEmail: "teammate@esb.co.id", Cards: []user_dashboard_dto.Card{{IssueKey: "OMS-1", Summary: "Test"}}},
	}}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/team", "")
	handler.GetTeam(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"assigneeEmail":"teammate@esb.co.id"`) {
		t.Errorf("body missing group: %s", w.Body.String())
	}
}

func TestGetTeam_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{}
	svc.teamViewErr = errors.New("db down")
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/team", "")
	handler.GetTeam(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

Check `fakeUserDashboardService` in the test file — it currently has `GetTeamView` returning `(f.teamView, nil)` with no error field. Add a `teamViewErr error` field to the struct and change the method to `return f.teamView, f.teamViewErr`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -run TestGetTeam -v`
Expected: FAIL (compile error: `handler.GetTeam undefined`, `f.teamViewErr undefined`)

- [ ] **Step 3: Rename `TeamView` to `GetTeam` in the interface**

In `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`, change `TeamView(c *gin.Context)` to:

```go
	// GetTeam returns the Team view as JSON (Tech Spec "Angular Dashboard
	// Revamp", Repository 1 Chapter 3).
	GetTeam(c *gin.Context)
```

- [ ] **Step 4: Replace `TeamView` with `GetTeam` in `user_dashboard_handler_impl.go`**

```go
func (h *userDashboardHandlerImpl) GetTeam(c *gin.Context) {
	view, err := h.dashboardSvc.GetTeamView(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load team data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": view.Groups})
}
```

Remove the now-unused `teamTemplate` field and its `TeamView` usage — but check first whether `teamTemplate` is used anywhere else (it isn't, once this change is made). Remove its initialization from `NewUserDashboardHandler` and the `teamPageData` type if nothing else references it.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -v`
Expected: PASS. Delete any old test that called `handler.TeamView(c)` directly if one exists (search the test file for `.TeamView(` first).

- [ ] **Step 6: Update the module router**

In `modules/user_dashboard/router.go`:

```go
func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler user_dashboard_handler.UserDashboardHandler) {
	dashboard.GET("/overdue", handler.OverdueView)

	api.GET("/board", handler.GetBoard)
	api.GET("/team", handler.GetTeam)
	api.POST("/preference", handler.UpdatePreference)
	api.POST("/tickets/:issueKey/transition", handler.TransitionTicket)
	api.POST("/credential", handler.SaveCredential)
	api.DELETE("/credential", handler.DeleteCredential)
}
```

- [ ] **Step 7: Full regression, gofmt, commit**

```bash
go build ./... && go vet ./... && go test ./...
gofmt -w modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go
go build ./... && go test ./...
git add modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go
git commit -m "feat: add GET /api/dashboard/team JSON endpoint for Team view"
```

---

### Task 4: `GET /api/dashboard/overdue` — Overdue View

**Files:**
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go`
- Modify: `modules/user_dashboard/router.go`

**Interfaces:**
- Consumes: `UserDashboardService.GetOverdueView(ctx) ([]user_dashboard_dto.OverdueItem, error)` (existing, unchanged)
- Produces: `UserDashboardHandler.GetOverdue(c *gin.Context)` — `200 {"items": [...]}`

**Global Constraint reminder:** the old HTML page routes stay exactly as they are until Plan C — this task **adds** `GetOverdue` alongside the existing `OverdueView`, it does not rename or remove it (same pattern Task 3 used for `GetTeam`/`TeamView`).

- [ ] **Step 1: Write the failing test**

```go
func TestGetOverdue_ReturnsItemsAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{overdueItems: []user_dashboard_dto.OverdueItem{
		{Card: user_dashboard_dto.Card{IssueKey: "OMS-1", Summary: "Test"}, AssigneeEmail: "teammate@esb.co.id"},
	}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/overdue", "")
	handler.GetOverdue(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"issueKey":"OMS-1"`) || !strings.Contains(w.Body.String(), `"assigneeEmail":"teammate@esb.co.id"`) {
		t.Errorf("body missing overdue item: %s", w.Body.String())
	}
}

func TestGetOverdue_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{}
	svc.overdueErr = errors.New("db down")
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/overdue", "")
	handler.GetOverdue(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

Add an `overdueErr error` field to `fakeUserDashboardService`, change `GetOverdueView` to `return f.overdueItems, f.overdueErr`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -run TestGetOverdue -v`
Expected: FAIL (compile error)

- [ ] **Step 3: Add `GetOverdue` to the interface, alongside the existing `OverdueView`**

```go
	OverdueView(c *gin.Context)
	// GetOverdue returns the Overdue view as JSON (Tech Spec "Angular
	// Dashboard Revamp", Repository 1 Chapter 4).
	GetOverdue(c *gin.Context)
```

- [ ] **Step 4: Add `GetOverdue` to `user_dashboard_handler_impl.go`, alongside the existing `OverdueView`**

```go
func (h *userDashboardHandlerImpl) GetOverdue(c *gin.Context) {
	items, err := h.dashboardSvc.GetOverdueView(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load overdue data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
```

Leave `OverdueView`, `overdueTemplate`, and `overduePageData` exactly as they are — the old HTML page route stays untouched per the plan's Global Constraints (only removed in Plan C). Do not remove any existing field, type, method, or import.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./modules/user_dashboard/user_dashboard_handler/... -v`
Expected: PASS. `OverdueView`'s own existing tests must still pass unchanged — this task only adds `GetOverdue` and its tests, nothing is deleted.

- [ ] **Step 6: Update the module router — add the new JSON route alongside the existing HTML route**

```go
func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler user_dashboard_handler.UserDashboardHandler) {
	dashboard.GET("", handler.Dashboard)
	dashboard.GET("/team", handler.TeamView)
	dashboard.GET("/overdue", handler.OverdueView)

	api.GET("/board", handler.GetBoard)
	api.GET("/team", handler.GetTeam)
	api.GET("/overdue", handler.GetOverdue)
	api.POST("/preference", handler.UpdatePreference)
	api.POST("/tickets/:issueKey/transition", handler.TransitionTicket)
	api.POST("/credential", handler.SaveCredential)
	api.DELETE("/credential", handler.DeleteCredential)
}
```

This only adds `api.GET("/overdue", handler.GetOverdue)` — every other line (including `dashboard.GET("/overdue", handler.OverdueView)`) must be present exactly as it already is in the current file; do not remove or rename anything.

- [ ] **Step 7: Full regression, gofmt, commit**

```bash
go build ./... && go vet ./... && go test ./...
gofmt -w modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go
go build ./... && go test ./...
git add modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_test.go modules/user_dashboard/router.go
git commit -m "feat: add GET /api/dashboard/overdue JSON endpoint for Overdue view

All 3 user_dashboard JSON view endpoints (board, team, overdue) are
now in place, alongside their existing HTML-rendering counterparts
(Dashboard, TeamView, OverdueView), which stay untouched until Plan C."
```

---

### Task 5: `GET /api/dashboard/admin/users` — Admin User List

**Files:**
- Modify: `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler.go`
- Modify: `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_impl.go`
- Modify: `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_test.go`
- Modify: `modules/admin_dashboard/router.go`
- Modify: `router.go`

**Interfaces:**
- Consumes: `AdminDashboardService.ListUsers(ctx) ([]admin_dashboard_dto.UserRow, error)` (existing, unchanged)
- Produces: `AdminDashboardHandler.ListUsers(c *gin.Context)` — `200 {"users": [...]}` (note: this is the **handler** method, distinct from the already-existing **service** method of the same name in a different package — no ambiguity in Go since they're always called as `h.svc.ListUsers(...)` vs `handler.ListUsers(...)`)

**Global Constraint reminder:** the old HTML admin page (`GET /jira-notif/dashboard/admin`, served by the existing `UserList` method) stays exactly as it is until Plan C — this task **adds** `ListUsers` alongside it, it does not rename or remove `UserList`.

- [ ] **Step 1: Write the failing test**

Add to `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_test.go`:

```go
func TestListUsers_ReturnsUsersAsJSON(t *testing.T) {
	svc := &fakeAdminDashboardService{users: []admin_dashboard_dto.UserRow{
		{UserID: 2, Email: "dev@esb.co.id", FlagActive: true, HasCredential: true, CredentialMasked: "••••ab3f"},
	}}
	handler := NewAdminDashboardHandler(svc, testCfg())

	w, c := newAdminTestContext("GET", "/api/dashboard/admin/users", "", nil)
	handler.ListUsers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"email":"dev@esb.co.id"`) || !strings.Contains(w.Body.String(), `"credentialMasked":"••••ab3f"`) {
		t.Errorf("body missing user row: %s", w.Body.String())
	}
}
```

The existing `fakeAdminDashboardService` returns `(f.users, nil)` from `ListUsers` with no error field. Add an `err error` field to the struct and change `ListUsers` to `return f.users, f.err`, then add the error-path test (the spec's Testing table for this chapter requires it):

```go
func TestListUsers_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeAdminDashboardService{err: errors.New("db down")}
	handler := NewAdminDashboardHandler(svc, testCfg())

	w, c := newAdminTestContext("GET", "/api/dashboard/admin/users", "", nil)
	handler.ListUsers(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

Add `"errors"` to the test file's imports if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/admin_dashboard/admin_dashboard_handler/... -run TestListUsers -v`
Expected: FAIL (compile error: `handler.ListUsers undefined`, `f.err undefined`)

- [ ] **Step 3: Add `ListUsers` to the interface, alongside the existing `UserList`**

In `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler.go`:

```go
package admin_dashboard_handler

import "github.com/gin-gonic/gin"

type AdminDashboardHandler interface {
	UserList(c *gin.Context)
	// ListUsers returns the admin user list as JSON (Tech Spec "Angular
	// Dashboard Revamp", Repository 1 Chapter 5).
	ListUsers(c *gin.Context)
	ToggleActive(c *gin.Context)
	EditJiraAccountID(c *gin.Context)
	DeleteUser(c *gin.Context)
	DeleteUserCredential(c *gin.Context)
	BroadcastMessage(c *gin.Context)
}
```

- [ ] **Step 4: Add `ListUsers` to `admin_dashboard_handler_impl.go`, alongside the existing `UserList`**

```go
func (h *adminDashboardHandlerImpl) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat daftar user, coba lagi"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}
```

Leave `UserList`, the `template` field, its initialization in `NewAdminDashboardHandler`, and the `adminPageData` type exactly as they are — the old HTML admin page stays untouched per the plan's Global Constraints (only removed in Plan C). Do not remove any existing field, type, method, or import.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./modules/admin_dashboard/admin_dashboard_handler/... -v`
Expected: PASS. `TestUserList_RendersUsers` (and any other existing test calling `handler.UserList(c)`) must still pass unchanged — this task only adds `ListUsers` and its tests, nothing is deleted.

- [ ] **Step 6: Update the module router to accept two groups, keeping the existing HTML route**

Replace the full contents of `modules/admin_dashboard/router.go`:

```go
package admin_dashboard

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/admin_dashboard/admin_dashboard_handler"
)

// Router mounts the Admin Dashboard's HTML page and JSON actions on
// actionGroup (unchanged paths under the old /dashboard/admin/* prefix)
// and the new JSON user list on apiGroup (new GET /api/dashboard/admin/users
// path, Tech Spec "Angular Dashboard Revamp", Repository 1 Chapter 5) — the
// caller is responsible for applying AuthMiddleware/AuthMiddlewareAPI AND
// AdminMiddleware to both groups before passing them here, same division
// of responsibility as before.
func Router(actionGroup *gin.RouterGroup, apiGroup *gin.RouterGroup, handler admin_dashboard_handler.AdminDashboardHandler) {
	actionGroup.GET("", handler.UserList)
	actionGroup.POST("/users/:userID/active", handler.ToggleActive)
	actionGroup.POST("/users/:userID/jira-account-id", handler.EditJiraAccountID)
	actionGroup.DELETE("/users/:userID/credential", handler.DeleteUserCredential)
	actionGroup.DELETE("/users/:userID", handler.DeleteUser)
	actionGroup.POST("/broadcast", handler.BroadcastMessage)

	apiGroup.GET("/users", handler.ListUsers)
}
```

This deliberately keeps the existing HTML route (`actionGroup.GET("", handler.UserList)`) and the 4 existing POST/DELETE routes on their current path prefix (`/dashboard/admin/*`) and only adds the new `GET /users` on a separate API-prefixed group — per the spec's Open Question #2 resolution (do not move the existing admin action endpoints, only add the new one under `/api/`).

- [ ] **Step 7: Update the call site in `router.go`**

Find:

```go
	dashboardAdmin := dashboardPages.Group("/admin")
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))
	admin_dashboard.Router(dashboardAdmin, adminDashboardHandler)
```

Replace with:

```go
	dashboardAdmin := dashboardPages.Group("/admin")
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	apiDashboardAdmin := dashboardAPI.Group("/admin")
	apiDashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	admin_dashboard.Router(dashboardAdmin, apiDashboardAdmin, adminDashboardHandler)
```

`dashboardAdmin` (old page group, under `dashboardPages` which carries the redirect-based `AuthMiddleware`) keeps the 4 action routes with **zero path or behavior change**. `apiDashboardAdmin` (new, under `dashboardAPI` which carries `AuthMiddlewareAPI` as of Task 2 Step 7) carries only the new `GET /users`. Both get `AdminMiddleware` on top, exactly as before.

- [ ] **Step 8: Full regression, gofmt, commit**

```bash
go build ./... && go vet ./... && go test ./...
gofmt -w modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler.go modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_impl.go modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_test.go modules/admin_dashboard/router.go router.go
go build ./... && go test ./...
git add modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler.go modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_impl.go modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler_test.go modules/admin_dashboard/router.go router.go
git commit -m "feat: add GET /api/dashboard/admin/users JSON endpoint

The 4 existing admin action endpoints (toggle active, edit Jira
account ID, delete credential, delete user, broadcast) keep their
exact current paths and auth middleware, per Tech Spec Open Question
#2 — only the user list gains a new JSON endpoint on the API group."
```

---

### Task 6: `GET /api/dashboard/report` — Sprint Report

**Files:**
- Modify: `modules/sprint_report/sprint_report_handler/sprint_report_handler.go`
- Modify: `modules/sprint_report/sprint_report_handler/sprint_report_handler_impl.go`
- Modify: `modules/sprint_report/sprint_report_handler/sprint_report_handler_test.go`
- Modify: `modules/sprint_report/router.go`
- Modify: `router.go`

**Interfaces:**
- Consumes: `SprintReportService.ListAvailableSprints(ctx) ([]string, error)` (existing, unchanged)
- Consumes: `SprintReportService.GenerateReport(ctx, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error)` (existing, unchanged)
- Produces: `SprintReportHandler.GetReport(c *gin.Context)` — `200 {"availableSprints": [...], "capPercent": number, "report": object|null}`

**Global Constraint reminder:** the old HTML page (`GET /jira-notif/dashboard/report`, served by the existing `Report` method) stays exactly as it is until Plan C — this task **adds** `GetReport` alongside it, it does not rename or remove `Report`.

- [ ] **Step 1: Write the failing test**

Add to `modules/sprint_report/sprint_report_handler/sprint_report_handler_test.go` (check this file's existing fakes first — likely `fakeSprintReportService` with `ListAvailableSprints`/`GenerateReport`):

```go
func TestGetReport_WithoutSprintParamReturnsNullReport(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42", "Sprint 43"}}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("GET", "/api/dashboard/report", nil)
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"Sprint 42"`) {
		t.Errorf("body missing availableSprints: %s", body)
	}
	if !strings.Contains(body, `"report":null`) {
		t.Errorf("body = %q, want report:null when no sprint selected", body)
	}
}

func TestGetReport_WithSprintParamGeneratesReport(t *testing.T) {
	svc := &fakeSprintReportService{
		availableSprints: []string{"Sprint 42"},
		report:           sprint_report_dto.Report{SumEstimatedHours: 8, SumActualHours: 7.5},
	}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("GET", "/api/dashboard/report?sprint=Sprint+42", nil)
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"sumEstimatedHours":8`) {
		t.Errorf("body missing generated report: %s", w.Body.String())
	}
	if svc.lastSelectedSprints == nil || svc.lastSelectedSprints[0] != "Sprint 42" {
		t.Errorf("GenerateReport called with %v, want [Sprint 42]", svc.lastSelectedSprints)
	}
}

func TestGetReport_InvalidCapFallsBackToDefault(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42"}, report: sprint_report_dto.Report{}}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("GET", "/api/dashboard/report?sprint=Sprint+42&cap=notanumber", nil)
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastCapPercent != defaultCapPercent {
		t.Errorf("GenerateReport called with cap=%v, want default %v", svc.lastCapPercent, defaultCapPercent)
	}
}

func TestGetReport_ListAvailableSprintsErrorReturns500(t *testing.T) {
	svc := &fakeSprintReportService{listErr: errors.New("jira down")}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("GET", "/api/dashboard/report", nil)
	handler.GetReport(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetReport_GenerateReportErrorReturns500(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42"}, generateErr: errors.New("jira down")}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("GET", "/api/dashboard/report?sprint=Sprint+42", nil)
	handler.GetReport(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

Check the existing `fakeSprintReportService` and whatever test-context helper this file already uses (likely `newReportTestContext` or similar, modeled on the pattern in other handler test files — if the exact helper name differs, use whatever this file already has, keeping the request/response assertions identical). Add fields as needed: `lastSelectedSprints []string`, `lastCapPercent float64`, `listErr error`, `generateErr error` — and make `GenerateReport` record its args before returning `(f.report, f.generateErr)`, `ListAvailableSprints` return `(f.availableSprints, f.listErr)`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./modules/sprint_report/sprint_report_handler/... -run TestGetReport -v`
Expected: FAIL (compile error: `handler.GetReport undefined`)

- [ ] **Step 3: Add `GetReport` to the interface, alongside the existing `Report`**

```go
package sprint_report_handler

import "github.com/gin-gonic/gin"

type SprintReportHandler interface {
	Report(c *gin.Context)
	// GetReport returns the Sprint Report page's data as JSON — available
	// sprints always included, generated report included only when at least
	// one sprint was selected via query param (Tech Spec "Angular Dashboard
	// Revamp", Repository 1 Chapter 6).
	GetReport(c *gin.Context)
}
```

- [ ] **Step 4: Add `GetReport` to `sprint_report_handler_impl.go`, alongside the existing `Report`**

```go
func (h *sprintReportHandlerImpl) GetReport(c *gin.Context) {
	ctx := c.Request.Context()

	availableSprints, err := h.svc.ListAvailableSprints(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sprint list"})
		return
	}

	selected := c.QueryArray("sprint")

	capPercent := defaultCapPercent
	if raw := c.Query("cap"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			capPercent = parsed
		}
	}

	var report interface{}
	if len(selected) > 0 {
		generated, err := h.svc.GenerateReport(ctx, selected, capPercent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
			return
		}
		report = generated
	}

	c.JSON(http.StatusOK, gin.H{
		"availableSprints": availableSprints,
		"capPercent":       capPercent,
		"report":           report,
	})
}
```

(`report` as `interface{}` left `nil` serializes to JSON `null` — matches the spec's contract exactly.) Leave `Report`, `template`, `templateFuncs`, `reportPageData`, the `h.template` field/initialization in `NewSprintReportHandler`, and all existing imports exactly as they are — the old HTML page stays untouched per the plan's Global Constraints (only removed in Plan C). Do not remove any existing field, type, method, or import.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./modules/sprint_report/sprint_report_handler/... -v`
Expected: PASS. Any existing test calling `handler.Report(c)` must still pass unchanged — this task only adds `GetReport` and its tests, nothing is deleted.

- [ ] **Step 6: Update the module router to accept two groups, keeping the existing HTML route**

```go
package sprint_report

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/sprint_report/sprint_report_handler"
)

func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler sprint_report_handler.SprintReportHandler) {
	dashboard.GET("/report", handler.Report)
	api.GET("/report", handler.GetReport)
}
```

This keeps the existing `dashboard.GET("/report", handler.Report)` HTML route exactly as it is and adds the new JSON route on a second group — same two-group shape `user_dashboard.Router` already uses.

- [ ] **Step 7: Update the call site in `router.go`**

Find:

```go
	user_dashboard.Router(dashboardPages, dashboardAPI, userDashboardHandler)
	sprint_report.Router(dashboardPages, sprintReportHandler)
```

Change to:

```go
	user_dashboard.Router(dashboardPages, dashboardAPI, userDashboardHandler)
	sprint_report.Router(dashboardPages, dashboardAPI, sprintReportHandler)
```

- [ ] **Step 8: Full regression, gofmt, commit**

```bash
go build ./... && go vet ./... && go test ./...
gofmt -w modules/sprint_report/sprint_report_handler/sprint_report_handler.go modules/sprint_report/sprint_report_handler/sprint_report_handler_impl.go modules/sprint_report/sprint_report_handler/sprint_report_handler_test.go modules/sprint_report/router.go router.go
go build ./... && go test ./...
git add modules/sprint_report/sprint_report_handler/sprint_report_handler.go modules/sprint_report/sprint_report_handler/sprint_report_handler_impl.go modules/sprint_report/sprint_report_handler/sprint_report_handler_test.go modules/sprint_report/router.go router.go
git commit -m "feat: add GET /api/dashboard/report JSON endpoint for Sprint Report

All 6 backend chapters of the Angular Dashboard Revamp tech spec are
now in place (session-check, board, team, overdue, admin users,
report). Old HTML page routes and templates are untouched — they're
removed in a later plan (Chapter 7a/7b), once the Angular frontend
fully replaces them."
```

---

## End of Plan A

After this plan, jira-u-bot has (in addition to everything that existed before):

- `GET /jira-notif/api/auth/me`
- `GET /jira-notif/api/dashboard/board`
- `GET /jira-notif/api/dashboard/team`
- `GET /jira-notif/api/dashboard/overdue`
- `GET /jira-notif/api/dashboard/admin/users`
- `GET /jira-notif/api/dashboard/report`

All existing routes (HTML pages, POST/DELETE actions) are completely unaffected except that `/api/dashboard/*` (preference, transition, credential) now returns 401 JSON instead of redirecting on an invalid session — a deliberate, spec-mandated change needed for Angular to handle session expiry correctly.

**Next:** Plan B (Angular frontend, Repository 2, 10 chapters) consumes these endpoints. Plan C (Chapter 7a/7b) wires Angular into the Go binary and deletes the old HTML routes/templates — only after Plan B is complete.
