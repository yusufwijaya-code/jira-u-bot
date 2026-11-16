# Auto Join Google Group (Visibility App Nudge) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Setiap user Nudge yang login di web otomatis dimasukkan ke Google Group `nudge-users@esb.co.id` (yang dipasang sekali di Visibility Chat App), supaya bisa menemukan dan meng-install app Nudge tanpa admin menambah email satu per satu.

**Architecture:** Modul baru `modules/chat_group` berisi client Cloud Identity Groups API (`groups:lookup` + `memberships.create`) dengan token OAuth operator **terpisah** (scope `cloud-identity.groups`), repository di `ms_user` (`chatGroupAddedDate`, `chatGroupError`), dan service `EnsureMember` yang dipanggil asinkron dari login hook web + sweep tiap 1 jam. Status chat-app menambah `groupState` untuk banner dashboard. Fitur mati otomatis bila `NUDGE_GROUP_EMAIL` kosong atau token tidak ada.

**Tech Stack:** Go 1.x + Gin + MySQL (`database/sql`), `golang.org/x/oauth2`, Angular 19 standalone + signals, Jasmine/Karma.

**Spec:** `docs/tech-spec/chat-app-group/Tech Spec - Auto Join Google Group Nudge.md`

## Global Constraints

- **Gate:** Task 0 (spike) harus **GO** sebelum Task 1 dimulai. Bila NO-GO, plan berhenti; tidak ada kode yang ditulis.
- Branch tetap `feat/nudge` (tidak membuat branch baru — keputusan user untuk seluruh pekerjaan Nudge).
- Fitur mati bila `NUDGE_GROUP_EMAIL` kosong **atau** file `GOOGLE_GROUPS_TOKEN_PATH` tidak ada: tidak ada panggilan API, tidak ada perubahan DB, `groupState = "none"`.
- Token Groups **terpisah** dari token Chat operator (`GOOGLE_OAUTH_TOKEN_PATH`); default `./data/groups_oauth_token.json`, permission file `0600`.
- Scope OAuth: `https://www.googleapis.com/auth/cloud-identity.groups` saja.
- Base URL API: `https://cloudidentity.googleapis.com/v1`.
- Request membership: `{"preferredMemberKey":{"id":"<email>"},"roles":[{"name":"MEMBER"}]}`. HTTP `409` = sudah anggota = **sukses**.
- Login tidak boleh menunggu atau gagal karena fitur ini (panggilan asinkron, error hanya di-log).
- Sweep: tiap 1 jam, maks **50** user per run, jeda **200 ms** antar panggilan; berhenti di 401 pertama.
- `chatGroupError` maks 255 karakter.
- `groupState`: `none | pending | ready | failed`; `pending` berlaku < **24 jam** sejak `chatGroupAddedDate`.
- Teks banner (persis):
  - pending: `App Nudge sedang disiapkan untuk akunmu (maks. 24 jam). Sementara itu notifikasi tetap dikirim lewat DM.`
  - failed: `Akunmu belum bisa ditambahkan ke app Nudge. Hubungi admin Nudge.`
- Commit message: `type(chat-group): …` diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

---

## File Structure

| File | Tanggung jawab |
|------|----------------|
| `migrations/0020_chat_group.sql` | Kolom `chatGroupAddedDate`, `chatGroupError` di `ms_user` |
| `migrations/migrate.go` | Embed + apply 0020 |
| `config/config.go`, `.env.example` | `NUDGE_GROUP_EMAIL`, `GOOGLE_GROUPS_TOKEN_PATH` |
| `modules/chat_group/client.go` | HTTP client Cloud Identity: `Lookup`, `AddMember` |
| `modules/chat_group/oauth.go` | Token source dari file + `RunSetupFlow` (CLI) |
| `modules/chat_group/repository.go` | Query `ms_user` untuk join state |
| `modules/chat_group/service.go` | `EnsureMember`, `Sweep`, `Run` |
| `modules/chat_app/chat_app_dto/chat_app_dto.go`, `modules/chat_app/chat_app_repository/chat_app_repository_impl.go` | `State.GroupAddedDate`, `State.GroupError` |
| `modules/chat_app_activation/chat_app_activation.go` | `groupState` di status |
| `main.go` | Flag `--setup-groups-oauth`, wiring login hook, worker sweep |
| `frontend/src/app/core/services/chat-app.service.ts` | Tipe `groupState` |
| `frontend/src/app/shared/ui/chat-app-banner.component.ts` (+ spec) | Teks pending/failed |
| `README.md` | Dokumentasi setup |

---

### Task 0: Spike — buktikan API boleh dipakai (manual, oleh user)

**Files:** tidak ada.

- [ ] **Step 1:** User membuat grup `nudge-users@esb.co.id` di https://groups.google.com (Groups for Business, bukan grup dinamis, tidak locked, "Who can view members" = pemilik/manager).
- [ ] **Step 2:** Di Cloud Console project `jira-u-bot` → APIs & Services → Library → **Cloud Identity API** → Enable.
- [ ] **Step 3:** Di https://developers.google.com/oauthplayground, scope `https://www.googleapis.com/auth/cloud-identity.groups`, Authorize dengan `yusuf.wijaya@esb.co.id`, Exchange code.
- [ ] **Step 4:** `GET https://cloudidentity.googleapis.com/v1/groups:lookup?groupKey.id=nudge-users@esb.co.id` → catat `groups/{id}`.
- [ ] **Step 5:** `POST https://cloudidentity.googleapis.com/v1/groups/{id}/memberships` body `{"preferredMemberKey":{"id":"<email teman>"},"roles":[{"name":"MEMBER"}]}`.
- [ ] **Step 6: Keputusan.** Sukses (`"done": true` dan anggota muncul) → **GO**. "Access blocked"/403 → **NO-GO**, hentikan plan dan laporkan. "API not enabled" pada project Playground → ulangi Step 3–5 dengan client OAuth project `jira-u-bot` (Task 2 `--setup-groups-oauth` lalu `curl`).

---

### Task 1: Migrasi + state grup di `ms_user`

**Files:**
- Create: `migrations/0020_chat_group.sql`
- Modify: `migrations/migrate.go` (embed + `addColumnIfNotExists` setelah `boardScopeSQL`)
- Modify: `modules/chat_app/chat_app_dto/chat_app_dto.go` (`State`)
- Modify: `modules/chat_app/chat_app_repository/chat_app_repository_impl.go` (`stateColumns`, `scanState`)
- Test: `modules/chat_app/chat_app_repository/chat_app_repository_test.go`

**Interfaces:**
- Produces: `chat_app_dto.State.GroupAddedDate *time.Time`, `chat_app_dto.State.GroupError string` — dibaca Task 5.

- [ ] **Step 1: Write the failing test** (append ke `chat_app_repository_test.go`)

```go
func TestFindByEmail_ReadsGroupJoinState(t *testing.T) {
	repo, exec := setup(t)
	ctx := context.Background()
	s, _, err := repo.FindByEmail(ctx, testEmail)
	if err != nil || s.GroupAddedDate != nil || s.GroupError != "" {
		t.Fatalf("fresh user = %+v, %v; want no group state", s, err)
	}
	exec(`UPDATE ms_user SET chatGroupAddedDate = '2026-11-16 09:00:00', chatGroupError = 'boom' WHERE email = ?`, testEmail)
	s, _, err = repo.FindByEmail(ctx, testEmail)
	if err != nil || s.GroupAddedDate == nil || s.GroupError != "boom" {
		t.Fatalf("after update = %+v, %v; want added date and error", s, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/chat_app/chat_app_repository/ -run TestFindByEmail_ReadsGroupJoinState -v`
Expected: FAIL — compile error `s.GroupAddedDate undefined`.

- [ ] **Step 3: Implement**

`migrations/0020_chat_group.sql`:
```sql
-- Auto Join Google Group: when the user's email was added to the group that
-- makes the Nudge Chat App visible, and the last error doing so.
ALTER TABLE ms_user
  ADD COLUMN chatGroupAddedDate DATETIME NULL,
  ADD COLUMN chatGroupError VARCHAR(255) NULL;
```

`migrations/migrate.go` — tambah embed setelah `boardScopeSQL`:
```go
//go:embed 0020_chat_group.sql
var chatGroupSQL string
```
dan di `apply`, tepat setelah blok `boardScopeSQL`:
```go
	// Auto Join Google Group — one ALTER, so a duplicate-column error means it already ran.
	if err := addColumnIfNotExists(db, chatGroupSQL); err != nil {
		return err
	}
```

`chat_app_dto.State` — tambah field:
```go
	// GroupAddedDate: when the email joined the group that makes the app
	// visible (nil = not yet); GroupError: the last failure doing so.
	GroupAddedDate *time.Time
	GroupError     string
```

`chat_app_repository_impl.go`:
```go
const stateColumns = `email, chatAppSpaceID, chatAppUserID, chatAppActivatedDate, chatSender, chatGroupAddedDate, chatGroupError`

func scanState(row rowScanner) (chat_app_dto.State, error) {
	var s chat_app_dto.State
	var space, user, sender, groupErr sql.NullString
	var activated, groupAdded sql.NullTime
	if err := row.Scan(&s.Email, &space, &user, &activated, &sender, &groupAdded, &groupErr); err != nil {
		return chat_app_dto.State{}, err
	}
	s.SpaceID = space.String
	s.ChatUserID = user.String
	if activated.Valid {
		t := activated.Time
		s.ActivatedDate = &t
	}
	if sender.Valid {
		v := sender.String
		s.Sender = &v
	}
	if groupAdded.Valid {
		t := groupAdded.Time
		s.GroupAddedDate = &t
	}
	s.GroupError = groupErr.String
	return s, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./migrations/ ./modules/chat_app/... -v -count=1`
Expected: PASS (semua test lama tetap lulus).

- [ ] **Step 5: Commit**

```bash
git add migrations/0020_chat_group.sql migrations/migrate.go modules/chat_app/chat_app_dto/chat_app_dto.go modules/chat_app/chat_app_repository
git commit -m "feat(chat-group): store group join state on ms_user"
```

---

### Task 2: Config + token OAuth Groups + CLI setup

**Files:**
- Modify: `config/config.go`, `.env.example`
- Create: `modules/chat_group/oauth.go`, `modules/chat_group/oauth_test.go`
- Modify: `main.go` (`shouldRunSetupMode` → mode string)

**Interfaces:**
- Produces: `config.AppConfig.NudgeGroupEmail string`, `config.AppConfig.GoogleGroupsTokenPath string`; `chat_group.Scope` const; `chat_group.TokenSource(cfg *config.AppConfig) (oauth2.TokenSource, bool, error)` (`bool=false` = fitur mati); `chat_group.RunSetupFlow(cfg *config.AppConfig) error`.

- [ ] **Step 1: Write the failing test** (`modules/chat_group/oauth_test.go`)

```go
package chat_group

import (
	"os"
	"path/filepath"
	"testing"

	"nudge/config"
)

func TestTokenSource_OffWhenGroupOrTokenMissing(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "groups.json")
	cfg := &config.AppConfig{GoogleOAuthClientID: "id", GoogleOAuthClientSecret: "s", GoogleGroupsTokenPath: tokenPath}

	if _, on, err := TokenSource(cfg); on || err != nil {
		t.Fatalf("no group email: on=%v err=%v, want off", on, err)
	}
	cfg.NudgeGroupEmail = "nudge-users@esb.co.id"
	if _, on, err := TokenSource(cfg); on || err != nil {
		t.Fatalf("no token file: on=%v err=%v, want off", on, err)
	}
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if ts, on, err := TokenSource(cfg); !on || err != nil || ts == nil {
		t.Fatalf("configured: on=%v err=%v, want on", on, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/chat_group/ -run TestTokenSource -v`
Expected: FAIL — package/func undefined.

- [ ] **Step 3: Implement**

`config/config.go` — field (setelah `GeminiModel`):
```go
	// Auto Join Google Group: the group in the Chat App's Visibility list
	// (empty = feature off) and the operator's Groups-scoped OAuth token.
	NudgeGroupEmail       string
	GoogleGroupsTokenPath string
```
default + load:
```go
	v.SetDefault("GOOGLE_GROUPS_TOKEN_PATH", "./data/groups_oauth_token.json")
	// …
	cfg.NudgeGroupEmail = strings.TrimSpace(v.GetString("NUDGE_GROUP_EMAIL"))
	cfg.GoogleGroupsTokenPath = v.GetString("GOOGLE_GROUPS_TOKEN_PATH")
```

`.env.example` — tambahkan di akhir:
```
# Auto Join Google Group: group in the Chat App Visibility list (empty = off)
# and the operator token from `./nudge --setup-groups-oauth`.
NUDGE_GROUP_EMAIL=
GOOGLE_GROUPS_TOKEN_PATH=./data/groups_oauth_token.json
```

`modules/chat_group/oauth.go`:
```go
// Package chat_group adds Nudge users to the Google Group that makes the
// Nudge Chat App visible (Tech Spec "Auto Join Google Group Nudge").
package chat_group

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"nudge/config"
)

// Scope is the only scope the operator grants for this feature.
const Scope = "https://www.googleapis.com/auth/cloud-identity.groups"

func oauthConfig(cfg *config.AppConfig, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: cfg.GoogleOAuthClientID, ClientSecret: cfg.GoogleOAuthClientSecret,
		Endpoint: google.Endpoint, Scopes: []string{Scope}, RedirectURL: redirect,
	}
}

// TokenSource returns the operator's Groups token; on=false (no error) when
// the feature is not configured: no group email, no client, or no token file.
func TokenSource(cfg *config.AppConfig) (oauth2.TokenSource, bool, error) {
	if cfg.NudgeGroupEmail == "" || cfg.GoogleOAuthClientID == "" || cfg.GoogleOAuthClientSecret == "" {
		return nil, false, nil
	}
	b, err := os.ReadFile(cfg.GoogleGroupsTokenPath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, false, fmt.Errorf("chat_group: token file: %w", err)
	}
	return oauthConfig(cfg, "").TokenSource(context.Background(), &tok), true, nil
}

// RunSetupFlow is the one-time consent for `--setup-groups-oauth`: same
// loopback flow as the Chat token, but its own scope and token file.
func RunSetupFlow(cfg *config.AppConfig) error {
	oc := oauthConfig(cfg, "http://localhost:8089/")
	codeCh := make(chan string)
	srv := &http.Server{Addr: "localhost:8089", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Authorization complete — you can close this tab and return to the terminal.")
		go func() { codeCh <- r.URL.Query().Get("code") }()
	})}
	go srv.ListenAndServe()
	fmt.Println("Open this URL, log in as the OWNER of", cfg.NudgeGroupEmail, "and click Allow:")
	fmt.Println()
	fmt.Println(oc.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce))
	code := <-codeCh
	srv.Shutdown(context.Background())
	tok, err := oc.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.GoogleGroupsTokenPath), 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(tok)
	if err := os.WriteFile(cfg.GoogleGroupsTokenPath, b, 0600); err != nil {
		return err
	}
	fmt.Println("Success! Token saved to", cfg.GoogleGroupsTokenPath)
	return nil
}
```

`main.go` — ganti `shouldRunSetupMode` dengan:
```go
// setupMode returns the one-time OAuth consent flow requested on the
// command line ("" = normal start).
func setupMode(args []string) string {
	for _, a := range args[1:] {
		if a == "--setup-oauth" || a == "--setup-groups-oauth" {
			return a
		}
	}
	return ""
}
```
dan di awal `main()`:
```go
	if mode := setupMode(os.Args); mode != "" {
		var cfg config.AppConfig
		if err := config.LoadConfig(&cfg); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		run := chat_oauth_service.RunSetupFlow
		if mode == "--setup-groups-oauth" {
			run = chat_group.RunSetupFlow
		}
		if err := run(&cfg); err != nil {
			log.Fatalf("oauth setup failed: %v", err)
		}
		return
	}
```
(Perbarui test `shouldRunSetupMode` bila ada: `grep -rn shouldRunSetupMode *.go`.)

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./modules/chat_group/ ./config/ . -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add config/config.go .env.example modules/chat_group/oauth.go modules/chat_group/oauth_test.go main.go
git commit -m "feat(chat-group): separate Groups OAuth token and --setup-groups-oauth"
```

---

### Task 3: Client Cloud Identity Groups API

**Files:**
- Create: `modules/chat_group/client.go`, `modules/chat_group/client_test.go`

**Interfaces:**
- Consumes: `oauth2.TokenSource` (Task 2).
- Produces: `chat_group.NewClient(ts oauth2.TokenSource, groupEmail, baseURL string) *Client`; `(*Client).AddMember(ctx context.Context, email string) error`; `chat_group.ErrUnauthorized`; `chat_group.DefaultBaseURL = "https://cloudidentity.googleapis.com/v1"`.

- [ ] **Step 1: Write the failing test** (`client_test.go`)

```go
package chat_group

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func fakeAPI(t *testing.T, memberStatus int, calls *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(body))
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		switch {
		case r.URL.Path == "/groups:lookup":
			io.WriteString(w, `{"name":"groups/abc"}`)
		case r.URL.Path == "/groups/abc/memberships":
			w.WriteHeader(memberStatus)
			io.WriteString(w, `{"done":true}`)
		default:
			w.WriteHeader(404)
		}
	}))
}

func client(url string) *Client {
	return NewClient(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"}), "nudge-users@esb.co.id", url)
}

func TestAddMember_LooksUpOnceThenAdds(t *testing.T) {
	var calls []string
	srv := fakeAPI(t, 200, &calls)
	defer srv.Close()
	c := client(srv.URL)
	for _, e := range []string{"a@esb.co.id", "b@esb.co.id"} {
		if err := c.AddMember(context.Background(), e); err != nil {
			t.Fatalf("AddMember(%s) = %v", e, err)
		}
	}
	if len(calls) != 3 || !strings.Contains(calls[0], "groupKey.id=nudge-users%40esb.co.id") {
		t.Fatalf("calls = %v, want 1 lookup + 2 adds", calls)
	}
	if !strings.Contains(calls[1], `"preferredMemberKey":{"id":"a@esb.co.id"}`) || !strings.Contains(calls[1], `"roles":[{"name":"MEMBER"}]`) {
		t.Errorf("add body = %s", calls[1])
	}
}

func TestAddMember_ConflictMeansAlreadyMember(t *testing.T) {
	var calls []string
	srv := fakeAPI(t, 409, &calls)
	defer srv.Close()
	if err := client(srv.URL).AddMember(context.Background(), "a@esb.co.id"); err != nil {
		t.Fatalf("409 = %v, want nil", err)
	}
}

func TestAddMember_UnauthorizedAndOtherErrors(t *testing.T) {
	for status, want := range map[int]error{401: ErrUnauthorized} {
		var calls []string
		srv := fakeAPI(t, status, &calls)
		err := client(srv.URL).AddMember(context.Background(), "a@esb.co.id")
		srv.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
	var calls []string
	srv := fakeAPI(t, 403, &calls)
	defer srv.Close()
	err := client(srv.URL).AddMember(context.Background(), "a@esb.co.id")
	if err == nil || errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "403") {
		t.Errorf("403: err = %v, want a non-auth error mentioning 403", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/chat_group/ -run TestAddMember -v`
Expected: FAIL — `NewClient` undefined.

- [ ] **Step 3: Implement** (`client.go`)

```go
package chat_group

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const DefaultBaseURL = "https://cloudidentity.googleapis.com/v1"

// ErrUnauthorized: the operator token is invalid or revoked (HTTP 401).
var ErrUnauthorized = errors.New("chat_group: unauthorized")

// Client adds members to one Google Group through the Cloud Identity Groups API.
type Client struct {
	http  *http.Client
	base  string
	group string

	mu   sync.Mutex
	name string // "groups/{id}", cached after the first lookup
}

func NewClient(ts oauth2.TokenSource, groupEmail, baseURL string) *Client {
	hc := oauth2.NewClient(context.Background(), ts)
	hc.Timeout = 15 * time.Second
	return &Client{http: hc, base: baseURL, group: groupEmail}
}

func (c *Client) do(ctx context.Context, method, u string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	return res.StatusCode, b, nil
}

func (c *Client) groupName(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name != "" {
		return c.name, nil
	}
	status, body, err := c.do(ctx, http.MethodGet, c.base+"/groups:lookup?groupKey.id="+url.QueryEscape(c.group), nil)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("chat_group: lookup %s: HTTP %d: %s", c.group, status, body)
	}
	var out struct{ Name string `json:"name"` }
	if err := json.Unmarshal(body, &out); err != nil || out.Name == "" {
		return "", fmt.Errorf("chat_group: lookup %s: unexpected response %s", c.group, body)
	}
	c.name = out.Name
	return c.name, nil
}

// AddMember adds email as a MEMBER; already being a member (409) is success.
func (c *Client) AddMember(ctx context.Context, email string) error {
	name, err := c.groupName(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"preferredMemberKey": map[string]string{"id": email},
		"roles":              []map[string]string{{"name": "MEMBER"}},
	}
	status, resBody, err := c.do(ctx, http.MethodPost, c.base+"/"+name+"/memberships", body)
	switch {
	case err != nil:
		return err
	case status == http.StatusOK || status == http.StatusConflict:
		return nil
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	}
	return fmt.Errorf("chat_group: add %s: HTTP %d: %s", email, status, resBody)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./modules/chat_group/ -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/chat_group/client.go modules/chat_group/client_test.go
git commit -m "feat(chat-group): Cloud Identity Groups client (lookup + add member)"
```

---

### Task 4: Repository + service `EnsureMember` / sweep + wiring

**Files:**
- Create: `modules/chat_group/repository.go`, `modules/chat_group/service.go`, `modules/chat_group/service_test.go`
- Modify: `main.go` (login hook + worker)

**Interfaces:**
- Consumes: `(*Client).AddMember`, `ErrUnauthorized` (Task 3); `TokenSource` (Task 2).
- Produces: `chat_group.Store` interface; `chat_group.NewRepository(db *sql.DB) Store`; `chat_group.NewService(adder Adder, store Store) *Service`; `(*Service).EnsureMember(ctx, email string)`; `(*Service).Sweep(ctx) int`; `(*Service).Run(ctx)`.

- [ ] **Step 1: Write the failing test** (`service_test.go`)

```go
package chat_group

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeAdder struct {
	errs  map[string]error
	added []string
}

func (f *fakeAdder) AddMember(_ context.Context, email string) error {
	f.added = append(f.added, email)
	return f.errs[email]
}

type fakeStore struct {
	needs   map[string]bool
	pending []string
	marked  []string
	failed  []string
}

func (f *fakeStore) NeedsJoin(_ context.Context, email string) (bool, error) { return f.needs[email], nil }
func (f *fakeStore) Pending(_ context.Context, limit int) ([]string, error) {
	if len(f.pending) > limit {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}
func (f *fakeStore) MarkAdded(_ context.Context, email string) error {
	f.marked = append(f.marked, email)
	return nil
}
func (f *fakeStore) MarkFailed(_ context.Context, email, msg string) error {
	f.failed = append(f.failed, email+"|"+msg)
	return nil
}

func TestEnsureMember_AddsOnlyWhenNeeded(t *testing.T) {
	a := &fakeAdder{}
	st := &fakeStore{needs: map[string]bool{"new@esb.co.id": true}}
	svc := NewService(a, st)
	svc.EnsureMember(context.Background(), "new@esb.co.id")
	svc.EnsureMember(context.Background(), "done@esb.co.id")
	if strings.Join(a.added, ",") != "new@esb.co.id" || strings.Join(st.marked, ",") != "new@esb.co.id" {
		t.Errorf("added=%v marked=%v, want only the user that needs it", a.added, st.marked)
	}
}

func TestEnsureMember_RecordsAShortenedError(t *testing.T) {
	long := strings.Repeat("x", 400)
	a := &fakeAdder{errs: map[string]error{"new@esb.co.id": errors.New(long)}}
	st := &fakeStore{needs: map[string]bool{"new@esb.co.id": true}}
	NewService(a, st).EnsureMember(context.Background(), "new@esb.co.id")
	if len(st.marked) != 0 || len(st.failed) != 1 || len(st.failed[0]) > len("new@esb.co.id|")+255 {
		t.Errorf("marked=%v failed=%v, want one error ≤255 chars", st.marked, st.failed)
	}
}

func TestSweep_CapsAtFiftyAndStopsOnUnauthorized(t *testing.T) {
	var pending []string
	for i := 0; i < 60; i++ {
		pending = append(pending, string(rune('a'+i%26))+strings.Repeat("y", i)+"@esb.co.id")
	}
	a := &fakeAdder{}
	svc := NewService(a, &fakeStore{pending: pending})
	svc.pause = 0
	if n := svc.Sweep(context.Background()); n != 50 || len(a.added) != 50 {
		t.Fatalf("sweep = %d added %d, want 50", n, len(a.added))
	}

	a2 := &fakeAdder{errs: map[string]error{pending[1]: ErrUnauthorized}}
	st2 := &fakeStore{pending: pending[:5]}
	svc2 := NewService(a2, st2)
	svc2.pause = 0
	if n := svc2.Sweep(context.Background()); n != 1 || len(a2.added) != 2 || len(st2.failed) != 0 {
		t.Errorf("sweep = %d added=%v failed=%v, want stop at the 401 without blaming the user", n, a2.added, st2.failed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/chat_group/ -run 'TestEnsureMember|TestSweep' -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 3: Implement**

`repository.go`:
```go
package chat_group

import (
	"context"
	"database/sql"
)

// Store is the join state on ms_user.
type Store interface {
	// NeedsJoin: active, not yet added, and has not activated the app.
	NeedsJoin(ctx context.Context, email string) (bool, error)
	// Pending lists up to limit such users (oldest first).
	Pending(ctx context.Context, limit int) ([]string, error)
	MarkAdded(ctx context.Context, email string) error
	MarkFailed(ctx context.Context, email, msg string) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Store { return &repository{db: db} }

const needsJoin = `flagActive = 1 AND chatGroupAddedDate IS NULL AND (chatAppSpaceID IS NULL OR chatAppSpaceID = '')`

func (r *repository) NeedsJoin(ctx context.Context, email string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ms_user WHERE email = ? AND `+needsJoin, email).Scan(&n)
	return n > 0, err
}

func (r *repository) Pending(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT email FROM ms_user WHERE `+needsJoin+` ORDER BY userID LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *repository) MarkAdded(ctx context.Context, email string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET chatGroupAddedDate = CURRENT_TIMESTAMP, chatGroupError = NULL WHERE email = ?`, email)
	return err
}

func (r *repository) MarkFailed(ctx context.Context, email, msg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET chatGroupError = ? WHERE email = ?`, msg, email)
	return err
}
```

`service.go`:
```go
package chat_group

import (
	"context"
	"errors"
	"log"
	"time"
)

const (
	sweepEvery = time.Hour
	sweepLimit = 50
	maxErrLen  = 255
)

// Adder is the Groups API call (Client.AddMember).
type Adder interface {
	AddMember(ctx context.Context, email string) error
}

type Service struct {
	adder Adder
	store Store
	pause time.Duration // between sweep calls; 0 in tests
}

func NewService(adder Adder, store Store) *Service {
	return &Service{adder: adder, store: store, pause: 200 * time.Millisecond}
}

// join adds one email and records the outcome; ErrUnauthorized is returned
// unrecorded (the operator token is broken, not the user).
func (s *Service) join(ctx context.Context, email string) error {
	err := s.adder.AddMember(ctx, email)
	switch {
	case err == nil:
		if mErr := s.store.MarkAdded(ctx, email); mErr != nil {
			log.Printf("chat_group: marking %s added failed: %v", email, mErr)
		}
		return nil
	case errors.Is(err, ErrUnauthorized):
		return err
	}
	msg := err.Error()
	if len(msg) > maxErrLen {
		msg = msg[:maxErrLen]
	}
	log.Printf("chat_group: adding %s failed: %v", email, err)
	if mErr := s.store.MarkFailed(ctx, email, msg); mErr != nil {
		log.Printf("chat_group: recording the failure of %s failed: %v", email, mErr)
	}
	return err
}

// EnsureMember adds email to the group when it still needs to be (login hook).
func (s *Service) EnsureMember(ctx context.Context, email string) {
	need, err := s.store.NeedsJoin(ctx, email)
	if err != nil {
		log.Printf("chat_group: NeedsJoin(%s) failed: %v", email, err)
		return
	}
	if !need {
		return
	}
	if err := s.join(ctx, email); errors.Is(err, ErrUnauthorized) {
		log.Printf("chat_group: operator Groups token rejected (401) — run --setup-groups-oauth again")
	}
}

// Sweep adds up to sweepLimit pending users and returns how many joined;
// it stops at the first 401.
func (s *Service) Sweep(ctx context.Context) int {
	emails, err := s.store.Pending(ctx, sweepLimit)
	if err != nil {
		log.Printf("chat_group: listing pending users failed: %v", err)
		return 0
	}
	joined := 0
	for _, e := range emails {
		err := s.join(ctx, e)
		if errors.Is(err, ErrUnauthorized) {
			log.Printf("chat_group: operator Groups token rejected (401) — sweep stopped; run --setup-groups-oauth again")
			break
		}
		if err == nil {
			joined++
		}
		select {
		case <-ctx.Done():
			return joined
		case <-time.After(s.pause):
		}
	}
	return joined
}

// Run sweeps now and then every hour until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		if n := s.Sweep(ctx); n > 0 {
			log.Printf("chat_group: %d user(s) added to the Nudge group", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
```

`main.go` — sebelum `authHandler := auth_handler.NewAuthHandler(...)`:
```go
	// Auto Join Google Group (off unless NUDGE_GROUP_EMAIL + token are set).
	var chatGroupSvc *chat_group.Service
	if ts, on, err := chat_group.TokenSource(&cfg); err != nil {
		log.Printf("main: Groups token unreadable, auto join off: %v", err)
	} else if on {
		chatGroupSvc = chat_group.NewService(chat_group.NewClient(ts, cfg.NudgeGroupEmail, chat_group.DefaultBaseURL), chat_group.NewRepository(db))
	}
```
di dalam login hook yang sudah ada (setelah `SavePicture`):
```go
				if chatGroupSvc != nil {
					email := claims.Email
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						chatGroupSvc.EnsureMember(ctx, email)
					}()
				}
```
dan worker (setelah worker `chat_log.RunRetention`):
```go
	if chatGroupSvc != nil {
		shutdownMgr.AddWorker()
		go func() {
			defer shutdownMgr.WorkerDone()
			chatGroupSvc.Run(shutdownMgr.Context())
		}()
	}
```
Catatan: `authHandler` dibuat sebelum `shutdownMgr`; pastikan `chatGroupSvc` dideklarasikan sebelum keduanya (baris di atas `authHandler`).

- [ ] **Step 4: Run tests**

Run: `go build ./... && go vet ./... && go test ./modules/chat_group/ -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/chat_group/repository.go modules/chat_group/service.go modules/chat_group/service_test.go main.go
git commit -m "feat(chat-group): join the Nudge group on login and hourly sweep"
```

---

### Task 5: `groupState` di status chat-app

**Files:**
- Modify: `modules/chat_app_activation/chat_app_activation.go`
- Modify: `main.go` (`GroupEnabled: chatGroupSvc != nil` di `chat_app_activation.Deps`)
- Test: `modules/chat_app_activation/chat_app_activation_test.go` (tambah test; ikuti fake `StateStore` yang ada di file test itu)

**Interfaces:**
- Consumes: `chat_app_dto.State.GroupAddedDate`, `.GroupError` (Task 1).
- Produces: JSON `groupState` (`"none"|"pending"|"ready"|"failed"`) di `GET /api/dashboard/chat-app/status`; `chat_app_activation.Deps.GroupEnabled bool`; fungsi murni `GroupState(s chat_app_dto.State, enabled bool, now time.Time) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestGroupState(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	ago := func(h int) *time.Time { t := now.Add(-time.Duration(h) * time.Hour); return &t }
	cases := []struct {
		name    string
		s       chat_app_dto.State
		enabled bool
		want    string
	}{
		{"feature off", chat_app_dto.State{}, false, "none"},
		{"activated", chat_app_dto.State{SpaceID: "spaces/x", GroupError: "boom"}, true, "ready"},
		{"failed", chat_app_dto.State{GroupError: "boom"}, true, "failed"},
		{"not processed yet", chat_app_dto.State{}, true, "pending"},
		{"added 2h ago", chat_app_dto.State{GroupAddedDate: ago(2)}, true, "pending"},
		{"added 25h ago", chat_app_dto.State{GroupAddedDate: ago(25)}, true, "ready"},
	}
	for _, c := range cases {
		if got := GroupState(c.s, c.enabled, now); got != c.want {
			t.Errorf("%s: GroupState = %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/chat_app_activation/ -run TestGroupState -v`
Expected: FAIL — `GroupState` undefined.

- [ ] **Step 3: Implement**

```go
// groupPropagation is how long Google may take to apply a group change.
const groupPropagation = 24 * time.Hour

// GroupState says whether the app is visible to the user yet (Tech Spec
// "Auto Join Google Group Nudge", AC2.1).
func GroupState(s chat_app_dto.State, enabled bool, now time.Time) string {
	switch {
	case !enabled:
		return "none"
	case s.Activated():
		return "ready"
	case s.GroupError != "":
		return "failed"
	case s.GroupAddedDate == nil || now.Sub(*s.GroupAddedDate) < groupPropagation:
		return "pending"
	}
	return "ready"
}
```
`Deps` tambah `GroupEnabled bool`; `StatusResponse` tambah `GroupState string \`json:"groupState"\``; di `status()` isi `GroupState: GroupState(s, h.d.GroupEnabled && h.d.AppEnabled, time.Now())`. Di `main.go` tambahkan `GroupEnabled: chatGroupSvc != nil` pada `chat_app_activation.Deps{...}`.

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./modules/chat_app_activation/ -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/chat_app_activation main.go
git commit -m "feat(chat-group): expose groupState on chat-app status"
```

---

### Task 6: Banner dashboard pending/failed

**Files:**
- Modify: `frontend/src/app/core/services/chat-app.service.ts`
- Modify: `frontend/src/app/shared/ui/chat-app-banner.component.ts`
- Test: `frontend/src/app/shared/ui/chat-app-banner.component.spec.ts` (buat bila belum ada; ikuti pola spec komponen shared lain)

**Interfaces:**
- Consumes: `groupState` dari Task 5.

- [ ] **Step 1: Write the failing test**

```ts
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { signal } from '@angular/core';
import { ChatAppBannerComponent } from './chat-app-banner.component';
import { ChatAppService, ChatAppStatus } from '../../core/services/chat-app.service';

const base: ChatAppStatus = {
  enabled: true, activated: false, activatedDate: null, oneClick: true, sender: null,
  effectiveSender: 'operator', operatorAccount: false, operatorName: 'Yusuf', groupState: 'ready',
};

function render(status: ChatAppStatus) {
  const svc = { status: signal<ChatAppStatus | null>(status), refresh: () => undefined, activateUrl: () => '/x' };
  TestBed.configureTestingModule({ imports: [ChatAppBannerComponent], providers: [provideRouter([]), { provide: ChatAppService, useValue: svc }] });
  const f = TestBed.createComponent(ChatAppBannerComponent);
  f.detectChanges();
  return f.nativeElement as HTMLElement;
}

describe('ChatAppBannerComponent groupState', () => {
  beforeEach(() => { try { localStorage.removeItem('nudge.chatAppBanner.snoozedUntil'); } catch { /* ignore */ } });

  it('explains the wait while the group change propagates', () => {
    const el = render({ ...base, groupState: 'pending' });
    expect(el.querySelector('[data-group-pending]')?.textContent).toContain('App Nudge sedang disiapkan untuk akunmu (maks. 24 jam)');
    expect(el.querySelector('[data-activate]')).not.toBeNull();
  });

  it('tells the user to contact the admin when joining failed', () => {
    const el = render({ ...base, groupState: 'failed' });
    expect(el.querySelector('[data-group-failed]')?.textContent).toContain('Hubungi admin Nudge');
  });

  it('keeps the normal text when ready', () => {
    const el = render(base);
    expect(el.querySelector('[data-group-pending]')).toBeNull();
    expect(el.textContent).toContain('Notifikasi bisa datang langsung dari app');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include=src/app/shared/ui/chat-app-banner.component.spec.ts`
Expected: FAIL — `groupState` not in type / selectors missing.

- [ ] **Step 3: Implement**

`chat-app.service.ts` — di `ChatAppStatus`:
```ts
  /** Visibility through the Nudge Google Group (Tech Spec "Auto Join Google Group Nudge"). */
  groupState: 'none' | 'pending' | 'ready' | 'failed';
```
`chat-app-banner.component.ts` — ganti blok `<p>…</p>` dengan:
```html
        <p>
          @if (s.groupState === 'pending') {
            <span data-group-pending>App Nudge sedang disiapkan untuk akunmu (maks. 24 jam). Sementara itu notifikasi tetap dikirim lewat DM.</span>
          } @else if (s.groupState === 'failed') {
            <span data-group-failed>Akunmu belum bisa ditambahkan ke app Nudge. Hubungi admin Nudge.</span>
          } @else if (s.operatorAccount) {
            Akunmu hanya bisa menerima notifikasi lewat app <strong>Nudge</strong>. Aktifkan sekarang.
          } @else {
            Notifikasi bisa datang langsung dari app <strong>Nudge</strong> di Google Chat, bukan dari akun {{ s.operatorName }}.
          }
        </p>
```

- [ ] **Step 4: Run tests + build**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: semua test PASS, build sukses.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/core/services/chat-app.service.ts frontend/src/app/shared/ui/chat-app-banner.component.ts frontend/src/app/shared/ui/chat-app-banner.component.spec.ts
git commit -m "feat(chat-group): banner explains pending and failed group join"
```

---

### Task 7: Dokumentasi, deploy, aktivasi di produksi

**Files:**
- Modify: `README.md`

- [ ] **Step 1: README** — tambah di bagian Chat App:

```markdown
- **Auto Join Google Group**: app Nudge terlihat oleh anggota grup `NUDGE_GROUP_EMAIL` (dipasang sekali di Chat API › Visibility). User yang login web otomatis ditambahkan ke grup (asinkron + sweep tiap jam, maks 50/run) lewat Cloud Identity Groups API dengan token operator terpisah (`./nudge --setup-groups-oauth`, scope `cloud-identity.groups`, file `GOOGLE_GROUPS_TOKEN_PATH`). Perubahan grup bisa butuh ≤ 24 jam; selama itu banner dashboard bilang "sedang disiapkan". Kosongkan `NUDGE_GROUP_EMAIL` untuk mematikan.
```

- [ ] **Step 2: Full verification**

Run: `go vet ./... && go test ./... && cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: semua PASS.

- [ ] **Step 3: Commit + merge + deploy** (hanya bila user sudah setuju deploy)

```bash
git add README.md
git commit -m "docs(chat-group): document auto join Google Group"
git push origin feat/nudge
git checkout main && git merge --no-ff feat/nudge -m "Merge branch 'feat/nudge'" && git push origin main && git checkout feat/nudge
```
Tunggu release `…-<sha main>` aktif di `/var/www/nudge/current`.

- [ ] **Step 4: Token di lokal, upload ke server**

Di mesin lokal (butuh browser): `go run . --setup-groups-oauth` dengan `.env` lokal berisi `GOOGLE_OAUTH_CLIENT_ID/SECRET` yang sama dengan server dan `GOOGLE_GROUPS_TOKEN_PATH=./data/groups_oauth_token.json`; login sebagai pemilik grup. Upload file ke `/var/www/nudge/shared/data/groups_oauth_token.json` (chmod 600).

- [ ] **Step 5: Aktifkan di server**

Tambah ke `/var/www/nudge/shared/.env`:
```
NUDGE_GROUP_EMAIL=nudge-users@esb.co.id
GOOGLE_GROUPS_TOKEN_PATH=/var/www/nudge/shared/data/groups_oauth_token.json
```
`systemctl restart nudge`.

- [ ] **Step 6: Visibility**

Cloud Console → Google Chat API → Configuration → Visibility: `yusuf.wijaya@esb.co.id, nudge-users@esb.co.id` → **Save**.

- [ ] **Step 7: Verifikasi**

- `journalctl -u nudge | grep chat_group` → "N user(s) added to the Nudge group".
- `SELECT email, chatGroupAddedDate, chatGroupError FROM ms_user;` → 6 user (selain yang sudah aktivasi) punya `chatGroupAddedDate`.
- groups.google.com → anggota grup bertambah.
- Dashboard user non-aktivasi menampilkan banner "sedang disiapkan".
