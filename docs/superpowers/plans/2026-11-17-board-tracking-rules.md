# Aturan Board (Admin CRUD Tracking Rules) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aturan tracking per project Jira (status dihitung, status selesai, status review, field Story Point baca/tulis) pindah dari konstanta Go ke tabel `ms_tracking_rule` yang dikelola admin di halaman **Admin › Aturan Board**, tanpa mengubah perilaku OMS & CORE yang sudah ada.

**Architecture:** Tabel `ms_tracking_rule` (seed OMS & CORE identik dengan kode lama) dibaca modul baru `modules/tracking_rule` lalu dipasang ke **registry in-memory** di `jira_client_dto` (`atomic.Pointer` snapshot). Fungsi publik `RuleFor`/`RuleForIssue`/`TrackedIssuesJQL`/`IsTrackedProjectKey` tetap ada tetapi membaca snapshot; JQL `project in (...)`, daftar `fields=` dan pembacaan Story Point jadi dinamis per project. Registry dimuat sinkron saat start (sebelum poller/backfill), di-refresh setiap simpan admin dan tiap 5 menit. Frontend menambah halaman daftar + form (lookup Jira, chip status, field SP berurutan, preview kalimat) dan Pengaturan memakai status workflow board dari Jira.

**Tech Stack:** Go 1.26 + Gin 1.12 + MySQL (`database/sql`, `go-sql-driver/mysql`), `sync/atomic`, Angular 19 standalone + signals, Jasmine/Karma.

**Spec:** `docs/tech-spec/board-tracking-rules/Tech Spec - Aturan Board (Admin CRUD).md`

## Global Constraints

- Branch tetap `feat/nudge` (keputusan user untuk seluruh pekerjaan Nudge — tidak membuat branch baru).
- Migrasi: `migrations/0021_tracking_rule.sql` (CREATE TABLE IF NOT EXISTS) + `migrations/0021b_tracking_rule_seed.sql` (`INSERT IGNORE`, tidak menimpa editan admin — AC1.3). `0020` sudah dipakai `chat_group`.
- Seed persis: OMS → tracked `["Code Process"]`, done `Code Review`, review NULL, SP baca `["customfield_10016"]`, tulis card `customfield_10016`, tulis subtask `customfield_10016`; CORE → tracked `["Code Process","CODE REVIEW"]`, done `Code Completed`, review `CODE REVIEW`, SP baca `["customfield_10125","customfield_11053"]`, tulis card `customfield_10125`, tulis subtask `customfield_11053`.
- Project key: `TRIM` + `UPPER`, wajib `^[A-Z][A-Z0-9_]{1,19}$`.
- "Hapus" = nonaktifkan (`isActive = 0`). Menonaktifkan aturan aktif terakhir → `422 last_active_rule`.
- Registry: builtin default = seed (dipakai sampai load pertama & di unit test); startup `Reload` sinkron sebelum poller/backfill, gagal → `log.Fatalf`; refresh setiap simpan + tiap **5 menit**; refresh gagal → ruleset lama dipakai (log `ERROR`), tidak pernah jatuh ke kosong.
- Tidak ada aturan aktif → `TrackedIssuesJQL()` = `""`, poller skip cycle (bukan error Jira), log `WARN`.
- Sesi tiket project yang dinonaktifkan ditutup dengan `CloseSession(key, 0)` — tanpa menambah waktu, tanpa notifikasi.
- Cache: lookup Jira per project **10 menit**; status board (Pengaturan) **1 jam**.
- Error API: bentuk `{error, message?, fields?}`; kode `400 validation_failed|invalid_project_key|invalid_body|invalid_board_id`, `404 project_not_found|rule_not_found|board_not_found`, `409 rule_exists|already_running`, `422 last_active_rule`, `502 jira_unavailable`.
- Route gin param board memakai `:boardId` (harus sama dengan `/boards/:boardId/avatar` yang sudah ada — gin panic bila nama wildcard beda).
- UI: dropdown pakai `app-select`; kontrol custom; transisi fade/slide, chip pop-in, hover/focus, dan `prefers-reduced-motion`.
- Test Go yang menyentuh MySQL memakai `nudge/base/testutil.OpenTestDB`; key test memakai prefix `ZZTRT` dan dibersihkan lewat `t.Cleanup`.
- Test Go yang memanggil `jira_client_dto.SetRules` wajib mengembalikan builtin: `t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })`.
- Verifikasi backend: `go build ./... && go vet ./... && go test ./... -count=1` (MySQL lokal port 3308). Frontend (di `frontend/`): `npx ng test --watch=false --browsers=ChromeHeadless` + `npx ng build`.
- Commit: `git commit -m "feat(tracking-rules): …" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"` (pesan diakhiri baris kosong + Co-Authored-By).

---

## File Structure

| File | Tanggung jawab |
|------|----------------|
| `migrations/0021_tracking_rule.sql`, `migrations/0021b_tracking_rule_seed.sql`, `migrations/migrate.go`, `migrations/migrate_test.go` | Tabel + seed OMS/CORE |
| `modules/jira_client/jira_client_dto/tracking_rules.go` (+ test) | Registry atomic: `TrackingRule` diperluas, `SetRules`, `BuiltinRules`, `ActiveProjectKeys`, `StoryPointFieldIDs`, `ProjectsJQLIn`, `IsTrackedProject`, `RuleFor`/`TrackedIssuesJQL` dari snapshot |
| `modules/jira_client/jira_client_dto/issue_relations.go` | `IsTrackedProjectKey` dari registry |
| `modules/jira_client/jira_client_dto/jira_client_dto.go` (+ test) | `IssueFields.Numbers` + `UnmarshalJSON`, `FirstNumber`, `Issue.StoryPoints()`; field SP bertipe + `EffectiveStoryPoints` dihapus |
| `modules/jira_client/jira_client_dto/constants.go` | Komentar: konstanta SP hanya untuk builtin/seed/test |
| `modules/jira_client/jira_client_dto/project_meta.go` | DTO Jira project / statuses / field |
| `modules/jira_client/jira_client_service/jira_agile_client.go`, `jira_client_service_impl.go` (+ tests) | `issueFieldsParam` dinamis, JQL `project in` dari registry, skip bila kosong |
| `modules/jira_client/jira_client_service/jira_project_meta.go` (+ test) | `ProjectMetaClient`: `GetProject`, `GetProjectStatuses`, `GetNumberFields` |
| `modules/poller/poller_service/poller_service_impl.go` (+ test) | `Issue.StoryPoints()`, skip cycle bila JQL kosong, sesi project nonaktif → `CloseSession(key, 0)` |
| `modules/sprint_backfill/.../sprint_backfill_service_impl.go` (+ test), `modules/user_dashboard/.../user_dashboard_service_impl.go` (+ test) | `Issue.StoryPoints()` |
| `modules/issue_create/issue_create_service/meta.go`, `service.go` (+ test) | Field SP tulis per jenis card & `handled` dari rule project |
| `modules/tracking_rule/dto.go` | `Rule`, `RuleView`, `RuleInput`, `Lookup`, error sentinel, `ValidationError` |
| `modules/tracking_rule/repository.go` (+ test) | CRUD `ms_tracking_rule` |
| `modules/tracking_rule/service.go`, `validate.go` (+ tests) | CRUD + validasi ke data Jira, lookup cache, `Reload`, `RunRefresh`, `Recalculate`, `BoardStatuses` |
| `modules/tracking_rule/handler.go` (+ test) | HTTP + `AdminRoutes` / `UserRoutes` |
| `router.go`, `main.go` | `MountTrackingRules`; Reload sinkron sebelum poller; worker refresh |
| `frontend/src/app/core/services/tracking-rule.service.ts` (+ spec) | HTTP client |
| `frontend/src/app/features/admin/tracking-rules/recalculate-action.ts` | Aksi toast "Hitung ulang semua user" |
| `frontend/src/app/features/admin/tracking-rules/tracking-rules.component.{ts,html,scss,spec.ts}` | Halaman daftar |
| `frontend/src/app/features/admin/tracking-rules/tracking-rule-form.component.{ts,html,scss,spec.ts}` | Form tambah/edit |
| `frontend/src/app/app.routes.ts`, `layouts/admin-shell/admin-shell.component.{html,spec.ts}` | Route + menu "Aturan Board" |
| `frontend/src/app/features/dashboard/settings/settings.component.ts` (+ spec) | Status dari API, fallback statis |
| `README.md` | Dokumentasi |

---

### Task 1: Migrasi `ms_tracking_rule` + seed OMS & CORE

**Files:**
- Create: `migrations/0021_tracking_rule.sql`, `migrations/0021b_tracking_rule_seed.sql`
- Modify: `migrations/migrate.go` (embed setelah `chatGroupSQL`; apply setelah blok `chatGroupSQL`, sebelum `return runOnce(...)`)
- Test: `migrations/migrate_test.go`

**Interfaces:**
- Produces: tabel `ms_tracking_rule` (kolom persis spec) berisi baris `OMS` & `CORE` — dibaca Task 7.

- [ ] **Step 1: Write the failing test** (append ke `migrations/migrate_test.go`; tambah import `"encoding/json"` dan `"reflect"`)

```go
func TestApply_SeedsTrackingRulesWithoutOverwritingEdits(t *testing.T) {
	db := openTestConn(t)
	if err := Apply(db); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	type seeded struct {
		tracked, fields       []string
		done, write, writeSub string
		review                sql.NullString
	}
	read := func(key string) seeded {
		t.Helper()
		var s seeded
		var tracked, fields string
		err := db.QueryRow(`SELECT trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask
			FROM ms_tracking_rule WHERE projectKey = ?`, key).Scan(&tracked, &s.done, &s.review, &fields, &s.write, &s.writeSub)
		if err != nil {
			t.Fatalf("seed row %s: %v", key, err)
		}
		if err := json.Unmarshal([]byte(tracked), &s.tracked); err != nil {
			t.Fatalf("trackedStatuses of %s: %v", key, err)
		}
		if err := json.Unmarshal([]byte(fields), &s.fields); err != nil {
			t.Fatalf("storyPointFields of %s: %v", key, err)
		}
		return s
	}

	core := read("CORE")
	if !reflect.DeepEqual(core.tracked, []string{"Code Process", "CODE REVIEW"}) || core.done != "Code Completed" ||
		core.review.String != "CODE REVIEW" || !reflect.DeepEqual(core.fields, []string{"customfield_10125", "customfield_11053"}) ||
		core.write != "customfield_10125" || core.writeSub != "customfield_11053" {
		t.Errorf("CORE seed = %+v", core)
	}
	oms := read("OMS")
	if !reflect.DeepEqual(oms.tracked, []string{"Code Process"}) || oms.done != "Code Review" || oms.review.Valid ||
		!reflect.DeepEqual(oms.fields, []string{"customfield_10016"}) || oms.write != "customfield_10016" || oms.writeSub != "customfield_10016" {
		t.Errorf("OMS seed = %+v", oms)
	}

	// AC1.3: a restart/deploy must not overwrite what an admin edited.
	var original string
	if err := db.QueryRow(`SELECT projectName FROM ms_tracking_rule WHERE projectKey = 'OMS'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`UPDATE ms_tracking_rule SET projectName = ? WHERE projectKey = 'OMS'`, original) })
	if _, err := db.Exec(`UPDATE ms_tracking_rule SET projectName = 'ESB OMS (edited)' WHERE projectKey = 'OMS'`); err != nil {
		t.Fatal(err)
	}
	if err := Apply(db); err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}
	var name string
	db.QueryRow(`SELECT projectName FROM ms_tracking_rule WHERE projectKey = 'OMS'`).Scan(&name)
	if name != "ESB OMS (edited)" {
		t.Errorf("projectName after re-apply = %q, the seed must not overwrite an admin edit", name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./migrations/ -run TestApply_SeedsTrackingRules -v -count=1`
Expected: FAIL — `Table 'jira_db.ms_tracking_rule' doesn't exist`.

- [ ] **Step 3: Implement**

`migrations/0021_tracking_rule.sql`:
```sql
-- Aturan Board (Tech Spec "Aturan Board (Admin CRUD)"): which statuses of a
-- Jira project count toward a ticket's budget and where its Story Points live.
CREATE TABLE IF NOT EXISTS ms_tracking_rule (
  projectKey                  VARCHAR(20)  NOT NULL                COMMENT 'Jira project key, UPPER',
  projectName                 VARCHAR(255) NOT NULL DEFAULT ''     COMMENT 'Project name cached from Jira',
  trackedStatuses             JSON         NOT NULL                COMMENT 'Statuses the budget clock runs in',
  doneStatus                  VARCHAR(100) NOT NULL                COMMENT 'Finished status (early completion)',
  reviewStatus                VARCHAR(100) NULL                    COMMENT 'Tracked review status; must be in trackedStatuses',
  storyPointFields            JSON         NOT NULL                COMMENT 'customfield ids, order = read priority',
  storyPointWriteField        VARCHAR(50)  NOT NULL                COMMENT 'SP field Buat Card writes for a normal card',
  storyPointWriteFieldSubtask VARCHAR(50)  NOT NULL                COMMENT 'SP field Buat Card writes for a subtask',
  isActive                    TINYINT(1)   NOT NULL DEFAULT 1,
  editedBy                    VARCHAR(255) NOT NULL DEFAULT 'seed' COMMENT 'Last admin email',
  editedDate                  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (projectKey)
);
```

`migrations/0021b_tracking_rule_seed.sql`:
```sql
-- OMS & CORE exactly as the old hardcoded rules. INSERT IGNORE: never
-- overwrites an admin's edits on later starts (AC1.3).
INSERT IGNORE INTO ms_tracking_rule (projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask) VALUES
  ('OMS',  'ESB OMS',  JSON_ARRAY('Code Process'),                'Code Review',    NULL,          JSON_ARRAY('customfield_10016'),                      'customfield_10016', 'customfield_10016'),
  ('CORE', 'ESB Core', JSON_ARRAY('Code Process', 'CODE REVIEW'), 'Code Completed', 'CODE REVIEW', JSON_ARRAY('customfield_10125', 'customfield_11053'), 'customfield_10125', 'customfield_11053');
```

`migrations/migrate.go` — setelah `var chatGroupSQL string`:
```go
//go:embed 0021_tracking_rule.sql
var trackingRuleSQL string

//go:embed 0021b_tracking_rule_seed.sql
var trackingRuleSeedSQL string
```
dan di `apply`, tepat setelah blok `chatGroupSQL` (sebelum komentar "The new toggle starts ON…"):
```go
	// Aturan Board — CREATE TABLE IF NOT EXISTS + INSERT IGNORE seed of OMS
	// & CORE (identical to the old hardcoded rules); both idempotent.
	if _, err := db.Exec(trackingRuleSQL); err != nil {
		return err
	}
	if _, err := db.Exec(trackingRuleSeedSQL); err != nil {
		return err
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./migrations/ -v -count=1`
Expected: PASS (semua test migrasi lama tetap lulus).

- [ ] **Step 5: Commit**

```bash
git add migrations/0021_tracking_rule.sql migrations/0021b_tracking_rule_seed.sql migrations/migrate.go migrations/migrate_test.go
git commit -m "feat(tracking-rules): add ms_tracking_rule with OMS and CORE seed" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Registry in-memory di `jira_client_dto`

**Files:**
- Modify: `modules/jira_client/jira_client_dto/tracking_rules.go` (ganti seluruh bagian dari atas file sampai akhir `TrackedIssuesJQL`, baris 1–108; `TrackedRunTimes`/`TrackedSeconds` tetap)
- Modify: `modules/jira_client/jira_client_dto/issue_relations.go:39-47`
- Test: `modules/jira_client/jira_client_dto/tracking_rules_test.go`

**Interfaces:**
- Produces (package `jira_client_dto`):
  - `type TrackingRule struct { ProjectKey string; TrackedStatuses []string; DoneStatus string; ReviewStatus string; StoryPointFields []string; StoryPointWriteField string; StoryPointWriteFieldSubtask string }` (method `IsTracked/IsDone/IsReview/TracksReview` tetap)
  - `func BuiltinRules() []TrackingRule` — identik seed, urut `[CORE, OMS]`
  - `func SetRules(active []TrackingRule)` — semua rule yang diberikan dianggap aktif; key di-UPPER; rule tanpa `TrackedStatuses` diabaikan
  - `func ActiveProjectKeys() []string` (urut), `func StoryPointFieldIDs() []string` (gabungan field baca, urut project lalu prioritas, tanpa duplikat), `func ProjectsJQLIn() string` (`"project in (CORE, OMS)"` atau `""`), `func IsTrackedProject(projectKey string) bool`
  - `RuleFor`, `RuleForIssue`, `TrackedIssuesJQL` (`""` bila kosong), `IsTrackedProjectKey` — tanda tangan tidak berubah.

- [ ] **Step 1: Write the failing test** (append ke `tracking_rules_test.go`; tambah import `"reflect"`)

```go
// withRules swaps the registry for one test and restores the builtin rules.
func withRules(t *testing.T, rules ...TrackingRule) {
	t.Helper()
	SetRules(rules)
	t.Cleanup(func() { SetRules(BuiltinRules()) })
}

func TestBuiltinRules_MatchTheSeed(t *testing.T) {
	core, oms := RuleFor("CORE"), RuleFor("OMS")
	if !reflect.DeepEqual(core.StoryPointFields, []string{"customfield_10125", "customfield_11053"}) ||
		core.StoryPointWriteField != "customfield_10125" || core.StoryPointWriteFieldSubtask != "customfield_11053" {
		t.Errorf("CORE = %+v", core)
	}
	if !reflect.DeepEqual(oms.StoryPointFields, []string{"customfield_10016"}) ||
		oms.StoryPointWriteField != "customfield_10016" || oms.StoryPointWriteFieldSubtask != "customfield_10016" {
		t.Errorf("OMS = %+v", oms)
	}
	if got := ProjectsJQLIn(); got != "project in (CORE, OMS)" {
		t.Errorf("ProjectsJQLIn() = %q", got)
	}
	if got := StoryPointFieldIDs(); !reflect.DeepEqual(got, []string{"customfield_10125", "customfield_11053", "customfield_10016"}) {
		t.Errorf("StoryPointFieldIDs() = %v", got)
	}
}

func TestSetRules_DrivesJQLKeysAndFields(t *testing.T) {
	withRules(t,
		TrackingRule{ProjectKey: "pos", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done", StoryPointFields: []string{"customfield_1", "customfield_2"}},
		TrackingRule{ProjectKey: "OMS", TrackedStatuses: []string{"Code Process"}, DoneStatus: "Code Review", StoryPointFields: []string{"customfield_10016", "customfield_1"}},
		TrackingRule{ProjectKey: "EMPTY", DoneStatus: "Done"}, // no tracked status: ignored
	)
	if got := ActiveProjectKeys(); !reflect.DeepEqual(got, []string{"OMS", "POS"}) {
		t.Errorf("ActiveProjectKeys() = %v", got)
	}
	if got := ProjectsJQLIn(); got != "project in (OMS, POS)" {
		t.Errorf("ProjectsJQLIn() = %q", got)
	}
	want := `(project = OMS AND status in ("Code Process")) OR (project = POS AND status in ("In Progress"))`
	if got := TrackedIssuesJQL(); got != want {
		t.Errorf("JQL = %s\nwant  %s", got, want)
	}
	if got := StoryPointFieldIDs(); !reflect.DeepEqual(got, []string{"customfield_10016", "customfield_1", "customfield_2"}) {
		t.Errorf("StoryPointFieldIDs() = %v", got)
	}
	if !IsTrackedProjectKey("POS-1") || IsTrackedProjectKey("CORE-1") || !IsTrackedProject("pos") {
		t.Error("tracked projects must follow the registry")
	}
	if RuleFor("CORE").IsTracked("CODE REVIEW") {
		t.Error("a project without an active rule falls back to the legacy rule (only Code Process)")
	}
}

func TestSetRules_EmptyTracksNothing(t *testing.T) {
	withRules(t)
	if TrackedIssuesJQL() != "" || ProjectsJQLIn() != "" || len(ActiveProjectKeys()) != 0 || len(StoryPointFieldIDs()) != 0 || IsTrackedProjectKey("OMS-1") {
		t.Error("no active rule must track nothing (AC1.4)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/jira_client/jira_client_dto/ -run 'TestBuiltinRules|TestSetRules' -v -count=1`
Expected: FAIL — compile error `undefined: SetRules`.

- [ ] **Step 3: Implement**

`tracking_rules.go` — ganti baris 1–108 (package clause s.d. akhir `TrackedIssuesJQL`) dengan:
```go
package jira_client_dto

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// TrackingRule says which statuses of a project count toward a ticket's
// time budget and where its Story Points live (Tech Spec "Aturan Board").
// Rules come from ms_tracking_rule through SetRules.
type TrackingRule struct {
	// ProjectKey: the Jira project ("CORE"); "" for the fallback rule.
	ProjectKey string
	// TrackedStatuses: the budget clock runs while the ticket is in any of
	// these; moving between them never stops it.
	TrackedStatuses []string
	// DoneStatus: leaving the tracked statuses for this one is "finished" —
	// the early-completion notice fires (if under budget).
	DoneStatus string
	// ReviewStatus (optional): a tracked status that is the code review.
	ReviewStatus string
	// StoryPointFields: custom field IDs holding Story Points, in read
	// priority (the first non-null one wins).
	StoryPointFields []string
	// StoryPointWriteField / StoryPointWriteFieldSubtask: where Buat Card
	// writes Story Points for a normal card / a subtask.
	StoryPointWriteField        string
	StoryPointWriteFieldSubtask string
}

// ruleSet is one immutable snapshot of the active rules: readers load it
// once (lock-free) and never see a half-swapped registry.
type ruleSet struct {
	byKey map[string]TrackingRule
	keys  []string // sorted
}

var currentRules atomic.Pointer[ruleSet]

func init() { SetRules(BuiltinRules()) }

// BuiltinRules are OMS & CORE exactly as the 0021b seed — the registry's
// content until tracking_rule loads the table (and in unit tests).
func BuiltinRules() []TrackingRule {
	return []TrackingRule{
		{
			ProjectKey:                  "CORE",
			TrackedStatuses:             []string{StatusCodeProcess, StatusCodeReviewCore},
			DoneStatus:                  StatusCodeCompletedCore,
			ReviewStatus:                StatusCodeReviewCore,
			StoryPointFields:            []string{StoryPointFieldIDCoreEstimate, StoryPointFieldIDCore},
			StoryPointWriteField:        StoryPointFieldIDCoreEstimate,
			StoryPointWriteFieldSubtask: StoryPointFieldIDCore,
		},
		{
			ProjectKey:                  "OMS",
			TrackedStatuses:             []string{StatusCodeProcess},
			DoneStatus:                  StatusCodeReview,
			StoryPointFields:            []string{StoryPointFieldID},
			StoryPointWriteField:        StoryPointFieldID,
			StoryPointWriteFieldSubtask: StoryPointFieldID,
		},
	}
}

// SetRules installs the ACTIVE rules (every rule passed counts as active).
// Keys are upper-cased; a rule without a project key or tracked status is
// skipped. An empty list tracks nothing: TrackedIssuesJQL returns "".
func SetRules(active []TrackingRule) {
	set := &ruleSet{byKey: make(map[string]TrackingRule, len(active))}
	for _, r := range active {
		key := strings.ToUpper(strings.TrimSpace(r.ProjectKey))
		if key == "" || len(r.TrackedStatuses) == 0 {
			continue
		}
		r.ProjectKey = key
		r.TrackedStatuses = append([]string(nil), r.TrackedStatuses...)
		r.StoryPointFields = append([]string(nil), r.StoryPointFields...)
		if _, dup := set.byKey[key]; !dup {
			set.keys = append(set.keys, key)
		}
		set.byKey[key] = r
	}
	sort.Strings(set.keys)
	currentRules.Store(set)
}

func snapshot() *ruleSet { return currentRules.Load() }

// defaultTrackingRule applies to a project without an active rule (the
// legacy behavior: only Code Process counts, Code Review is "done").
var defaultTrackingRule = TrackingRule{TrackedStatuses: []string{StatusCodeProcess}, DoneStatus: StatusCodeReview}

// RuleFor returns the tracking rule of a project key ("OMS", "CORE").
func RuleFor(projectKey string) TrackingRule {
	if rule, ok := snapshot().byKey[strings.ToUpper(projectKey)]; ok {
		return rule
	}
	return defaultTrackingRule
}

// RuleForIssue returns the tracking rule of an issue key ("CORE-123").
func RuleForIssue(issueKey string) TrackingRule {
	return RuleFor(ProjectKeyOf(issueKey))
}

// IsTrackedProject reports whether projectKey has an active rule.
func IsTrackedProject(projectKey string) bool {
	_, ok := snapshot().byKey[strings.ToUpper(projectKey)]
	return ok
}

// ActiveProjectKeys returns the projects with an active rule, sorted.
func ActiveProjectKeys() []string {
	return append([]string(nil), snapshot().keys...)
}

// StoryPointFieldIDs is every active rule's Story Point fields (project
// order, then priority), without duplicates — the fields= of issue searches.
func StoryPointFieldIDs() []string {
	set := snapshot()
	seen := map[string]bool{}
	var out []string
	for _, key := range set.keys {
		for _, id := range set.byKey[key].StoryPointFields {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// ProjectsJQLIn is "project in (CORE, OMS)" over the active projects, or ""
// when none is active (callers then skip the search).
func ProjectsJQLIn() string {
	keys := snapshot().keys
	if len(keys) == 0 {
		return ""
	}
	return "project in (" + strings.Join(keys, ", ") + ")"
}

// ProjectKeyOf returns "CORE" for "CORE-123" ("" when there's no dash).
func ProjectKeyOf(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return ""
}

// IsTracked reports whether status counts toward the budget.
func (r TrackingRule) IsTracked(status string) bool {
	for _, tracked := range r.TrackedStatuses {
		if strings.EqualFold(tracked, status) {
			return true
		}
	}
	return false
}

// IsDone reports whether status is the project's "finished" destination.
func (r TrackingRule) IsDone(status string) bool {
	return r.DoneStatus != "" && strings.EqualFold(r.DoneStatus, status)
}

// IsReview reports whether status is the project's tracked review status.
func (r TrackingRule) IsReview(status string) bool {
	return r.ReviewStatus != "" && strings.EqualFold(r.ReviewStatus, status)
}

// TracksReview reports whether this project counts code review time.
func (r TrackingRule) TracksReview() bool {
	return r.ReviewStatus != ""
}

// TrackedIssuesJQL is the poller's search: every ticket of an active project
// in one of that project's tracked statuses; "" when no rule is active.
// Deterministic (sorted) so it is testable and cache-friendly.
func TrackedIssuesJQL() string {
	set := snapshot()
	clauses := make([]string, 0, len(set.keys))
	for _, key := range set.keys {
		rule := set.byKey[key]
		quoted := make([]string, len(rule.TrackedStatuses))
		for i, status := range rule.TrackedStatuses {
			quoted[i] = fmt.Sprintf("%q", status)
		}
		clauses = append(clauses, fmt.Sprintf(`(project = %s AND status in (%s))`, key, strings.Join(quoted, ", ")))
	}
	return strings.Join(clauses, " OR ")
}
```

`issue_relations.go` — ganti `IsTrackedProjectKey`:
```go
// IsTrackedProjectKey reports whether issueKey is a well-formed key of a
// project with an active tracking rule — anything else never reaches a JQL.
func IsTrackedProjectKey(issueKey string) bool {
	return issueKeyPattern.MatchString(issueKey) && IsTrackedProject(ProjectKeyOf(issueKey))
}
```

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./modules/jira_client/... -count=1`
Expected: PASS (test lama `TestTrackedIssuesJQL`, `TestRuleFor_*`, `TestIsTrackedProjectKey` tetap lulus dengan builtin).

- [ ] **Step 5: Commit**

```bash
git add modules/jira_client/jira_client_dto/tracking_rules.go modules/jira_client/jira_client_dto/tracking_rules_test.go modules/jira_client/jira_client_dto/issue_relations.go
git commit -m "feat(tracking-rules): read tracking rules from an atomic registry" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Story Point dinamis per project + `fields=` dari registry

**Files:**
- Modify: `modules/jira_client/jira_client_dto/jira_client_dto.go` (struct `IssueFields` baris 13–43, `EffectiveStoryPoints` baris 136–148, import)
- Modify: `modules/jira_client/jira_client_dto/constants.go:12-18` (komentar)
- Modify: `modules/jira_client/jira_client_service/jira_agile_client.go:61-66, 81-98`, `jira_client_service_impl.go:47, 146`
- Modify: `modules/poller/poller_service/poller_service_impl.go:76, 353`, `modules/sprint_backfill/sprint_backfill_service/sprint_backfill_service_impl.go:247`, `modules/user_dashboard/user_dashboard_service/user_dashboard_service_impl.go:315`
- Test: `modules/jira_client/jira_client_dto/jira_client_dto_test.go`, `modules/jira_client/jira_client_service/jira_client_service_test.go`, `modules/poller/poller_service/poller_service_test.go`, `modules/sprint_backfill/sprint_backfill_service/sprint_backfill_service_impl_test.go`, `modules/user_dashboard/user_dashboard_service/user_dashboard_service_test.go`

**Interfaces:**
- Consumes: `RuleForIssue`, `StoryPointFieldIDs`, `SetRules`, `BuiltinRules` (Task 2).
- Produces: `IssueFields.Numbers map[string]*float64` (`json:"-"`, diisi `UnmarshalJSON` untuk setiap `customfield_*` bernilai angka); `func (f IssueFields) FirstNumber(ids ...string) *float64`; `func (i Issue) StoryPoints() *float64`; `func issueFieldsParam(withSprint bool) string` (package `jira_client_service`). Field `StoryPoints`, `StoryPointsCore`, `StoryPointsCoreEstimate` dan `EffectiveStoryPoints()` **dihapus**.

- [ ] **Step 1: Write the failing test** — di `jira_client_dto_test.go`, hapus 4 test `TestEffectiveStoryPoints_*` (baris 10–51) dan ganti dengan:

```go
func TestIssueFields_UnmarshalKeepsNumericCustomFields(t *testing.T) {
	raw := `{"key":"CORE-1","fields":{"summary":"x","customfield_10125":3,"customfield_11053":null,
		"customfield_10020":[{"id":1,"state":"active","boardId":26}],"customfield_12001":"text"}}`
	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.Fields.Summary != "x" || len(issue.Fields.SprintRaw) == 0 {
		t.Errorf("regular fields lost: %+v", issue.Fields)
	}
	if v := issue.Fields.Numbers["customfield_10125"]; v == nil || *v != 3 {
		t.Errorf("customfield_10125 = %v, want 3", v)
	}
	for _, id := range []string{"customfield_11053", "customfield_10020", "customfield_12001"} {
		if _, ok := issue.Fields.Numbers[id]; ok {
			t.Errorf("%s must not be kept (null / not a number)", id)
		}
	}
}

// legacyEffectiveStoryPoints is the old global coalesce (10016 → 10125 →
// 11053), kept only to prove the per-project rules give the same result.
func legacyEffectiveStoryPoints(n map[string]*float64) *float64 {
	for _, id := range []string{"customfield_10016", "customfield_10125", "customfield_11053"} {
		if v := n[id]; v != nil {
			return v
		}
	}
	return nil
}

func TestIssueStoryPoints_ParityWithTheLegacyCoalesce(t *testing.T) {
	// Live data: OMS never fills CORE's fields and vice versa (constants.go).
	cases := []struct {
		key     string
		numbers map[string]*float64
	}{
		{"OMS-1", map[string]*float64{"customfield_10016": floatPtr(3)}},
		{"OMS-2", nil},
		{"CORE-1", map[string]*float64{"customfield_10125": floatPtr(3), "customfield_11053": floatPtr(5)}},
		{"CORE-2", map[string]*float64{"customfield_11053": floatPtr(5)}},
		{"CORE-3", map[string]*float64{"customfield_10125": floatPtr(2)}},
		{"CORE-4", nil},
	}
	for _, c := range cases {
		got := Issue{Key: c.key, Fields: IssueFields{Numbers: c.numbers}}.StoryPoints()
		want := legacyEffectiveStoryPoints(c.numbers)
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("%s: StoryPoints() = %v, legacy = %v", c.key, got, want)
		}
	}
}

func TestIssueStoryPoints_FollowsTheProjectRuleOrder(t *testing.T) {
	withRules(t, TrackingRule{ProjectKey: "POS", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done",
		StoryPointFields: []string{"customfield_2", "customfield_1"}})
	issue := Issue{Key: "POS-1", Fields: IssueFields{Numbers: map[string]*float64{"customfield_1": floatPtr(1), "customfield_2": floatPtr(2)}}}
	if got := issue.StoryPoints(); got == nil || *got != 2 {
		t.Errorf("POS-1 = %v, want 2 (customfield_2 has priority)", got)
	}
	// A project without a rule reads every active rule's fields.
	unknown := Issue{Key: "HR-1", Fields: IssueFields{Numbers: map[string]*float64{"customfield_1": floatPtr(1)}}}
	if got := unknown.StoryPoints(); got == nil || *got != 1 {
		t.Errorf("HR-1 = %v, want 1", got)
	}
}
```

Di `jira_client_service_test.go` tambahkan (dan import tetap):
```go
func TestSearchCodeProcessIssues_FieldsFollowTheRegistry(t *testing.T) {
	jira_client_dto.SetRules([]jira_client_dto.TrackingRule{{ProjectKey: "POS", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done", StoryPointFields: []string{"customfield_777"}}})
	t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })
	var fields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields = r.URL.Query().Get("fields")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{}})
	}))
	defer server.Close()

	if _, err := NewJiraClientService(newTestConfig(server.URL)).SearchCodeProcessIssues(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fields != "summary,status,assignee,issuetype,customfield_777,customfield_10020" {
		t.Errorf("fields = %q, want the active rules' Story Point fields + Sprint", fields)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/jira_client/... -count=1`
Expected: FAIL — compile error `issue.Fields.Numbers undefined` / `StoryPoints is not a method`.

- [ ] **Step 3: Implement**

`jira_client_dto.go` — import menjadi:
```go
import (
	"encoding/json"
	"regexp"
	"strings"
)
```
Ganti struct `IssueFields` (baris 13–43) dengan:
```go
type IssueFields struct {
	Summary   string    `json:"summary"`
	Status    Status    `json:"status"`
	Assignee  *Assignee `json:"assignee"`
	IssueType IssueType `json:"issuetype"`
	// Parent, Subtasks and IssueLinks are only requested by the dashboard's
	// active-sprint search (Tech Spec "Subtask & Linked Item Branches").
	Parent     *IssueRef   `json:"parent,omitempty"`
	Subtasks   []IssueRef  `json:"subtasks,omitempty"`
	IssueLinks []IssueLink `json:"issuelinks,omitempty"`
	// SprintRaw is the Sprint field (customfield_10020) — confirmed live
	// 2026-11-01 identical for OMS and CORE: an array of objects
	// {id, name, state, boardId, goal, startDate, endDate}, state values
	// lowercase ("active", "closed", "future"). Kept as json.RawMessage
	// because ActiveSprintName() also defends against a legacy
	// greenhopper string format that was never actually observed live but
	// costs nothing to handle defensively.
	SprintRaw json.RawMessage `json:"customfield_10020"`
	// Numbers holds every numeric custom field of the response by ID
	// (null ones are left out). Story Points live here — which field
	// depends on the project's tracking rule (Tech Spec "Aturan Board");
	// read them through Issue.StoryPoints().
	Numbers map[string]*float64 `json:"-"`
}

// UnmarshalJSON decodes the regular fields, then keeps every numeric
// customfield_* in Numbers — the Story Point fields are configured per
// project at runtime, so they can't be struct tags.
func (f *IssueFields) UnmarshalJSON(data []byte) error {
	type plain IssueFields
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for id, value := range raw {
		if !strings.HasPrefix(id, "customfield_") || strings.TrimSpace(string(value)) == "null" {
			continue
		}
		var n float64
		if json.Unmarshal(value, &n) != nil {
			continue
		}
		if p.Numbers == nil {
			p.Numbers = map[string]*float64{}
		}
		p.Numbers[id] = &n
	}
	*f = IssueFields(p)
	return nil
}

// FirstNumber returns the first non-null numeric custom field among ids.
func (f IssueFields) FirstNumber(ids ...string) *float64 {
	for _, id := range ids {
		if v := f.Numbers[id]; v != nil {
			return v
		}
	}
	return nil
}
```
(`var n float64` dideklarasikan di dalam loop, jadi `&n` aman per iterasi.)

Ganti `EffectiveStoryPoints` (baris 136–148) dengan:
```go
// StoryPoints returns the issue's Story Points: the first non-null of its
// project's StoryPointFields (priority order). A project without an active
// rule reads every active rule's fields, like the old global coalesce.
func (i Issue) StoryPoints() *float64 {
	fields := RuleForIssue(i.Key).StoryPointFields
	if len(fields) == 0 {
		fields = StoryPointFieldIDs()
	}
	return i.Fields.FirstNumber(fields...)
}
```

`constants.go` — ganti komentar baris 12–17 (`// StoryPointFieldIDCore is … // EffectiveStoryPoints) to support both projects.`) dengan:
```go
// StoryPointFieldIDCore is the Jira custom field ID the CORE project uses
// for "Story Points" — OMS and CORE use different custom fields for the
// same concept. Since Tech Spec "Aturan Board" the fields come from
// ms_tracking_rule; these constants only feed BuiltinRules (= the seed)
// and tests.
```

`jira_agile_client.go` — hapus `var dashboardIssueFields = …` (baris 81–82) dan tambahkan di tempat yang sama:
```go
// issueFieldsParam is the fields= of every issue search: the basics, the
// Story Point fields of every active tracking rule and, withSprint, the
// Sprint field.
func issueFieldsParam(withSprint bool) string {
	fields := append([]string{"summary", "status", "assignee", "issuetype"}, jira_client_dto.StoryPointFieldIDs()...)
	if withSprint {
		fields = append(fields, jira_client_dto.SprintFieldID)
	}
	return strings.Join(fields, ",")
}
```
lalu `params.Set("fields", dashboardIssueFields+",parent,subtasks,issuelinks")` → `params.Set("fields", issueFieldsParam(true)+",parent,subtasks,issuelinks")`, dan `params.Set("fields", dashboardIssueFields)` → `params.Set("fields", issueFieldsParam(true))`.

`jira_client_service_impl.go` — baris 47 → `params.Set("fields", issueFieldsParam(true))`; baris 146 → `params.Set("fields", issueFieldsParam(false))`.

Pemanggil:
- `poller_service_impl.go:76` → `sp := issue.StoryPoints()`; `:353` → `sp := issue.StoryPoints()`.
- `sprint_backfill_service_impl.go:247` → `sp := issue.StoryPoints()`.
- `user_dashboard_service_impl.go:315` → `if sp := issue.StoryPoints(); sp != nil {`.

Test lama yang memakai field bertipe:
- `jira_client_service_test.go:68-70` →
```go
	if got := issues[0].StoryPoints(); got == nil || *got != 3.0 {
		t.Errorf("StoryPoints() = %v, want 3.0", got)
	}
```
- `jira_client_service_test.go:307-315` →
```go
	if issues[0].Fields.Numbers["customfield_10016"] != nil {
		t.Errorf("customfield_10016 = %v, want nil for a CORE ticket", *issues[0].Fields.Numbers["customfield_10016"])
	}
	if got := issues[0].StoryPoints(); got == nil || *got != 1.5 {
		t.Errorf("StoryPoints() = %v, want 1.5 (CORE's customfield_11053)", got)
	}
```
- `jira_client_service_test.go:441-442` →
```go
	if got := issues[0].StoryPoints(); got == nil || *got != 3 {
		t.Errorf("StoryPoints() = %v, want 3 — Story Point Estimate must be recognized even when Story Points is empty", got)
	}
```
- `poller_service_test.go` — tambah helper setelah `storyPointsPtr`:
```go
// spNumbers puts sp on the first Story Point field of key's project
// (OMS: customfield_10016, CORE: customfield_10125).
func spNumbers(key string, sp float64) map[string]*float64 {
	return map[string]*float64{jira_client_dto.RuleForIssue(key).StoryPointFields[0]: storyPointsPtr(sp)}
}
```
  lalu: baris 255 `StoryPoints: storyPointsPtr(sp),` → `Numbers:   spNumbers(key, sp),`; baris 391 `jira_client_dto.IssueFields{StoryPoints: storyPointsPtr(3)}` → `jira_client_dto.IssueFields{Numbers: spNumbers("OMS-1", 3)}`; baris 589 `StoryPoints: storyPointsPtr(3),` → `Numbers:     spNumbers("OMS-1", 3),`; baris 673 `StoryPointsCore: storyPointsPtr(sp), // CORE project's field, customfield_11053` → `Numbers:         map[string]*float64{"customfield_11053": storyPointsPtr(sp)}, // CORE's "Story Points" field`; baris 712 `StoryPointsCore: storyPointsPtr(1.5),` → `Numbers:         map[string]*float64{"customfield_11053": storyPointsPtr(1.5)},`.
- `sprint_backfill_service_impl_test.go:27` → `SprintRaw: json.RawMessage(sprint), Numbers: map[string]*float64{"customfield_10125": points},`
- `user_dashboard_service_test.go:757-758` → `… Status: jira_client_dto.Status{Name: "Code Completed"}, Numbers: map[string]*float64{"customfield_10125": &three}}},` dan `… Status: jira_client_dto.Status{Name: "To Do"}, Numbers: map[string]*float64{"customfield_10016": &two}}},`

- [ ] **Step 4: Run tests**

Run: `go build ./... && go vet ./... && go test ./modules/... -count=1`
Expected: PASS (termasuk test paritas dan semua test poller/backfill/dashboard lama).

- [ ] **Step 5: Commit**

```bash
git add modules/jira_client modules/poller/poller_service modules/sprint_backfill/sprint_backfill_service modules/user_dashboard/user_dashboard_service
git commit -m "feat(tracking-rules): read story points per project rule" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: JQL `project in` dari registry + poller (skip & tutup sesi project nonaktif)

**Files:**
- Modify: `modules/jira_client/jira_client_service/jira_client_service_impl.go` (`SearchCodeProcessIssues` baris 42–59, `SearchAllIssuesByAssignee` baris 142–158)
- Modify: `modules/jira_client/jira_client_service/jira_agile_client.go` (`SearchActiveSprintIssuesByAssignee` baris 61–76, `SearchSprintIssuesUpdatedWithin` baris 191–205)
- Modify: `modules/poller/poller_service/poller_service_impl.go` (`pollOnce`, `backfillActiveSessions`)
- Test: `modules/jira_client/jira_client_service/jira_agile_client_test.go`, `modules/poller/poller_service/poller_service_test.go`

**Interfaces:**
- Consumes: `ProjectsJQLIn`, `TrackedIssuesJQL`, `IsTrackedProjectKey`, `SetRules`, `BuiltinRules` (Task 2).
- Produces: semua search mengembalikan `nil, nil` tanpa request bila tidak ada project aktif; poller tidak melakukan apa pun bila `TrackedIssuesJQL() == ""`.

- [ ] **Step 1: Write the failing tests**

`jira_agile_client_test.go` (append):
```go
func TestProjectScopedSearches_FollowTheRegistry(t *testing.T) {
	var jqls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql := r.URL.Query().Get("jql")
		if r.Method == http.MethodPost {
			var body searchJQLRequest
			json.NewDecoder(r.Body).Decode(&body)
			jql = body.JQL
		}
		jqls = append(jqls, jql)
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{}})
	}))
	defer server.Close()
	cfg := newTestConfig(server.URL)
	ctx := context.Background()
	t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })

	jira_client_dto.SetRules([]jira_client_dto.TrackingRule{
		{ProjectKey: "POS", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done"},
		{ProjectKey: "OMS", TrackedStatuses: []string{"Code Process"}, DoneStatus: "Code Review"},
	})
	NewJiraClientService(cfg).SearchAllIssuesByAssignee(ctx, nil, "acc-1")
	NewJiraAgileClient(cfg).SearchActiveSprintIssuesByAssignee(ctx, nil, "acc-1")
	NewJiraAgileClient(cfg).SearchSprintIssuesUpdatedWithin(ctx, []string{"acc-1"}, 0)
	if len(jqls) != 3 {
		t.Fatalf("requests = %d, want 3", len(jqls))
	}
	for _, jql := range jqls {
		if !strings.HasPrefix(jql, "project in (OMS, POS) AND ") {
			t.Errorf("jql = %q, want the active projects of the registry", jql)
		}
	}

	// No active project: no request and no error (AC1.4).
	jira_client_dto.SetRules(nil)
	jqls = nil
	if issues, err := NewJiraClientService(cfg).SearchCodeProcessIssues(ctx); issues != nil || err != nil {
		t.Errorf("SearchCodeProcessIssues = %v, %v", issues, err)
	}
	if issues, err := NewJiraClientService(cfg).SearchAllIssuesByAssignee(ctx, nil, "acc-1"); issues != nil || err != nil {
		t.Errorf("SearchAllIssuesByAssignee = %v, %v", issues, err)
	}
	if issues, err := NewJiraAgileClient(cfg).SearchActiveSprintIssuesByAssignee(ctx, nil, "acc-1"); issues != nil || err != nil {
		t.Errorf("SearchActiveSprintIssuesByAssignee = %v, %v", issues, err)
	}
	if issues, err := NewJiraAgileClient(cfg).SearchSprintIssuesUpdatedWithin(ctx, []string{"acc-1"}, 0); issues != nil || err != nil {
		t.Errorf("SearchSprintIssuesUpdatedWithin = %v, %v", issues, err)
	}
	if len(jqls) != 0 {
		t.Errorf("requests sent with no active project: %v", jqls)
	}
}
```

`poller_service_test.go` (append):
```go
// --- Aturan Board (Tech Spec "Aturan Board (Admin CRUD)") ---

func TestPollOnce_DeactivatedProjectClosesSessionWithoutTime(t *testing.T) {
	t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })
	jira_client_dto.SetRules([]jira_client_dto.TrackingRule{jira_client_dto.RuleFor("CORE")}) // OMS deactivated
	issueSession := newFakeIssueSessionServiceForPoller()
	// 72h back always spans business hours, so the old exit path would add time.
	start := time.Now().Add(-72 * time.Hour)
	issueSession.sessions["OMS-1"] = &issue_session_dto.IssueSession{
		IssueKey: "OMS-1", ProjectKey: "OMS", StoryPoints: 3, FlagActive: true, CurrentSessionStartedDate: &start, AssigneeEmail: "dev@esb.co.id",
	}
	reminderEngine := &fakeReminderEngineService{}
	poller := newTestPoller(&fakeJiraClientService{}, issueSession, reminderEngine, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	added, closed := issueSession.closeCalls["OMS-1"]
	if !closed || added != 0 {
		t.Errorf("CloseSession(OMS-1) closed=%v added=%d, want closed with 0 seconds (AC4.5)", closed, added)
	}
	if len(reminderEngine.evaluateCalls) != 0 {
		t.Errorf("evaluated = %v, want none", reminderEngine.evaluateCalls)
	}
}

func TestPollOnce_NoActiveRuleSkipsTheCycle(t *testing.T) {
	issue := assignedIssue("OMS-1", "dev@esb.co.id", 3) // built before the registry is emptied
	t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })
	jira_client_dto.SetRules(nil)
	issueSession := newFakeIssueSessionServiceForPoller()
	start := time.Now().Add(-72 * time.Hour)
	issueSession.sessions["OMS-2"] = &issue_session_dto.IssueSession{IssueKey: "OMS-2", StoryPoints: 3, FlagActive: true, CurrentSessionStartedDate: &start}
	poller := newTestPoller(&fakeJiraClientService{issues: []jira_client_dto.Issue{issue}}, issueSession, &fakeReminderEngineService{}, newFakeRegistrationRepositoryForPoller("dev@esb.co.id"))

	if err := poller.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if err := poller.backfillActiveSessions(context.Background()); err != nil {
		t.Fatalf("backfillActiveSessions: %v", err)
	}
	if len(issueSession.openAtCalls) != 0 || len(issueSession.closeCalls) != 0 {
		t.Errorf("opened %v / closed %v, want nothing touched when no rule is active (AC1.4)", issueSession.openAtCalls, issueSession.closeCalls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./modules/jira_client/jira_client_service/ ./modules/poller/... -run 'TestProjectScopedSearches|TestPollOnce_Deactivated|TestPollOnce_NoActiveRule' -v -count=1`
Expected: FAIL — JQL masih `project in (CORE, OMS)`; poller menambah waktu & membuka sesi.

- [ ] **Step 3: Implement**

`jira_client_service_impl.go` — `SearchCodeProcessIssues`, setelah `jql := jira_client_dto.TrackedIssuesJQL()`:
```go
	if jql == "" {
		return nil, nil // no active tracking rule (AC1.4)
	}
```
`SearchAllIssuesByAssignee` — ganti baris `jql := fmt.Sprintf(`project in (CORE, OMS) AND assignee = "%s" ORDER BY updated DESC`, accountID)` dengan:
```go
	projects := jira_client_dto.ProjectsJQLIn()
	if projects == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`%s AND assignee = "%s" ORDER BY updated DESC`, projects, accountID)
```
`jira_agile_client.go` — `SearchActiveSprintIssuesByAssignee`, ganti baris JQL dengan:
```go
	projects := jira_client_dto.ProjectsJQLIn()
	if projects == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`%s AND assignee = "%s" AND sprint in openSprints() ORDER BY updated DESC`, projects, accountID)
```
`SearchSprintIssuesUpdatedWithin` — ganti baris `jql := fmt.Sprintf(`project in (CORE, OMS) AND sprint in openSprints() AND assignee in (%s)`, …)` dengan:
```go
	projects := jira_client_dto.ProjectsJQLIn()
	if projects == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`%s AND sprint in openSprints() AND assignee in (%s)`, projects, strings.Join(quoted, ","))
```
Perbarui komentar interface `JiraAgileClient` yang menyebut "CORE/OMS" menjadi "every project with an active tracking rule".

`poller_service_impl.go` — di awal `pollOnce`:
```go
	// No active tracking rule (only possible via a manual DB change): search
	// nothing and leave every session as it is (AC1.4).
	if jira_client_dto.TrackedIssuesJQL() == "" {
		return nil
	}
```
di awal `backfillActiveSessions` blok yang sama. Di loop exit events, tepat setelah blok `if staleKeys[key] { … continue }`:
```go
		// The project's rule was deactivated (Tech Spec "Aturan Board",
		// AC4.5): stop like a ticket leaving the active sprint — no time
		// added, no notice (status_watch no longer searches that project).
		// Checked after the search, so a rule switched off mid-cycle closes
		// the session on the next cycle instead of adding time now.
		if !jira_client_dto.IsTrackedProjectKey(key) {
			if err := p.issueSession.CloseSession(ctx, key, 0); err != nil {
				return fmt.Errorf("poller: CloseSession(%s) (project no longer tracked) failed: %w", key, err)
			}
			continue
		}
```

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./modules/jira_client/... ./modules/poller/... ./modules/status_watch/... ./modules/user_dashboard/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/jira_client/jira_client_service modules/poller/poller_service
git commit -m "feat(tracking-rules): scope jira searches and the poller to active rules" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Buat Card — field SP tulis per jenis card dari rule project

**Files:**
- Modify: `modules/issue_create/issue_create_service/meta.go:9-45`
- Modify: `modules/issue_create/issue_create_service/service.go:222`
- Test: `modules/issue_create/issue_create_service/service_test.go`

**Interfaces:**
- Consumes: `jira_client_dto.RuleFor(project) TrackingRule` dengan `StoryPointFields`, `StoryPointWriteField`, `StoryPointWriteFieldSubtask` (Task 2).
- Produces: `func mapType(rule jira_client_dto.TrackingRule, t jira_client_dto.CreateMetaIssueType, fields []jira_client_dto.CreateMetaField) TypeMeta`. `TypeMeta.StoryPointField` = field tulis sesuai jenis card bila ada di createmeta tipe itu, selain itu `""` (input SP disembunyikan oleh frontend yang sudah ada: `@if (t.storyPointField)`).

- [ ] **Step 1: Write the failing test** (append ke `service_test.go`)

```go
func coreJira() *fakeJira {
	number := jira_client_dto.FieldSchema{Type: "number"}
	return &fakeJira{
		types: []jira_client_dto.CreateMetaIssueType{{ID: "10001", Name: "Story"}, {ID: "10003", Name: "Subtask", Subtask: true}, {ID: "10004", Name: "Bug"}},
		fields: map[string][]jira_client_dto.CreateMetaField{
			"10001": {
				{FieldID: "customfield_10125", Name: "Story Point Estimate", Required: true, Schema: number},
				{FieldID: "customfield_11053", Name: "Story Points", Schema: number},
			},
			"10003": {
				{FieldID: "parent", Name: "Parent", Required: true, Schema: jira_client_dto.FieldSchema{Type: "issuelink"}},
				{FieldID: "customfield_10125", Name: "Story Point Estimate", Schema: number},
				{FieldID: "customfield_11053", Name: "Story Points", Schema: number},
			},
			"10004": {{FieldID: "customfield_11053", Name: "Story Points", Schema: number}},
		},
	}
}

func TestMeta_CoreWritesEstimateOnCardsAndStoryPointsOnSubtasks(t *testing.T) {
	m, err := newSvc(coreJira(), &fakeAI{}).Meta(context.Background(), "dev@esb.co.id", 26)
	if err != nil {
		t.Fatal(err)
	}
	story, sub, bug := m.Types[0], m.Types[1], m.Types[2]
	if story.StoryPointField != "customfield_10125" || len(story.Fields) != 0 {
		t.Fatalf("story = %+v, want SP on customfield_10125 and no dynamic field (SP fields are handled)", story)
	}
	if sub.StoryPointField != "customfield_11053" {
		t.Fatalf("subtask = %+v, want SP on customfield_11053 (AC5.3)", sub)
	}
	if bug.StoryPointField != "" {
		t.Fatalf("bug = %+v, want SP hidden: customfield_10125 is not on its createmeta (AC5.3b)", bug)
	}
}

func TestCreateSubtasks_CoreWritesTheSubtaskField(t *testing.T) {
	j := coreJira()
	j.brief.Key = "CORE-5"
	j.brief.Fields.Project.Key = "CORE"
	pts := 2.0
	res, err := newSvc(j, &fakeAI{}).CreateSubtasks(context.Background(), "dev@esb.co.id", 26, "CORE-5", []BulkRow{{ClientID: "1", Title: "A", StoryPoints: &pts}})
	if err != nil || res[0].Key == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if j.created[0]["customfield_11053"] != 2.0 || j.created[0]["customfield_10125"] != nil {
		t.Fatalf("subtask fields = %v, want SP on customfield_11053 only", j.created[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/issue_create/... -run 'TestMeta_Core|TestCreateSubtasks_Core' -v -count=1`
Expected: FAIL — story memakai `customfield_11053` dan `customfield_10125` dirender sebagai field dinamis.

- [ ] **Step 3: Implement**

`meta.go` — ganti `var handled …` dan `mapType` (baris 9–45) dengan:
```go
// baseHandled: fields the form handles itself — never rendered as dynamic
// fields. Every Story Point field of the project's rule is handled too.
var baseHandled = map[string]bool{
	"project": true, "issuetype": true, "summary": true, "description": true, "reporter": true,
	"assignee": true, "parent": true, jira_client_dto.SprintFieldID: true,
}

func handledFor(rule jira_client_dto.TrackingRule, id string) bool {
	if baseHandled[id] || id == rule.StoryPointWriteField || id == rule.StoryPointWriteFieldSubtask {
		return true
	}
	for _, sp := range rule.StoryPointFields {
		if sp == id {
			return true
		}
	}
	return false
}

// mapType turns one createmeta issue type into the form's TypeMeta. rule is
// the project's tracking rule (Tech Spec "Aturan Board", AC5.3): the SP
// input writes StoryPointWriteFieldSubtask for a subtask type and
// StoryPointWriteField otherwise — only when that field is on the type's
// createmeta; otherwise the SP input is hidden (AC5.3b).
func mapType(rule jira_client_dto.TrackingRule, t jira_client_dto.CreateMetaIssueType, fields []jira_client_dto.CreateMetaField) TypeMeta {
	tm := TypeMeta{ID: t.ID, Name: t.Name, Subtask: t.Subtask, Supported: true, Fields: []Field{}}
	ids := map[string]bool{}
	for _, f := range fields {
		ids[f.ID()] = true
	}
	write := rule.StoryPointWriteField
	if t.Subtask {
		write = rule.StoryPointWriteFieldSubtask
	}
	if write != "" && ids[write] {
		tm.StoryPointField = write
	}
	tm.ParentAllowed = ids["parent"]
	for _, f := range fields {
		if handledFor(rule, f.ID()) || !f.Required {
			continue
		}
		kind, multi := kindOf(f.Schema)
		if kind == "" {
			tm.Supported = false
			continue
		}
		field := Field{ID: f.ID(), Name: f.Name, Kind: kind, Multi: multi, Required: true}
		for _, v := range f.AllowedValues {
			field.Options = append(field.Options, Option{ID: v.ID, Label: v.Label()})
		}
		tm.Fields = append(tm.Fields, field)
	}
	return tm
}
```
`service.go:222` → `tm := mapType(jira_client_dto.RuleFor(project), t, fields)`.

- [ ] **Step 4: Run tests**

Run: `go test ./modules/issue_create/... -count=1`
Expected: PASS (test OMS lama `TestMeta_MapsDynamicFieldsAndCaches` tetap: OMS menulis `customfield_10016`).

- [ ] **Step 5: Commit**

```bash
git add modules/issue_create/issue_create_service
git commit -m "feat(tracking-rules): pick the buat card story point field from the rule" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Jira client — project, status workflow, field angka

**Files:**
- Create: `modules/jira_client/jira_client_dto/project_meta.go`
- Create: `modules/jira_client/jira_client_service/jira_project_meta.go`
- Test: `modules/jira_client/jira_client_service/jira_project_meta_test.go`

**Interfaces:**
- Produces (`jira_client_dto`): `ProjectInfo{Key, Name string; AvatarURLs map[string]string}` + `func (p ProjectInfo) AvatarURL() string`; `ProjectIssueTypeStatuses{Name string; Subtask bool; Statuses []Status}`; `JiraField{ID, Name string; Custom bool; Schema FieldSchema}`; `NumberField{ID string \`json:"id"\`; Name string \`json:"name"\`; StoryPointLike bool \`json:"storyPointLike"\`}`.
- Produces (`jira_client_service`): `type ProjectMetaClient interface { GetProject(ctx context.Context, projectKey string) (jira_client_dto.ProjectInfo, error); GetProjectStatuses(ctx context.Context, projectKey string) ([]string, error); GetNumberFields(ctx context.Context) ([]jira_client_dto.NumberField, error) }`; `func NewProjectMetaClient(cfg *config.AppConfig) ProjectMetaClient`; `var ErrProjectNotFound`.

- [ ] **Step 1: Write the failing test** (`jira_project_meta_test.go`)

```go
package jira_client_service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

func TestProjectMetaClient_ReadsProjectStatusesAndNumberFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Errorf("missing bot credential on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/rest/api/3/project/POS":
			json.NewEncoder(w).Encode(map[string]any{"key": "POS", "name": "ESB POS", "avatarUrls": map[string]string{"48x48": "https://jira/a.png"}})
		case "/rest/api/3/project/POS/statuses":
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Story", "subtask": false, "statuses": []map[string]any{{"name": "To Do"}, {"name": "In Progress"}, {"name": "Done"}}},
				{"name": "Sub-task", "subtask": true, "statuses": []map[string]any{{"name": "To Do"}, {"name": "IN PROGRESS"}, {"name": "Review"}}},
			})
		case "/rest/api/3/field":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "summary", "name": "Summary", "schema": map[string]any{"type": "string"}},
				{"id": "timespent", "name": "Time Spent", "schema": map[string]any{"type": "number"}},
				{"id": "customfield_200", "name": "Effort", "custom": true, "schema": map[string]any{"type": "number"}},
				{"id": "customfield_10016", "name": "Story point estimate", "custom": true, "schema": map[string]any{"type": "number"}},
				{"id": "customfield_300", "name": "Team", "custom": true, "schema": map[string]any{"type": "option"}},
				{"id": "customfield_400", "name": "Notes"},
			})
		case "/rest/api/3/project/NOPE", "/rest/api/3/project/NOPE/statuses":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := NewProjectMetaClient(newTestConfig(server.URL))
	ctx := context.Background()

	p, err := c.GetProject(ctx, "POS")
	if err != nil || p.Name != "ESB POS" || p.AvatarURL() != "https://jira/a.png" {
		t.Errorf("project = %+v, %v", p, err)
	}
	statuses, err := c.GetProjectStatuses(ctx, "POS")
	if err != nil || !reflect.DeepEqual(statuses, []string{"To Do", "In Progress", "Done", "Review"}) {
		t.Errorf("statuses = %v, %v (unique over every issue type, first spelling wins)", statuses, err)
	}
	fields, err := c.GetNumberFields(ctx)
	want := []jira_client_dto.NumberField{
		{ID: "customfield_10016", Name: "Story point estimate", StoryPointLike: true},
		{ID: "customfield_200", Name: "Effort"},
	}
	if err != nil || !reflect.DeepEqual(fields, want) {
		t.Errorf("fields = %+v, %v; want numeric custom fields, story point first", fields, err)
	}
	if _, err := c.GetProject(ctx, "NOPE"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("GetProject(NOPE) err = %v", err)
	}
	if _, err := c.GetProjectStatuses(ctx, "NOPE"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("GetProjectStatuses(NOPE) err = %v", err)
	}
}

func TestProjectMetaClient_ServerErrorIsNotNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := NewProjectMetaClient(newTestConfig(server.URL)).GetProject(context.Background(), "POS")
	if err == nil || errors.Is(err, ErrProjectNotFound) {
		t.Errorf("err = %v, want a non-404 error", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/jira_client/jira_client_service/ -run TestProjectMetaClient -v -count=1`
Expected: FAIL — `undefined: NewProjectMetaClient`.

- [ ] **Step 3: Implement**

`jira_client_dto/project_meta.go`:
```go
package jira_client_dto

// Jira project data the Admin "Aturan Board" form picks from (Tech Spec
// "Aturan Board (Admin CRUD)").

// ProjectInfo is GET /rest/api/3/project/{key}.
type ProjectInfo struct {
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	AvatarURLs map[string]string `json:"avatarUrls"`
}

// AvatarURL is the project's 48x48 avatar ("" when Jira sends none).
func (p ProjectInfo) AvatarURL() string { return p.AvatarURLs["48x48"] }

// ProjectIssueTypeStatuses is one entry of GET /rest/api/3/project/{key}/statuses:
// the statuses one issue type's workflow uses.
type ProjectIssueTypeStatuses struct {
	Name     string   `json:"name"`
	Subtask  bool     `json:"subtask"`
	Statuses []Status `json:"statuses"`
}

// JiraField is one entry of GET /rest/api/3/field.
type JiraField struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Custom bool        `json:"custom"`
	Schema FieldSchema `json:"schema"`
}

// NumberField is a numeric custom field an admin can pick as a Story Point
// field; StoryPointLike = its name contains "story point".
type NumberField struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StoryPointLike bool   `json:"storyPointLike"`
}
```

`jira_client_service/jira_project_meta.go`:
```go
package jira_client_service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// ProjectMetaClient reads a project's identity and workflow statuses and the
// site's numeric fields with the bot credential (Tech Spec "Aturan Board").
// Kept apart from JiraClientService so its many test fakes stay valid.
type ProjectMetaClient interface {
	GetProject(ctx context.Context, projectKey string) (jira_client_dto.ProjectInfo, error)
	// GetProjectStatuses: unique status names over every issue type of the
	// project (case-insensitive, first spelling wins), in Jira's order.
	GetProjectStatuses(ctx context.Context, projectKey string) ([]string, error)
	// GetNumberFields: numeric customfield_* fields, "story point" names
	// first, then by name.
	GetNumberFields(ctx context.Context) ([]jira_client_dto.NumberField, error)
}

// ErrProjectNotFound is Jira's 404 for a project key.
var ErrProjectNotFound = errors.New("jira_client: project not found")

type projectMetaClient struct{ base *jiraClientServiceImpl }

func NewProjectMetaClient(cfg *config.AppConfig) ProjectMetaClient {
	return &projectMetaClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func projectErr(err error, op string) error {
	var httpErr *curl_helper.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return ErrProjectNotFound
	}
	return fmt.Errorf("jira_client: %s failed: %w", op, err)
}

func (c *projectMetaClient) GetProject(ctx context.Context, projectKey string) (jira_client_dto.ProjectInfo, error) {
	u := fmt.Sprintf("%s/rest/api/3/project/%s", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey))
	res, err := curl_helper.CurlRequest[jira_client_dto.ProjectInfo](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return jira_client_dto.ProjectInfo{}, projectErr(err, "GetProject("+projectKey+")")
	}
	return *res, nil
}

func (c *projectMetaClient) GetProjectStatuses(ctx context.Context, projectKey string) ([]string, error) {
	u := fmt.Sprintf("%s/rest/api/3/project/%s/statuses", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey))
	res, err := curl_helper.CurlRequest[[]jira_client_dto.ProjectIssueTypeStatuses](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, projectErr(err, "GetProjectStatuses("+projectKey+")")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, issueType := range *res {
		for _, s := range issueType.Statuses {
			k := strings.ToLower(strings.TrimSpace(s.Name))
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s.Name)
		}
	}
	return out, nil
}

func (c *projectMetaClient) GetNumberFields(ctx context.Context) ([]jira_client_dto.NumberField, error) {
	res, err := curl_helper.CurlRequest[[]jira_client_dto.JiraField](ctx, c.base.cfg.JiraBaseURL+"/rest/api/3/field", c.base.authHeaders(), nil, http.MethodGet, 15000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: GetNumberFields failed: %w", err)
	}
	out := []jira_client_dto.NumberField{}
	for _, f := range *res {
		// Only custom fields: IssueFields.Numbers keeps customfield_* only.
		if f.Schema.Type != "number" || !strings.HasPrefix(f.ID, "customfield_") {
			continue
		}
		out = append(out, jira_client_dto.NumberField{ID: f.ID, Name: f.Name, StoryPointLike: strings.Contains(strings.ToLower(f.Name), "story point")})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StoryPointLike != out[j].StoryPointLike {
			return out[i].StoryPointLike
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./modules/jira_client/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/jira_client/jira_client_dto/project_meta.go modules/jira_client/jira_client_service/jira_project_meta.go modules/jira_client/jira_client_service/jira_project_meta_test.go
git commit -m "feat(tracking-rules): read jira project statuses and number fields" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Modul `tracking_rule` — DTO + repository MySQL

**Files:**
- Create: `modules/tracking_rule/dto.go`, `modules/tracking_rule/repository.go`
- Test: `modules/tracking_rule/repository_test.go`

**Interfaces:**
- Consumes: tabel `ms_tracking_rule` (Task 1), `jira_client_dto.TrackingRule`, `jira_client_dto.NumberField`, `jira_client_dto.BuiltinRules` (Task 2/6).
- Produces (package `tracking_rule`):
  - `type Rule struct { ProjectKey, ProjectName string; TrackedStatuses []string; DoneStatus, ReviewStatus string; StoryPointFields []string; StoryPointWriteField, StoryPointWriteFieldSubtask string; IsActive bool; EditedBy string; EditedDate time.Time }` + `func (r Rule) Tracking() jira_client_dto.TrackingRule`
  - `FieldRef`, `RuleView`, `RuleInput`, `Lookup` (JSON persis spec), `ValidationError{Fields map[string]string}`
  - errors: `ErrInvalidProjectKey, ErrProjectNotFound, ErrJiraUnavailable, ErrRuleExists, ErrRuleNotFound, ErrLastActiveRule, ErrAlreadyRunning, ErrBoardNotFound`
  - `func NewRepository(db *sql.DB) *Repository` dengan `List(ctx) ([]Rule, error)`, `Find(ctx, key) (Rule, bool, error)`, `Insert(ctx, Rule) error` (`ErrRuleExists` pada duplicate), `Update(ctx, Rule) error`, `SetActive(ctx, key string, active bool, editedBy string) error`.

- [ ] **Step 1: Write the failing test** (`repository_test.go`)

```go
package tracking_rule

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"nudge/base/testutil"
	"nudge/modules/jira_client/jira_client_dto"
)

func setupRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenTestDB(t)
	cleanup := func() { db.Exec(`DELETE FROM ms_tracking_rule WHERE projectKey LIKE 'ZZTRT%'`) }
	cleanup()
	t.Cleanup(cleanup)
	return NewRepository(db)
}

// AC1.1/AC1.2: the seeded rows are exactly the builtin rules the code used.
func TestRepository_SeedEqualsTheBuiltinRules(t *testing.T) {
	rules, err := setupRepo(t).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Rule{}
	for _, r := range rules {
		byKey[r.ProjectKey] = r
	}
	for _, want := range jira_client_dto.BuiltinRules() {
		got, ok := byKey[want.ProjectKey]
		if !ok || !got.IsActive || !reflect.DeepEqual(got.Tracking(), want) {
			t.Errorf("%s = %+v, want %+v", want.ProjectKey, got.Tracking(), want)
		}
	}
}

func TestRepository_InsertFindUpdateSetActive(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	rule := Rule{
		ProjectKey: "ZZTRT", ProjectName: "Test", TrackedStatuses: []string{"In Progress", "Review"}, DoneStatus: "Done", ReviewStatus: "Review",
		StoryPointFields: []string{"customfield_1", "customfield_2"}, StoryPointWriteField: "customfield_1", StoryPointWriteFieldSubtask: "customfield_2",
		IsActive: true, EditedBy: "admin@esb.co.id",
	}
	if err := repo.Insert(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, rule); !errors.Is(err, ErrRuleExists) {
		t.Fatalf("second insert err = %v, want ErrRuleExists", err)
	}
	got, found, err := repo.Find(ctx, "ZZTRT")
	got.EditedDate = rule.EditedDate
	if err != nil || !found || !reflect.DeepEqual(got, rule) {
		t.Fatalf("Find = %+v, %v, %v", got, found, err)
	}

	rule.ProjectName, rule.TrackedStatuses, rule.ReviewStatus, rule.EditedBy = "Test 2", []string{"In Progress"}, "", "other@esb.co.id"
	if err := repo.Update(ctx, rule); err != nil {
		t.Fatal(err)
	}
	got, _, _ = repo.Find(ctx, "ZZTRT")
	if got.ProjectName != "Test 2" || got.ReviewStatus != "" || len(got.TrackedStatuses) != 1 || got.EditedBy != "other@esb.co.id" || !got.IsActive {
		t.Fatalf("after update = %+v", got)
	}

	if err := repo.SetActive(ctx, "ZZTRT", false, "x@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = repo.Find(ctx, "ZZTRT")
	if got.IsActive || got.EditedBy != "x@esb.co.id" {
		t.Fatalf("after deactivate = %+v", got)
	}
	if _, found, err := repo.Find(ctx, "ZZTRT_NONE"); found || err != nil {
		t.Fatalf("unknown key: found=%v err=%v", found, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/tracking_rule/ -v -count=1`
Expected: FAIL — package/`NewRepository` undefined.

- [ ] **Step 3: Implement**

`modules/tracking_rule/dto.go`:
```go
// Package tracking_rule manages ms_tracking_rule (Tech Spec "Aturan Board
// (Admin CRUD)"): admin CRUD validated against Jira, and loading the active
// rules into the jira_client_dto registry every tracking module reads.
package tracking_rule

import (
	"errors"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
)

// Rule is one ms_tracking_rule row.
type Rule struct {
	ProjectKey                  string
	ProjectName                 string
	TrackedStatuses             []string
	DoneStatus                  string
	ReviewStatus                string // "" = none (NULL)
	StoryPointFields            []string
	StoryPointWriteField        string
	StoryPointWriteFieldSubtask string
	IsActive                    bool
	EditedBy                    string
	EditedDate                  time.Time
}

// Tracking is the rule as the registry holds it.
func (r Rule) Tracking() jira_client_dto.TrackingRule {
	return jira_client_dto.TrackingRule{
		ProjectKey:                  r.ProjectKey,
		TrackedStatuses:             r.TrackedStatuses,
		DoneStatus:                  r.DoneStatus,
		ReviewStatus:                r.ReviewStatus,
		StoryPointFields:            r.StoryPointFields,
		StoryPointWriteField:        r.StoryPointWriteField,
		StoryPointWriteFieldSubtask: r.StoryPointWriteFieldSubtask,
	}
}

// FieldRef is a Story Point field with its Jira name.
type FieldRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RuleView is one rule in the API (GET list item, POST/PUT response).
type RuleView struct {
	ProjectKey                  string     `json:"projectKey"`
	ProjectName                 string     `json:"projectName"`
	TrackedStatuses             []string   `json:"trackedStatuses"`
	DoneStatus                  string     `json:"doneStatus"`
	ReviewStatus                *string    `json:"reviewStatus"`
	StoryPointFields            []FieldRef `json:"storyPointFields"`
	StoryPointWriteField        string     `json:"storyPointWriteField"`
	StoryPointWriteFieldSubtask string     `json:"storyPointWriteFieldSubtask"`
	IsActive                    bool       `json:"isActive"`
	EditedBy                    string     `json:"editedBy"`
	EditedDate                  time.Time  `json:"editedDate"`
}

// RuleInput is the POST/PUT body (projectKey is ignored on PUT).
type RuleInput struct {
	ProjectKey                  string   `json:"projectKey"`
	TrackedStatuses             []string `json:"trackedStatuses"`
	DoneStatus                  string   `json:"doneStatus"`
	ReviewStatus                *string  `json:"reviewStatus"`
	StoryPointFields            []string `json:"storyPointFields"`
	StoryPointWriteField        string   `json:"storyPointWriteField"`
	StoryPointWriteFieldSubtask string   `json:"storyPointWriteFieldSubtask"`
}

// Lookup is what the form picks from: GET /admin/tracking-rules/jira/:projectKey.
type Lookup struct {
	ProjectKey   string                        `json:"projectKey"`
	ProjectName  string                        `json:"projectName"`
	AvatarURL    string                        `json:"avatarUrl"`
	Statuses     []string                      `json:"statuses"`
	NumberFields []jira_client_dto.NumberField `json:"numberFields"`
}

var (
	ErrInvalidProjectKey = errors.New("tracking_rule: invalid project key")
	ErrProjectNotFound   = errors.New("tracking_rule: project not found in jira")
	ErrJiraUnavailable   = errors.New("tracking_rule: jira unavailable")
	ErrRuleExists        = errors.New("tracking_rule: rule exists")
	ErrRuleNotFound      = errors.New("tracking_rule: rule not found")
	ErrLastActiveRule    = errors.New("tracking_rule: last active rule")
	ErrAlreadyRunning    = errors.New("tracking_rule: recalculation already running")
	ErrBoardNotFound     = errors.New("tracking_rule: board not found")
)

// ValidationError carries one message per invalid input (400 validation_failed).
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "tracking_rule: validation failed" }
```

`modules/tracking_rule/repository.go`:
```go
package tracking_rule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

const mysqlDuplicateKeyErrNo = 1062

const ruleColumns = `projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields,
	storyPointWriteField, storyPointWriteFieldSubtask, isActive, editedBy, editedDate`

// Repository reads and writes ms_tracking_rule.
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type rowScanner interface{ Scan(dest ...any) error }

func scanRule(row rowScanner) (Rule, error) {
	var r Rule
	var tracked, fields []byte
	var review sql.NullString
	if err := row.Scan(&r.ProjectKey, &r.ProjectName, &tracked, &r.DoneStatus, &review, &fields,
		&r.StoryPointWriteField, &r.StoryPointWriteFieldSubtask, &r.IsActive, &r.EditedBy, &r.EditedDate); err != nil {
		return Rule{}, err
	}
	if err := json.Unmarshal(tracked, &r.TrackedStatuses); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: trackedStatuses of %s: %w", r.ProjectKey, err)
	}
	if err := json.Unmarshal(fields, &r.StoryPointFields); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: storyPointFields of %s: %w", r.ProjectKey, err)
	}
	r.ReviewStatus = review.String
	return r, nil
}

func (r *Repository) List(ctx context.Context) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM ms_tracking_rule ORDER BY projectKey`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) Find(ctx context.Context, projectKey string) (Rule, bool, error) {
	rule, err := scanRule(r.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM ms_tracking_rule WHERE projectKey = ?`, projectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, false, nil
	}
	if err != nil {
		return Rule{}, false, err
	}
	return rule, true, nil
}

// jsonArg encodes a list for a JSON column. Sent as a string: MySQL rejects
// a JSON value built from the binary charset a []byte argument gets.
func jsonArg(list []string) (string, error) {
	if list == nil {
		list = []string{}
	}
	b, err := json.Marshal(list)
	return string(b), err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) Insert(ctx context.Context, rule Rule) error {
	tracked, err := jsonArg(rule.TrackedStatuses)
	if err != nil {
		return err
	}
	fields, err := jsonArg(rule.StoryPointFields)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO ms_tracking_rule
		(projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask, isActive, editedBy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ProjectKey, rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, rule.IsActive, rule.EditedBy)
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateKeyErrNo {
		return ErrRuleExists
	}
	return err
}

// Update rewrites a rule's settings (not isActive). The caller checks that
// the row exists: MySQL reports 0 affected rows for an unchanged row.
func (r *Repository) Update(ctx context.Context, rule Rule) error {
	tracked, err := jsonArg(rule.TrackedStatuses)
	if err != nil {
		return err
	}
	fields, err := jsonArg(rule.StoryPointFields)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE ms_tracking_rule SET projectName = ?, trackedStatuses = ?, doneStatus = ?, reviewStatus = ?,
		storyPointFields = ?, storyPointWriteField = ?, storyPointWriteFieldSubtask = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP
		WHERE projectKey = ?`,
		rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, rule.EditedBy, rule.ProjectKey)
	return err
}

func (r *Repository) SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_tracking_rule SET isActive = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP WHERE projectKey = ?`,
		active, editedBy, projectKey)
	return err
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./modules/tracking_rule/ -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/tracking_rule
git commit -m "feat(tracking-rules): add tracking rule repository" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Service `tracking_rule` — validasi, CRUD, lookup cache, Reload, refresh, hitung ulang, status board

**Files:**
- Create: `modules/tracking_rule/validate.go`, `modules/tracking_rule/service.go`
- Test: `modules/tracking_rule/service_test.go`

**Interfaces:**
- Consumes: `Rule`, `RuleView`, `RuleInput`, `Lookup`, errors (Task 7); `jira_client_service.ProjectMetaClient`, `jira_client_service.ErrProjectNotFound` (Task 6); `jira_client_dto.SetRules` (Task 2); `board_dto.Board`.
- Produces:
  - `func NormalizeProjectKey(raw string) (string, bool)`
  - `type Store interface { List(ctx context.Context) ([]Rule, error); Find(ctx context.Context, projectKey string) (Rule, bool, error); Insert(ctx context.Context, rule Rule) error; Update(ctx context.Context, rule Rule) error; SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error }` (dipenuhi `*Repository`)
  - `type Recalculator interface { RunAll(ctx context.Context) int }` (dipenuhi `sprint_backfill_service.SprintBackfillService`)
  - `type BoardLookup interface { Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error) }` (dipenuhi `board_service.BoardService`)
  - `type Deps struct { Store Store; Jira jira_client_service.ProjectMetaClient; Boards BoardLookup; Recalc Recalculator; Apply func([]jira_client_dto.TrackingRule); Now func() time.Time }`
  - `func NewService(d Deps) *Service` dengan: `Reload(ctx) error`, `RunRefresh(ctx)`, `List(ctx) ([]RuleView, error)`, `Lookup(ctx, projectKey string) (Lookup, error)`, `Create(ctx, in RuleInput, editor string) (RuleView, error)`, `Update(ctx, projectKey string, in RuleInput, editor string) (RuleView, error)`, `SetActive(ctx, projectKey string, active bool, editor string) (RuleView, error)`, `Recalculate() error`, `BoardStatuses(ctx, boardID int64) ([]string, error)`; const `RefreshInterval = 5 * time.Minute`.

- [ ] **Step 1: Write the failing test** (`service_test.go`)

```go
package tracking_rule

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
)

var ctx = context.Background()

type fakeStore struct {
	rules   map[string]Rule
	listErr error
}

func newFakeStore(rules ...Rule) *fakeStore {
	s := &fakeStore{rules: map[string]Rule{}}
	for _, r := range rules {
		s.rules[r.ProjectKey] = r
	}
	return s
}

func (s *fakeStore) List(context.Context) ([]Rule, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectKey < out[j].ProjectKey })
	return out, nil
}

func (s *fakeStore) Find(_ context.Context, key string) (Rule, bool, error) {
	r, ok := s.rules[key]
	return r, ok, nil
}

func (s *fakeStore) Insert(_ context.Context, r Rule) error {
	if _, ok := s.rules[r.ProjectKey]; ok {
		return ErrRuleExists
	}
	s.rules[r.ProjectKey] = r
	return nil
}

func (s *fakeStore) Update(_ context.Context, r Rule) error {
	s.rules[r.ProjectKey] = r
	return nil
}

func (s *fakeStore) SetActive(_ context.Context, key string, active bool, by string) error {
	r := s.rules[key]
	r.IsActive, r.EditedBy = active, by
	s.rules[key] = r
	return nil
}

type fakeJira struct {
	projects map[string]jira_client_dto.ProjectInfo
	statuses map[string][]string
	fields   []jira_client_dto.NumberField
	err      error
	calls    int
}

func (f *fakeJira) GetProject(_ context.Context, key string) (jira_client_dto.ProjectInfo, error) {
	f.calls++
	if f.err != nil {
		return jira_client_dto.ProjectInfo{}, f.err
	}
	p, ok := f.projects[key]
	if !ok {
		return jira_client_dto.ProjectInfo{}, jira_client_service.ErrProjectNotFound
	}
	return p, nil
}

func (f *fakeJira) GetProjectStatuses(_ context.Context, key string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.statuses[key]
	if !ok {
		return nil, jira_client_service.ErrProjectNotFound
	}
	return s, nil
}

func (f *fakeJira) GetNumberFields(context.Context) ([]jira_client_dto.NumberField, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fields, nil
}

func newFakeJira() *fakeJira {
	return &fakeJira{
		projects: map[string]jira_client_dto.ProjectInfo{"POS": {Key: "POS", Name: "ESB POS"}, "CORE": {Key: "CORE", Name: "ESB Core"}, "OMS": {Key: "OMS", Name: "ESB OMS"}},
		statuses: map[string][]string{
			"POS":  {"To Do", "In Progress", "Code Review", "Done"},
			"CORE": {"To Do", "Code Process", "CODE REVIEW", "Code Completed", "Done"},
			"OMS":  {"To Do", "Code Process", "Code Review", "Done"},
		},
		fields: []jira_client_dto.NumberField{
			{ID: "customfield_10016", Name: "Story point estimate", StoryPointLike: true},
			{ID: "customfield_10125", Name: "Story Point Estimate", StoryPointLike: true},
			{ID: "customfield_11053", Name: "Story Points", StoryPointLike: true},
			{ID: "customfield_200", Name: "Effort"},
		},
	}
}

type fakeRecalc struct{ started, release chan struct{} }

func (f *fakeRecalc) RunAll(context.Context) int {
	if f.started != nil {
		f.started <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	return 0
}

type fakeBoards map[int64]board_dto.Board

func (f fakeBoards) Get(_ context.Context, id int64) (board_dto.Board, bool, error) {
	b, ok := f[id]
	return b, ok, nil
}

// seedRules are OMS & CORE as the 0021b seed (= the builtin rules).
func seedRules() []Rule {
	var out []Rule
	for _, t := range jira_client_dto.BuiltinRules() {
		out = append(out, Rule{
			ProjectKey: t.ProjectKey, ProjectName: "ESB " + t.ProjectKey, TrackedStatuses: t.TrackedStatuses, DoneStatus: t.DoneStatus,
			ReviewStatus: t.ReviewStatus, StoryPointFields: t.StoryPointFields, StoryPointWriteField: t.StoryPointWriteField,
			StoryPointWriteFieldSubtask: t.StoryPointWriteFieldSubtask, IsActive: true, EditedBy: "seed",
		})
	}
	return out // [CORE, OMS]
}

type harness struct {
	svc     *Service
	store   *fakeStore
	jira    *fakeJira
	applied [][]jira_client_dto.TrackingRule
	now     time.Time
}

func newHarness(rules ...Rule) *harness {
	h := &harness{store: newFakeStore(rules...), jira: newFakeJira(), now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	h.svc = NewService(Deps{
		Store: h.store, Jira: h.jira, Recalc: &fakeRecalc{},
		Boards: fakeBoards{26: {BoardID: 26, ProjectKey: "CORE"}, 99: {BoardID: 99}},
		Apply:  func(r []jira_client_dto.TrackingRule) { h.applied = append(h.applied, r) },
		Now:    func() time.Time { return h.now },
	})
	return h
}

func (h *harness) lastAppliedKeys() []string {
	if len(h.applied) == 0 {
		return nil
	}
	keys := []string{}
	for _, r := range h.applied[len(h.applied)-1] {
		keys = append(keys, r.ProjectKey)
	}
	return keys
}

func validPOS() RuleInput {
	review := "Code Review"
	return RuleInput{
		ProjectKey: " pos ", TrackedStatuses: []string{"in progress", "Code Review"}, DoneStatus: "done", ReviewStatus: &review,
		StoryPointFields: []string{"customfield_10016", "customfield_200"}, StoryPointWriteField: "customfield_10016", StoryPointWriteFieldSubtask: "customfield_200",
	}
}

func TestReload_AppliesActiveRulesAndKeepsTheLastOnError(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	if err := h.svc.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.lastAppliedKeys(); !reflect.DeepEqual(got, []string{"CORE"}) {
		t.Fatalf("applied = %v, want only the active CORE", got)
	}
	if !reflect.DeepEqual(h.applied[0][0], jira_client_dto.BuiltinRules()[0]) {
		t.Errorf("CORE = %+v, want the builtin rule", h.applied[0][0])
	}
	h.store.listErr = errors.New("db down")
	if err := h.svc.Reload(ctx); err == nil {
		t.Fatal("a failed reload must return its error")
	}
	if len(h.applied) != 1 {
		t.Error("a failed reload must not touch the registry (AC5.6)")
	}
}

func TestReload_NoActiveRuleAppliesAnEmptySet(t *testing.T) {
	h := newHarness()
	if err := h.svc.Reload(ctx); err != nil || len(h.applied) != 1 || len(h.applied[0]) != 0 {
		t.Fatalf("err=%v applied=%v, want one empty set (AC1.4)", err, h.applied)
	}
}

func TestLookup_NormalizesCachesAndMapsJiraErrors(t *testing.T) {
	h := newHarness()
	l, err := h.svc.Lookup(ctx, " pos ")
	if err != nil || l.ProjectKey != "POS" || l.ProjectName != "ESB POS" || len(l.Statuses) != 4 || len(l.NumberFields) != 4 {
		t.Fatalf("lookup = %+v, %v", l, err)
	}
	h.svc.Lookup(ctx, "POS")
	if h.jira.calls != 1 {
		t.Errorf("jira calls = %d, want 1 (cached 10 minutes)", h.jira.calls)
	}
	h.now = h.now.Add(11 * time.Minute)
	h.svc.Lookup(ctx, "POS")
	if h.jira.calls != 2 {
		t.Errorf("jira calls = %d, want 2 after the cache expired", h.jira.calls)
	}
	if _, err := h.svc.Lookup(ctx, "1POS"); !errors.Is(err, ErrInvalidProjectKey) {
		t.Errorf("bad key err = %v", err)
	}
	if _, err := h.svc.Lookup(ctx, "NOPE"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v", err)
	}
	h.jira.err = errors.New("timeout")
	if _, err := h.svc.Lookup(ctx, "HR"); !errors.Is(err, ErrJiraUnavailable) {
		t.Errorf("jira down err = %v", err)
	}
}

func TestCreate_RejectsInputThatDoesNotMatchJira(t *testing.T) {
	cases := map[string]struct {
		mutate func(*RuleInput)
		field  string
	}{
		"bad key":            {func(in *RuleInput) { in.ProjectKey = "pos-1" }, "projectKey"},
		"no tracked":         {func(in *RuleInput) { in.TrackedStatuses, in.ReviewStatus = nil, nil }, "trackedStatuses"},
		"unknown status":     {func(in *RuleInput) { in.TrackedStatuses = append(in.TrackedStatuses, "Testing") }, "trackedStatuses"},
		"done is tracked":    {func(in *RuleInput) { in.DoneStatus = "In Progress" }, "doneStatus"},
		"no done":            {func(in *RuleInput) { in.DoneStatus = " " }, "doneStatus"},
		"review not tracked": {func(in *RuleInput) { r := "To Do"; in.ReviewStatus = &r }, "reviewStatus"},
		"no fields":          {func(in *RuleInput) { in.StoryPointFields = nil }, "storyPointFields"},
		"not a number field": {func(in *RuleInput) { in.StoryPointFields = append(in.StoryPointFields, "customfield_999") }, "storyPointFields"},
		"duplicate field":    {func(in *RuleInput) { in.StoryPointFields = append(in.StoryPointFields, "customfield_10016") }, "storyPointFields"},
		"write not read":     {func(in *RuleInput) { in.StoryPointWriteField = "customfield_11053" }, "storyPointWriteField"},
		"no subtask write":   {func(in *RuleInput) { in.StoryPointWriteFieldSubtask = "" }, "storyPointWriteFieldSubtask"},
	}
	for name, c := range cases {
		h := newHarness(seedRules()...)
		in := validPOS()
		c.mutate(&in)
		_, err := h.svc.Create(ctx, in, "admin@esb.co.id")
		var v *ValidationError
		if !errors.As(err, &v) || v.Fields[c.field] == "" {
			t.Errorf("%s: err = %v, want a %s error (AC3.7)", name, err, c.field)
		}
		if _, stored := h.store.rules["POS"]; stored {
			t.Errorf("%s: an invalid rule was stored", name)
		}
	}
}

func TestCreate_StoresJiraSpellingReloadsAndRefusesDuplicates(t *testing.T) {
	h := newHarness(seedRules()...)
	got, err := h.svc.Create(ctx, validPOS(), "admin@esb.co.id")
	if err != nil {
		t.Fatal(err)
	}
	stored := h.store.rules["POS"]
	if !reflect.DeepEqual(stored.TrackedStatuses, []string{"In Progress", "Code Review"}) || stored.DoneStatus != "Done" ||
		stored.ReviewStatus != "Code Review" || stored.ProjectName != "ESB POS" || !stored.IsActive || stored.EditedBy != "admin@esb.co.id" {
		t.Fatalf("stored = %+v", stored)
	}
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"CORE", "OMS", "POS"}) {
		t.Errorf("registry = %v, want POS added right after the save (AC3.4)", keys)
	}
	if got.ProjectKey != "POS" || got.ReviewStatus == nil || *got.ReviewStatus != "Code Review" ||
		!reflect.DeepEqual(got.StoryPointFields, []FieldRef{{ID: "customfield_10016", Name: "Story point estimate"}, {ID: "customfield_200", Name: "Effort"}}) {
		t.Errorf("view = %+v", got)
	}
	if _, err := h.svc.Create(ctx, validPOS(), "admin@esb.co.id"); !errors.Is(err, ErrRuleExists) {
		t.Errorf("duplicate err = %v, want ErrRuleExists (AC3.6)", err)
	}
	in := validPOS()
	in.ProjectKey = "NOPE"
	if _, err := h.svc.Create(ctx, in, "admin@esb.co.id"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v (AC3.5)", err)
	}
}

func TestUpdate_KeepsTheActiveFlagAndTheKey(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	in := RuleInput{ProjectKey: "IGNORED", TrackedStatuses: []string{"Code Process"}, DoneStatus: "Code Review",
		StoryPointFields: []string{"customfield_10016"}, StoryPointWriteField: "customfield_10016", StoryPointWriteFieldSubtask: "customfield_10016"}
	got, err := h.svc.Update(ctx, "oms", in, "admin@esb.co.id")
	if err != nil || got.ProjectKey != "OMS" || got.IsActive || got.EditedBy != "admin@esb.co.id" || got.ReviewStatus != nil {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if _, stored := h.store.rules["IGNORED"]; stored {
		t.Error("the body's projectKey must be ignored on PUT")
	}
	if _, err := h.svc.Update(ctx, "POS", in, "admin@esb.co.id"); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("unknown rule err = %v", err)
	}
}

func TestSetActive_NeverLeavesZeroActiveRules(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	if _, err := h.svc.SetActive(ctx, "CORE", false, "admin@esb.co.id"); !errors.Is(err, ErrLastActiveRule) {
		t.Fatalf("err = %v, want ErrLastActiveRule (AC4.3)", err)
	}
	if got, err := h.svc.SetActive(ctx, "oms", true, "admin@esb.co.id"); err != nil || !got.IsActive {
		t.Fatalf("activate OMS = %+v, %v", got, err)
	}
	if _, err := h.svc.SetActive(ctx, "CORE", false, "admin@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"OMS"}) {
		t.Errorf("registry = %v, want only OMS (AC4.2)", keys)
	}
	if _, err := h.svc.SetActive(ctx, "POS", true, "admin@esb.co.id"); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("unknown rule err = %v", err)
	}
}

func TestList_ShowsFieldNamesAndFallsBackToIDs(t *testing.T) {
	h := newHarness(seedRules()...)
	list, err := h.svc.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	want := []FieldRef{{ID: "customfield_10125", Name: "Story Point Estimate"}, {ID: "customfield_11053", Name: "Story Points"}}
	if !reflect.DeepEqual(list[0].StoryPointFields, want) || list[0].ReviewStatus == nil || list[1].ReviewStatus != nil {
		t.Errorf("CORE/OMS views = %+v", list)
	}
	down := newHarness(seedRules()...)
	down.jira.err = errors.New("jira down")
	list, err = down.svc.List(ctx)
	if err != nil || list[1].StoryPointFields[0] != (FieldRef{ID: "customfield_10016", Name: "customfield_10016"}) {
		t.Errorf("with jira down = %+v, %v; want the ID as name", list, err)
	}
}

func TestRecalculate_OneRunAtATime(t *testing.T) {
	h := newHarness(seedRules()...)
	recalc := &fakeRecalc{started: make(chan struct{}, 1), release: make(chan struct{})}
	h.svc.d.Recalc = recalc
	if err := h.svc.Recalculate(); err != nil {
		t.Fatal(err)
	}
	<-recalc.started
	if err := h.svc.Recalculate(); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second call err = %v, want ErrAlreadyRunning", err)
	}
	close(recalc.release)
}

func TestBoardStatuses_CachedPerProjectForAnHour(t *testing.T) {
	h := newHarness()
	got, err := h.svc.BoardStatuses(ctx, 26)
	if err != nil || !reflect.DeepEqual(got, newFakeJira().statuses["CORE"]) {
		t.Fatalf("statuses = %v, %v", got, err)
	}
	h.jira.err = errors.New("jira down")
	if again, err := h.svc.BoardStatuses(ctx, 26); err != nil || len(again) != 5 {
		t.Errorf("cached = %v, %v", again, err)
	}
	h.now = h.now.Add(61 * time.Minute)
	if _, err := h.svc.BoardStatuses(ctx, 26); !errors.Is(err, ErrJiraUnavailable) {
		t.Errorf("expired + jira down err = %v", err)
	}
	for _, id := range []int64{99, 7} { // board without project / unknown board
		if _, err := h.svc.BoardStatuses(ctx, id); !errors.Is(err, ErrBoardNotFound) {
			t.Errorf("board %d err = %v", id, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/tracking_rule/ -run 'TestReload|TestLookup|TestCreate|TestUpdate|TestSetActive|TestList|TestRecalculate|TestBoardStatuses' -v -count=1`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Implement**

`modules/tracking_rule/validate.go`:
```go
package tracking_rule

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,19}$`)

// NormalizeProjectKey trims and upper-cases raw (AC3.8); ok=false when the
// result is not a valid Jira project key.
func NormalizeProjectKey(raw string) (string, bool) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	return key, projectKeyPattern.MatchString(key)
}

func containsFold(list []string, value string) bool {
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// buildRule validates in against the project's Jira data (AC3.7) and returns
// the rule with Jira's spelling of every status; errs holds one message per
// invalid input (empty = valid).
func buildRule(in RuleInput, l Lookup) (Rule, map[string]string) {
	errs := map[string]string{}
	jiraStatus := func(name string) (string, bool) {
		for _, s := range l.Statuses {
			if strings.EqualFold(s, strings.TrimSpace(name)) {
				return s, true
			}
		}
		return "", false
	}
	notInJira := func(name string) string {
		return fmt.Sprintf("Status %q tidak ada di workflow Jira project ini", strings.TrimSpace(name))
	}

	var rule Rule
	for _, raw := range in.TrackedStatuses {
		status, ok := jiraStatus(raw)
		if !ok {
			errs["trackedStatuses"] = notInJira(raw)
			continue
		}
		if !containsFold(rule.TrackedStatuses, status) {
			rule.TrackedStatuses = append(rule.TrackedStatuses, status)
		}
	}
	if len(in.TrackedStatuses) == 0 {
		errs["trackedStatuses"] = "Pilih minimal satu status yang dihitung"
	}

	switch done, ok := jiraStatus(in.DoneStatus); {
	case strings.TrimSpace(in.DoneStatus) == "":
		errs["doneStatus"] = "Status selesai wajib dipilih"
	case !ok:
		errs["doneStatus"] = notInJira(in.DoneStatus)
	case containsFold(rule.TrackedStatuses, done):
		errs["doneStatus"] = "Status selesai tidak boleh termasuk status yang dihitung"
	default:
		rule.DoneStatus = done
	}

	if in.ReviewStatus != nil && strings.TrimSpace(*in.ReviewStatus) != "" {
		review, ok := jiraStatus(*in.ReviewStatus)
		if !ok || !containsFold(rule.TrackedStatuses, review) {
			errs["reviewStatus"] = "Status review harus salah satu status yang dihitung"
		} else {
			rule.ReviewStatus = review
		}
	}

	numeric := map[string]bool{}
	for _, f := range l.NumberFields {
		numeric[f.ID] = true
	}
	for _, raw := range in.StoryPointFields {
		id := strings.TrimSpace(raw)
		switch {
		case !numeric[id]:
			errs["storyPointFields"] = fmt.Sprintf("Field %s tidak ada di Jira atau bukan field angka", id)
		case slices.Contains(rule.StoryPointFields, id):
			errs["storyPointFields"] = fmt.Sprintf("Field %s dipilih lebih dari sekali", id)
		default:
			rule.StoryPointFields = append(rule.StoryPointFields, id)
		}
	}
	if len(in.StoryPointFields) == 0 {
		errs["storyPointFields"] = "Pilih minimal satu field Story Point"
	}

	writeField := func(name, raw, label string) string {
		value := strings.TrimSpace(raw)
		switch {
		case value == "":
			errs[name] = "Field SP tulis (" + label + ") wajib dipilih"
		case !slices.Contains(rule.StoryPointFields, value):
			errs[name] = "Field SP tulis (" + label + ") harus salah satu field SP baca"
		}
		return value
	}
	rule.StoryPointWriteField = writeField("storyPointWriteField", in.StoryPointWriteField, "card biasa")
	rule.StoryPointWriteFieldSubtask = writeField("storyPointWriteFieldSubtask", in.StoryPointWriteFieldSubtask, "subtask")
	return rule, errs
}
```

`modules/tracking_rule/service.go`:
```go
package tracking_rule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
)

const (
	// RefreshInterval: rules changed outside this process (another
	// instance, a manual DB edit) apply within this window (AC5.4).
	RefreshInterval = 5 * time.Minute
	lookupTTL       = 10 * time.Minute
	boardStatusTTL  = time.Hour
)

type Store interface {
	List(ctx context.Context) ([]Rule, error)
	Find(ctx context.Context, projectKey string) (Rule, bool, error)
	Insert(ctx context.Context, rule Rule) error
	Update(ctx context.Context, rule Rule) error
	SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error
}

// Recalculator is sprint_backfill_service's "every user" run.
type Recalculator interface {
	RunAll(ctx context.Context) int
}

// BoardLookup is the cached ms_board read of board_service.
type BoardLookup interface {
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
}

type Deps struct {
	Store  Store
	Jira   jira_client_service.ProjectMetaClient
	Boards BoardLookup
	Recalc Recalculator
	// Apply installs the active rules; defaults to jira_client_dto.SetRules.
	Apply func([]jira_client_dto.TrackingRule)
	Now   func() time.Time
}

type cachedLookup struct {
	lookup Lookup
	at     time.Time
}

type cachedFields struct {
	list []jira_client_dto.NumberField
	at   time.Time
}

type cachedStatuses struct {
	list []string
	at   time.Time
}

type Service struct {
	d             Deps
	mu            sync.Mutex
	lookups       map[string]cachedLookup
	fields        *cachedFields
	boardStatuses map[string]cachedStatuses
	recalcRunning atomic.Bool
}

func NewService(d Deps) *Service {
	if d.Apply == nil {
		d.Apply = jira_client_dto.SetRules
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d, lookups: map[string]cachedLookup{}, boardStatuses: map[string]cachedStatuses{}}
}

// Reload loads the table and installs the active rules. On error the
// registry keeps its last rules (AC5.6).
func (s *Service) Reload(ctx context.Context) error {
	rules, err := s.d.Store.List(ctx)
	if err != nil {
		return fmt.Errorf("tracking_rule: reload: %w", err)
	}
	active := make([]jira_client_dto.TrackingRule, 0, len(rules))
	for _, r := range rules {
		if r.IsActive {
			active = append(active, r.Tracking())
		}
	}
	if len(active) == 0 {
		log.Printf("tracking_rule: WARN no active tracking rule — the poller searches nothing")
	}
	s.d.Apply(active)
	return nil
}

// RunRefresh reloads every RefreshInterval until ctx ends.
func (s *Service) RunRefresh(ctx context.Context) {
	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil {
				log.Printf("tracking_rule: ERROR refresh failed, keeping the last rules: %v", err)
			}
		}
	}
}

func jiraErr(err error) error {
	if errors.Is(err, jira_client_service.ErrProjectNotFound) {
		return ErrProjectNotFound
	}
	log.Printf("tracking_rule: jira lookup failed: %v", err)
	return ErrJiraUnavailable
}

func (s *Service) numberFields(ctx context.Context) ([]jira_client_dto.NumberField, error) {
	s.mu.Lock()
	cached := s.fields
	s.mu.Unlock()
	if cached != nil && s.d.Now().Sub(cached.at) < lookupTTL {
		return cached.list, nil
	}
	list, err := s.d.Jira.GetNumberFields(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.fields = &cachedFields{list: list, at: s.d.Now()}
	s.mu.Unlock()
	return list, nil
}

// Lookup is the project's Jira data for the form (AC3.1), cached 10 minutes.
func (s *Service) Lookup(ctx context.Context, projectKey string) (Lookup, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return Lookup{}, ErrInvalidProjectKey
	}
	s.mu.Lock()
	cached, hit := s.lookups[key]
	s.mu.Unlock()
	if hit && s.d.Now().Sub(cached.at) < lookupTTL {
		return cached.lookup, nil
	}
	project, err := s.d.Jira.GetProject(ctx, key)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	statuses, err := s.d.Jira.GetProjectStatuses(ctx, key)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	fields, err := s.numberFields(ctx)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	l := Lookup{ProjectKey: key, ProjectName: project.Name, AvatarURL: project.AvatarURL(), Statuses: statuses, NumberFields: fields}
	s.mu.Lock()
	s.lookups[key] = cachedLookup{lookup: l, at: s.d.Now()}
	s.mu.Unlock()
	return l, nil
}

// fieldNames maps field ID → Jira name; empty when Jira is down (views then
// show the ID as the name).
func (s *Service) fieldNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	fields, err := s.numberFields(ctx)
	if err != nil {
		log.Printf("tracking_rule: field names unavailable: %v", err)
		return names
	}
	for _, f := range fields {
		names[f.ID] = f.Name
	}
	return names
}

func view(r Rule, names map[string]string) RuleView {
	v := RuleView{
		ProjectKey: r.ProjectKey, ProjectName: r.ProjectName, TrackedStatuses: r.TrackedStatuses, DoneStatus: r.DoneStatus,
		StoryPointFields: make([]FieldRef, 0, len(r.StoryPointFields)), StoryPointWriteField: r.StoryPointWriteField,
		StoryPointWriteFieldSubtask: r.StoryPointWriteFieldSubtask, IsActive: r.IsActive, EditedBy: r.EditedBy, EditedDate: r.EditedDate,
	}
	if r.ReviewStatus != "" {
		review := r.ReviewStatus
		v.ReviewStatus = &review
	}
	for _, id := range r.StoryPointFields {
		name := names[id]
		if name == "" {
			name = id
		}
		v.StoryPointFields = append(v.StoryPointFields, FieldRef{ID: id, Name: name})
	}
	return v
}

func (s *Service) List(ctx context.Context) ([]RuleView, error) {
	rules, err := s.d.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	names := s.fieldNames(ctx)
	out := make([]RuleView, 0, len(rules))
	for _, r := range rules {
		out = append(out, view(r, names))
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, in RuleInput, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(in.ProjectKey)
	if !ok {
		return RuleView{}, &ValidationError{Fields: map[string]string{"projectKey": "Project key: huruf besar, angka atau _, 2–20 karakter, diawali huruf"}}
	}
	if _, found, err := s.d.Store.Find(ctx, key); err != nil {
		return RuleView{}, err
	} else if found {
		return RuleView{}, ErrRuleExists
	}
	l, err := s.Lookup(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	rule, errs := buildRule(in, l)
	if len(errs) > 0 {
		return RuleView{}, &ValidationError{Fields: errs}
	}
	rule.ProjectKey, rule.ProjectName, rule.IsActive, rule.EditedBy = key, l.ProjectName, true, editor
	if err := s.d.Store.Insert(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

func (s *Service) Update(ctx context.Context, projectKey string, in RuleInput, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return RuleView{}, ErrRuleNotFound
	}
	existing, found, err := s.d.Store.Find(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	if !found {
		return RuleView{}, ErrRuleNotFound
	}
	l, err := s.Lookup(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	rule, errs := buildRule(in, l)
	if len(errs) > 0 {
		return RuleView{}, &ValidationError{Fields: errs}
	}
	rule.ProjectKey, rule.ProjectName, rule.IsActive, rule.EditedBy = key, l.ProjectName, existing.IsActive, editor
	if rule.ProjectName == "" {
		rule.ProjectName = existing.ProjectName
	}
	if err := s.d.Store.Update(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

// SetActive switches a rule on/off ("hapus" = nonaktifkan). The last active
// rule can't be switched off (AC4.3).
func (s *Service) SetActive(ctx context.Context, projectKey string, active bool, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return RuleView{}, ErrRuleNotFound
	}
	rules, err := s.d.Store.List(ctx)
	if err != nil {
		return RuleView{}, err
	}
	var target *Rule
	activeCount := 0
	for i := range rules {
		if rules[i].IsActive {
			activeCount++
		}
		if rules[i].ProjectKey == key {
			target = &rules[i]
		}
	}
	if target == nil {
		return RuleView{}, ErrRuleNotFound
	}
	if !active && target.IsActive && activeCount == 1 {
		return RuleView{}, ErrLastActiveRule
	}
	if err := s.d.Store.SetActive(ctx, key, active, editor); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

// afterSave refreshes the registry in the same request (AC3.4/AC5.4) and
// returns the stored rule. A failed reload is logged, not returned: the row
// is saved and the 5-minute refresh retries.
func (s *Service) afterSave(ctx context.Context, key string) (RuleView, error) {
	if err := s.Reload(ctx); err != nil {
		log.Printf("tracking_rule: ERROR reload after saving %s failed (the periodic refresh retries): %v", key, err)
	}
	saved, found, err := s.d.Store.Find(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	if !found {
		return RuleView{}, ErrRuleNotFound
	}
	return view(saved, s.fieldNames(ctx)), nil
}

// Recalculate starts sprint_backfill RunAll in the background; one run at a time.
func (s *Service) Recalculate() error {
	if !s.recalcRunning.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	go func() {
		defer s.recalcRunning.Store(false)
		added := s.d.Recalc.RunAll(context.Background())
		log.Printf("tracking_rule: recalculation for every user done (%d session(s) added)", added)
	}()
	return nil
}

// BoardStatuses is the workflow of the board's project (Pengaturan, AC5.5),
// cached an hour per project.
func (s *Service) BoardStatuses(ctx context.Context, boardID int64) ([]string, error) {
	board, found, err := s.d.Boards.Get(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if !found || board.ProjectKey == "" {
		return nil, ErrBoardNotFound
	}
	s.mu.Lock()
	cached, hit := s.boardStatuses[board.ProjectKey]
	s.mu.Unlock()
	if hit && s.d.Now().Sub(cached.at) < boardStatusTTL {
		return cached.list, nil
	}
	list, err := s.d.Jira.GetProjectStatuses(ctx, board.ProjectKey)
	if err != nil {
		log.Printf("tracking_rule: statuses of board %d (%s) failed: %v", boardID, board.ProjectKey, err)
		return nil, ErrJiraUnavailable
	}
	s.mu.Lock()
	s.boardStatuses[board.ProjectKey] = cachedStatuses{list: list, at: s.d.Now()}
	s.mu.Unlock()
	return list, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./modules/tracking_rule/ && go test ./modules/tracking_rule/ -race -count=1`
Expected: PASS (race detector bersih).

- [ ] **Step 5: Commit**

```bash
git add modules/tracking_rule/validate.go modules/tracking_rule/service.go modules/tracking_rule/service_test.go
git commit -m "feat(tracking-rules): add tracking rule service with jira validation" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Handler + routes + wiring `main.go` (Reload sinkron, refresh 5 menit)

**Files:**
- Create: `modules/tracking_rule/handler.go`
- Test: `modules/tracking_rule/handler_test.go`
- Modify: `router.go` (import + `MountTrackingRules` setelah `MountIssueCreate`)
- Modify: `main.go` (import; konstruksi + `Reload` setelah blok `recalcLeave = …` dan sebelum `shutdownMgr := shutdown.NewManager()`; worker refresh; mount setelah `MountChatLog`)

**Interfaces:**
- Consumes: `*Service` methods (Task 8), `auth_middleware.CurrentUserKey`, `auth_dto.Session`.
- Produces: `type API interface { List(ctx context.Context) ([]RuleView, error); Lookup(ctx context.Context, projectKey string) (Lookup, error); Create(ctx context.Context, in RuleInput, editor string) (RuleView, error); Update(ctx context.Context, projectKey string, in RuleInput, editor string) (RuleView, error); SetActive(ctx context.Context, projectKey string, active bool, editor string) (RuleView, error); Recalculate() error; BoardStatuses(ctx context.Context, boardID int64) ([]string, error) }`; `func NewHandler(api API) *Handler`; `func AdminRoutes(g *gin.RouterGroup, h *Handler)`; `func UserRoutes(g *gin.RouterGroup, h *Handler)`; `func MountTrackingRules(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *tracking_rule.Handler)` (package `main`).
- Routes (spec): `GET /api/dashboard/admin/tracking-rules`, `GET …/tracking-rules/jira/:projectKey`, `POST …/tracking-rules`, `PUT …/tracking-rules/:projectKey`, `PUT …/tracking-rules/:projectKey/active`, `POST …/tracking-rules/recalculate`, `GET /api/dashboard/boards/:boardId/statuses`.

- [ ] **Step 1: Write the failing test** (`handler_test.go`)

```go
package tracking_rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type fakeAPI struct {
	err       error
	gotKey    string
	gotEditor string
	gotActive *bool
	gotInput  RuleInput
}

func (f *fakeAPI) List(context.Context) ([]RuleView, error) {
	return []RuleView{{ProjectKey: "CORE"}}, f.err
}
func (f *fakeAPI) Lookup(_ context.Context, key string) (Lookup, error) {
	f.gotKey = key
	return Lookup{ProjectKey: "POS"}, f.err
}
func (f *fakeAPI) Create(_ context.Context, in RuleInput, editor string) (RuleView, error) {
	f.gotInput, f.gotEditor = in, editor
	return RuleView{ProjectKey: "POS"}, f.err
}
func (f *fakeAPI) Update(_ context.Context, key string, in RuleInput, editor string) (RuleView, error) {
	f.gotKey, f.gotInput, f.gotEditor = key, in, editor
	return RuleView{ProjectKey: key}, f.err
}
func (f *fakeAPI) SetActive(_ context.Context, key string, active bool, editor string) (RuleView, error) {
	f.gotKey, f.gotActive, f.gotEditor = key, &active, editor
	return RuleView{ProjectKey: key, IsActive: active}, f.err
}
func (f *fakeAPI) Recalculate() error { return f.err }
func (f *fakeAPI) BoardStatuses(context.Context, int64) ([]string, error) {
	return []string{"To Do"}, f.err
}

func serve(api *fakeAPI, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "admin@esb.co.id"})
	}
	h := NewHandler(api)
	AdminRoutes(r.Group("/api/dashboard/admin", auth), h)
	UserRoutes(r.Group("/api/dashboard", auth), h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRoutes_HappyPaths(t *testing.T) {
	cases := []struct {
		method, path, body string
		code               int
		frag               string
	}{
		{http.MethodGet, "/api/dashboard/admin/tracking-rules", "", http.StatusOK, `"rules":[{"projectKey":"CORE"`},
		{http.MethodGet, "/api/dashboard/admin/tracking-rules/jira/pos", "", http.StatusOK, `"projectKey":"POS"`},
		{http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"POS","trackedStatuses":["In Progress"]}`, http.StatusCreated, `"projectKey":"POS"`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/CORE", `{"doneStatus":"Done"}`, http.StatusOK, `"projectKey":"CORE"`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/OMS/active", `{"active":false}`, http.StatusOK, `"isActive":false`},
		{http.MethodPost, "/api/dashboard/admin/tracking-rules/recalculate", ``, http.StatusAccepted, `"started":true`},
		{http.MethodGet, "/api/dashboard/boards/26/statuses", "", http.StatusOK, `"statuses":["To Do"]`},
	}
	for _, c := range cases {
		api := &fakeAPI{}
		w := serve(api, c.method, c.path, c.body)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.frag) {
			t.Errorf("%s %s = %d %s, want %d with %s", c.method, c.path, w.Code, w.Body, c.code, c.frag)
		}
	}
	api := &fakeAPI{}
	serve(api, http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"POS","trackedStatuses":["In Progress"]}`)
	if api.gotEditor != "admin@esb.co.id" || api.gotInput.ProjectKey != "POS" || len(api.gotInput.TrackedStatuses) != 1 {
		t.Errorf("create got editor=%q input=%+v", api.gotEditor, api.gotInput)
	}
	serve(api, http.MethodPut, "/api/dashboard/admin/tracking-rules/OMS/active", `{"active":false}`)
	if api.gotKey != "OMS" || api.gotActive == nil || *api.gotActive {
		t.Errorf("set active got key=%q active=%v", api.gotKey, api.gotActive)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
		frag string
	}{
		{&ValidationError{Fields: map[string]string{"doneStatus": "wajib"}}, http.StatusBadRequest, `"fields":{"doneStatus":"wajib"}`},
		{ErrInvalidProjectKey, http.StatusBadRequest, `"error":"invalid_project_key"`},
		{ErrProjectNotFound, http.StatusNotFound, `Project POS tidak ditemukan di Jira`},
		{ErrRuleNotFound, http.StatusNotFound, `"error":"rule_not_found"`},
		{ErrRuleExists, http.StatusConflict, `"error":"rule_exists"`},
		{ErrLastActiveRule, http.StatusUnprocessableEntity, `"error":"last_active_rule"`},
		{ErrAlreadyRunning, http.StatusConflict, `"error":"already_running"`},
		{ErrBoardNotFound, http.StatusNotFound, `"error":"board_not_found"`},
		{ErrJiraUnavailable, http.StatusBadGateway, `"error":"jira_unavailable"`},
		{context.DeadlineExceeded, http.StatusInternalServerError, `"error":"internal_error"`},
	}
	for _, c := range cases {
		w := serve(&fakeAPI{err: c.err}, http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"pos"}`)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.frag) {
			t.Errorf("%v: %d %s, want %d with %s", c.err, w.Code, w.Body, c.code, c.frag)
		}
	}
}

func TestBadRequests(t *testing.T) {
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/dashboard/admin/tracking-rules", `{`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/CORE/active", `{}`},
		{http.MethodGet, "/api/dashboard/boards/abc/statuses", ``},
	} {
		if w := serve(&fakeAPI{}, c.method, c.path, c.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", c.method, c.path, w.Code)
		}
	}
}

// The user route shares /boards/:boardId with the existing avatar route —
// a different wildcard name would make gin panic at startup.
func TestUserRoutes_ShareTheBoardsWildcard(t *testing.T) {
	r := gin.New()
	g := r.Group("/api/dashboard")
	g.GET("/boards/:boardId/avatar", func(*gin.Context) {})
	UserRoutes(g, NewHandler(&fakeAPI{}))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/tracking_rule/ -run 'TestRoutes|TestErrorMapping|TestBadRequests|TestUserRoutes' -v -count=1`
Expected: FAIL — `undefined: NewHandler`.

- [ ] **Step 3: Implement**

`modules/tracking_rule/handler.go`:
```go
package tracking_rule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

// API is what the handler needs from Service.
type API interface {
	List(ctx context.Context) ([]RuleView, error)
	Lookup(ctx context.Context, projectKey string) (Lookup, error)
	Create(ctx context.Context, in RuleInput, editor string) (RuleView, error)
	Update(ctx context.Context, projectKey string, in RuleInput, editor string) (RuleView, error)
	SetActive(ctx context.Context, projectKey string, active bool, editor string) (RuleView, error)
	Recalculate() error
	BoardStatuses(ctx context.Context, boardID int64) ([]string, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Admin › Aturan Board on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/tracking-rules", h.list)
	g.GET("/tracking-rules/jira/:projectKey", h.lookup)
	g.POST("/tracking-rules", h.create)
	g.POST("/tracking-rules/recalculate", h.recalculate)
	g.PUT("/tracking-rules/:projectKey", h.update)
	g.PUT("/tracking-rules/:projectKey/active", h.setActive)
}

// UserRoutes mounts the board workflow statuses (Pengaturan) on a group
// already behind AuthMiddlewareAPI. :boardId matches /boards/:boardId/avatar.
func UserRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/boards/:boardId/statuses", h.boardStatuses)
}

const maxBodyBytes = 64 << 10

func editor(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func displayKey(raw string) string {
	key, _ := NormalizeProjectKey(raw)
	return key
}

func fail(c *gin.Context, err error, key string) {
	var v *ValidationError
	switch {
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": "Periksa isian yang ditandai", "fields": v.Fields})
	case errors.Is(err, ErrInvalidProjectKey):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_key", "message": "Format project key tidak valid"})
	case errors.Is(err, ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found", "message": fmt.Sprintf("Project %s tidak ditemukan di Jira", key)})
	case errors.Is(err, ErrRuleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "rule_not_found", "message": "Aturan board tidak ditemukan"})
	case errors.Is(err, ErrRuleExists):
		c.JSON(http.StatusConflict, gin.H{"error": "rule_exists", "message": fmt.Sprintf("Project %s sudah punya aturan — ubah lewat Edit", key)})
	case errors.Is(err, ErrLastActiveRule):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "last_active_rule", "message": "Minimal satu board harus aktif"})
	case errors.Is(err, ErrAlreadyRunning):
		c.JSON(http.StatusConflict, gin.H{"error": "already_running", "message": "Hitung ulang masih berjalan"})
	case errors.Is(err, ErrBoardNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "board_not_found", "message": "Board tidak dikenal"})
	case errors.Is(err, ErrJiraUnavailable):
		c.JSON(http.StatusBadGateway, gin.H{"error": "jira_unavailable", "message": "Jira tidak merespons, coba lagi"})
	default:
		log.Printf("tracking_rule: request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "Terjadi kesalahan, coba lagi"})
	}
}

func bindInput(c *gin.Context) (RuleInput, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Format data tidak valid"})
		return in, false
	}
	return in, true
}

func (h *Handler) list(c *gin.Context) {
	rules, err := h.api.List(c.Request.Context())
	if err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *Handler) lookup(c *gin.Context) {
	key := c.Param("projectKey")
	l, err := h.api.Lookup(c.Request.Context(), key)
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, l)
}

func (h *Handler) create(c *gin.Context) {
	in, ok := bindInput(c)
	if !ok {
		return
	}
	rule, err := h.api.Create(c.Request.Context(), in, editor(c))
	if err != nil {
		fail(c, err, displayKey(in.ProjectKey))
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) update(c *gin.Context) {
	key := c.Param("projectKey")
	in, ok := bindInput(c)
	if !ok {
		return
	}
	rule, err := h.api.Update(c.Request.Context(), key, in, editor(c))
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) setActive(c *gin.Context) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Active == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Isi active: true atau false"})
		return
	}
	key := c.Param("projectKey")
	rule, err := h.api.SetActive(c.Request.Context(), key, *body.Active, editor(c))
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) recalculate(c *gin.Context) {
	if err := h.api.Recalculate(); err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

func (h *Handler) boardStatuses(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_board_id", "message": "Board tidak valid"})
		return
	}
	statuses, err := h.api.BoardStatuses(c.Request.Context(), id)
	if err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"statuses": statuses})
}
```

`router.go` — tambah import `"nudge/modules/tracking_rule"` dan di akhir file:
```go
// MountTrackingRules mounts Admin › Aturan Board (admin only) and the board
// workflow statuses Pengaturan offers (any logged-in user) — Tech Spec
// "Aturan Board (Admin CRUD)".
func MountTrackingRules(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *tracking_rule.Handler) {
	user := r.Group(cfg.BasePath + "/api/dashboard")
	user.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	tracking_rule.UserRoutes(user, h)

	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	tracking_rule.AdminRoutes(admin, h)
}
```

`main.go` — import `"nudge/modules/tracking_rule"`; setelah blok `recalcLeave = func(email string) { … }` dan **sebelum** `shutdownMgr := shutdown.NewManager()`:
```go
	// Tech Spec "Aturan Board (Admin CRUD)": tracking rules come from
	// ms_tracking_rule — loaded before the poller and the backfill start,
	// refreshed on every admin save and every 5 minutes.
	trackingRuleSvc := tracking_rule.NewService(tracking_rule.Deps{
		Store:  tracking_rule.NewRepository(db),
		Jira:   jira_client_service.NewProjectMetaClient(&cfg),
		Boards: boardSvc,
		Recalc: sprintBackfillSvc,
	})
	if err := trackingRuleSvc.Reload(context.Background()); err != nil {
		log.Fatalf("failed to load tracking rules: %v", err)
	}
```
setelah worker `sprintBackfillSvc.RunDaily`:
```go
	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		trackingRuleSvc.RunRefresh(shutdownMgr.Context())
	}()
```
dan setelah `MountChatLog(...)`:
```go
	MountTrackingRules(router, &cfg, authSvc, tracking_rule.NewHandler(trackingRuleSvc))
```

- [ ] **Step 4: Run tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS (seluruh backend, termasuk `main_test.go`).

- [ ] **Step 5: Commit**

```bash
git add modules/tracking_rule/handler.go modules/tracking_rule/handler_test.go router.go main.go
git commit -m "feat(tracking-rules): expose tracking rule api and load rules at startup" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Frontend — `TrackingRuleService` + aksi toast hitung ulang

**Files:**
- Create: `frontend/src/app/core/services/tracking-rule.service.ts`, `frontend/src/app/core/services/tracking-rule.service.spec.ts`
- Create: `frontend/src/app/features/admin/tracking-rules/recalculate-action.ts`

**Interfaces:**
- Consumes: API Task 9.
- Produces: tipe `FieldRef`, `TrackingRule`, `TrackingRuleInput`, `NumberField`, `JiraProjectLookup`, `TrackingRuleField`, `TrackingRuleError`; `PROJECT_KEY_PATTERN`, `normalizeProjectKey(raw: string): string`; `TrackingRuleService` dengan `list(): Observable<{ rules: TrackingRule[] }>`, `lookup(projectKey: string): Observable<JiraProjectLookup>`, `create(input: TrackingRuleInput): Observable<TrackingRule>`, `update(projectKey: string, input: TrackingRuleInput): Observable<TrackingRule>`, `setActive(projectKey: string, active: boolean): Observable<TrackingRule>`, `recalculate(): Observable<{ started: boolean }>`, `boardStatuses(boardId: number): Observable<{ statuses: string[] }>`; `RECALCULATE_LABEL`, `recalculateAction(api, toast): ToastAction`.

- [ ] **Step 1: Write the failing test** (`tracking-rule.service.spec.ts`)

```ts
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PROJECT_KEY_PATTERN, TrackingRuleInput, TrackingRuleService, normalizeProjectKey } from './tracking-rule.service';

describe('TrackingRuleService', () => {
  it('calls the admin tracking-rule endpoints and the board statuses endpoint', () => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    const svc = TestBed.inject(TrackingRuleService);
    const http = TestBed.inject(HttpTestingController);
    const input: TrackingRuleInput = {
      trackedStatuses: ['In Progress'], doneStatus: 'Done', reviewStatus: null,
      storyPointFields: ['customfield_1'], storyPointWriteField: 'customfield_1', storyPointWriteFieldSubtask: 'customfield_1',
    };

    svc.list().subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/admin/tracking-rules')).flush({ rules: [] });
    svc.lookup('POS').subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/admin/tracking-rules/jira/POS')).flush({});
    svc.create({ ...input, projectKey: 'POS' }).subscribe();
    const created = http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/api/dashboard/admin/tracking-rules'));
    expect(created.request.body.projectKey).toBe('POS');
    created.flush({});
    svc.update('POS', input).subscribe();
    http.expectOne((r) => r.method === 'PUT' && r.url.endsWith('/api/dashboard/admin/tracking-rules/POS')).flush({});
    svc.setActive('POS', false).subscribe();
    const active = http.expectOne((r) => r.method === 'PUT' && r.url.endsWith('/api/dashboard/admin/tracking-rules/POS/active'));
    expect(active.request.body).toEqual({ active: false });
    active.flush({});
    svc.recalculate().subscribe();
    http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/api/dashboard/admin/tracking-rules/recalculate')).flush({ started: true });
    svc.boardStatuses(26).subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/boards/26/statuses')).flush({ statuses: [] });
    http.verify();
  });

  it('normalizes and validates project keys like the server (AC3.8)', () => {
    expect(normalizeProjectKey(' pos ')).toBe('POS');
    expect(PROJECT_KEY_PATTERN.test('POS')).toBeTrue();
    expect(PROJECT_KEY_PATTERN.test('P')).toBeFalse();
    expect(PROJECT_KEY_PATTERN.test('1POS')).toBeFalse();
    expect(PROJECT_KEY_PATTERN.test('POS-1')).toBeFalse();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include='**/tracking-rule.service.spec.ts'`
Expected: FAIL — `Cannot find module './tracking-rule.service'`.

- [ ] **Step 3: Implement**

`frontend/src/app/core/services/tracking-rule.service.ts`:
```ts
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** A Story Point field with its Jira name. */
export interface FieldRef {
  id: string;
  name: string;
}

/** One board rule (Tech Spec "Aturan Board (Admin CRUD)"). */
export interface TrackingRule {
  projectKey: string;
  projectName: string;
  trackedStatuses: string[];
  doneStatus: string;
  reviewStatus: string | null;
  /** Read priority order. */
  storyPointFields: FieldRef[];
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
  isActive: boolean;
  editedBy: string;
  editedDate: string;
}

export interface TrackingRuleInput {
  /** POST only; the path carries the key on PUT. */
  projectKey?: string;
  trackedStatuses: string[];
  doneStatus: string;
  reviewStatus: string | null;
  storyPointFields: string[];
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
}

export interface NumberField {
  id: string;
  name: string;
  storyPointLike: boolean;
}

export interface JiraProjectLookup {
  projectKey: string;
  projectName: string;
  avatarUrl: string;
  statuses: string[];
  /** Story-point-like fields first. */
  numberFields: NumberField[];
}

export type TrackingRuleField =
  | 'projectKey'
  | 'trackedStatuses'
  | 'doneStatus'
  | 'reviewStatus'
  | 'storyPointFields'
  | 'storyPointWriteField'
  | 'storyPointWriteFieldSubtask';

export interface TrackingRuleError {
  error: string;
  message?: string;
  fields?: Partial<Record<TrackingRuleField, string>>;
}

/** Same rule as the server (AC3.8). */
export const PROJECT_KEY_PATTERN = /^[A-Z][A-Z0-9_]{1,19}$/;

export function normalizeProjectKey(raw: string): string {
  return (raw ?? '').trim().toUpperCase();
}

@Injectable({ providedIn: 'root' })
export class TrackingRuleService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/admin/tracking-rules`;

  list(): Observable<{ rules: TrackingRule[] }> {
    return this.http.get<{ rules: TrackingRule[] }>(this.base);
  }

  lookup(projectKey: string): Observable<JiraProjectLookup> {
    return this.http.get<JiraProjectLookup>(`${this.base}/jira/${encodeURIComponent(projectKey)}`);
  }

  create(input: TrackingRuleInput): Observable<TrackingRule> {
    return this.http.post<TrackingRule>(this.base, input);
  }

  update(projectKey: string, input: TrackingRuleInput): Observable<TrackingRule> {
    return this.http.put<TrackingRule>(`${this.base}/${encodeURIComponent(projectKey)}`, input);
  }

  setActive(projectKey: string, active: boolean): Observable<TrackingRule> {
    return this.http.put<TrackingRule>(`${this.base}/${encodeURIComponent(projectKey)}/active`, { active });
  }

  /** Starts the "every user" recalculation; 409 while one is running. */
  recalculate(): Observable<{ started: boolean }> {
    return this.http.post<{ started: boolean }>(`${this.base}/recalculate`, {});
  }

  /** The board project's Jira workflow statuses (Pengaturan, AC5.5). */
  boardStatuses(boardId: number): Observable<{ statuses: string[] }> {
    return this.http.get<{ statuses: string[] }>(`${environment.apiBasePath}/api/dashboard/boards/${boardId}/statuses`);
  }
}
```

`frontend/src/app/features/admin/tracking-rules/recalculate-action.ts`:
```ts
import { HttpErrorResponse } from '@angular/common/http';
import { TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ToastAction, ToastService } from '../../../core/ui/toast.service';

export const RECALCULATE_LABEL = 'Hitung ulang semua user';

/** Toast action after a rule change (AC3.4): rebuilds every user's active-sprint time from Jira. */
export function recalculateAction(api: TrackingRuleService, toast: ToastService): ToastAction {
  return {
    label: RECALCULATE_LABEL,
    run: () => {
      api.recalculate().subscribe({
        next: () => toast.info('Hitung ulang berjalan', 'Waktu sprint aktif semua user dihitung ulang dari changelog Jira di background.'),
        error: (err: HttpErrorResponse) =>
          err?.status === 409
            ? toast.info('Hitung ulang masih berjalan', 'Tunggu sampai selesai, lalu coba lagi.')
            : toast.error('Gagal memulai hitung ulang', 'Coba lagi sebentar.'),
      });
    },
  };
}
```

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include='**/tracking-rule.service.spec.ts'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/core/services/tracking-rule.service.ts frontend/src/app/core/services/tracking-rule.service.spec.ts frontend/src/app/features/admin/tracking-rules/recalculate-action.ts
git commit -m "feat(tracking-rules): add tracking rule http client" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Frontend — halaman daftar Aturan Board + route + menu Admin

**Files:**
- Create: `frontend/src/app/features/admin/tracking-rules/tracking-rules.component.{ts,html,scss,spec.ts}`
- Modify: `frontend/src/app/app.routes.ts` (children `admin`, setelah `chat-logs`)
- Modify: `frontend/src/app/layouts/admin-shell/admin-shell.component.html` (nav item setelah "Pesan"), `admin-shell.component.spec.ts:24-29`

**Interfaces:**
- Consumes: `TrackingRuleService.list/setActive/recalculate`, `recalculateAction`, `RECALCULATE_LABEL` (Task 10); `ConfirmService.ask`, `ToastService.show/error`.
- Produces: `TrackingRulesComponent` (selector `app-tracking-rules`) di route `/admin/tracking-rules`; route `tracking-rules/new` dan `tracking-rules/:key` memuat `TrackingRuleFormComponent` (dibuat di Task 12 — Step 3 task ini menambah route untuk daftar saja; route form ditambah di Task 12).

- [ ] **Step 1: Write the failing test** (`tracking-rules.component.spec.ts`)

```ts
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { TrackingRulesComponent } from './tracking-rules.component';
import { TrackingRule, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const CORE: TrackingRule = {
  projectKey: 'CORE', projectName: 'ESB Core', trackedStatuses: ['Code Process', 'CODE REVIEW'], doneStatus: 'Code Completed', reviewStatus: 'CODE REVIEW',
  storyPointFields: [{ id: 'customfield_10125', name: 'Story Point Estimate' }, { id: 'customfield_11053', name: 'Story Points' }],
  storyPointWriteField: 'customfield_10125', storyPointWriteFieldSubtask: 'customfield_11053', isActive: true, editedBy: 'seed', editedDate: '2026-11-16T09:00:00+07:00',
};
const OMS: TrackingRule = {
  ...CORE, projectKey: 'OMS', projectName: 'ESB OMS', trackedStatuses: ['Code Process'], doneStatus: 'Code Review', reviewStatus: null,
  storyPointFields: [{ id: 'customfield_10016', name: 'Story point estimate' }], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
};
const POS: TrackingRule = { ...OMS, projectKey: 'POS', projectName: 'ESB POS', isActive: false };

describe('TrackingRulesComponent', () => {
  let fixture: ComponentFixture<TrackingRulesComponent>;
  let api: jasmine.SpyObj<TrackingRuleService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  let toast: jasmine.SpyObj<ToastService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const component = () => fixture.componentInstance as any;

  beforeEach(async () => {
    api = jasmine.createSpyObj('TrackingRuleService', ['list', 'setActive', 'recalculate']);
    api.list.and.returnValue(of({ rules: [CORE, OMS, POS] }));
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [TrackingRulesComponent],
      providers: [
        provideRouter([]),
        { provide: TrackingRuleService, useValue: api },
        { provide: ConfirmService, useValue: confirm },
        { provide: ToastService, useValue: toast },
      ],
    }).compileComponents();
    fixture = TestBed.createComponent(TrackingRulesComponent);
    fixture.detectChanges();
  });

  it('lists every rule with its statuses and Story Point fields in priority order (AC2.2)', () => {
    const rows = el().querySelectorAll('tr[data-rule]');
    expect(rows.length).toBe(3);
    const core = el().querySelector('tr[data-rule="CORE"]')!;
    for (const text of ['ESB Core', 'Code Process', 'CODE REVIEW', 'Code Completed', 'Story Point Estimate', 'customfield_10125', 'seed']) {
      expect(core.textContent).toContain(text);
    }
    const order = Array.from(core.querySelectorAll('[data-sp]')).map((li) => li.getAttribute('data-sp'));
    expect(order).toEqual(['customfield_10125', 'customfield_11053']);
    expect(el().querySelector('[data-add-rule]')?.getAttribute('href')).toBe('/admin/tracking-rules/new');
  });

  it('dims an inactive rule and offers Aktifkan (AC2.4)', () => {
    const pos = el().querySelector('tr[data-rule="POS"]')!;
    expect(pos.classList).toContain('inactive');
    expect(pos.querySelector('[data-activate="POS"]')).not.toBeNull();
    expect(pos.textContent).toContain('Nonaktif');
  });

  it('asks before deactivating and toasts with the recalculation action (AC4.2, AC3.4)', async () => {
    confirm.ask.and.resolveTo(true);
    api.setActive.and.returnValue(of({ ...CORE, isActive: false }));
    api.recalculate.and.returnValue(of({ started: true }));

    await component().deactivate(CORE);
    fixture.detectChanges();

    expect(confirm.ask.calls.mostRecent().args[0].message).toBe('Tiket CORE berhenti dipantau. Riwayat tetap tersimpan.');
    expect(confirm.ask.calls.mostRecent().args[0].tone).toBe('danger');
    expect(api.setActive).toHaveBeenCalledWith('CORE', false);
    expect(el().querySelector('tr[data-rule="CORE"]')!.classList).toContain('inactive');
    const action = toast.show.calls.mostRecent().args[4]!;
    expect(action.label).toBe('Hitung ulang semua user');
    action.run();
    expect(api.recalculate).toHaveBeenCalled();
  });

  it('does nothing when the confirmation is cancelled', async () => {
    confirm.ask.and.resolveTo(false);
    await component().deactivate(CORE);
    expect(api.setActive).not.toHaveBeenCalled();
  });

  it('explains the last-active-rule refusal (AC4.3)', async () => {
    confirm.ask.and.resolveTo(true);
    api.setActive.and.returnValue(throwError(() => ({ status: 422, error: { error: 'last_active_rule' } })));
    await component().deactivate(CORE);
    expect(toast.error).toHaveBeenCalledWith('Minimal satu board harus aktif', jasmine.any(String));
  });
});
```

`admin-shell.component.spec.ts` — ganti baris 26 dan 28:
```ts
    expect(links).toEqual(['Dashboard', 'User', 'Broadcast Message', 'Pesan', 'Aturan Board']);
```
```ts
    expect(hrefs).toEqual(['/admin', '/admin/users', '/admin/broadcast', '/admin/chat-logs', '/admin/tracking-rules']);
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include='**/tracking-rules.component.spec.ts' --include='**/admin-shell.component.spec.ts'`
Expected: FAIL — komponen belum ada; menu belum punya "Aturan Board".

- [ ] **Step 3: Implement**

`tracking-rules.component.ts`:
```ts
import { Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { TrackingRule, TrackingRuleError, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { recalculateAction } from './recalculate-action';

/**
 * Admin › Aturan Board (Tech Spec "Aturan Board (Admin CRUD)"): every Jira
 * project Nudge tracks — which statuses count, when a ticket is finished and
 * where its Story Points live. "Hapus" = nonaktifkan.
 */
@Component({
  selector: 'app-tracking-rules',
  standalone: true,
  imports: [RouterLink, IconComponent],
  templateUrl: './tracking-rules.component.html',
  styleUrl: './tracking-rules.component.scss',
})
export class TrackingRulesComponent {
  private api = inject(TrackingRuleService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  protected rules = signal<TrackingRule[]>([]);
  protected loading = signal(true);
  protected error = signal<string | null>(null);
  protected busyKey = signal<string | null>(null);
  protected activeCount = computed(() => this.rules().filter((r) => r.isActive).length);

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.api.list().subscribe({
      next: (res) => {
        this.rules.set(res.rules ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Gagal memuat aturan board, coba lagi');
        this.loading.set(false);
      },
    });
  }

  protected formatEdited(iso: string): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleString('id-ID', {
      timeZone: 'Asia/Jakarta', day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }

  protected async deactivate(rule: TrackingRule): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: `Nonaktifkan ${rule.projectKey}?`,
      message: `Tiket ${rule.projectKey} berhenti dipantau. Riwayat tetap tersimpan.`,
      confirmText: 'Nonaktifkan',
      tone: 'danger',
      icon: 'power',
    });
    if (confirmed) this.setActive(rule, false);
  }

  protected activate(rule: TrackingRule): void {
    this.setActive(rule, true);
  }

  private setActive(rule: TrackingRule, active: boolean): void {
    this.busyKey.set(rule.projectKey);
    this.api.setActive(rule.projectKey, active).subscribe({
      next: (saved) => {
        this.busyKey.set(null);
        this.rules.update((list) => list.map((r) => (r.projectKey === saved.projectKey ? saved : r)));
        this.toast.show(
          'success',
          active ? `${saved.projectKey} dipantau lagi` : `${saved.projectKey} dinonaktifkan`,
          active
            ? 'Tiket yang masih di status dihitung dibuka lagi pada cycle poller berikutnya.'
            : 'Sesi yang sedang berjalan ditutup tanpa menambah waktu.',
          10000,
          recalculateAction(this.api, this.toast),
        );
      },
      error: (err: HttpErrorResponse) => {
        this.busyKey.set(null);
        if (err?.status === 422) {
          this.toast.error('Minimal satu board harus aktif', 'Aktifkan board lain dulu sebelum menonaktifkan yang ini.');
          return;
        }
        this.toast.error('Gagal menyimpan', (err?.error as TrackingRuleError | undefined)?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
```

`tracking-rules.component.html`:
```html
<div class="page-stack">
  <section class="card">
    <header class="card-header with-actions">
      <span class="card-icon"><app-icon name="layers" [size]="20" /></span>
      <div>
        <h2>Aturan Board</h2>
        <p>Status yang dihitung budget, status selesai, dan field Story Point tiap project Jira. Berlaku untuk semua user.</p>
      </div>
      <a class="btn btn-primary btn-sm tr-add" routerLink="/admin/tracking-rules/new" data-add-rule>
        <app-icon name="plus" [size]="14" /> Tambah aturan
      </a>
    </header>

    @if (loading()) {
      <div class="table-skeleton" aria-busy="true">
        @for (i of [1, 2, 3]; track i) {
          <div class="skeleton skeleton-row"></div>
        }
      </div>
    } @else if (error()) {
      <div class="empty-state">
        <div class="empty-icon danger"><app-icon name="alert-circle" [size]="24" /></div>
        <p class="msg error">{{ error() }}</p>
        <button type="button" class="btn btn-ghost btn-sm" (click)="load()">Coba lagi</button>
      </div>
    } @else if (rules().length === 0) {
      <div class="empty-state"><p class="msg">Belum ada aturan board.</p></div>
    } @else {
      <div class="table-wrap">
        <table class="admin-users tr-table">
          <thead>
            <tr>
              <th>Board</th><th>Status dihitung</th><th>Selesai</th><th>Review</th><th>Field SP (urut prioritas)</th>
              <th>Status</th><th>Diubah</th><th class="th-actions">Aksi</th>
            </tr>
          </thead>
          <tbody>
            @for (rule of rules(); track rule.projectKey; let i = $index) {
              <tr
                [attr.data-rule]="rule.projectKey"
                [class.inactive]="!rule.isActive"
                [class.busy]="busyKey() === rule.projectKey"
                [style.animation-delay.ms]="i * 40"
              >
                <td>
                  <div class="tr-board">
                    <span class="tr-key">{{ rule.projectKey }}</span>
                    <span class="tr-name">{{ rule.projectName || '—' }}</span>
                  </div>
                </td>
                <td>
                  <div class="tr-chips">
                    @for (s of rule.trackedStatuses; track s) {
                      <span class="tr-chip">{{ s }}</span>
                    }
                  </div>
                </td>
                <td><span class="tr-chip done">{{ rule.doneStatus }}</span></td>
                <td>
                  @if (rule.reviewStatus) {
                    <span class="tr-chip review">{{ rule.reviewStatus }}</span>
                  } @else {
                    <span class="muted">—</span>
                  }
                </td>
                <td>
                  <ol class="tr-fields">
                    @for (f of rule.storyPointFields; track f.id) {
                      <li [attr.data-sp]="f.id">
                        <strong>{{ f.name }}</strong><small>{{ f.id }}</small>
                        @if (f.id === rule.storyPointWriteField) {
                          <span class="tr-tag">tulis card</span>
                        }
                        @if (f.id === rule.storyPointWriteFieldSubtask) {
                          <span class="tr-tag">tulis subtask</span>
                        }
                      </li>
                    }
                  </ol>
                </td>
                <td><span class="status-badge" [class.active]="rule.isActive">{{ rule.isActive ? 'Aktif' : 'Nonaktif' }}</span></td>
                <td>
                  <div class="tr-edited">
                    <span>{{ rule.editedBy }}</span>
                    <small>{{ formatEdited(rule.editedDate) }}</small>
                  </div>
                </td>
                <td class="actions">
                  <div class="tr-actions">
                    <a class="btn btn-ghost btn-sm" [routerLink]="['/admin/tracking-rules', rule.projectKey]" [attr.data-edit]="rule.projectKey">Edit</a>
                    @if (rule.isActive) {
                      <button type="button" class="btn btn-danger-ghost btn-sm" [attr.data-deactivate]="rule.projectKey" [disabled]="busyKey() !== null" (click)="deactivate(rule)">
                        Nonaktifkan
                      </button>
                    } @else {
                      <button type="button" class="btn btn-primary btn-sm" [attr.data-activate]="rule.projectKey" [disabled]="busyKey() !== null" (click)="activate(rule)">
                        Aktifkan
                      </button>
                    }
                  </div>
                </td>
              </tr>
            }
          </tbody>
        </table>
      </div>
    }
  </section>
</div>
```

`tracking-rules.component.scss`:
```scss
// Keyframes fade-up / pop-in are global (styles.scss).
:host { display: block; animation: fade-up 260ms var(--ease) backwards; }
.tr-add { margin-left: auto; transition: transform var(--fast) var(--ease), box-shadow var(--fast) var(--ease);
  &:hover { transform: translateY(-1px); box-shadow: var(--shadow-primary); }
  &:active { transform: scale(.97); } }
.tr-table tbody tr { animation: fade-up 260ms var(--ease) backwards; transition: opacity var(--normal) var(--ease), background-color var(--fast) var(--ease); }
.tr-table tbody tr.inactive { opacity: .55; &:hover { opacity: .85; } }
.tr-board { display: flex; flex-direction: column; gap: 2px; }
.tr-key { font-weight: 800; color: var(--ink); letter-spacing: .02em; }
.tr-name { font-size: .78rem; color: var(--text-faint); }
.tr-chips { display: flex; flex-wrap: wrap; gap: 4px; }
.tr-chip { display: inline-flex; align-items: center; padding: 3px 10px; border-radius: 999px; background: var(--primary-soft); color: var(--primary-strong);
  font-size: .76rem; font-weight: 600; white-space: nowrap; animation: pop-in 220ms var(--ease) backwards;
  &.done { background: var(--success-soft); color: var(--success); }
  &.review { background: var(--accent-soft); color: #b4630a; } }
.tr-fields { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 4px; counter-reset: sp;
  li { display: flex; align-items: baseline; flex-wrap: wrap; gap: 6px; counter-increment: sp; font-size: .8rem;
    &::before { content: counter(sp); display: inline-grid; place-items: center; width: 18px; height: 18px; border-radius: 50%;
      background: var(--surface-2); border: 1px solid var(--border); font-size: .68rem; color: var(--text-muted); }
    small { color: var(--text-faint); font-size: .72rem; } } }
.tr-tag { padding: 1px 6px; border-radius: 6px; background: var(--surface-2); border: 1px dashed var(--border-strong); font-size: .66rem; color: var(--text-muted); }
.tr-edited { display: flex; flex-direction: column; font-size: .78rem; small { color: var(--text-faint); } }
.tr-actions { display: inline-flex; gap: 6px; justify-content: flex-end;
  .btn { transition: transform var(--fast) var(--ease), background-color var(--fast) var(--ease), color var(--fast) var(--ease);
    &:active:not(:disabled) { transform: scale(.96); } } }

@media (prefers-reduced-motion: reduce) {
  :host, .tr-table tbody tr, .tr-chip { animation: none; }
  .tr-table tbody tr, .tr-add, .tr-actions .btn { transition: none; }
}
```

`app.routes.ts` — tambahkan setelah child `chat-logs`:
```ts
      // Tech Spec "Aturan Board (Admin CRUD)".
      {
        path: 'tracking-rules',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rules.component').then((m) => m.TrackingRulesComponent),
        title: 'Aturan Board · Admin · Nudge',
        data: { heading: 'Aturan Board', subheading: 'Status & field Story Point yang dipantau per project Jira', crumbs: ['Admin', 'Aturan Board'] },
      },
```

`admin-shell.component.html` — setelah link "Pesan":
```html
      <a class="nav-item" routerLink="/admin/tracking-rules" routerLinkActive="active" (click)="closeSidebar()">
        <app-icon name="layers" />
        <span>Aturan Board</span>
      </a>
```
(perbarui juga komentar di `admin-shell.component.ts` menjadi "(Dashboard / User / Broadcast Message / Pesan / Aturan Board)").

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: PASS; build sukses.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/features/admin/tracking-rules frontend/src/app/app.routes.ts frontend/src/app/layouts/admin-shell
git commit -m "feat(tracking-rules): add admin board rule list page" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: Frontend — form tambah/edit (lookup Jira, chip status, field SP berurutan, preview)

**Files:**
- Create: `frontend/src/app/features/admin/tracking-rules/tracking-rule-form.component.{ts,html,scss,spec.ts}`
- Modify: `frontend/src/app/app.routes.ts` (2 route setelah `tracking-rules`)

**Interfaces:**
- Consumes: `TrackingRuleService.list/lookup/create/update`, `PROJECT_KEY_PATTERN`, `normalizeProjectKey`, tipe Task 10; `recalculateAction`; `SelectComponent`/`SelectOption`; route param `key`.
- Produces: `TrackingRuleFormComponent` (selector `app-tracking-rule-form`) di `/admin/tracking-rules/new` dan `/admin/tracking-rules/:key`.

- [ ] **Step 1: Write the failing test** (`tracking-rule-form.component.spec.ts`)

```ts
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { TrackingRuleFormComponent } from './tracking-rule-form.component';
import { JiraProjectLookup, TrackingRule, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ToastService } from '../../../core/ui/toast.service';

const POS: JiraProjectLookup = {
  projectKey: 'POS', projectName: 'ESB POS', avatarUrl: '',
  statuses: ['To Do', 'In Progress', 'Code Review', 'Done'],
  numberFields: [
    { id: 'customfield_10016', name: 'Story point estimate', storyPointLike: true },
    { id: 'customfield_200', name: 'Effort', storyPointLike: false },
  ],
};

const CORE_RULE: TrackingRule = {
  projectKey: 'CORE', projectName: 'ESB Core', trackedStatuses: ['Code Process', 'CODE REVIEW'], doneStatus: 'Code Completed', reviewStatus: 'CODE REVIEW',
  storyPointFields: [{ id: 'customfield_10125', name: 'Story Point Estimate' }, { id: 'customfield_11053', name: 'Story Points' }],
  storyPointWriteField: 'customfield_10125', storyPointWriteFieldSubtask: 'customfield_11053', isActive: true, editedBy: 'seed', editedDate: '2026-11-16T09:00:00+07:00',
};

describe('TrackingRuleFormComponent', () => {
  let api: jasmine.SpyObj<TrackingRuleService>;
  let toast: jasmine.SpyObj<ToastService>;

  function setup(key: string | null): void {
    api = jasmine.createSpyObj('TrackingRuleService', ['list', 'lookup', 'create', 'update', 'setActive', 'recalculate']);
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    TestBed.configureTestingModule({
      imports: [TrackingRuleFormComponent],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { snapshot: { paramMap: convertToParamMap(key ? { key } : {}) } } },
        { provide: TrackingRuleService, useValue: api },
        { provide: ToastService, useValue: toast },
      ],
    });
  }

  function create() {
    const fixture = TestBed.createComponent(TrackingRuleFormComponent);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, c: fixture.componentInstance as any };
  }

  it('looks the project up only for a valid key and normalizes it (AC3.1, AC3.8)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    const button = () => el.querySelector('[data-lookup]') as HTMLButtonElement;
    c.onKeyInput('x');
    fixture.detectChanges();
    expect(button().disabled).toBeTrue();
    c.onKeyInput(' pos ');
    fixture.detectChanges();
    button().click();
    fixture.detectChanges();
    expect(api.lookup).toHaveBeenCalledWith('POS');
    expect(el.querySelector('[data-project]')?.textContent).toContain('ESB POS');
    expect(el.querySelectorAll('[data-status]').length).toBe(4);
  });

  it('shows the not-found message from Jira (AC3.5)', () => {
    setup(null);
    api.lookup.and.returnValue(throwError(() => ({ status: 404 })));
    const { fixture, el, c } = create();
    c.onKeyInput('NOPE');
    c.runLookup();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-error]')?.textContent).toContain('Project NOPE tidak ditemukan di Jira');
  });

  it('keeps done out of the tracked statuses and review inside them (AC3.2)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.toggleTracked('Code Review');
    c.setDone('Done');
    c.setReview('Code Review');
    expect(c.doneOptions().map((o: { value: string }) => o.value)).toEqual(['', 'To Do', 'Done']);
    c.toggleTracked('Code Review');
    expect(c.review()).toBe('');
    c.toggleTracked('Done');
    expect(c.done()).toBe('');
  });

  it('orders Story Point fields and resets a write field whose field is removed', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.addField('customfield_200');
    c.addField('customfield_10016');
    expect(c.writeField()).toBe('customfield_200'); // default = the first field
    c.moveField(1, -1);
    expect(c.spFields()).toEqual(['customfield_10016', 'customfield_200']);
    c.removeField('customfield_200');
    expect(c.writeField()).toBe('');
    expect(c.writeFieldSubtask()).toBe('');
  });

  it('previews the rule as a sentence and saves it with the recalculation toast (AC3.3, AC3.4)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    api.create.and.returnValue(of({ ...CORE_RULE, projectKey: 'POS' }));
    const { fixture, el, c } = create();
    const router = TestBed.inject(Router);
    spyOn(router, 'navigate').and.resolveTo(true);
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');
    fixture.detectChanges();

    expect(el.querySelector('[data-preview]')?.textContent?.trim()).toBe(
      'Budget berjalan selama In Progress; selesai saat Done. Review tidak dihitung. ' +
        'SP dibaca dari Story point estimate (customfield_10016); ' +
        'ditulis ke Story point estimate (customfield_10016) (card biasa) / Story point estimate (customfield_10016) (subtask).',
    );
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.create).toHaveBeenCalledWith({
      projectKey: 'POS', trackedStatuses: ['In Progress'], doneStatus: 'Done', reviewStatus: null,
      storyPointFields: ['customfield_10016'], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
    });
    expect(toast.show.calls.mostRecent().args[4]!.label).toBe('Hitung ulang semua user');
    expect(router.navigate).toHaveBeenCalledWith(['/admin/tracking-rules']);
  });

  it('links to Edit on 409 and shows server field errors (AC3.6, AC3.7)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');

    api.create.and.returnValue(throwError(() => ({ status: 409, error: { error: 'rule_exists' } })));
    c.save();
    fixture.detectChanges();
    expect(el.querySelector('[data-exists] a')?.getAttribute('href')).toBe('/admin/tracking-rules/POS');

    api.create.and.returnValue(throwError(() => ({ status: 400, error: { error: 'validation_failed', fields: { doneStatus: 'Status selesai tidak boleh termasuk status yang dihitung' } } })));
    c.save();
    fixture.detectChanges();
    expect(el.textContent).toContain('Status selesai tidak boleh termasuk status yang dihitung');
  });

  it('loads a stored rule on Edit and blocks saving while a stored status is gone from Jira (AC4.1, AC4.4)', () => {
    setup('CORE');
    api.list.and.returnValue(of({ rules: [CORE_RULE] }));
    api.lookup.and.returnValue(of({
      projectKey: 'CORE', projectName: 'ESB Core', avatarUrl: '',
      statuses: ['To Do', 'Code Process', 'CODE REVIEW', 'Done'],
      numberFields: [
        { id: 'customfield_10125', name: 'Story Point Estimate', storyPointLike: true },
        { id: 'customfield_11053', name: 'Story Points', storyPointLike: true },
      ],
    }));
    api.update.and.returnValue(of(CORE_RULE));
    const { fixture, el, c } = create();

    expect((el.querySelector('[data-key-input]') as HTMLInputElement).readOnly).toBeTrue();
    expect(api.lookup).toHaveBeenCalledWith('CORE');
    expect(el.querySelector('[data-status="Code Process"]')?.classList).toContain('on');
    expect(el.querySelector('[data-stale-statuses]')?.textContent).toContain('Code Completed');
    expect((el.querySelector('[data-save]') as HTMLButtonElement).disabled).toBeTrue();

    c.dropStaleStatus('Code Completed');
    c.setDone('Done');
    fixture.detectChanges();
    expect(el.querySelector('[data-stale-statuses]')).toBeNull();
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.update).toHaveBeenCalledWith('CORE', jasmine.objectContaining({ doneStatus: 'Done', reviewStatus: 'CODE REVIEW' }));
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include='**/tracking-rule-form.component.spec.ts'`
Expected: FAIL — komponen belum ada.

- [ ] **Step 3: Implement**

`tracking-rule-form.component.ts`:
```ts
import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import {
  JiraProjectLookup,
  PROJECT_KEY_PATTERN,
  TrackingRule,
  TrackingRuleError,
  TrackingRuleField,
  TrackingRuleInput,
  TrackingRuleService,
  normalizeProjectKey,
} from '../../../core/services/tracking-rule.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { recalculateAction } from './recalculate-action';

const MAX_FIELD_SUGGESTIONS = 12;

function hasFold(list: string[], value: string): boolean {
  return list.some((v) => v.toLowerCase() === value.toLowerCase());
}

/**
 * Tambah / edit satu aturan board (Tech Spec "Aturan Board (Admin CRUD)",
 * US-3/US-4): statuses and Story Point fields are picked from the project's
 * Jira data; stored values Jira no longer has are flagged ⚠ and must be
 * dropped before saving.
 */
@Component({
  selector: 'app-tracking-rule-form',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent, SelectComponent],
  templateUrl: './tracking-rule-form.component.html',
  styleUrl: './tracking-rule-form.component.scss',
})
export class TrackingRuleFormComponent {
  private api = inject(TrackingRuleService);
  private toast = inject(ToastService);
  private router = inject(Router);

  /** The rule being edited (route /admin/tracking-rules/:key); null = new rule. */
  protected readonly editKey: string | null = inject(ActivatedRoute).snapshot.paramMap.get('key');
  protected readonly isEdit = this.editKey !== null;

  protected keyInput = signal(this.editKey ?? '');
  protected lookup = signal<JiraProjectLookup | null>(null);
  protected lookingUp = signal(false);
  protected lookupError = signal<string | null>(null);
  protected loadError = signal<string | null>(null);
  protected avatarFailed = signal(false);

  protected tracked = signal<string[]>([]);
  protected done = signal('');
  protected review = signal('');
  protected spFields = signal<string[]>([]);
  protected writeField = signal('');
  protected writeFieldSubtask = signal('');
  protected fieldQuery = signal('');

  protected saving = signal(false);
  protected fieldErrors = signal<Partial<Record<TrackingRuleField, string>>>({});
  protected existingKey = signal<string | null>(null);
  /** Field names of the stored rule, shown until (or without) the Jira lookup. */
  private storedNames = signal(new Map<string, string>());

  protected keyValid = computed(() => PROJECT_KEY_PATTERN.test(normalizeProjectKey(this.keyInput())));
  protected statuses = computed(() => this.lookup()?.statuses ?? []);
  private fieldNames = computed(() => {
    const names = new Map(this.storedNames());
    for (const f of this.lookup()?.numberFields ?? []) names.set(f.id, f.name);
    return names;
  });

  /** Stored statuses no longer in the project's Jira workflow (AC4.4). */
  protected staleStatuses = computed(() => {
    if (!this.lookup()) return [];
    const all = this.statuses();
    const used = [...this.tracked(), this.done(), this.review()].filter((s) => s !== '');
    return [...new Set(used)].filter((s) => !hasFold(all, s));
  });
  /** Stored Story Point fields Jira no longer has as a number field. */
  protected staleFields = computed(() => {
    const l = this.lookup();
    if (!l) return [];
    return this.spFields().filter((id) => !l.numberFields.some((f) => f.id === id));
  });

  protected doneOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih status selesai…' },
    ...this.statuses()
      .filter((s) => !hasFold(this.tracked(), s))
      .map((s) => ({ value: s, label: s })),
  ]);
  protected reviewOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Tidak ada — review tidak dihitung' },
    ...this.tracked().map((s) => ({ value: s, label: s })),
  ]);
  protected writeOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih field…' },
    ...this.spFields().map((id) => ({ value: id, label: this.fieldLabel(id) })),
  ]);
  /** Number fields not picked yet, story-point-like first (server order), filtered by the search box. */
  protected suggestions = computed(() => {
    const q = this.fieldQuery().trim().toLowerCase();
    const picked = this.spFields();
    return (this.lookup()?.numberFields ?? [])
      .filter((f) => !picked.includes(f.id))
      .filter((f) => !q || f.name.toLowerCase().includes(q) || f.id.includes(q))
      .slice(0, MAX_FIELD_SUGGESTIONS);
  });

  /** AC3.3 — live sentence of what the rule does. */
  protected preview = computed(() => {
    const label = (id: string) => (id ? this.fieldLabel(id) : '…');
    const tracked = this.tracked();
    const fields = this.spFields();
    return (
      `Budget berjalan selama ${tracked.length ? tracked.join(' atau ') : '…'}; selesai saat ${this.done() || '…'}. ` +
      `Review ${this.review() ? 'ikut' : 'tidak'} dihitung. ` +
      `SP dibaca dari ${fields.length ? fields.map(label).join(' → ') : '…'}; ` +
      `ditulis ke ${label(this.writeField())} (card biasa) / ${label(this.writeFieldSubtask())} (subtask).`
    );
  });

  protected canSave = computed(
    () =>
      !!this.lookup() &&
      !this.saving() &&
      this.tracked().length > 0 &&
      this.done() !== '' &&
      this.spFields().length > 0 &&
      this.writeField() !== '' &&
      this.writeFieldSubtask() !== '' &&
      this.staleStatuses().length === 0 &&
      this.staleFields().length === 0,
  );

  constructor() {
    if (this.editKey) this.loadRule(this.editKey);
  }

  protected fieldLabel(id: string): string {
    const name = this.fieldNames().get(id);
    return name ? `${name} (${id})` : id;
  }

  private loadRule(key: string): void {
    this.api.list().subscribe({
      next: (res) => {
        const rule = res.rules.find((r) => r.projectKey === key);
        if (!rule) {
          this.loadError.set(`Aturan untuk ${key} tidak ditemukan.`);
          return;
        }
        this.apply(rule);
        // AC4.1: reload the Jira data so the choices are current.
        this.runLookup();
      },
      error: () => this.loadError.set('Gagal memuat aturan, coba lagi.'),
    });
  }

  private apply(rule: TrackingRule): void {
    this.storedNames.set(new Map(rule.storyPointFields.map((f) => [f.id, f.name] as [string, string])));
    this.tracked.set([...rule.trackedStatuses]);
    this.done.set(rule.doneStatus);
    this.review.set(rule.reviewStatus ?? '');
    this.spFields.set(rule.storyPointFields.map((f) => f.id));
    this.writeField.set(rule.storyPointWriteField);
    this.writeFieldSubtask.set(rule.storyPointWriteFieldSubtask);
  }

  protected onKeyInput(value: string): void {
    this.keyInput.set(value ?? '');
    const current = this.lookup();
    // Another key: what was picked for the old project no longer applies.
    if (current && normalizeProjectKey(value ?? '') !== current.projectKey) {
      this.lookup.set(null);
      this.tracked.set([]);
      this.done.set('');
      this.review.set('');
      this.spFields.set([]);
      this.writeField.set('');
      this.writeFieldSubtask.set('');
    }
  }

  protected runLookup(): void {
    const key = normalizeProjectKey(this.keyInput());
    if (!PROJECT_KEY_PATTERN.test(key) || this.lookingUp()) return;
    this.keyInput.set(key);
    this.lookingUp.set(true);
    this.lookupError.set(null);
    this.existingKey.set(null);
    this.avatarFailed.set(false);
    this.api.lookup(key).subscribe({
      next: (l) => {
        this.lookingUp.set(false);
        this.lookup.set(l);
      },
      error: (err: HttpErrorResponse) => {
        this.lookingUp.set(false);
        this.lookup.set(null);
        this.lookupError.set(
          err?.status === 404
            ? `Project ${key} tidak ditemukan di Jira`
            : err?.status === 400
              ? 'Format project key tidak valid'
              : 'Jira tidak merespons, coba lagi',
        );
      },
    });
  }

  protected isTracked(status: string): boolean {
    return hasFold(this.tracked(), status);
  }

  protected toggleTracked(status: string): void {
    const next = this.isTracked(status)
      ? this.tracked().filter((s) => s.toLowerCase() !== status.toLowerCase())
      : [...this.tracked(), status];
    this.tracked.set(next);
    if (this.done() && hasFold(next, this.done())) this.done.set('');
    if (this.review() && !hasFold(next, this.review())) this.review.set('');
    this.clearErrors('trackedStatuses', 'doneStatus', 'reviewStatus');
  }

  protected setDone(value: string): void {
    this.done.set(value ?? '');
    this.clearErrors('doneStatus');
  }

  protected setReview(value: string): void {
    this.review.set(value ?? '');
    this.clearErrors('reviewStatus');
  }

  protected setWrite(which: 'card' | 'subtask', value: string): void {
    if (which === 'card') {
      this.writeField.set(value ?? '');
      this.clearErrors('storyPointWriteField');
    } else {
      this.writeFieldSubtask.set(value ?? '');
      this.clearErrors('storyPointWriteFieldSubtask');
    }
  }

  protected addField(id: string): void {
    if (this.spFields().includes(id)) return;
    const next = [...this.spFields(), id];
    this.spFields.set(next);
    // Default write field = the first read field.
    if (!this.writeField()) this.writeField.set(next[0]);
    if (!this.writeFieldSubtask()) this.writeFieldSubtask.set(next[0]);
    this.fieldQuery.set('');
    this.clearErrors('storyPointFields');
  }

  protected moveField(index: number, step: -1 | 1): void {
    const list = [...this.spFields()];
    const target = index + step;
    if (target < 0 || target >= list.length) return;
    [list[index], list[target]] = [list[target], list[index]];
    this.spFields.set(list);
  }

  protected removeField(id: string): void {
    this.spFields.set(this.spFields().filter((f) => f !== id));
    // A write dropdown pointing at the removed field is reset.
    if (this.writeField() === id) this.writeField.set('');
    if (this.writeFieldSubtask() === id) this.writeFieldSubtask.set('');
  }

  protected dropStaleStatus(status: string): void {
    this.tracked.set(this.tracked().filter((s) => s !== status));
    if (this.done() === status) this.done.set('');
    if (this.review() === status || (this.review() && !hasFold(this.tracked(), this.review()))) this.review.set('');
  }

  private clearErrors(...fields: TrackingRuleField[]): void {
    const errors = { ...this.fieldErrors() };
    for (const f of fields) delete errors[f];
    this.fieldErrors.set(errors);
  }

  protected save(): void {
    const l = this.lookup();
    if (!l || !this.canSave()) return;
    const key = l.projectKey;
    const input: TrackingRuleInput = {
      trackedStatuses: this.tracked(),
      doneStatus: this.done(),
      reviewStatus: this.review() || null,
      storyPointFields: this.spFields(),
      storyPointWriteField: this.writeField(),
      storyPointWriteFieldSubtask: this.writeFieldSubtask(),
    };
    this.saving.set(true);
    this.existingKey.set(null);
    const request = this.isEdit ? this.api.update(key, input) : this.api.create({ ...input, projectKey: key });
    request.subscribe({
      next: () => {
        this.saving.set(false);
        this.toast.show(
          'success',
          'Aturan board tersimpan',
          `${key} berlaku mulai cycle poller berikutnya. Waktu yang sudah tercatat bisa dihitung ulang dari Jira.`,
          10000,
          recalculateAction(this.api, this.toast),
        );
        this.router.navigate(['/admin/tracking-rules']);
      },
      error: (err: HttpErrorResponse) => {
        this.saving.set(false);
        const body = err?.error as TrackingRuleError | undefined;
        if (err?.status === 400 && body?.fields) {
          this.fieldErrors.set(body.fields);
          return;
        }
        if (err?.status === 409) {
          this.existingKey.set(key);
          return;
        }
        this.toast.error('Gagal menyimpan aturan', body?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
```

`tracking-rule-form.component.html`:
```html
<div class="page-stack trf">
  @if (loadError()) {
    <section class="card">
      <div class="empty-state">
        <div class="empty-icon danger"><app-icon name="alert-circle" [size]="24" /></div>
        <p class="msg error">{{ loadError() }}</p>
        <a class="btn btn-ghost btn-sm" routerLink="/admin/tracking-rules">Kembali ke daftar</a>
      </div>
    </section>
  } @else {
    <section class="card trf-card" data-section="project">
      <header class="card-header">
        <span class="card-icon"><app-icon name="layers" [size]="20" /></span>
        <div>
          <h2>Project Jira</h2>
          <p>
            {{ isEdit ? 'Project key tidak bisa diubah. Data Jira dimuat ulang supaya pilihan selalu terbaru.' : 'Ketik project key lalu cek di Jira untuk memuat status & field angkanya.' }}
          </p>
        </div>
      </header>
      <div class="trf-key">
        <div class="trf-field">
          <span>Project key</span>
          <input
            type="text"
            data-key-input
            [ngModel]="keyInput()"
            (ngModelChange)="onKeyInput($event)"
            (keydown.enter)="runLookup()"
            [readonly]="isEdit"
            placeholder="mis. POS"
            maxlength="20"
            autocomplete="off"
            aria-label="Project key"
          />
        </div>
        <button type="button" class="btn btn-primary" data-lookup [class.loading]="lookingUp()" [disabled]="!keyValid() || lookingUp()" (click)="runLookup()">
          <span class="spinner" aria-hidden="true"></span>
          <span class="btn-label"><app-icon name="search" [size]="16" /> {{ isEdit ? 'Muat ulang dari Jira' : 'Cek di Jira' }}</span>
        </button>
      </div>
      @if (fieldErrors().projectKey) {
        <p class="trf-err">{{ fieldErrors().projectKey }}</p>
      }
      @if (lookupError()) {
        <p class="msg error trf-pop" data-lookup-error><app-icon name="alert-circle" [size]="16" /> {{ lookupError() }}</p>
      }
      @if (existingKey()) {
        <p class="msg error trf-pop" data-exists>
          <app-icon name="info" [size]="16" /> Project {{ existingKey() }} sudah punya aturan.
          <a [routerLink]="['/admin/tracking-rules', existingKey()]">Ubah lewat Edit</a>
        </p>
      }
      @if (lookup(); as l) {
        <div class="trf-project trf-pop" data-project>
          @if (l.avatarUrl && !avatarFailed()) {
            <img [src]="l.avatarUrl" alt="" width="40" height="40" (error)="avatarFailed.set(true)" />
          } @else {
            <span class="trf-avatar">{{ l.projectKey.slice(0, 2) }}</span>
          }
          <div>
            <strong>{{ l.projectName }}</strong>
            <small>{{ l.projectKey }} · {{ l.statuses.length }} status · {{ l.numberFields.length }} field angka</small>
          </div>
        </div>
      }
    </section>

    @if (lookup(); as l) {
      <section class="card trf-card" data-section="statuses">
        <header class="card-header">
          <span class="card-icon"><app-icon name="clock" [size]="20" /></span>
          <div>
            <h2>Status</h2>
            <p>Budget berjalan selama tiket di status yang dihitung; pindah ke status selesai = selesai lebih cepat.</p>
          </div>
        </header>
        <span class="trf-label">Status dihitung <em>klik untuk memilih, bisa lebih dari satu</em></span>
        <div class="trf-chips" role="group" aria-label="Status dihitung">
          @for (s of statuses(); track s; let i = $index) {
            <button
              type="button"
              class="trf-chip"
              [class.on]="isTracked(s)"
              [attr.aria-pressed]="isTracked(s)"
              [attr.data-status]="s"
              [style.animation-delay.ms]="i * 25"
              (click)="toggleTracked(s)"
            >
              @if (isTracked(s)) {
                <app-icon name="check" [size]="12" />
              }
              {{ s }}
            </button>
          }
        </div>
        @if (fieldErrors().trackedStatuses) {
          <p class="trf-err">{{ fieldErrors().trackedStatuses }}</p>
        }
        @if (staleStatuses().length) {
          <div class="trf-stale trf-pop" data-stale-statuses>
            @for (s of staleStatuses(); track s) {
              <span class="trf-chip stale">
                ⚠ {{ s }} — tidak ada lagi di Jira
                <button type="button" class="trf-x" [attr.aria-label]="'Buang ' + s" (click)="dropStaleStatus(s)"><app-icon name="x" [size]="12" /></button>
              </span>
            }
          </div>
        }
        <div class="trf-row">
          <div class="trf-field">
            <span>Status selesai</span>
            <app-select data-done [options]="doneOptions()" [ngModel]="done()" (ngModelChange)="setDone($event)" ariaLabel="Status selesai" />
            @if (fieldErrors().doneStatus) {
              <p class="trf-err">{{ fieldErrors().doneStatus }}</p>
            }
          </div>
          <div class="trf-field">
            <span>Status review (opsional)</span>
            <app-select data-review [options]="reviewOptions()" [ngModel]="review()" (ngModelChange)="setReview($event)" ariaLabel="Status review" />
            @if (fieldErrors().reviewStatus) {
              <p class="trf-err">{{ fieldErrors().reviewStatus }}</p>
            }
          </div>
        </div>
      </section>

      <section class="card trf-card" data-section="fields">
        <header class="card-header">
          <span class="card-icon"><app-icon name="target" [size]="20" /></span>
          <div>
            <h2>Field Story Point</h2>
            <p>Dibaca berurutan: field pertama yang terisi di tiket yang dipakai sebagai budget.</p>
          </div>
        </header>
        <ol class="trf-fields">
          @for (id of spFields(); track id; let i = $index; let first = $first; let last = $last) {
            <li class="trf-field-row" [class.stale]="staleFields().includes(id)" [attr.data-sp-field]="id">
              <span class="trf-rank">{{ i + 1 }}</span>
              <span class="trf-field-name">
                {{ fieldLabel(id) }}
                @if (staleFields().includes(id)) {
                  <em>⚠ tidak ada lagi di Jira</em>
                }
              </span>
              <button type="button" class="icon-btn" aria-label="Naikkan prioritas" [disabled]="first" (click)="moveField(i, -1)">
                <app-icon class="trf-flip" name="chevron-down" [size]="14" />
              </button>
              <button type="button" class="icon-btn" aria-label="Turunkan prioritas" [disabled]="last" (click)="moveField(i, 1)">
                <app-icon name="chevron-down" [size]="14" />
              </button>
              <button type="button" class="icon-btn" [attr.aria-label]="'Hapus ' + id" (click)="removeField(id)">
                <app-icon name="x" [size]="14" />
              </button>
            </li>
          } @empty {
            <li class="trf-empty">Belum ada field. Tambahkan dari daftar di bawah.</li>
          }
        </ol>
        @if (fieldErrors().storyPointFields) {
          <p class="trf-err">{{ fieldErrors().storyPointFields }}</p>
        }
        <div class="trf-add">
          <label class="search-input">
            <app-icon name="search" [size]="16" />
            <input type="search" placeholder="Cari field angka…" [ngModel]="fieldQuery()" (ngModelChange)="fieldQuery.set($event)" aria-label="Cari field angka" />
          </label>
          <div class="trf-chips">
            @for (f of suggestions(); track f.id) {
              <button type="button" class="trf-chip add" [class.sp]="f.storyPointLike" [attr.data-add-field]="f.id" (click)="addField(f.id)">
                <app-icon name="plus" [size]="12" /> {{ f.name }} <small>{{ f.id }}</small>
              </button>
            } @empty {
              <span class="muted">Tidak ada field angka yang cocok.</span>
            }
          </div>
        </div>
        <div class="trf-row">
          <div class="trf-field">
            <span>Field SP tulis (card biasa)</span>
            <app-select data-write [options]="writeOptions()" [ngModel]="writeField()" (ngModelChange)="setWrite('card', $event)" ariaLabel="Field SP tulis card biasa" />
            @if (fieldErrors().storyPointWriteField) {
              <p class="trf-err">{{ fieldErrors().storyPointWriteField }}</p>
            }
          </div>
          <div class="trf-field">
            <span>Field SP tulis (subtask)</span>
            <app-select data-write-subtask [options]="writeOptions()" [ngModel]="writeFieldSubtask()" (ngModelChange)="setWrite('subtask', $event)" ariaLabel="Field SP tulis subtask" />
            @if (fieldErrors().storyPointWriteFieldSubtask) {
              <p class="trf-err">{{ fieldErrors().storyPointWriteFieldSubtask }}</p>
            }
          </div>
        </div>
      </section>

      <section class="card trf-card" data-section="preview">
        <header class="card-header">
          <span class="card-icon accent"><app-icon name="zap" [size]="20" /></span>
          <div>
            <h2>Preview</h2>
            <p>Begini Nudge memantau tiket {{ l.projectKey }}.</p>
          </div>
        </header>
        <p class="trf-sentence" data-preview aria-live="polite">{{ preview() }}</p>
        <div class="card-footer">
          @if (staleStatuses().length || staleFields().length) {
            <p class="msg error"><app-icon name="alert-circle" [size]="16" /> Buang nilai bertanda ⚠ dulu sebelum menyimpan.</p>
          }
          <span class="spacer"></span>
          <a class="btn btn-ghost" routerLink="/admin/tracking-rules">Batal</a>
          <button type="button" class="btn btn-primary" data-save [class.loading]="saving()" [disabled]="!canSave()" (click)="save()">
            <span class="spinner" aria-hidden="true"></span>
            <span class="btn-label"><app-icon name="check" [size]="16" /> Simpan</span>
          </button>
        </div>
      </section>
    } @else if (!lookupError()) {
      <section class="card trf-card trf-placeholder">
        <app-icon name="info" [size]="18" />
        <span>{{ isEdit && !lookingUp() ? 'Memuat aturan…' : 'Cek project di Jira dulu untuk memilih status dan field Story Point.' }}</span>
      </section>
    }
  }
</div>
```

`tracking-rule-form.component.scss`:
```scss
// Keyframes fade-up / pop-in / fade-in are global (styles.scss).
:host { display: block; animation: fade-up 260ms var(--ease) backwards; }
.trf-card { animation: fade-up 280ms var(--ease) backwards;
  @for $i from 2 through 4 { &:nth-child(#{$i}) { animation-delay: #{($i - 1) * 50}ms; } } }
.trf-key { display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap;
  input { text-transform: uppercase; letter-spacing: .04em; } }
.trf-field { display: flex; flex-direction: column; gap: 6px; flex: 1; min-width: 220px; font-size: .82rem; color: var(--text-muted);
  input { font: inherit; font-size: .92rem; color: var(--text); height: 40px; padding: 0 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface);
    transition: border-color var(--fast) var(--ease), box-shadow var(--fast) var(--ease), background-color var(--fast) var(--ease);
    &:hover:not([readonly]) { border-color: var(--border-strong); }
    &:focus { outline: 0; border-color: var(--primary-light); box-shadow: 0 0 0 3px var(--primary-ring); }
    &[readonly] { background: var(--surface-2); color: var(--text-muted); cursor: not-allowed; } }
  app-select { color: var(--text); } }
.trf-row { display: flex; gap: 14px; flex-wrap: wrap; margin-top: 16px; }
.trf-label { display: block; margin-bottom: 8px; font-size: .82rem; font-weight: 600; color: var(--text);
  em { font-style: normal; font-weight: 400; color: var(--text-faint); margin-left: 6px; } }
.trf-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.trf-chip { display: inline-flex; align-items: center; gap: 5px; padding: 6px 12px; border: 1px solid var(--border); border-radius: 999px; background: var(--surface);
  font: inherit; font-size: .8rem; color: var(--text-muted); cursor: pointer; animation: pop-in 220ms var(--ease) backwards;
  transition: border-color var(--fast) var(--ease), background-color var(--fast) var(--ease), color var(--fast) var(--ease),
    transform var(--fast) var(--ease-spring), box-shadow var(--fast) var(--ease);
  &:hover { border-color: var(--primary-light); color: var(--text); transform: translateY(-1px); }
  &:active { transform: scale(.96); }
  &:focus-visible { outline: 0; box-shadow: 0 0 0 3px var(--primary-ring); }
  &.on { border-color: var(--primary); background: var(--primary-soft); color: var(--primary-strong); font-weight: 600; }
  &.add small { color: var(--text-faint); font-size: .7rem; }
  &.add.sp { border-style: dashed; border-color: var(--primary-light); }
  &.stale { cursor: default; border-color: #f5c08a; background: var(--accent-soft); color: #b4630a; &:hover { transform: none; } } }
.trf-x { display: inline-grid; place-items: center; width: 18px; height: 18px; border: 0; border-radius: 50%; background: transparent; color: inherit; cursor: pointer;
  transition: background-color var(--fast) var(--ease);
  &:hover { background: rgba(0, 0, 0, .08); } }
.trf-stale { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
.trf-fields { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.trf-field-row { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-2);
  animation: fade-up 220ms var(--ease) backwards; transition: border-color var(--fast) var(--ease), box-shadow var(--fast) var(--ease);
  &:hover { border-color: var(--border-strong); box-shadow: var(--shadow-xs); }
  &.stale { border-color: #f5c08a; background: var(--accent-soft); }
  em { font-style: normal; color: #b4630a; font-size: .74rem; margin-left: 6px; } }
.trf-rank { display: inline-grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: var(--primary-soft); color: var(--primary-strong); font-size: .72rem; font-weight: 700; }
.trf-field-name { flex: 1; font-size: .84rem; color: var(--text); }
.trf-flip { display: inline-flex; transform: rotate(180deg); }
.trf-empty { padding: 10px 12px; border: 1px dashed var(--border-strong); border-radius: var(--radius-sm); color: var(--text-faint); font-size: .82rem; }
.trf-add { display: flex; flex-direction: column; gap: 10px; }
.trf-project { display: flex; align-items: center; gap: 12px; margin-top: 16px; padding: 12px; border-radius: var(--radius-sm); background: var(--surface-2);
  img, .trf-avatar { width: 40px; height: 40px; border-radius: 10px; }
  .trf-avatar { display: grid; place-items: center; background: var(--gradient); color: #fff; font-weight: 800; }
  div { display: flex; flex-direction: column; }
  small { color: var(--text-faint); font-size: .76rem; } }
.trf-pop { animation: pop-in 240ms var(--ease) backwards; }
.trf-err { margin: 6px 0 0; color: var(--error); font-size: .78rem; animation: fade-in 200ms var(--ease); }
.trf-sentence { margin: 0; padding: 14px 16px; border-left: 3px solid var(--primary); border-radius: var(--radius-sm); background: var(--primary-soft);
  color: var(--ink); line-height: 1.6; font-size: .9rem; }
.trf-placeholder { display: flex; align-items: center; gap: 10px; color: var(--text-muted); font-size: .86rem; }

@media (prefers-reduced-motion: reduce) {
  :host, .trf-card, .trf-chip, .trf-field-row, .trf-pop, .trf-err { animation: none; }
  .trf-chip, .trf-field-row, .trf-field input, .trf-x { transition: none; }
}
```

`app.routes.ts` — setelah route `tracking-rules`:
```ts
      {
        path: 'tracking-rules/new',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rule-form.component').then((m) => m.TrackingRuleFormComponent),
        title: 'Tambah Aturan · Admin · Nudge',
        data: { heading: 'Tambah Aturan Board', subheading: 'Pilih status dan field Story Point dari data Jira', crumbs: ['Admin', 'Aturan Board', 'Tambah'] },
      },
      {
        path: 'tracking-rules/:key',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rule-form.component').then((m) => m.TrackingRuleFormComponent),
        title: 'Edit Aturan · Admin · Nudge',
        data: { heading: 'Edit Aturan Board', subheading: 'Status dan field dimuat ulang dari Jira', crumbs: ['Admin', 'Aturan Board', 'Edit'] },
      },
```

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: PASS; build sukses.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/features/admin/tracking-rules frontend/src/app/app.routes.ts
git commit -m "feat(tracking-rules): add board rule form with jira lookup and preview" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: Frontend Pengaturan — status workflow board dari API + fallback statis

**Files:**
- Modify: `frontend/src/app/features/dashboard/settings/settings.component.ts` (import, field, `statusOptions`, constructor, method baru)
- Test: `frontend/src/app/features/dashboard/settings/settings.component.spec.ts`

**Interfaces:**
- Consumes: `TrackingRuleService.boardStatuses(boardId)` (Task 10), endpoint Task 9.
- Produces: `protected jiraStatuses = signal<string[] | null>(null)`; `statusOptions` = `jiraStatuses() ?? statusOptionsFor(statusRule())`.

- [ ] **Step 1: Write the failing test** (append di dalam `describe('SettingsComponent', …)`; tambah import `HttpTestingController` dari `'@angular/common/http/testing'`)

```ts
  it('offers the board workflow statuses from Jira and falls back to the static list (AC5.5)', () => {
    const http = TestBed.inject(HttpTestingController);
    context.boards.set([OMS, { ...OMS, boardId: 26, name: 'ESB Core Board', projectKey: 'CORE', reviewTracked: true }]);
    context.selectedId.set(26);
    fixture.detectChanges();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/boards/26/statuses')).flush({ statuses: ['To Do', 'Doing', 'Done'] });
    fixture.detectChanges();
    expect(component().statusOptions()).toEqual(['To Do', 'Doing', 'Done']);

    context.selectedId.set(28);
    fixture.detectChanges();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/boards/28/statuses')).flush({ error: 'jira_unavailable' }, { status: 502, statusText: 'Bad Gateway' });
    fixture.detectChanges();
    expect(component().statusOptions()).toContain('Testing & Review');
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless --include='**/settings.component.spec.ts'`
Expected: FAIL — `Expected one matching request … found none`.

- [ ] **Step 3: Implement** (`settings.component.ts`)

Import:
```ts
import { TrackingRuleService } from '../../../core/services/tracking-rule.service';
```
Ganti baris `protected statusOptions = computed(() => statusOptionsFor(this.statusRule()));` dengan:
```ts
  private trackingRules = inject(TrackingRuleService);
  /** The scope board's workflow statuses from Jira; null = default scope, loading or Jira failed. */
  protected jiraStatuses = signal<string[] | null>(null);
  private statusesSub?: Subscription;
  /** Destination statuses for the status-change filter: the board's Jira workflow, else the static lists. */
  protected statusOptions = computed(() => this.jiraStatuses() ?? statusOptionsFor(this.statusRule()));
```
Di `constructor()`, setelah effect yang ada:
```ts
    // Pengaturan › Notif perubahan status: the board's real workflow (AC5.5).
    effect(() => {
      const boardId = this.scopeBoardId();
      untracked(() => this.loadBoardStatuses(boardId));
    });
```
Method baru (setelah `loadSettings`):
```ts
  private loadBoardStatuses(boardId: number | null): void {
    this.statusesSub?.unsubscribe();
    this.jiraStatuses.set(null);
    if (boardId === null) return;
    this.statusesSub = this.trackingRules.boardStatuses(boardId).subscribe({
      next: (res) => this.jiraStatuses.set(res.statuses?.length ? res.statuses : null),
      error: () => this.jiraStatuses.set(null),
    });
  }
```
Perbarui komentar di atas `CORE_STATUSES`: `/** Fallback when Jira can't answer: statuses of the CORE and OMS workflows; matching is case-insensitive. */`.

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: PASS (test lama `names the tracked statuses…` tetap lulus karena request yang tidak di-flush → fallback statis).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/features/dashboard/settings/settings.component.ts frontend/src/app/features/dashboard/settings/settings.component.spec.ts
git commit -m "feat(tracking-rules): offer board workflow statuses in settings" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 14: README + verifikasi akhir

**Files:**
- Modify: `README.md` (bullet Admin panel setelah "Broadcast Message" baris 48; bullet Pengaturan baris 38; daftar modul setelah `modules/admin_dashboard` baris 123)

**Interfaces:** tidak ada.

- [ ] **Step 1: Update README**

Setelah bullet `- **Broadcast Message** — pengumuman lewat DM ke semua user aktif`:
```markdown
    - **Aturan Board** (`/admin/tracking-rules`) — per project Jira: status yang dihitung budget, status selesai, status review (opsional), field Story Point yang dibaca (urut prioritas) dan field SP yang ditulis Buat Card untuk card biasa / subtask. Status & field dipilih dari data Jira (**Cek di Jira**, cache 10 menit); preview kalimat realtime; nilai tersimpan yang sudah tidak ada di Jira ditandai ⚠ dan wajib dibuang. Hapus = **nonaktifkan** (tiket project itu berhenti dipantau, sesi berjalan ditutup tanpa menambah waktu & tanpa notifikasi); minimal satu board aktif. Disimpan di `ms_tracking_rule` (seed OMS & CORE identik aturan lama, `INSERT IGNORE`), dimuat ke registry in-memory saat start, setiap simpan, dan tiap 5 menit. Toast simpan punya tombol **Hitung ulang semua user** (backfill changelog semua user di background). Endpoint: `/api/dashboard/admin/tracking-rules{,/jira/:projectKey,/:projectKey,/:projectKey/active,/recalculate}`
```
Di bullet **Perubahan status tiket** (baris 38), ganti `daftar status mengikuti workflow project board terpilih` dengan `daftar status mengikuti workflow Jira project board terpilih (\`GET /api/dashboard/boards/:boardId/statuses\`, cache 1 jam; fallback daftar statis bila Jira gagal)`.
Setelah baris `- \`modules/admin_dashboard\` — …`:
```markdown
- `modules/tracking_rule` — Aturan Board: CRUD `ms_tracking_rule` tervalidasi ke data Jira, registry aturan (`jira_client_dto.SetRules`) yang dibaca poller/reminder/status watch/backfill/dashboard/Buat Card, refresh tiap 5 menit
```

- [ ] **Step 2: Full verification**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS.

Run: `cd frontend && npx ng test --watch=false --browsers=ChromeHeadless && npx ng build`
Expected: PASS; build sukses.

- [ ] **Step 3: Manual smoke (lokal)** — jalankan backend + frontend, login sebagai `ADMIN_EMAIL`:
  1. `SELECT projectKey, isActive FROM ms_tracking_rule` → `CORE 1`, `OMS 1`.
  2. Log poller: JQL sama dengan sebelum perubahan (`(project = CORE AND status in ("Code Process", "CODE REVIEW")) OR (project = OMS AND status in ("Code Process"))`).
  3. Admin › Aturan Board: tambah project uji → muncul di daftar & JQL cycle berikutnya → nonaktifkan → hilang dari JQL, tidak ada notifikasi.
  4. Coba nonaktifkan semua → toast "Minimal satu board harus aktif".
  5. Pengaturan › Notif perubahan status untuk board CORE: daftar status berasal dari Jira.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs(tracking-rules): document admin board rules" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage**

| Spec item | Task |
|---|---|
| AC1.1 seed persis | 1 (test seed), 7 (`TestRepository_SeedEqualsTheBuiltinRules`) |
| AC1.2 paritas perilaku | 2 (builtin = seed, JQL lama tetap), 3 (`TestIssueStoryPoints_ParityWithTheLegacyCoalesce`), 7 |
| AC1.3 seed tidak menimpa | 1 |
| AC1.4 tidak ada aturan aktif | 2 (JQL ""), 4 (search & poller skip), 8 (Reload WARN) |
| AC2.1 menu + route | 11 |
| AC2.2 tabel | 11 |
| AC2.3 non-admin 403 | 9 (`AdminMiddleware` di `MountTrackingRules`) |
| AC2.4 baris nonaktif + Aktifkan | 11 |
| AC3.1 lookup Jira | 6, 8, 12 |
| AC3.2 kontrol form | 12 |
| AC3.3 preview | 12 |
| AC3.4 201 + refresh + toast hitung ulang | 8 (afterSave), 9, 10, 12 |
| AC3.5 404 project_not_found | 8, 9, 12 |
| AC3.6 409 rule_exists + link Edit | 8, 9, 12 |
| AC3.7 validasi server | 8 (`buildRule`), 9, 12 |
| AC3.8 normalisasi key | 8, 10 |
| AC4.1 edit reload Jira, key terkunci | 12 |
| AC4.2 nonaktifkan + konfirmasi | 8, 11 (+ JQL/relasi via registry 2/4) |
| AC4.3 422 last_active_rule | 8, 9, 11 |
| AC4.4 ⚠ nilai tak ada di Jira | 12 |
| AC4.5 sesi ditutup `CloseSession(key, 0)` | 4 |
| AC4.6 aktif lagi → `OpenOrResumeSessionAt` | perilaku poller yang ada (tidak diubah); registry 2/8 |
| AC5.1 JQL & `IsTrackedProjectKey` dari registry | 2, 4 |
| AC5.2 SP per project + `fields=` | 3 |
| AC5.3 / AC5.3b Buat Card | 5 |
| AC5.4 refresh on save + 5 menit | 8, 9 |
| AC5.5 status board di Pengaturan | 8 (`BoardStatuses`), 9, 13 |
| AC5.6 refresh gagal → aturan lama | 8 |
| README / deploy verify (spec #10) | 14 |

**Placeholder scan:** tidak ada TBD/TODO/"similar to"; setiap langkah kode berisi kode lengkap.

**Type consistency (dicek):** `SetRules/BuiltinRules/ActiveProjectKeys/StoryPointFieldIDs/ProjectsJQLIn/IsTrackedProject` (Task 2) dipakai sama di Task 3/4/7/8; `Issue.StoryPoints()` + `IssueFields.Numbers` (Task 3) dipakai Task 4 test helper; `ProjectMetaClient` + `ErrProjectNotFound` (Task 6) dipakai Task 8; `Store`/`Repository` method set (`Update`/`SetActive` → `error`) sama di Task 7 & 8; `API` (Task 9) = method `*Service` (Task 8); frontend `TrackingRuleService` method (Task 10) = yang dipanggil Task 11/12/13; `recalculateAction`/`RECALCULATE_LABEL` (Task 10) dipakai Task 11/12.
