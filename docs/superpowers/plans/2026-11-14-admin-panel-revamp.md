# Admin Panel Revamp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin mendapat shell sendiri (`/admin`) dengan sidebar Dashboard / User / Broadcast Message, tombol ke dashboard umum, dan halaman detail user tempat admin mengatur notifikasi, mute, dan Jira credential milik user tersebut (user diberi tahu via DM).

**Architecture:** Backend: handler settings di `user_dashboard_handler` diekstrak menjadi `SettingsHandler` yang diparametrisasi oleh *target resolver* (sesi vs `:userID`) dan *change listener* (DM). Endpoint user memakai resolver sesi; endpoint admin baru memakai resolver `:userID` + listener DM. Frontend: komponen Pengaturan meng-inject abstract class `SettingsApi` / `SettingsScope` (default `useExisting` service lama) + token `SETTINGS_MODE`; halaman admin detail meng-provide implementasi admin. Admin pindah ke `AdminShellComponent` di route top-level `/admin`.

**Tech Stack:** Go 1.x + Gin (backend), Angular 19 standalone + signals + Karma/Jasmine (frontend).

**Spec:** `docs/tech-spec/admin-panel-revamp/Tech Spec - Admin Panel Revamp.md`

## Global Constraints

- Endpoint user (`/api/dashboard/settings…`, `/api/dashboard/credential`, `/api/dashboard/boards`) **tidak berubah** perilaku/bentuk responsnya; semua test lama tetap hijau.
- Endpoint admin baru di grup `/api/dashboard/admin` (sudah `AuthMiddlewareAPI` + `AdminMiddleware`). Nama param path **`:userID`** (harus sama dengan route admin yang sudah ada — gin menolak nama wildcard berbeda di segmen yang sama).
- DM ke user: best-effort, async, error hanya di-log; tidak dikirim bila admin == target; tidak dikirim bila DM notifier nil.
- Teks UI & DM Bahasa Indonesia; identifier/code English.
- Spec frontend lama (`settings`, `mute-panel`, `jira-credential`, `dashboard-shell`) tetap hijau tanpa mengubah assertion-nya.
- Verifikasi per task: `go test ./...` + `go vet ./...` (backend) / `npx ng test --watch=false` + `npx ng build` di `frontend/` (frontend).
- Commit hanya atas persetujuan user (CLAUDE.md: tampilkan code review + opsi 1/2/3 setelah tiap task). Scope commit = nama fitur, mis. `admin-panel`.

---

### Task 1: Extract `SettingsHandler` (user_dashboard)

**Files:**
- Create: `modules/user_dashboard/user_dashboard_handler/settings_handler.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go` (hapus body GetSettings/UpdateNotificationSettings/ResetNotificationSettings/Mute/Unmute/ListBoards/SaveCredential/DeleteCredential → delegasi)
- Test: `modules/user_dashboard/user_dashboard_handler/settings_handler_test.go`

**Interfaces:**
- Produces:
  ```go
  type SettingsTarget struct { UserID int64; Email string }
  type SettingsChangeKind string // "notification" | "reset" | "mute" | "unmute" | "credential_updated" | "credential_deleted"
  type SettingsChange struct { Kind SettingsChangeKind; BoardID int64; Until *time.Time }
  type TargetResolver func(c *gin.Context) (SettingsTarget, bool)   // false = response already written
  type ChangeListener func(c *gin.Context, target SettingsTarget, change SettingsChange)
  func SessionTarget(c *gin.Context) (SettingsTarget, bool)
  func NewSettingsHandler(dashboardSvc user_dashboard_service.UserDashboardService, jiraCredential jira_credential_service.JiraCredentialService, userSettings user_setting_service.UserSettingService, resolve TargetResolver, onChange ChangeListener) *SettingsHandler
  // methods (gin handlers): GetSettings, UpdateNotification, ResetNotification, Mute, Unmute, ListBoards, SaveCredential, DeleteCredential
  ```

- [ ] **Step 1: Write failing tests** (`settings_handler_test.go`, reuse fakes from `user_dashboard_handler_test.go`)

```go
package user_dashboard_handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func targetResolver(id int64, email string) TargetResolver {
	return func(c *gin.Context) (SettingsTarget, bool) { return SettingsTarget{UserID: id, Email: email}, true }
}

type recordedChange struct {
	target SettingsTarget
	change SettingsChange
}

func newTargetSettingsHandler(settings *fakeUserSettingsForHandler, cred *fakeJiraCredentialForHandler, dash *fakeUserDashboardService, changes *[]recordedChange) *SettingsHandler {
	return NewSettingsHandler(dash, cred, settings, targetResolver(42, "target@esb.co.id"),
		func(c *gin.Context, t SettingsTarget, ch SettingsChange) { *changes = append(*changes, recordedChange{t, ch}) })
}

func TestSettingsHandler_UsesResolvedTargetNotSession(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	var changes []recordedChange
	h := newTargetSettingsHandler(settings, &fakeJiraCredentialForHandler{}, &fakeUserDashboardService{}, &changes)

	w, c := newTestContextWithUser("PUT", "/x?board=28", `{"reminderTiers":["1h"],"overageTiers":[],"overageRepeatMinutes":5,"notifyEarlyFinish":true,"notifyReturnedToTodo":true,"quietHours":null}`)
	h.UpdateNotification(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if settings.savedFor != 42 || settings.savedBoard != 28 {
		t.Errorf("saved for user %d board %d, want 42/28", settings.savedFor, settings.savedBoard)
	}
	if len(changes) != 1 || changes[0].change.Kind != ChangeNotification || changes[0].change.BoardID != 28 || changes[0].target.UserID != 42 {
		t.Errorf("changes = %+v, want one notification change for user 42 board 28", changes)
	}
}

func TestSettingsHandler_ResolverFalseStopsRequest(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	h := NewSettingsHandler(&fakeUserDashboardService{}, &fakeJiraCredentialForHandler{}, settings,
		func(c *gin.Context) (SettingsTarget, bool) { c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"}); return SettingsTarget{}, false },
		nil)

	w, c := newTestContextWithUser("POST", "/x", `{"minutes":60}`)
	h.Mute(c)

	if w.Code != http.StatusNotFound || settings.mutedTo != nil {
		t.Errorf("status=%d mutedTo=%v, want 404 and no mute", w.Code, settings.mutedTo)
	}
}

func TestSettingsHandler_NoChangeOnValidationError(t *testing.T) {
	var changes []recordedChange
	h := newTargetSettingsHandler(&fakeUserSettingsForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserDashboardService{}, &changes)

	w, c := newTestContextWithUser("POST", "/x", `{"minutes":0}`)
	h.Mute(c)

	if w.Code != http.StatusBadRequest || len(changes) != 0 {
		t.Errorf("status=%d changes=%d, want 400 and none", w.Code, len(changes))
	}
}

func TestSettingsHandler_EmitsChangeForEachWrite(t *testing.T) {
	var changes []recordedChange
	cred := &fakeJiraCredentialForHandler{}
	h := newTargetSettingsHandler(&fakeUserSettingsForHandler{}, cred, &fakeUserDashboardService{}, &changes)

	_, c := newTestContextWithUser("DELETE", "/x?board=28", "")
	h.ResetNotification(c)
	_, c = newTestContextWithUser("POST", "/x", `{"until":"forever"}`)
	h.Mute(c)
	_, c = newTestContextWithUser("DELETE", "/x?board=28", "")
	h.Unmute(c)
	_, c = newTestContextWithUser("POST", "/x", `{"jiraEmail":"t@esb.co.id","apiToken":"tok"}`)
	h.SaveCredential(c)
	_, c = newTestContextWithUser("DELETE", "/x", "")
	h.DeleteCredential(c)

	want := []SettingsChangeKind{ChangeReset, ChangeMute, ChangeUnmute, ChangeCredentialUpdated, ChangeCredentialDeleted}
	if len(changes) != len(want) {
		t.Fatalf("got %d changes, want %d: %+v", len(changes), len(want), changes)
	}
	for i, k := range want {
		if changes[i].change.Kind != k {
			t.Errorf("change[%d] = %s, want %s", i, changes[i].change.Kind, k)
		}
	}
	if changes[1].change.Until == nil {
		t.Error("mute change must carry Until")
	}
}

func TestSettingsHandler_ListBoardsUsesTargetEmail(t *testing.T) {
	dash := &fakeUserDashboardService{}
	var changes []recordedChange
	h := newTargetSettingsHandler(&fakeUserSettingsForHandler{}, &fakeJiraCredentialForHandler{}, dash, &changes)

	w, c := newTestContextWithUser("GET", "/x", "")
	h.ListBoards(c)

	if w.Code != http.StatusOK || dash.boardsEmail != "target@esb.co.id" {
		t.Errorf("status=%d email=%q, want 200 and target@esb.co.id", w.Code, dash.boardsEmail)
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./modules/user_dashboard/...` → FAIL `undefined: NewSettingsHandler`.

- [ ] **Step 3: Implement `settings_handler.go`** — move the 8 handler bodies from `user_dashboard_handler_impl.go` verbatim into `*SettingsHandler` methods, with these exact edits:
  - first line after `boardParam` (or at the top when there is no board param): `target, ok := h.resolve(c); if !ok { return }`
  - every `currentUser(c).UserID` / `user.UserID` → `target.UserID`; `user.Email` → `target.Email`
  - after each successful write, before the 200 response: `h.emit(c, target, SettingsChange{Kind: …, BoardID: boardID, Until: &until})` (Until only for mute; credential changes BoardID 0)
  - keep helpers `notificationJSON`, `muteStateJSON`, `muteActiveJSON`, `nonNil`, `boardParam`, `muteRequest`, `saveCredentialRequest` in `user_dashboard_handler_impl.go` (same package).

```go
// SettingsHandler serves the notification / mute / credential / board-list
// endpoints for ONE target user: the session's user on /api/dashboard, a
// path-selected user on /api/dashboard/admin/users/:userID (Tech Spec
// "Admin Panel Revamp"). Both mounts share this code so their request and
// response shapes can't drift apart.
type SettingsHandler struct {
	dashboardSvc   user_dashboard_service.UserDashboardService
	jiraCredential jira_credential_service.JiraCredentialService
	userSettings   user_setting_service.UserSettingService
	resolve        TargetResolver
	onChange       ChangeListener
}

func SessionTarget(c *gin.Context) (SettingsTarget, bool) {
	u := currentUser(c)
	return SettingsTarget{UserID: u.UserID, Email: u.Email}, true
}

func (h *SettingsHandler) emit(c *gin.Context, target SettingsTarget, change SettingsChange) {
	if h.onChange != nil {
		h.onChange(c, target, change)
	}
}
```

  In `user_dashboard_handler_impl.go`: add field `settings *SettingsHandler`, set in `NewUserDashboardHandler` via `NewSettingsHandler(dashboardSvc, jiraCredential, userSettings, SessionTarget, nil)` (constructor signature unchanged), and reduce the 8 methods to delegations, e.g. `func (h *userDashboardHandlerImpl) Mute(c *gin.Context) { h.settings.Mute(c) }` (`UpdateNotificationSettings` → `h.settings.UpdateNotification`, `ResetNotificationSettings` → `h.settings.ResetNotification`). Drop the now-unused `userSettings` field only if nothing else uses it.

- [ ] **Step 4: Run** `go test ./modules/user_dashboard/... && go vet ./...` → PASS (old + new tests).
- [ ] **Step 5: Review + commit** `refactor(admin-panel): extract SettingsHandler with target resolver`

---

### Task 2: Admin endpoints for a user's settings

**Files:**
- Modify: `modules/admin_dashboard/admin_dashboard_service/admin_dashboard_service.go`, `…_impl.go` (+ `GetUser`)
- Create: `modules/admin_dashboard/admin_dashboard_handler/user_settings.go`
- Modify: `modules/admin_dashboard/admin_dashboard_handler/admin_dashboard_handler.go` (+ `GetUser`), `…_impl.go`
- Modify: `modules/admin_dashboard/router.go`, `router.go` (root), `main.go`
- Test: `admin_dashboard_service_test.go`, `admin_dashboard_handler_test.go`, new `user_settings_test.go`

**Interfaces:**
- Consumes: Task 1 `SettingsTarget`, `NewSettingsHandler`, `ChangeListener`.
- Produces:
  ```go
  var ErrUserNotFound = errors.New("user not found")
  // AdminDashboardService
  GetUser(ctx context.Context, userID int64) (admin_dashboard_dto.UserRow, error) // ErrUserNotFound
  // admin_dashboard_handler
  func AdminTarget(svc admin_dashboard_service.AdminDashboardService) user_dashboard_handler.TargetResolver
  // AdminDashboardHandler gains: GetUser(c *gin.Context)
  // admin_dashboard.Router(actionGroup, apiGroup, handler, userSettings *user_dashboard_handler.SettingsHandler)
  ```

- [ ] **Step 1: Failing tests**

Service (`admin_dashboard_service_test.go`; `fakeRegistrationRepoForAdmin.FindAll` already returns `f.users` — check field name and reuse):
```go
func TestGetUser_ReturnsRowWithCredential(t *testing.T) {
	repo := &fakeRegistrationRepoForAdmin{users: []registration_dto.User{{UserID: 7, Email: "a@esb.co.id", FlagActive: true}}}
	cred := &fakeJiraCredentialForAdmin{maskedPreview: "••••50CF", maskedFound: true}
	svc := NewAdminDashboardService(repo, cred, nil, "bot@esb.co.id")

	row, err := svc.GetUser(context.Background(), 7)
	if err != nil || row.Email != "a@esb.co.id" || !row.HasCredential || row.CredentialMasked != "••••50CF" {
		t.Errorf("row=%+v err=%v", row, err)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "")
	if _, err := svc.GetUser(context.Background(), 99); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}
```
(Adapt fake field names to the existing fakes in that file.)

Handler (`user_settings_test.go`):
```go
func TestAdminTarget_BadIDIs400(t *testing.T) {
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "abc"}})
	if _, ok := AdminTarget(&fakeAdminDashboardService{})(c); ok || w.Code != http.StatusBadRequest {
		t.Errorf("ok=%v status=%d, want false/400", ok, w.Code)
	}
}

func TestAdminTarget_MissingUserIs404(t *testing.T) {
	svc := &fakeAdminDashboardService{getUserErr: admin_dashboard_service.ErrUserNotFound}
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	if _, ok := AdminTarget(svc)(c); ok || w.Code != http.StatusNotFound {
		t.Errorf("ok=%v status=%d, want false/404", ok, w.Code)
	}
}

func TestAdminTarget_ResolvesUser(t *testing.T) {
	svc := &fakeAdminDashboardService{user: admin_dashboard_dto.UserRow{UserID: 9, Email: "t@esb.co.id"}}
	_, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	target, ok := AdminTarget(svc)(c)
	if !ok || target.UserID != 9 || target.Email != "t@esb.co.id" {
		t.Errorf("target=%+v ok=%v", target, ok)
	}
}

func TestGetUser_Handler(t *testing.T) {
	svc := &fakeAdminDashboardService{user: admin_dashboard_dto.UserRow{UserID: 9, Email: "t@esb.co.id"}}
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	NewAdminDashboardHandler(svc).GetUser(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"email":"t@esb.co.id"`) {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
}
```
Add to `fakeAdminDashboardService`: fields `user admin_dashboard_dto.UserRow; getUserErr error` and
```go
func (f *fakeAdminDashboardService) GetUser(ctx context.Context, userID int64) (admin_dashboard_dto.UserRow, error) {
	return f.user, f.getUserErr
}
```

- [ ] **Step 2: Run** `go test ./modules/admin_dashboard/...` → FAIL (undefined).

- [ ] **Step 3: Implement**

Service:
```go
// ErrUserNotFound: no registered user has that userID.
var ErrUserNotFound = errors.New("user not found")

// GetUser returns one user's row (same shape as ListUsers). The user base
// is small, so it filters ListUsers instead of adding a repository query.
func (s *adminDashboardServiceImpl) GetUser(ctx context.Context, userID int64) (admin_dashboard_dto.UserRow, error) {
	rows, err := s.ListUsers(ctx)
	if err != nil {
		return admin_dashboard_dto.UserRow{}, err
	}
	for _, r := range rows {
		if r.UserID == userID {
			return r, nil
		}
	}
	return admin_dashboard_dto.UserRow{}, ErrUserNotFound
}
```

Handler `user_settings.go`:
```go
// AdminTarget resolves /users/:userID to the user whose settings an admin
// is editing (Tech Spec "Admin Panel Revamp"): 400 bad id, 404 unknown user.
func AdminTarget(svc admin_dashboard_service.AdminDashboardService) user_dashboard_handler.TargetResolver {
	return func(c *gin.Context) (user_dashboard_handler.SettingsTarget, bool) {
		id, ok := userIDParam(c)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
			return user_dashboard_handler.SettingsTarget{}, false
		}
		user, err := svc.GetUser(c.Request.Context(), id)
		if errors.Is(err, admin_dashboard_service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
			return user_dashboard_handler.SettingsTarget{}, false
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat user"})
			return user_dashboard_handler.SettingsTarget{}, false
		}
		return user_dashboard_handler.SettingsTarget{UserID: user.UserID, Email: user.Email}, true
	}
}

func (h *adminDashboardHandlerImpl) GetUser(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}
	user, err := h.svc.GetUser(c.Request.Context(), id)
	if errors.Is(err, admin_dashboard_service.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat user"})
		return
	}
	c.JSON(http.StatusOK, user)
}
```

Router `modules/admin_dashboard/router.go`:
```go
func Router(actionGroup, apiGroup *gin.RouterGroup, handler admin_dashboard_handler.AdminDashboardHandler, userSettings *user_dashboard_handler.SettingsHandler) {
	// … existing routes unchanged …
	apiGroup.GET("/users", handler.ListUsers)
	apiGroup.GET("/users/:userID", handler.GetUser)
	apiGroup.GET("/users/:userID/boards", userSettings.ListBoards)
	apiGroup.GET("/users/:userID/settings", userSettings.GetSettings)
	apiGroup.PUT("/users/:userID/settings/notification", userSettings.UpdateNotification)
	apiGroup.DELETE("/users/:userID/settings/notification", userSettings.ResetNotification)
	apiGroup.POST("/users/:userID/settings/mute", userSettings.Mute)
	apiGroup.DELETE("/users/:userID/settings/mute", userSettings.Unmute)
	apiGroup.PUT("/users/:userID/credential", userSettings.SaveCredential)
}
```
Root `router.go`: `SetupRouter` gains param `adminUserSettings *user_dashboard_handler.SettingsHandler`, passed to `admin_dashboard.Router`. Update every `SetupRouter(` call site (grep, incl. tests).

`main.go` (listener added in Task 3; nil for now):
```go
adminUserSettings := user_dashboard_handler.NewSettingsHandler(
	userDashboardSvc, jiraCredentialSvc, userSettingSvc, admin_dashboard_handler.AdminTarget(adminDashboardSvc), nil,
)
router := SetupRouter(&cfg, authSvc, authHandler, userDashboardHandler, adminDashboardHandler, sprintReportHandler, adminUserSettings)
```

- [ ] **Step 4: Run** `go build ./... && go test ./... && go vet ./...` → PASS.
- [ ] **Step 5: Review + commit** `feat(admin-panel): admin endpoints for a user's settings and credential`

---

### Task 3: DM the user on admin changes

**Files:**
- Create: `modules/admin_dashboard/admin_dashboard_service/change_notice.go` (+ `_test.go`)
- Modify: `admin_dashboard_service.go` (interface + `NotifyUserChanged`), `admin_dashboard_dto.go` (+ `UserChange`), `admin_dashboard_handler/user_settings.go` (+ `AdminChangeListener`), `admin_dashboard_handler_impl.go` (`DeleteUserCredential` notifies), `main.go`, handler fakes.

**Interfaces:**
- Produces:
  ```go
  // admin_dashboard_dto
  type UserChange struct { Kind string; BoardID int64; Until *time.Time }
  // AdminDashboardService
  NotifyUserChanged(actingUserID int64, actingEmail string, targetUserID int64, targetEmail string, change admin_dashboard_dto.UserChange)
  // admin_dashboard_service (pure)
  func ChangeNoticeText(adminEmail, boardName string, change admin_dashboard_dto.UserChange) string
  // admin_dashboard_handler
  func AdminChangeListener(svc admin_dashboard_service.AdminDashboardService) user_dashboard_handler.ChangeListener
  ```
- `NewAdminDashboardService` gains a `boards BoardNamer` param (`interface{ Get(ctx, boardID int64) (board_dto.Board, bool, error) }` — satisfied by `board_service.BoardService`).

- [ ] **Step 1: Failing tests** (`change_notice_test.go`)

```go
func TestChangeNoticeText(t *testing.T) {
	until := time.Date(2026, 11, 14, 16, 30, 0, 0, user_setting_dto.Jakarta)
	cases := []struct {
		name  string
		board string
		ch    admin_dashboard_dto.UserChange
		want  string
	}{
		{"notif default", "", admin_dashboard_dto.UserChange{Kind: "notification"}, "Admin a@esb.co.id mengubah pengaturan notifikasi kamu (default semua board)."},
		{"notif board", "ESB Core Board", admin_dashboard_dto.UserChange{Kind: "notification", BoardID: 5}, "Admin a@esb.co.id mengubah pengaturan notifikasi kamu (board ESB Core Board)."},
		{"reset", "ESB Core Board", admin_dashboard_dto.UserChange{Kind: "reset", BoardID: 5}, "Admin a@esb.co.id mengembalikan pengaturan board ESB Core Board kamu ke default."},
		{"mute", "", admin_dashboard_dto.UserChange{Kind: "mute", Until: &until}, "Admin a@esb.co.id membungkam notifikasi kamu (default semua board) sampai 14 Nov 16.30 WIB."},
		{"mute forever", "", admin_dashboard_dto.UserChange{Kind: "mute", Until: &user_setting_dto.MuteForeverUntil}, "Admin a@esb.co.id membungkam notifikasi kamu (default semua board) selamanya."},
		{"unmute", "X", admin_dashboard_dto.UserChange{Kind: "unmute", BoardID: 5}, "Admin a@esb.co.id mengaktifkan kembali notifikasi kamu (board X)."},
		{"cred up", "", admin_dashboard_dto.UserChange{Kind: "credential_updated"}, "Admin a@esb.co.id memperbarui Jira credential kamu."},
		{"cred del", "", admin_dashboard_dto.UserChange{Kind: "credential_deleted"}, "Admin a@esb.co.id menghapus Jira credential kamu."},
	}
	for _, tc := range cases {
		if got := ChangeNoticeText("a@esb.co.id", tc.board, tc.ch); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestNotifyUserChanged_SkipsSelfAndNilNotifier(t *testing.T) {
	dm := &fakeChatDMNotifierForAdmin{}
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, dm, "", nil).(*adminDashboardServiceImpl)
	svc.sendSync = true // test hook: no goroutine
	svc.NotifyUserChanged(1, "a@esb.co.id", 1, "a@esb.co.id", admin_dashboard_dto.UserChange{Kind: "notification"})
	if len(dm.sent) != 0 {
		t.Errorf("self change sent %d DMs, want 0", len(dm.sent))
	}
	svc.NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "credential_deleted"})
	if len(dm.sent) != 1 {
		t.Fatalf("sent %d DMs, want 1", len(dm.sent))
	}
	nilSvc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "", nil)
	nilSvc.NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "mute"}) // must not panic
}
```
(Adapt `dm.sent` to the existing fake's recorded field.) Handler test: `DeleteUserCredential` calls `NotifyUserChanged` with kind `credential_deleted` (record in fake).

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement**

```go
// change_notice.go
const noticeTimeout = 10 * time.Second

func scopeLabel(boardName string, boardID int64) string {
	if boardID == 0 {
		return "default semua board"
	}
	return "board " + boardName
}

// ChangeNoticeText is the DM a user gets when an admin changes their
// settings (Tech Spec "Admin Panel Revamp", AC4.1).
func ChangeNoticeText(adminEmail, boardName string, ch admin_dashboard_dto.UserChange) string {
	scope := scopeLabel(boardName, ch.BoardID)
	switch ch.Kind {
	case "notification":
		return fmt.Sprintf("Admin %s mengubah pengaturan notifikasi kamu (%s).", adminEmail, scope)
	case "reset":
		return fmt.Sprintf("Admin %s mengembalikan pengaturan %s kamu ke default.", adminEmail, scope)
	case "mute":
		if ch.Until == nil || user_setting_dto.IsForever(ch.Until) {
			return fmt.Sprintf("Admin %s membungkam notifikasi kamu (%s) selamanya.", adminEmail, scope)
		}
		when := ch.Until.In(user_setting_dto.Jakarta)
		return fmt.Sprintf("Admin %s membungkam notifikasi kamu (%s) sampai %d %s %02d.%02d WIB.", adminEmail, scope, when.Day(), shortMonth[when.Month()-1], when.Hour(), when.Minute())
	case "unmute":
		return fmt.Sprintf("Admin %s mengaktifkan kembali notifikasi kamu (%s).", adminEmail, scope)
	case "credential_updated":
		return fmt.Sprintf("Admin %s memperbarui Jira credential kamu.", adminEmail)
	case "credential_deleted":
		return fmt.Sprintf("Admin %s menghapus Jira credential kamu.", adminEmail)
	}
	return fmt.Sprintf("Admin %s mengubah pengaturan kamu.", adminEmail)
}

var shortMonth = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// NotifyUserChanged DMs targetEmail about an admin's change — best-effort:
// never for the admin's own account, never blocks the request, errors are
// only logged (AC4.2, AC4.3).
func (s *adminDashboardServiceImpl) NotifyUserChanged(actingUserID int64, actingEmail string, targetUserID int64, targetEmail string, ch admin_dashboard_dto.UserChange) {
	if s.dmNotifier == nil || actingUserID == targetUserID {
		return
	}
	send := func() {
		ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
		defer cancel()
		boardName := fmt.Sprintf("#%d", ch.BoardID)
		if ch.BoardID != 0 && s.boards != nil {
			if b, found, err := s.boards.Get(ctx, ch.BoardID); err == nil && found && b.Name != "" {
				boardName = b.Name
			}
		}
		if err := s.dmNotifier.SendPlainTextDM(ctx, targetEmail, ChangeNoticeText(actingEmail, boardName, ch)); err != nil {
			log.Printf("admin_dashboard_service: change notice DM to %s failed: %v", targetEmail, err)
		}
	}
	if s.sendSync {
		send()
		return
	}
	go send()
}
```
(`sendSync bool` unexported field for tests; `boards BoardNamer` field. Check `board_dto.Board` name field and adapt.)

Listener:
```go
// AdminChangeListener tells the target user about an admin's change.
func AdminChangeListener(svc admin_dashboard_service.AdminDashboardService) user_dashboard_handler.ChangeListener {
	return func(c *gin.Context, target user_dashboard_handler.SettingsTarget, ch user_dashboard_handler.SettingsChange) {
		admin := currentUser(c)
		svc.NotifyUserChanged(admin.UserID, admin.Email, target.UserID, target.Email,
			admin_dashboard_dto.UserChange{Kind: string(ch.Kind), BoardID: ch.BoardID, Until: ch.Until})
	}
}
```
`DeleteUserCredential` handler: after success, `if user, err := h.svc.GetUser(ctx, targetUserID); err == nil { h.svc.NotifyUserChanged(admin.UserID, admin.Email, user.UserID, user.Email, admin_dashboard_dto.UserChange{Kind: "credential_deleted"}) }` — resolve email BEFORE deleting is not needed (credential delete keeps the user).

`main.go`: pass `boardSvc` to `NewAdminDashboardService`, and `admin_dashboard_handler.AdminChangeListener(adminDashboardSvc)` instead of `nil` in `NewSettingsHandler`. Update existing `NewAdminDashboardService(` test call sites with the extra `nil`.

- [ ] **Step 4: Run** `go test ./... && go vet ./...` → PASS.
- [ ] **Step 5: Review + commit** `feat(admin-panel): DM users when an admin changes their settings`

---

### Task 4: Frontend DI adapters for the settings components

**Files:**
- Create: `frontend/src/app/core/services/settings-api.ts`
- Modify: `frontend/src/app/core/services/board.service.ts` (no behaviour change), `board-context.service.ts` (no behaviour change)
- Modify: `features/dashboard/settings/settings.component.ts`, `settings.component.html`, `mute-panel.component.ts`, `jira-credential.component.ts`
- Test: `frontend/src/app/core/services/settings-api.spec.ts`, plus extra cases in `mute-panel.component.spec.ts` / `settings.component.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  export abstract class SettingsApi {            // @Injectable({ providedIn: 'root', useExisting: BoardService })
    abstract getSettings(boardId?: number | null): Observable<SettingsResponse>;
    abstract saveNotificationSettings(s: NotificationSettings, boardId?: number | null): Observable<NotificationSettings>;
    abstract resetNotificationSettings(boardId: number): Observable<NotificationSettings>;
    abstract mute(r: MuteRequest, boardId?: number | null): Observable<MuteStatus>;
    abstract unmute(boardId?: number | null): Observable<MuteStatus>;
    abstract saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }>;
    abstract deleteCredential(): Observable<{ status: string }>;
  }
  export abstract class SettingsScope {          // @Injectable({ providedIn: 'root', useExisting: BoardContextService })
    abstract readonly boards: Signal<BoardSummary[]>;
    abstract readonly selectedId: Signal<number | null>;
    abstract select(boardId: number | null): void;
    abstract load(): void;
  }
  export type SettingsMode = 'self' | 'admin';
  export const SETTINGS_MODE = new InjectionToken<SettingsMode>('SETTINGS_MODE', { providedIn: 'root', factory: () => 'self' });
  ```
  `useExisting` makes existing specs that provide `BoardService` / `BoardContextService` mocks keep working unchanged.

- [ ] **Step 1: Failing test** (`settings-api.spec.ts`)
```ts
describe('SettingsApi / SettingsScope defaults', () => {
  it('resolve to BoardService and BoardContextService', () => {
    const boardService = { tag: 'board' };
    const context = { tag: 'context' };
    TestBed.configureTestingModule({
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: BoardContextService, useValue: context },
      ],
    });
    expect(TestBed.inject(SettingsApi) as unknown).toBe(boardService);
    expect(TestBed.inject(SettingsScope) as unknown).toBe(context);
    expect(TestBed.inject(SETTINGS_MODE)).toBe('self');
  });
});
```
New mute-panel case: with `{ provide: SETTINGS_MODE, useValue: 'admin' }` and global scope (`boardId` null), a successful mute **does not** call `muteState.set` and shows the status from the `status` input / last action (spy on `MuteStateService.set`).

- [ ] **Step 2: Run** `cd frontend && npx ng test --watch=false --include='**/settings-api.spec.ts'` → FAIL.

- [ ] **Step 3: Implement**
  - `settings-api.ts` as above (put `@Injectable({ providedIn: 'root', useExisting: … })` on the abstract classes; import types from `board.service` / `board-context.service`).
  - `settings.component.ts`: `private boardService = inject(SettingsApi)`; `protected boardContext = inject(SettingsScope)`; `private mode = inject(SETTINGS_MODE)`; in `loadSettings` replace `this.muteState.set(res.mute.global)` with `if (this.mode === 'self') this.muteState.set(res.mute.global);`. Expose `protected isAdmin = this.mode === 'admin'`.
  - `settings.component.html`: on the global mute panel add `[globalStatus]="globalMute()"` (new mute-panel input, ignored in self mode). `boardContext.select(null)` stays (works for both scopes).
  - `mute-panel.component.ts`: `inject(SettingsApi)`, `inject(SettingsScope)`, `inject(SETTINGS_MODE)`; rename input `boardStatus` → keep `boardStatus` (spec uses it) and add `globalStatus = input<MuteStatus | null>(null)`; `until`/`forever` for the global level read `muteState` in self mode, `globalStatus`/last action in admin mode; `apply()` calls `muteState.set` only when `!isBoard() && mode === 'self'`, and `scope.load()` for board level.
  - `jira-credential.component.ts`: `private boardService = inject(SettingsApi)`.

- [ ] **Step 4: Run** `npx ng test --watch=false && npx ng build` → all PASS (old specs untouched).
- [ ] **Step 5: Review + commit** `refactor(admin-panel): settings components depend on SettingsApi/SettingsScope`

---

### Task 5: `AdminShellComponent` + routing

**Files:**
- Create: `frontend/src/app/layouts/admin-shell/admin-shell.component.{ts,html,spec.ts}`
- Modify: `frontend/src/app/app.routes.ts`, `layouts/dashboard-shell/dashboard-shell.component.html` (Admin link → `/admin`), `frontend/src/styles.scss` (only `.admin-badge`, `.btn-home` if needed)
- Create (placeholders replaced in Task 6): none — Task 5 routes children to the existing `AdminComponent` (for `''` and `users`) and `BroadcastComponent` (for `broadcast`) so the app is runnable after this task.

**Interfaces:**
- Produces: route tree
  ```ts
  { path: 'admin', component: AdminShellComponent, canActivate: [AuthGuard, AdminGuard], children: [
      { path: '', pathMatch: 'full', component: …, title: 'Admin · Nudge', data: { heading: 'Dashboard', subheading: 'Ringkasan user Nudge', crumbs: ['Admin'] } },
      { path: 'users', component: …, title: 'User · Admin · Nudge', data: { heading: 'User', subheading: 'Kelola user dan pengaturannya', crumbs: ['Admin', 'User'] } },
      { path: 'users/:id', component: …, title: 'Detail User · Admin · Nudge', data: { heading: 'Detail User', subheading: 'Atur notifikasi, mute, dan credential user', crumbs: ['Admin', 'User'] } },
      { path: 'broadcast', component: …, title: 'Broadcast · Admin · Nudge', data: { heading: 'Broadcast Message', subheading: 'Kirim pengumuman ke semua user aktif', crumbs: ['Admin', 'Broadcast Message'] } },
  ]}
  // inside 'dashboard' children:
  { path: 'admin', redirectTo: '/admin', pathMatch: 'full' }
  ```

- [ ] **Step 1: Failing spec** (`admin-shell.component.spec.ts`, pattern of `dashboard-shell.component.spec.ts`: `provideRouter([])`, AuthService mock with `currentUser$ = of({ email: 'admin@esb.co.id', isAdmin: true })`)
```ts
it('renders the three admin menu items', () => {
  const links = [...fixture.nativeElement.querySelectorAll('.sidebar-nav .nav-item')].map((a: HTMLElement) => a.textContent!.trim());
  expect(links).toEqual(['Dashboard', 'User', 'Broadcast Message']);
});
it('has a Dashboard Utama button to /dashboard', () => {
  const btn: HTMLAnchorElement = fixture.nativeElement.querySelector('a.btn-home');
  expect(btn.textContent).toContain('Dashboard Utama');
  expect(btn.getAttribute('href')).toBe('/dashboard');
});
it('has no board picker', () => {
  expect(fixture.nativeElement.querySelector('.board-picker')).toBeNull();
});
it('shows the Admin badge under the logo', () => {
  expect(fixture.nativeElement.querySelector('.sidebar-brand .admin-badge')?.textContent).toContain('Admin');
});
```
Routing spec (in `app.routes` spec or a new `admin-routes.spec.ts`): navigating to `/dashboard/admin` with `provideRouter(routes)` and guards stubbed to `true` ends at `/admin`.

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** — copy the structure of `dashboard-shell.component.{ts,html}` (same CSS classes: `app-shell`, `sidebar`, `sidebar-nav`, `nav-item`, `topbar`, `user-menu`, drawer/scrim, outside-click + Escape handling, `page$` from deepest route data) with these differences:
  - brand links to `/admin`; under the brand name a `<span class="admin-badge">Admin</span>`
  - nav items: `/admin` (icon `home`, `[routerLinkActiveOptions]="{ exact: true }"`), `/admin/users` (icon `users`, not exact so `users/:id` is active), `/admin/broadcast` (icon `megaphone` — use an icon that exists in `icon.component.ts`; add one there if missing)
  - topbar-left: heading/subheading + breadcrumb from `data.crumbs` (`Admin › User`) — rendered as `<nav class="breadcrumb">`
  - topbar-right: `<a class="btn btn-ghost btn-sm btn-home" routerLink="/dashboard"><app-icon name="arrow-left" [size]="14" /> Dashboard Utama</a>` + user menu (email, `Administrator` badge, Logout). No board picker, no mute badge, no `getSettings` / `boardContext.load()` calls.
  - footer tip: "Perubahan pengaturan user oleh admin selalu diberitahukan ke user lewat DM."
  - `app.routes.ts` per Interfaces; remove the old `dashboard/admin` child (replaced by the redirect). `dashboard-shell.component.html`: `routerLink="/admin"`.

- [ ] **Step 4: Run** `npx ng test --watch=false && npx ng build` → PASS (update `dashboard-shell` spec only if it asserted the old `/dashboard/admin` href).
- [ ] **Step 5: Review + commit** `feat(admin-panel): admin shell with sidebar and main dashboard link`

---

### Task 6: Admin pages — Overview, Users, Broadcast

**Files:**
- Create: `frontend/src/app/features/admin/overview/admin-overview.component.{ts,html,spec.ts}`
- Create: `frontend/src/app/features/admin/users/admin-users.component.{ts,html,spec.ts}` (moved from `features/dashboard/admin/admin.component.*`)
- Create: `frontend/src/app/features/admin/broadcast/broadcast.component.{ts,html,spec.ts}` (moved via `git mv` from `features/dashboard/admin/broadcast.component.*`)
- Delete: `frontend/src/app/features/dashboard/admin/*`
- Modify: `app.routes.ts` (point children at the new components)

**Interfaces:**
- Consumes: `AdminService.listUsers()` (existing).
- Produces: `AdminUsersComponent` row click → `router.navigate(['/admin/users', user.userId])`.

- [ ] **Step 1: Failing specs**
  - `admin-overview.component.spec.ts`: given 4 users (2 active, 1 inactive w/o credential, 1 with credential) → stat values `4 / 3 / 1`; list "User tanpa credential" contains the 3 users without credential (max 5) as links `/admin/users/<id>`; list "User nonaktif" contains the inactive one; empty list shows "Semua user sudah …" text.
  - `admin-users.component.spec.ts`: `git mv` the old `admin.component.spec.ts` and rename the class; add: clicking a row's `.user-cell` navigates to `/admin/users/2` (spy `Router.navigate`); clicking "Nonaktifkan" does **not** navigate; each row has an "Atur" link to `/admin/users/<id>`. Stats grid is no longer on this page.
  - broadcast spec: moved unchanged.

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement**
  - Overview: `users = signal<UserRow[]>([])`, computed `activeCount`, `credentialCount`, `withoutCredential = users.filter(u => !u.hasCredential).slice(0, 5)`, `inactive = users.filter(u => !u.flagActive).slice(0, 5)`; template = existing `stat-grid` markup + two `section.card` lists (user-avatar, email, name, chevron) linking to detail; "Lihat semua" link to `/admin/users`.
  - Users: old `AdminComponent` minus the `stat-grid` and `<app-broadcast>`; `<tr (click)="open(user)">`; inside action cells and inputs add `(click)="$event.stopPropagation()"` on the `<td>` wrappers of Jira ID / credential / actions; add `<a class="btn btn-sm btn-soft" [routerLink]="['/admin/users', user.userId]">Atur</a>` as first action; table rows get `class="clickable"` (cursor pointer — add to `styles.scss`).
  - Broadcast page: routed directly to `BroadcastComponent` (it is already a self-contained card).

- [ ] **Step 4: Run** `npx ng test --watch=false && npx ng build` → PASS.
- [ ] **Step 5: Review + commit** `feat(admin-panel): admin overview, users and broadcast pages`

---

### Task 7: Admin user detail page

**Files:**
- Create: `frontend/src/app/core/services/admin-user-settings.api.ts` (`AdminUserSettingsApi`, `AdminUserScope`, `ADMIN_TARGET_USER_ID`)
- Modify: `frontend/src/app/core/services/admin.service.ts` (+ `getUser`, `listUserBoards`, `saveUserCredential`)
- Create: `frontend/src/app/features/admin/user-detail/admin-user-detail.component.{ts,html,spec.ts}`
- Modify: `frontend/src/app/features/dashboard/settings/jira-credential.component.ts` (admin copy: toast "Credential user tersimpan")
- Test: `admin-user-settings.api.spec.ts`

**Interfaces:**
- Consumes: Task 2 endpoints, Task 4 `SettingsApi`, `SettingsScope`, `SETTINGS_MODE`.
- Produces:
  ```ts
  export const ADMIN_TARGET_USER_ID = new InjectionToken<number>('ADMIN_TARGET_USER_ID');
  @Injectable() export class AdminUserSettingsApi extends SettingsApi { /* base = `${apiBasePath}/api/dashboard/admin/users/${id}` */ }
  @Injectable() export class AdminUserScope extends SettingsScope { boards; selectedId; loaded; select(id); load(); }
  // AdminService
  getUser(userId: number): Observable<UserRow>;
  listUserBoards(userId: number): Observable<{ boards: BoardSummary[] | null }>;
  ```

- [ ] **Step 1: Failing tests**
  - `admin-user-settings.api.spec.ts` (HttpTestingController, `ADMIN_TARGET_USER_ID = 12`):
    - `getSettings(28)` → `GET /nudge/api/dashboard/admin/users/12/settings?board=28` (use `environment.apiBasePath` in the expected URL)
    - `saveNotificationSettings(x, null)` → `PUT …/users/12/settings/notification` without `board`
    - `resetNotificationSettings(28)` → `DELETE …/settings/notification?board=28`
    - `mute({minutes:60}, null)` → `POST …/settings/mute`; `unmute(28)` → `DELETE …/settings/mute?board=28`
    - `saveCredential('a','b')` → `PUT …/users/12/credential` body `{jiraEmail:'a', apiToken:'b'}`
    - `deleteCredential()` → `DELETE /nudge/dashboard/admin/users/12/credential` (existing admin action path)
    - `AdminUserScope.load()` → `GET …/users/12/boards`; `select(28)` sets `selectedId` and navigates with `queryParams: { board: 28 }, queryParamsHandling: 'merge'`; **no** `localStorage` write (spy `localStorage.setItem` not called); a selected board missing after `load()` → `selectedId` null.
  - `admin-user-detail.component.spec.ts`: route param `id=12`; `getUser` 404 → shows "User tidak ditemukan" + link `/admin/users`; success → profile card with email, status, Jira account ID input, credential preview; renders `<app-settings>`; the component's injector resolves `SettingsApi` to `AdminUserSettingsApi` and `SETTINGS_MODE` to `'admin'`.

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement**
  ```ts
  @Component({
    selector: 'app-admin-user-detail',
    standalone: true,
    imports: [RouterLink, FormsModule, IconComponent, SettingsComponent],
    templateUrl: './admin-user-detail.component.html',
    providers: [
      { provide: ADMIN_TARGET_USER_ID, useFactory: () => Number(inject(ActivatedRoute).snapshot.paramMap.get('id')) },
      AdminUserSettingsApi,
      AdminUserScope,
      { provide: SettingsApi, useExisting: AdminUserSettingsApi },
      { provide: SettingsScope, useExisting: AdminUserScope },
      { provide: SETTINGS_MODE, useValue: 'admin' },
    ],
  })
  export class AdminUserDetailComponent implements OnInit { … }
  ```
  - `ngOnInit`: `adminService.getUser(id)` → `user` signal / `notFound` signal (status 404) / `error`; then `scope.load()`.
  - Profile card: avatar, email, name, status badge + toggle (reuse `AdminService.toggleActive`, error message from response — self-lockout shows the server's message), Jira account ID inline edit (`editJiraAccountId`), credential preview. The credential form itself comes from `<app-settings>` (its Jira API Key card) — no second form.
  - Scope picker above `<app-settings>`: a `<select>`-style list (reuse `SelectComponent` from `shared/ui`) with "Default semua board" + `scope.boards()` (name · sprint); `(change)` → `scope.select(id)`. Empty boards → helper text "User ini belum punya tiket di sprint aktif mana pun."
  - Banner on top: "Kamu sedang mengatur pengaturan milik **{email}**. Setiap perubahan akan diberitahukan ke user lewat DM."
  - `AdminUserScope`: same logic as `BoardContextService` but reading `?board=` only from the current URL, no localStorage, boards from `adminService.listUserBoards(id)`, and `select()` navigates relative to the current route (`relativeTo` not needed: `navigate([], …)`).

- [ ] **Step 4: Run** `npx ng test --watch=false && npx ng build` → PASS.
- [ ] **Step 5: Review + commit** `feat(admin-panel): admin user detail with settings, mute and credential`

---

### Task 8: Cleanup, docs, manual verification

**Files:**
- Modify: `README.md` (bagian dashboard/admin: menu admin baru, endpoint admin baru, DM notice)
- Modify: `docs/tech-spec/admin-panel-revamp/Tech Spec - Admin Panel Revamp.md` (`Document Status` → `Implemented`; catat deviasi bila ada, mis. path param `:userID`)
- Verify: `features/dashboard/admin/` sudah tidak ada; grep `dashboard/admin` di frontend hanya tersisa di `AdminService` (action paths backend) dan redirect route.

- [ ] **Step 1:** `go test ./... && go vet ./...`; `cd frontend && npx ng test --watch=false && npx ng build`.
- [ ] **Step 2:** Manual (dev server `npm start` + backend): login admin → user menu "Admin" → `/admin`; sidebar 3 menu; "Dashboard Utama" kembali ke `/dashboard`; `/dashboard/admin` redirect; buka user lain → ubah tier board, mute 1 jam, isi credential → target menerima DM; banner mute admin sendiri tidak berubah; buka akun sendiri via admin → tidak ada DM; `/admin/users/99999` → "User tidak ditemukan"; mobile width → drawer.
- [ ] **Step 3: Review + commit** `docs(admin-panel): README and spec status`
