# Papan Tiket & Cuti Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tambah menu **Papan Tiket** (pindah status tiket sprint aktif sesuai workflow Jira CORE/OMS) dan **Cuti** (hari cuti tidak dihitung ke waktu tiket dan tidak ada DM pada hari itu, termasuk cuti di tengah tiket berjalan).

**Architecture:** Kalender global `HolidayCalendar` diganti antarmuka `CalendarSource.ForEmail(ctx, email)`; `HolidayCalendar` sendiri mengimplementasikannya (test lama tetap jalan), dan service `user_leave` mengimplementasikannya sebagai libur ∪ cuti user (cache 5 menit). Simpan/hapus cuti tanggal lampau memicu `sprint_backfill.RecalculateForEmail` (async) yang menulis ulang `accumulatedSeconds` dari changelog. Pindah status memakai `GetTransitionsAs` / `TransitionIssueAs` dengan cek kepemilikan lewat pencarian sprint aktif user.

**Tech Stack:** Go 1.x + Gin + MySQL (`database/sql`), Angular 19 standalone + signals, Karma/Jasmine.

**Spec:** `docs/tech-spec/papan-tiket-cuti/Tech Spec - Papan Tiket & Cuti.md`

## Global Constraints

- Module Go `nudge`; base path `/nudge`; endpoint dashboard di bawah `/api/dashboard` (group sudah ber-`AuthMiddlewareAPI`).
- User sesi diambil dengan `currentUser(c).Email` — tidak pernah dari parameter request.
- Zona waktu: WIB (`time.FixedZone("WIB", 7*3600)` / `Asia/Jakarta`); tanggal format `2006-01-02`.
- Cuti: hari penuh; rentang maks 31 hari; tanggal paling awal = hari ini − 60 hari; catatan maks 200 karakter; Sabtu/Minggu & libur nasional dilewati.
- Cache kalender per user TTL 5 menit, di-invalidate saat simpan/hapus cuti.
- Recalc tidak mengubah `currentSessionStartedDate` maupun `flagActive`; `editedBy = 'leave-recalc'`.
- Status kode: 403 bukan tiket sendiri / bukan sprint aktif, 422 transisi tidak tersedia / rentang tanpa hari kerja, 502 Jira gagal, 400 validasi, 404 cuti tidak ada.
- Teks UI Bahasa Indonesia; kode & komentar English.
- File CRLF milik agent lain jangan di-`gofmt` ulang; stage hanya file plan ini. Commit hanya saat user minta.

---

## File Structure

| File | Tanggung jawab |
|---|---|
| `modules/business_hours/business_hours_dto/business_hours_dto.go` | + `CalendarSource`, `HolidayCalendar.ForEmail`, `HolidayCalendar.With` |
| 6 service (reminder_engine, poller, chat_interactive, user_dashboard, sprint_report, sprint_backfill) | field kalender → `CalendarSource`, panggil `ForEmail(ctx, session.AssigneeEmail)` |
| `migrations/0015_add_user_leave.sql`, `migrations/migrate.go` | tabel `tr_user_leave` |
| `modules/user_leave/user_leave_dto/user_leave_dto.go` | DTO |
| `modules/user_leave/user_leave_repository/*` | SQL CRUD |
| `modules/user_leave/user_leave_service/*` | validasi rentang, `CalendarSource` ber-cache, hook recalc |
| `modules/user_leave/user_leave_handler/*`, `modules/user_leave/router.go` | `GET/POST/DELETE /leaves` |
| `modules/issue_session/issue_session_repository/session_backfill_repository.go` | + `UpdateAccumulatedSeconds` |
| `modules/sprint_backfill/sprint_backfill_service/*` | + `RecalculateForEmail` |
| `modules/jira_client/jira_client_dto/jira_client_dto.go` | `Transition.Name` |
| `modules/user_dashboard/...` | `ListTransitions`, `Transition`, handler + route |
| `main.go`, `router.go` | wiring |
| `frontend/src/app/features/dashboard/board-page/*` | Papan Tiket |
| `frontend/src/app/features/dashboard/leave/*` | Cuti |
| `frontend/src/app/core/services/dashboard.service.ts`, `leave.service.ts` | API client |
| `frontend/src/app/app.routes.ts`, `layouts/dashboard-shell/dashboard-shell.component.html`, `shared/ui/icon.component.ts`, `styles.scss` | route, menu, ikon, gaya |

---

### Task 1: `CalendarSource` dan 6 service per-user

**Files:**
- Modify: `modules/business_hours/business_hours_dto/business_hours_dto.go`
- Create: `modules/business_hours/business_hours_dto/business_hours_dto_test.go`
- Modify: `modules/reminder_engine/reminder_engine_service/reminder_engine_service_impl.go` (field `holidayCalendar` baris 33, constructor 49, pemakaian 158/221/250/287/392)
- Modify: `modules/poller/poller_service/poller_service_impl.go` (29, 46, 192)
- Modify: `modules/chat_interactive/chat_interactive_service/chat_interactive_service_impl.go` (53, 62, 201, 203, 224)
- Modify: `modules/user_dashboard/user_dashboard_service/user_dashboard_service_impl.go` (31, 56, 316, 318, 368)
- Modify: `modules/sprint_report/sprint_report_service/sprint_report_service_impl.go` (16, 22, 76)
- Modify: `modules/sprint_backfill/sprint_backfill_service/sprint_backfill_service_impl.go` (43, 53, 176)
- Test: `modules/reminder_engine/reminder_engine_service/reminder_engine_service_test.go`

**Interfaces:**
- Produces: `business_hours_dto.CalendarSource` (`ForEmail(ctx context.Context, email string) HolidayCalendar`), `HolidayCalendar.With(dates []string) HolidayCalendar`. Semua constructor yang tadinya menerima `business_hours_dto.HolidayCalendar` kini menerima `business_hours_dto.CalendarSource` (nilai `HolidayCalendar` tetap valid).

- [ ] **Step 1: Test DTO**

```go
package business_hours_dto

import (
	"context"
	"testing"
	"time"
)

func TestHolidayCalendar_ForEmailReturnsItself(t *testing.T) {
	cal := HolidayCalendar{Holidays: map[string]bool{"2026-12-25": true}}
	got := cal.ForEmail(context.Background(), "any@esb.co.id")
	if !got.IsHoliday(time.Date(2026, 12, 25, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("ForEmail must keep the global holidays")
	}
}

func TestHolidayCalendar_WithAddsDatesWithoutMutatingOriginal(t *testing.T) {
	base := HolidayCalendar{Holidays: map[string]bool{"2026-12-25": true}}
	merged := base.With([]string{"2026-11-18"})
	if !merged.Holidays["2026-11-18"] || !merged.Holidays["2026-12-25"] {
		t.Fatalf("merged = %v", merged.Holidays)
	}
	if base.Holidays["2026-11-18"] {
		t.Fatal("With must not mutate the base calendar")
	}
}
```

- [ ] **Step 2: Jalankan → FAIL**

Run: `go test ./modules/business_hours/business_hours_dto/`
Expected: FAIL `cal.ForEmail undefined`.

- [ ] **Step 3: Implementasi DTO**

```go
package business_hours_dto

import (
	"context"
	"time"
)

type HolidayCalendar struct {
	Holidays map[string]bool
}

// CalendarSource yields the non-working days that apply to one user: the
// national holidays plus, when leave is recorded, that user's leave days
// (Tech Spec "Papan Tiket & Cuti", AC3.1).
type CalendarSource interface {
	ForEmail(ctx context.Context, email string) HolidayCalendar
}

// ForEmail makes a plain calendar a CalendarSource that ignores the user.
func (c HolidayCalendar) ForEmail(context.Context, string) HolidayCalendar { return c }

// With returns a copy of c that also treats dates ("2006-01-02") as holidays.
func (c HolidayCalendar) With(dates []string) HolidayCalendar {
	merged := make(map[string]bool, len(c.Holidays)+len(dates))
	for d, v := range c.Holidays {
		merged[d] = v
	}
	for _, d := range dates {
		merged[d] = true
	}
	return HolidayCalendar{Holidays: merged}
}

func (c HolidayCalendar) IsHoliday(t time.Time) bool {
	if c.Holidays == nil {
		return false
	}
	return c.Holidays[t.Format("2006-01-02")]
}
```

> Catatan: `IsHoliday` memakai `t.Format` apa adanya — pemanggil di `business_hours_service` sudah menormalkan ke WIB sebelum memanggilnya; jangan diubah.

- [ ] **Step 4: Jalankan → PASS**

Run: `go test ./modules/business_hours/...`

- [ ] **Step 5: Ganti tipe di 6 service**

Di setiap file service: ubah tipe field & parameter constructor `business_hours_dto.HolidayCalendar` → `business_hours_dto.CalendarSource`, lalu ganti setiap pemakaian `X.holidayCalendar` dengan kalender assignee:

reminder_engine (`EvaluateAndSend`, awal fungsi):
```go
cal := e.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)
elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, e.businessHours, cal)
```
Teruskan `cal` ke `sendTierIfDue` / `sendOverageIfDue` sebagai parameter baru `cal business_hours_dto.HolidayCalendar` (letakkan setelah `setting`), dan pakai `cal` di `IsWithinBusinessHours(time.Now(), cal)` dan `DeadlineAfterBusinessSeconds(..., cal)`. Di `NotifyIfEarlyCompletion`:
```go
deadline := e.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remaining, e.holidayCalendar.ForEmail(ctx, assigneeEmail))
```

poller (baris 192):
```go
additionalSeconds = p.businessHours.ElapsedBusinessSeconds(*session.CurrentSessionStartedDate, time.Now(), p.holidayCalendar.ForEmail(ctx, session.AssigneeEmail))
```

chat_interactive (201–203, 224): `cal := s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)` lalu pakai `cal`. Bila fungsi `formatTicketLine` belum menerima `ctx`, tambahkan parameter `ctx context.Context` pertama dan teruskan dari pemanggil.

user_dashboard (316–318, 368): sama — `cal := s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)`; tambahkan `ctx` ke helper bila belum ada.

sprint_report (76): `issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail))`.

sprint_backfill (176): closure di `backfillIssue`:
```go
cal := s.calendar.ForEmail(ctx, user.Email)
seconds, found := jira_client_dto.TrackedSeconds(histories, rule, func(start, end time.Time) int64 {
	return s.businessHours.ElapsedBusinessSeconds(start, end, cal)
})
```

- [ ] **Step 6: Test reminder engine — hari cuti assignee tidak mengirim DM**

Tambahkan di `reminder_engine_service_test.go` (pakai helper/fake yang sudah ada di file itu untuk membuat engine & session; `fakeBusinessHoursGate` sudah menerima `cal`):

```go
// leaveCalendar marks every day as a holiday for one email only.
type leaveCalendar struct{ email string }

func (l leaveCalendar) ForEmail(_ context.Context, email string) business_hours_dto.HolidayCalendar {
	if email != l.email {
		return business_hours_dto.HolidayCalendar{}
	}
	today := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
	return business_hours_dto.HolidayCalendar{Holidays: map[string]bool{today: true}}
}
```
Test `TestEvaluateAndSend_NoReminderOnAssigneeLeaveDay`: engine dibangun dengan `leaveCalendar{email: "dev@esb.co.id"}` dan business-hours **asli** (`business_hours_service.NewBusinessHoursService()` — cek nama constructor di `business_hours_service_impl.go`), session melewati satu tier → notifier fake tidak menerima pesan. Bila waktu test di luar jam kerja hasilnya juga 0 pesan; untuk menegaskan penyebabnya cuti, tambahkan kasus kontrol dengan email lain yang memakai `fakeBusinessHoursGate{within: true}` (pola yang sudah dipakai di file test) dan pastikan fake gate menerima kalender kosong untuk email lain, kalender berisi hari ini untuk email cuti:

```go
gate := &fakeBusinessHoursGate{within: true}
// ... setelah EvaluateAndSend untuk dev@esb.co.id:
if !gate.lastCal.IsHoliday(time.Now()) { t.Fatal("gate must receive the assignee's leave calendar") }
```
Jika `fakeBusinessHoursGate` belum merekam `lastCal`, tambahkan field `lastCal business_hours_dto.HolidayCalendar` dan set di `IsWithinBusinessHours`.

- [ ] **Step 7: Build & test semua**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (fakes lama yang mengoper `business_hours_dto.HolidayCalendar{}` tetap kompil karena tipe itu memenuhi `CalendarSource`).

---

### Task 2: Tabel, repository, dan service `user_leave` (+ kalender per user)

**Files:**
- Create: `migrations/0015_add_user_leave.sql`
- Modify: `migrations/migrate.go` (embed + langkah setelah `userNotificationSwitchSQL`)
- Create: `modules/user_leave/user_leave_dto/user_leave_dto.go`
- Create: `modules/user_leave/user_leave_repository/user_leave_repository.go`, `user_leave_repository_test.go`
- Create: `modules/user_leave/user_leave_service/user_leave_service.go`, `user_leave_service_impl.go`, `user_leave_service_test.go`
- Modify: `main.go` (buat service & oper ke 6 service menggantikan `holidayCalendar`)

**Interfaces:**
- Consumes: `business_hours_dto.CalendarSource`, `HolidayCalendar.With`.
- Produces:
  - `user_leave_dto.Leave{Date, Note string}`, `user_leave_dto.AddResult{Added, Skipped []string; Recalculating bool}`
  - `user_leave_repository.UserLeaveRepository` { `ListByEmail(ctx, email) ([]user_leave_dto.Leave, error)`; `Insert(ctx, email string, dates []string, note string) ([]string, error)`; `Delete(ctx, email, date string) (bool, error)`; `DatesByEmail(ctx, email) ([]string, error)` }
  - `user_leave_service.UserLeaveService` { `List`; `Add(ctx, email, from, to, note string) (user_leave_dto.AddResult, error)`; `Remove(ctx, email, date string) (recalculating bool, err error)`; `ForEmail(ctx, email) business_hours_dto.HolidayCalendar` }
  - `user_leave_service.ErrInvalidRange`, `ErrNoWorkingDay`, `ErrNotFound`
  - `user_leave_service.NewUserLeaveService(repo UserLeaveRepository, global business_hours_dto.HolidayCalendar, onPastChange func(email string)) UserLeaveService`

- [ ] **Step 1: Migrasi**

`migrations/0015_add_user_leave.sql`:
```sql
CREATE TABLE IF NOT EXISTS tr_user_leave (
  userLeaveID BIGINT AUTO_INCREMENT PRIMARY KEY,
  email       VARCHAR(255) NOT NULL,
  leaveDate   DATE         NOT NULL,
  note        VARCHAR(200) NULL,
  createdDate DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_user_leave_email_date (email, leaveDate),
  KEY idx_user_leave_date (leaveDate)
);
```
`migrate.go` — sebelum menulis cek `ls migrations/` lagi: bila agent lain sudah memakai 0015, pakai nomor berikutnya.
```go
//go:embed 0015_add_user_leave.sql
var addUserLeaveSQL string
```
dan ganti akhir fungsi yang sekarang `return addColumnIfNotExists(db, userNotificationSwitchSQL)` menjadi:
```go
	if err := addColumnIfNotExists(db, userNotificationSwitchSQL); err != nil {
		return err
	}
	// Tech Spec "Papan Tiket & Cuti" — CREATE TABLE IF NOT EXISTS.
	_, err := db.Exec(addUserLeaveSQL)
	return err
```

- [ ] **Step 2: DTO**

```go
package user_leave_dto

// Leave is one full day of leave (Tech Spec "Papan Tiket & Cuti").
type Leave struct {
	Date string `json:"date"` // 2006-01-02
	Note string `json:"note"`
}

// AddResult reports what a range request stored.
type AddResult struct {
	Added         []string `json:"added"`
	Skipped       []string `json:"skipped"` // weekends and national holidays
	Recalculating bool     `json:"recalculating"`
}
```

- [ ] **Step 3: Test repository (DB nyata, pola `testutil.OpenTestDB`)**

```go
package user_leave_repository

import (
	"context"
	"testing"

	"nudge/base/testutil"
)

func TestUserLeaveRepository_InsertListDelete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewUserLeaveRepository(db)
	ctx := context.Background()
	const email = "leave-test@esb.co.id"
	db.Exec(`DELETE FROM tr_user_leave WHERE email = ?`, email)
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_user_leave WHERE email = ?`, email) })

	added, err := repo.Insert(ctx, email, []string{"2026-11-18", "2026-11-19"}, "Cuti")
	if err != nil || len(added) != 2 {
		t.Fatalf("Insert = %v, %v", added, err)
	}
	again, err := repo.Insert(ctx, email, []string{"2026-11-19", "2026-11-20"}, "")
	if err != nil || len(again) != 1 || again[0] != "2026-11-20" {
		t.Fatalf("second Insert = %v, %v; want only 2026-11-20", again, err)
	}

	list, err := repo.ListByEmail(ctx, email)
	if err != nil || len(list) != 3 || list[0].Date != "2026-11-20" || list[2].Note != "Cuti" {
		t.Fatalf("ListByEmail = %+v, %v", list, err)
	}
	dates, _ := repo.DatesByEmail(ctx, email)
	if len(dates) != 3 {
		t.Fatalf("DatesByEmail = %v", dates)
	}

	if ok, err := repo.Delete(ctx, email, "2026-11-18"); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if ok, _ := repo.Delete(ctx, email, "2026-11-18"); ok {
		t.Fatal("deleting twice must report false")
	}
	if ok, _ := repo.Delete(ctx, "other@esb.co.id", "2026-11-19"); ok {
		t.Fatal("must not delete another user's leave")
	}
}
```

- [ ] **Step 4: Jalankan → FAIL** — `go test ./modules/user_leave/...` (undefined `NewUserLeaveRepository`).

- [ ] **Step 5: Repository**

```go
package user_leave_repository

import (
	"context"
	"database/sql"

	"nudge/modules/user_leave/user_leave_dto"
)

type UserLeaveRepository interface {
	ListByEmail(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
	// Insert stores each date once; it returns the dates that were new.
	Insert(ctx context.Context, email string, dates []string, note string) ([]string, error)
	Delete(ctx context.Context, email, date string) (bool, error)
	DatesByEmail(ctx context.Context, email string) ([]string, error)
}

type userLeaveRepositoryImpl struct{ db *sql.DB }

func NewUserLeaveRepository(db *sql.DB) UserLeaveRepository {
	return &userLeaveRepositoryImpl{db: db}
}

func (r *userLeaveRepositoryImpl) ListByEmail(ctx context.Context, email string) ([]user_leave_dto.Leave, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(leaveDate, '%Y-%m-%d'), COALESCE(note, '') FROM tr_user_leave WHERE email = ? ORDER BY leaveDate DESC`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []user_leave_dto.Leave{}
	for rows.Next() {
		var l user_leave_dto.Leave
		if err := rows.Scan(&l.Date, &l.Note); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *userLeaveRepositoryImpl) Insert(ctx context.Context, email string, dates []string, note string) ([]string, error) {
	var noteVal sql.NullString
	if note != "" {
		noteVal = sql.NullString{String: note, Valid: true}
	}
	var added []string
	for _, d := range dates {
		res, err := r.db.ExecContext(ctx,
			`INSERT IGNORE INTO tr_user_leave (email, leaveDate, note) VALUES (?, ?, ?)`, email, d, noteVal)
		if err != nil {
			return added, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = append(added, d)
		}
	}
	return added, nil
}

func (r *userLeaveRepositoryImpl) Delete(ctx context.Context, email, date string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tr_user_leave WHERE email = ? AND leaveDate = ?`, email, date)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *userLeaveRepositoryImpl) DatesByEmail(ctx context.Context, email string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(leaveDate, '%Y-%m-%d') FROM tr_user_leave WHERE email = ?`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
```

- [ ] **Step 6: Jalankan → PASS** — `go test ./modules/user_leave/user_leave_repository/`

- [ ] **Step 7: Test service (fake repo, `now` tetap)**

```go
package user_leave_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

type fakeRepo struct {
	dates map[string]string // date → note
	reads int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{dates: map[string]string{}} }

func (f *fakeRepo) ListByEmail(context.Context, string) ([]user_leave_dto.Leave, error) {
	var out []user_leave_dto.Leave
	for d, n := range f.dates {
		out = append(out, user_leave_dto.Leave{Date: d, Note: n})
	}
	return out, nil
}
func (f *fakeRepo) Insert(_ context.Context, _ string, dates []string, note string) ([]string, error) {
	var added []string
	for _, d := range dates {
		if _, ok := f.dates[d]; !ok {
			f.dates[d] = note
			added = append(added, d)
		}
	}
	return added, nil
}
func (f *fakeRepo) Delete(_ context.Context, _ string, date string) (bool, error) {
	_, ok := f.dates[date]
	delete(f.dates, date)
	return ok, nil
}
func (f *fakeRepo) DatesByEmail(context.Context, string) ([]string, error) {
	f.reads++
	var out []string
	for d := range f.dates {
		out = append(out, d)
	}
	return out, nil
}

var wib = time.FixedZone("WIB", 7*3600)

// Friday 2026-11-20 10:00 WIB.
func fixedNow() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, wib) }

func newSvc(repo *fakeRepo, recalc chan string) *userLeaveServiceImpl {
	global := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-24": true}}
	s := NewUserLeaveService(repo, global, func(email string) {
		if recalc != nil {
			recalc <- email
		}
	}).(*userLeaveServiceImpl)
	s.now = fixedNow
	return s
}

func TestAdd_SkipsWeekendAndHolidayAndReportsThem(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(repo, nil)
	// Sat 21 → Tue 24: 21,22 weekend; 23 working; 24 national holiday.
	res, err := s.Add(context.Background(), "dev@esb.co.id", "2026-11-21", "2026-11-24", "Cuti")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "2026-11-23" {
		t.Fatalf("Added = %v", res.Added)
	}
	if len(res.Skipped) != 3 {
		t.Fatalf("Skipped = %v", res.Skipped)
	}
	if res.Recalculating {
		t.Fatal("future-only leave must not recalculate")
	}
}

func TestAdd_PastDateTriggersRecalc(t *testing.T) {
	recalc := make(chan string, 1)
	s := newSvc(newFakeRepo(), recalc)
	res, err := s.Add(context.Background(), "dev@esb.co.id", "2026-11-18", "", "")
	if err != nil || !res.Recalculating {
		t.Fatalf("Add = %+v, %v", res, err)
	}
	select {
	case got := <-recalc:
		if got != "dev@esb.co.id" {
			t.Fatalf("recalc for %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("recalc hook not called")
	}
}

func TestAdd_Validation(t *testing.T) {
	s := newSvc(newFakeRepo(), nil)
	ctx := context.Background()
	cases := []struct{ from, to, note string; want error }{
		{"2026-11-25", "2026-11-23", "", ErrInvalidRange},  // to < from
		{"2026-11-23", "2026-12-25", "", ErrInvalidRange},  // > 31 days
		{"2026-09-01", "", "", ErrInvalidRange},            // > 60 days back
		{"bad", "", "", ErrInvalidRange},                   // format
		{"2026-11-23", "", string(make([]rune, 201)), ErrInvalidRange},
		{"2026-11-21", "2026-11-22", "", ErrNoWorkingDay},  // weekend only
	}
	for _, c := range cases {
		if _, err := s.Add(ctx, "dev@esb.co.id", c.from, c.to, c.note); !errors.Is(err, c.want) {
			t.Errorf("Add(%s,%s) err = %v, want %v", c.from, c.to, err, c.want)
		}
	}
}

func TestRemove_UnknownDateIsNotFound(t *testing.T) {
	s := newSvc(newFakeRepo(), nil)
	if _, err := s.Remove(context.Background(), "dev@esb.co.id", "2026-11-18"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestForEmail_MergesLeaveAndCachesUntilChange(t *testing.T) {
	repo := newFakeRepo()
	repo.dates["2026-11-18"] = ""
	s := newSvc(repo, nil)
	ctx := context.Background()

	cal := s.ForEmail(ctx, "dev@esb.co.id")
	if !cal.Holidays["2026-11-18"] || !cal.Holidays["2026-11-24"] {
		t.Fatalf("calendar = %v", cal.Holidays)
	}
	s.ForEmail(ctx, "dev@esb.co.id")
	if repo.reads != 1 {
		t.Fatalf("reads = %d, want cached", repo.reads)
	}
	if _, err := s.Add(ctx, "dev@esb.co.id", "2026-11-23", "", ""); err != nil {
		t.Fatal(err)
	}
	if !s.ForEmail(ctx, "dev@esb.co.id").Holidays["2026-11-23"] {
		t.Fatal("Add must invalidate the cache")
	}
	if s.ForEmail(ctx, "").Holidays["2026-11-18"] {
		t.Fatal("empty email must get the global calendar only")
	}
}
```

- [ ] **Step 8: Jalankan → FAIL** — `go test ./modules/user_leave/user_leave_service/`

- [ ] **Step 9: Service**

`user_leave_service.go`:
```go
package user_leave_service

import (
	"context"
	"errors"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

var (
	ErrInvalidRange = errors.New("user_leave: invalid date range")
	ErrNoWorkingDay = errors.New("user_leave: no working day in range")
	ErrNotFound     = errors.New("user_leave: leave not found")
)

// UserLeaveService manages a user's own leave days (Tech Spec "Papan Tiket
// & Cuti") and is the per-user CalendarSource for every time calculation.
type UserLeaveService interface {
	business_hours_dto.CalendarSource
	List(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
	Add(ctx context.Context, email, from, to, note string) (user_leave_dto.AddResult, error)
	Remove(ctx context.Context, email, date string) (recalculating bool, err error)
}
```

`user_leave_service_impl.go`:
```go
package user_leave_service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_repository"
)

const (
	dateLayout   = "2006-01-02"
	maxRangeDays = 31
	maxPastDays  = 60
	maxNoteRunes = 200
	cacheTTL     = 5 * time.Minute
)

var wib = time.FixedZone("WIB", 7*3600)

type cachedCalendar struct {
	cal     business_hours_dto.HolidayCalendar
	expires time.Time
}

type userLeaveServiceImpl struct {
	repo         user_leave_repository.UserLeaveRepository
	global       business_hours_dto.HolidayCalendar
	onPastChange func(email string)
	now          func() time.Time

	mu    sync.Mutex
	cache map[string]cachedCalendar
}

// NewUserLeaveService: onPastChange runs (in its own goroutine) whenever a
// change touches today or an earlier day, so tracked time can be rebuilt.
func NewUserLeaveService(repo user_leave_repository.UserLeaveRepository, global business_hours_dto.HolidayCalendar, onPastChange func(email string)) UserLeaveService {
	return &userLeaveServiceImpl{
		repo: repo, global: global, onPastChange: onPastChange,
		now: time.Now, cache: map[string]cachedCalendar{},
	}
}

func (s *userLeaveServiceImpl) today() time.Time {
	n := s.now().In(wib)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, wib)
}

func (s *userLeaveServiceImpl) List(ctx context.Context, email string) ([]user_leave_dto.Leave, error) {
	return s.repo.ListByEmail(ctx, email)
}

func (s *userLeaveServiceImpl) Add(ctx context.Context, email, from, to, note string) (user_leave_dto.AddResult, error) {
	if to == "" {
		to = from
	}
	start, err1 := time.ParseInLocation(dateLayout, from, wib)
	end, err2 := time.ParseInLocation(dateLayout, to, wib)
	if err1 != nil || err2 != nil {
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: date format", ErrInvalidRange)
	}
	today := s.today()
	switch {
	case end.Before(start):
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: end before start", ErrInvalidRange)
	case int(end.Sub(start).Hours()/24)+1 > maxRangeDays:
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: more than %d days", ErrInvalidRange, maxRangeDays)
	case start.Before(today.AddDate(0, 0, -maxPastDays)):
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: more than %d days ago", ErrInvalidRange, maxPastDays)
	case utf8.RuneCountInString(note) > maxNoteRunes:
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: note too long", ErrInvalidRange)
	}

	res := user_leave_dto.AddResult{Added: []string{}, Skipped: []string{}}
	var working []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday || s.global.IsHoliday(d) {
			res.Skipped = append(res.Skipped, d.Format(dateLayout))
			continue
		}
		working = append(working, d.Format(dateLayout))
	}
	if len(working) == 0 {
		return user_leave_dto.AddResult{}, ErrNoWorkingDay
	}

	added, err := s.repo.Insert(ctx, email, working, note)
	if err != nil {
		return user_leave_dto.AddResult{}, err
	}
	if added != nil {
		res.Added = added
	}
	s.invalidate(email)
	if len(added) > 0 && !start.After(today) {
		res.Recalculating = s.triggerRecalc(email)
	}
	return res, nil
}

func (s *userLeaveServiceImpl) Remove(ctx context.Context, email, date string) (bool, error) {
	day, err := time.ParseInLocation(dateLayout, date, wib)
	if err != nil {
		return false, fmt.Errorf("%w: date format", ErrInvalidRange)
	}
	ok, err := s.repo.Delete(ctx, email, date)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	s.invalidate(email)
	if !day.After(s.today()) {
		return s.triggerRecalc(email), nil
	}
	return false, nil
}

func (s *userLeaveServiceImpl) triggerRecalc(email string) bool {
	if s.onPastChange == nil {
		return false
	}
	go s.onPastChange(email)
	return true
}

func (s *userLeaveServiceImpl) invalidate(email string) {
	s.mu.Lock()
	delete(s.cache, email)
	s.mu.Unlock()
}

// ForEmail = national holidays ∪ the user's leave, cached for cacheTTL. A
// DB error falls back to the national holidays so reminders never stall.
func (s *userLeaveServiceImpl) ForEmail(ctx context.Context, email string) business_hours_dto.HolidayCalendar {
	if email == "" {
		return s.global
	}
	s.mu.Lock()
	if c, ok := s.cache[email]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		return c.cal
	}
	s.mu.Unlock()

	dates, err := s.repo.DatesByEmail(ctx, email)
	if err != nil {
		log.Printf("user_leave: leave of %s unavailable, using holidays only: %v", email, err)
		return s.global
	}
	cal := s.global
	if len(dates) > 0 {
		cal = s.global.With(dates)
	}
	s.mu.Lock()
	s.cache[email] = cachedCalendar{cal: cal, expires: s.now().Add(cacheTTL)}
	s.mu.Unlock()
	return cal
}
```

- [ ] **Step 10: Jalankan → PASS** — `go test ./modules/user_leave/...`

- [ ] **Step 11: Wiring `main.go`**

Setelah `holidayCalendar` dimuat (baris ±115) dan sebelum service pertama yang memakainya (chat_interactive, ±139):
```go
	// Per-user calendar (Tech Spec "Papan Tiket & Cuti"): holidays + leave.
	// recalcLeave is set once sprintBackfillSvc exists (it is built later).
	var recalcLeave func(email string)
	userLeaveSvc := user_leave_service.NewUserLeaveService(
		user_leave_repository.NewUserLeaveRepository(db), holidayCalendar,
		func(email string) {
			if recalcLeave != nil {
				recalcLeave(email)
			}
		},
	)
```
Ganti argumen `holidayCalendar` dengan `userLeaveSvc` di keenam constructor (baris 139, 162, 167, 175, 215, 219). Tambahkan import `nudge/modules/user_leave/user_leave_repository` dan `nudge/modules/user_leave/user_leave_service`.

- [ ] **Step 12: Build & test**

Run: `go build ./... && go vet ./... && go test ./...`

---

### Task 3: Recalc session sprint aktif dari changelog

**Files:**
- Modify: `modules/issue_session/issue_session_repository/session_backfill_repository.go`
- Modify: `modules/issue_session/issue_session_repository/session_backfill_repository_test.go`
- Modify: `modules/sprint_backfill/sprint_backfill_service/sprint_backfill_service.go` (interface), `sprint_backfill_service_impl.go`, `sprint_backfill_service_test.go`
- Modify: `main.go` (set `recalcLeave`)

**Interfaces:**
- Consumes: `CalendarSource` (Task 1), `recalcLeave` hook (Task 2).
- Produces: `SessionBackfillRepository.UpdateAccumulatedSeconds(ctx, issueKey string, seconds int64) error`; `SprintBackfillService.RecalculateForEmail(ctx context.Context, email string) (int, error)`; const `issue_session_repository.LeaveRecalcEditedBy = "leave-recalc"`.

- [ ] **Step 1: Test repo** — tambahkan ke `session_backfill_repository_test.go`:

```go
func TestSessionBackfill_UpdateAccumulatedSecondsKeepsActiveState(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewSessionBackfillRepository(db)
	ctx := context.Background()
	const key = "CORE-990002"
	db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key)
	t.Cleanup(func() { db.Exec(`DELETE FROM tr_issuesession WHERE issueKey = ?`, key) })
	if _, err := db.Exec(`INSERT INTO tr_issuesession (issueKey, projectKey, storyPoints, accumulatedSeconds, currentSessionStartedDate, flagActive, assigneeEmail, createdBy, createdDate)
		VALUES (?, 'CORE', 2, 50000, '2026-11-20 08:30:00', 1, 'dev@esb.co.id', 'poller', CURRENT_TIMESTAMP)`, key); err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateAccumulatedSeconds(ctx, key, 21600); err != nil {
		t.Fatal(err)
	}
	var acc int64
	var active bool
	var startedNull bool
	var editedBy string
	if err := db.QueryRow(`SELECT accumulatedSeconds, flagActive, currentSessionStartedDate IS NULL, editedBy FROM tr_issuesession WHERE issueKey = ?`, key).
		Scan(&acc, &active, &startedNull, &editedBy); err != nil {
		t.Fatal(err)
	}
	if acc != 21600 || !active || startedNull || editedBy != LeaveRecalcEditedBy {
		t.Fatalf("got acc=%d active=%v startedNull=%v editedBy=%q", acc, active, startedNull, editedBy)
	}
}
```

- [ ] **Step 2: FAIL** — `go test ./modules/issue_session/issue_session_repository/ -run UpdateAccumulated`

- [ ] **Step 3: Implementasi repo** — tambahkan ke interface dan impl:

```go
	// UpdateAccumulatedSeconds rewrites the time of past runs after a leave
	// change (Tech Spec "Papan Tiket & Cuti", AC4.2); the running period and
	// flagActive are left as they are.
	UpdateAccumulatedSeconds(ctx context.Context, issueKey string, seconds int64) error
```
```go
// LeaveRecalcEditedBy marks sessions rewritten by the leave recalculation.
const LeaveRecalcEditedBy = "leave-recalc"

func (r *sessionBackfillRepositoryImpl) UpdateAccumulatedSeconds(ctx context.Context, issueKey string, seconds int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tr_issuesession
		SET accumulatedSeconds = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP
		WHERE issueKey = ?`, seconds, LeaveRecalcEditedBy, issueKey)
	return err
}
```

- [ ] **Step 4: PASS** (repo test).

- [ ] **Step 5: Test service** — di `sprint_backfill_service_test.go`, perluas fake `SessionBackfillRepository` yang sudah ada dengan:

```go
func (f *fakeSessions) UpdateAccumulatedSeconds(_ context.Context, key string, seconds int64) error {
	if f.updated == nil {
		f.updated = map[string]int64{}
	}
	f.updated[key] = seconds
	return nil
}
```
(tambahkan field `updated map[string]int64`; sesuaikan nama tipe fake dengan yang ada di file.) Lalu:

```go
func TestRecalculateForEmail_ExcludesLeaveDayFromClosedRuns(t *testing.T) {
	// CORE-1: Code Process Mon 16 Nov 08:30 → To Do Wed 18 Nov 17:30 (3 full
	// days = 3 × 8h). The user then records leave on Tue 17 Nov.
	// Expect 2 days = 57600s. Build the issue/changelog with the helpers
	// already used by the backfill tests in this file (active sprint +
	// story points + a "Code Process" → "To Do" history).
	...
	sessions := &fakeSessions{existing: map[string]bool{"CORE-1": true}}
	leave := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-17": true}}
	svc := NewSprintBackfillService(jira, users, nil, sessions, business_hours_service.NewBusinessHoursService(), leave)

	n, err := svc.RecalculateForEmail(context.Background(), "dev@esb.co.id")
	if err != nil || n != 1 {
		t.Fatalf("RecalculateForEmail = %d, %v", n, err)
	}
	if got := sessions.updated["CORE-1"]; got != 2*8*3600 {
		t.Fatalf("accumulated = %d, want %d", got, 2*8*3600)
	}
}

func TestRecalculateForEmail_SkipsTicketsWithoutSession(t *testing.T) {
	// same setup, sessions.existing empty → updated stays empty, n == 0
}
```
Isi `...` dengan pola fixture yang sudah dipakai test backfill di file ini (lihat test `RunForEmail` yang ada untuk cara membuat `jira` fake, `users` fake, issue sprint aktif dan changelog). Jam kerja 08:30–17:30 dengan istirahat 12–13 = 8 jam/hari. Cek nama constructor business hours di `business_hours_service_impl.go` (mis. `NewBusinessHoursService`).

- [ ] **Step 6: FAIL** — `go test ./modules/sprint_backfill/...`

- [ ] **Step 7: Implementasi service**

Interface (`sprint_backfill_service.go`) tambah:
```go
	// RecalculateForEmail rewrites accumulatedSeconds of email's existing
	// active-sprint sessions from Jira's changelog with their current
	// calendar (holidays + leave) — Tech Spec "Papan Tiket & Cuti", AC4.2.
	// Returns how many sessions were rewritten.
	RecalculateForEmail(ctx context.Context, email string) (int, error)
```

Impl: pecah pengumpulan tiket di `runForUser` menjadi helper, lalu tambahkan recalc dengan mutex per email:

```go
// userIssues = the user's active-sprint tickets plus their own subtasks /
// linked items (AC3.1 of the backfill spec).
func (s *sprintBackfillServiceImpl) userIssues(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential) ([]jira_client_dto.Issue, error) {
	issues, err := s.jira.SearchActiveSprintIssuesByAssignee(ctx, cred, user.JiraAccountID)
	if err != nil {
		return nil, fmt.Errorf("active sprint search: %w", err)
	}
	seen := make(map[string]bool, len(issues))
	for _, issue := range issues {
		seen[issue.Key] = true
	}
	var relatedKeys []string
	for _, issue := range issues {
		for _, rel := range issue.Relations() {
			if !seen[rel.Key] {
				seen[rel.Key] = true
				relatedKeys = append(relatedKeys, rel.Key)
			}
		}
	}
	if len(relatedKeys) > 0 {
		related, err := s.jira.SearchIssuesByKeysForAssignee(ctx, cred, relatedKeys, user.JiraAccountID)
		if err != nil {
			log.Printf("sprint_backfill: related items of %s skipped: %v", user.Email, err)
		} else {
			issues = append(issues, related...)
		}
	}
	return issues, nil
}
```
`runForUser` jadi:
```go
func (s *sprintBackfillServiceImpl) runForUser(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential) (int, error) {
	issues, err := s.userIssues(ctx, user, cred)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, issue := range issues {
		ok, err := s.backfillIssue(ctx, user, cred, issue)
		if err != nil {
			log.Printf("sprint_backfill: %s skipped: %v", issue.Key, err)
			continue
		}
		if ok {
			added++
		}
	}
	return added, nil
}
```
Tambahkan field `recalcLocks sync.Map` pada struct dan:
```go
func (s *sprintBackfillServiceImpl) RecalculateForEmail(ctx context.Context, email string) (int, error) {
	lock, _ := s.recalcLocks.LoadOrStore(email, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	user, found, err := s.users.FindUserByEmail(ctx, email)
	if err != nil || !found || user == nil {
		return 0, err
	}
	var cred *jira_client_dto.Credential
	if s.credentials != nil {
		if c, err := s.credentials.ResolveCredential(ctx, email); err == nil {
			cred = c
		}
	}
	issues, err := s.userIssues(ctx, *user, cred)
	if err != nil {
		return 0, err
	}
	cal := s.calendar.ForEmail(ctx, email)
	elapsed := func(start, end time.Time) int64 { return s.businessHours.ElapsedBusinessSeconds(start, end, cal) }

	updated := 0
	for _, issue := range issues {
		if _, inSprint := issue.Fields.ActiveSprint(); !inSprint {
			continue // AC4.3
		}
		exists, err := s.sessions.SessionExists(ctx, issue.Key)
		if err != nil || !exists {
			continue
		}
		histories, err := s.jira.GetChangelog(ctx, cred, issue.Key)
		if err != nil {
			log.Printf("sprint_backfill: recalc %s skipped: %v", issue.Key, err)
			continue
		}
		seconds, found := jira_client_dto.TrackedSeconds(histories, jira_client_dto.RuleForIssue(issue.Key), elapsed)
		if !found {
			continue
		}
		if err := s.sessions.UpdateAccumulatedSeconds(ctx, issue.Key, seconds); err != nil {
			log.Printf("sprint_backfill: recalc %s failed: %v", issue.Key, err)
			continue
		}
		updated++
	}
	return updated, nil
}
```
Import `sync`.

- [ ] **Step 8: PASS** — `go test ./modules/sprint_backfill/... ./modules/issue_session/...`

- [ ] **Step 9: Sambungkan hook di `main.go`** setelah `sprintBackfillSvc` dibuat:
```go
	recalcLeave = func(email string) {
		n, err := sprintBackfillSvc.RecalculateForEmail(context.Background(), email)
		if err != nil {
			log.Printf("main: leave recalculation of %s failed: %v", email, err)
			return
		}
		log.Printf("main: leave recalculation of %s rewrote %d session(s)", email, n)
	}
```

- [ ] **Step 10:** `go build ./... && go vet ./... && go test ./...`

---

### Task 4: Endpoint `/leaves`

**Files:**
- Create: `modules/user_leave/user_leave_handler/user_leave_handler.go`, `user_leave_handler_test.go`
- Create: `modules/user_leave/router.go`
- Modify: `router.go` (root) — `SetupRouter` menerima handler baru & mount; `main.go` — buat handler, oper ke `SetupRouter`

**Interfaces:**
- Consumes: `user_leave_service.UserLeaveService`, errors Task 2. User sesi: pakai helper yang sama dengan handler dashboard (`currentUser(c)` ada di package `user_dashboard_handler`; cek apakah diekspor — bila tidak, baca email dengan cara yang sama seperti `currentUser` membaca dari `c` (lihat definisinya) dan duplikasi 3 baris itu di handler baru).
- Produces: `user_leave_handler.NewUserLeaveHandler(svc user_leave_service.UserLeaveService) *UserLeaveHandler` dengan `List`, `Add`, `Remove`; `user_leave.Router(api *gin.RouterGroup, h *user_leave_handler.UserLeaveHandler)`.

- [ ] **Step 1: Test handler** (gin test mode, fake service, user disuntik ke context dengan cara yang sama seperti `user_dashboard_handler_test.go` menyiapkan user sesi):

```go
func TestAdd_MapsErrorsToStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, http.StatusOK},
		{user_leave_service.ErrInvalidRange, http.StatusBadRequest},
		{user_leave_service.ErrNoWorkingDay, http.StatusUnprocessableEntity},
		{errors.New("db down"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		svc := &fakeLeaveService{addErr: c.err}
		w := perform(NewUserLeaveHandler(svc), http.MethodPost, "/leaves", `{"from":"2026-11-23","to":"2026-11-24","note":"x"}`)
		if w.Code != c.want {
			t.Errorf("err %v → %d, want %d", c.err, w.Code, c.want)
		}
		if c.err == nil && svc.gotEmail != "dev@esb.co.id" {
			t.Errorf("email = %q, want session user", svc.gotEmail)
		}
	}
}

func TestRemove_NotFoundIs404(t *testing.T) {
	svc := &fakeLeaveService{removeErr: user_leave_service.ErrNotFound}
	w := perform(NewUserLeaveHandler(svc), http.MethodDelete, "/leaves/2026-11-18", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestList_EmptyIsArray(t *testing.T) {
	w := perform(NewUserLeaveHandler(&fakeLeaveService{}), http.MethodGet, "/leaves", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"leaves":[]`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
```
`perform` membuat `gin.New()`, middleware yang men-set user sesi `dev@esb.co.id`, `user_leave.Router(r.Group(""), h)` — atau daftarkan route langsung — lalu `httptest`.

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implementasi**

```go
package user_leave_handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_service"
)

type UserLeaveHandler struct{ svc user_leave_service.UserLeaveService }

func NewUserLeaveHandler(svc user_leave_service.UserLeaveService) *UserLeaveHandler {
	return &UserLeaveHandler{svc: svc}
}

type addRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

func (h *UserLeaveHandler) List(c *gin.Context) {
	leaves, err := h.svc.List(c.Request.Context(), sessionEmail(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat cuti"})
		return
	}
	if leaves == nil {
		leaves = []user_leave_dto.Leave{}
	}
	c.JSON(http.StatusOK, gin.H{"leaves": leaves})
}

func (h *UserLeaveHandler) Add(c *gin.Context) {
	var req addRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.From == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tanggal mulai wajib diisi"})
		return
	}
	res, err := h.svc.Add(c.Request.Context(), sessionEmail(c), req.From, req.To, req.Note)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, res)
	case errors.Is(err, user_leave_service.ErrInvalidRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rentang tanggal tidak valid (maks 31 hari, paling lama 60 hari ke belakang)."})
	case errors.Is(err, user_leave_service.ErrNoWorkingDay):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Tidak ada hari kerja di rentang ini."})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan cuti"})
	}
}

func (h *UserLeaveHandler) Remove(c *gin.Context) {
	date := c.Param("date")
	recalculating, err := h.svc.Remove(c.Request.Context(), sessionEmail(c), date)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"deleted": date, "recalculating": recalculating})
	case errors.Is(err, user_leave_service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "cuti tidak ditemukan"})
	case errors.Is(err, user_leave_service.ErrInvalidRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": "format tanggal salah"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus cuti"})
	}
}
```
`sessionEmail(c)` = salinan dari cara `currentUser(c).Email` membaca user sesi di `user_dashboard_handler` (cek file definisinya; pakai key context yang sama).

`modules/user_leave/router.go`:
```go
package user_leave

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/user_leave/user_leave_handler"
)

// Router mounts the leave endpoints on the authenticated dashboard API group.
func Router(api *gin.RouterGroup, h *user_leave_handler.UserLeaveHandler) {
	api.GET("/leaves", h.List)
	api.POST("/leaves", h.Add)
	api.DELETE("/leaves/:date", h.Remove)
}
```
Root `router.go`: tambah parameter `userLeaveHandler *user_leave_handler.UserLeaveHandler` pada `SetupRouter` dan `user_leave.Router(dashboardAPI, userLeaveHandler)` setelah `sprint_report.Router(...)` (baris 61). Perbarui semua pemanggil `SetupRouter` (main.go + test router bila ada: `grep -rn "SetupRouter(" --include=*.go`).

- [ ] **Step 4: PASS** — `go test ./modules/user_leave/... && go build ./...`

---

### Task 5: Transisi tiket berdasarkan workflow Jira

**Files:**
- Modify: `modules/jira_client/jira_client_dto/jira_client_dto.go:200` — `Transition.Name`
- Modify: `modules/user_dashboard/user_dashboard_dto/*.go` — `TransitionOption`, `TicketTransitions`
- Modify: `modules/user_dashboard/user_dashboard_service/user_dashboard_service.go`, `user_dashboard_service_impl.go`, `user_dashboard_service_test.go`
- Modify: `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler.go`, `user_dashboard_handler_impl.go:219-237`, `user_dashboard_handler_test.go`
- Modify: `modules/user_dashboard/router.go`

**Interfaces:**
- Produces:
  - `user_dashboard_dto.TransitionOption{ID string "id"; Name string "name"; ToStatus string "toStatus"}`
  - `user_dashboard_dto.TicketTransitions{IssueKey "issueKey"; CurrentStatus "currentStatus"; Transitions []TransitionOption "transitions"}`
  - `UserDashboardService.ListTransitions(ctx, email, issueKey string) (user_dashboard_dto.TicketTransitions, error)`
  - `UserDashboardService.Transition(ctx, email, issueKey, transitionID string) (newStatus string, err error)`
  - `user_dashboard_service.ErrNotOwnTicket`, `ErrTransitionUnavailable`
  - `UserDashboardHandler.GetTransitions(c *gin.Context)`; route `GET /tickets/:issueKey/transitions`

- [ ] **Step 1: DTO Jira**

```go
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"` // workflow wording, e.g. "Start Development"
	To   Status `json:"to"`
}
```

- [ ] **Step 2: Test service** (di `user_dashboard_service_test.go`; `fakeJiraClientForDashboard` sudah punya `GetTransitionsAs`/`TransitionIssueAs` — beri field `transitions []jira_client_dto.Transition`, `transitioned string` bila belum ada; fake agile sudah mengembalikan issue sprint aktif):

```go
func TestListTransitions_ReturnsJiraWorkflowOptionsForOwnSprintTicket(t *testing.T) {
	// agile fake returns OMS-1 (status "To Do") for the user; Jira fake offers
	// only {ID:"11", Name:"Start Development", To:{Name:"Code Process"}}.
	got, err := svc.ListTransitions(ctx, "dev@esb.co.id", "OMS-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentStatus != "To Do" || len(got.Transitions) != 1 ||
		got.Transitions[0] != (user_dashboard_dto.TransitionOption{ID: "11", Name: "Start Development", ToStatus: "Code Process"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestListTransitions_RejectsTicketOutsideOwnActiveSprint(t *testing.T) {
	_, err := svc.ListTransitions(ctx, "dev@esb.co.id", "OMS-999")
	if !errors.Is(err, ErrNotOwnTicket) {
		t.Fatalf("err = %v", err)
	}
}

func TestTransition_RejectsIDNotOfferedNow(t *testing.T) {
	_, err := svc.Transition(ctx, "dev@esb.co.id", "OMS-1", "31")
	if !errors.Is(err, ErrTransitionUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestTransition_ExecutesAndReturnsNewStatus(t *testing.T) {
	status, err := svc.Transition(ctx, "dev@esb.co.id", "OMS-1", "11")
	if err != nil || status != "Code Process" || jira.transitioned != "OMS-1:11" {
		t.Fatalf("status=%q err=%v transitioned=%q", status, err, jira.transitioned)
	}
}
```
Bangun `svc` dengan helper constructor yang sudah dipakai test file ini.

- [ ] **Step 3: FAIL**

- [ ] **Step 4: Implementasi service**

```go
var (
	ErrNotOwnTicket          = errors.New("user_dashboard: not the caller's active-sprint ticket")
	ErrTransitionUnavailable = errors.New("user_dashboard: transition not available from the current status")
)

// ownSprintTicket finds issueKey among email's active-sprint tickets
// (Tech Spec "Papan Tiket & Cuti", AC1.6).
func (s *userDashboardServiceImpl) ownSprintTicket(ctx context.Context, email, issueKey string) (jira_client_dto.Issue, *jira_client_dto.Credential, error) {
	q, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return jira_client_dto.Issue{}, nil, err
	}
	for _, issue := range q.issues {
		if strings.EqualFold(issue.Key, issueKey) {
			return issue, q.cred, nil
		}
	}
	return jira_client_dto.Issue{}, nil, ErrNotOwnTicket
}

func (s *userDashboardServiceImpl) ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error) {
	issue, cred, err := s.ownSprintTicket(ctx, email, issueKey)
	if err != nil {
		return user_dashboard_dto.TicketTransitions{}, err
	}
	transitions, err := s.jiraClient.GetTransitionsAs(ctx, cred, issue.Key)
	if err != nil {
		return user_dashboard_dto.TicketTransitions{}, err
	}
	out := user_dashboard_dto.TicketTransitions{
		IssueKey: issue.Key, CurrentStatus: issue.Fields.Status.Name,
		Transitions: make([]user_dashboard_dto.TransitionOption, 0, len(transitions)),
	}
	for _, tr := range transitions {
		out.Transitions = append(out.Transitions, user_dashboard_dto.TransitionOption{ID: tr.ID, Name: tr.Name, ToStatus: tr.To.Name})
	}
	return out, nil
}

func (s *userDashboardServiceImpl) Transition(ctx context.Context, email, issueKey, transitionID string) (string, error) {
	issue, cred, err := s.ownSprintTicket(ctx, email, issueKey)
	if err != nil {
		return "", err
	}
	transitions, err := s.jiraClient.GetTransitionsAs(ctx, cred, issue.Key)
	if err != nil {
		return "", err
	}
	for _, tr := range transitions {
		if tr.ID == transitionID {
			if err := s.jiraClient.TransitionIssueAs(ctx, cred, issue.Key, tr.ID); err != nil {
				return "", err
			}
			return tr.To.Name, nil
		}
	}
	return "", ErrTransitionUnavailable
}
```
Tambahkan kedua method ke interface `UserDashboardService` (dengan komentar), import `errors`.

- [ ] **Step 5: PASS** — `go test ./modules/user_dashboard/user_dashboard_service/`

- [ ] **Step 6: Test handler** (perbarui fake `UserDashboardService` di `user_dashboard_handler_test.go` dengan dua method baru):

```go
func TestTransitionTicket_WithTransitionIDUsesService(t *testing.T) {
	// body {"transitionId":"11"} → svc.Transition called; 200 {"status":"Code Process"}
}
func TestTransitionTicket_LegacyTargetStillUsesRouteCommand(t *testing.T) {
	// existing TestTransitionTicket_DelegatesToRouteCommand must still pass unchanged
}
func TestTransitionTicket_MapsErrors(t *testing.T) {
	// ErrNotOwnTicket → 403, ErrTransitionUnavailable → 422, other → 502
}
func TestGetTransitions_MapsErrorsAndReturnsList(t *testing.T) {
	// ok → 200 JSON TicketTransitions; ErrNotOwnTicket → 403; other → 502
}
```
Tulis tiap test seperti `TestTransitionTicket_DelegatesToRouteCommand` yang sudah ada (pola request & user sesi sama), dengan fake mengembalikan error/hasil di atas dan assert `w.Code` + isi body.

- [ ] **Step 7: FAIL**

- [ ] **Step 8: Implementasi handler**

```go
type transitionRequest struct {
	Target       string `json:"target"`
	TransitionID string `json:"transitionId"`
}

func transitionErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, user_dashboard_service.ErrNotOwnTicket):
		return http.StatusForbidden, "Tiket ini bukan tiket kamu di sprint aktif."
	case errors.Is(err, user_dashboard_service.ErrTransitionUnavailable):
		return http.StatusUnprocessableEntity, "Transisi tidak tersedia untuk status sekarang."
	default:
		return http.StatusBadGateway, "Jira sedang tidak bisa dihubungi, coba lagi."
	}
}

func (h *userDashboardHandlerImpl) GetTransitions(c *gin.Context) {
	res, err := h.dashboardSvc.ListTransitions(c.Request.Context(), currentUser(c).Email, c.Param("issueKey"))
	if err != nil {
		code, msg := transitionErrorStatus(err)
		c.JSON(code, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *userDashboardHandlerImpl) TransitionTicket(c *gin.Context) {
	issueKey := c.Param("issueKey")
	var req transitionRequest
	if err := c.ShouldBindJSON(&req); err != nil || (req.TransitionID == "" && req.Target == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.TransitionID != "" {
		status, err := h.dashboardSvc.Transition(c.Request.Context(), currentUser(c).Email, issueKey, req.TransitionID)
		if err != nil {
			code, msg := transitionErrorStatus(err)
			c.JSON(code, gin.H{"error": msg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": issueKey + " dipindah ke " + status + ".", "status": status})
		return
	}
	// Legacy body (Beranda buttons): same path as the bot's /n-geser.
	command := "/n-geser<" + issueKey + ">:" + req.Target
	message := h.chatInteractive.RouteCommand(c.Request.Context(), currentUser(c).Email, command)
	c.JSON(http.StatusOK, gin.H{"message": message})
}
```
Tambahkan `GetTransitions(c *gin.Context)` ke interface `UserDashboardHandler`, dan `api.GET("/tickets/:issueKey/transitions", handler.GetTransitions)` di `modules/user_dashboard/router.go`.

> Cek: bila test lama mengirim body `{}` dan mengharapkan RouteCommand dipanggil, sesuaikan — body tanpa `target` maupun `transitionId` kini 400 (spec API 400).

- [ ] **Step 9:** `go build ./... && go vet ./... && go test ./...`

---

### Task 6: Frontend — Papan Tiket

**Files:**
- Modify: `frontend/src/app/core/services/dashboard.service.ts`
- Create: `frontend/src/app/features/dashboard/board-page/board-page.component.ts`, `.html`, `.spec.ts`
- Modify: `frontend/src/app/app.routes.ts`, `frontend/src/app/layouts/dashboard-shell/dashboard-shell.component.html`, `frontend/src/app/shared/ui/icon.component.ts`, `frontend/src/styles.scss`

**Interfaces:**
- Consumes: `GET /board` (`BoardResponse`), `GET /tickets/:key/transitions`, `POST /tickets/:key/transition {transitionId}`.
- Produces: `DashboardService.getTransitions(key): Observable<TicketTransitions>`, `DashboardService.moveTicket(key, transitionId): Observable<{message: string; status: string}>`; route `/dashboard/board`.

- [ ] **Step 1: Service API** — tambahkan ke `dashboard.service.ts`:

```ts
export interface TransitionOption {
  id: string;
  name: string;
  toStatus: string;
}

export interface TicketTransitions {
  issueKey: string;
  currentStatus: string;
  transitions: TransitionOption[];
}
```
di dalam class:
```ts
  /** Moves allowed by the ticket's Jira workflow right now (CORE/OMS differ). */
  getTransitions(issueKey: string): Observable<TicketTransitions> {
    return this.http.get<TicketTransitions>(`${this.base}/tickets/${encodeURIComponent(issueKey)}/transitions`);
  }

  moveTicket(issueKey: string, transitionId: string): Observable<{ message: string; status: string }> {
    return this.http.post<{ message: string; status: string }>(
      `${this.base}/tickets/${encodeURIComponent(issueKey)}/transition`,
      { transitionId },
    );
  }
```

- [ ] **Step 2: Spec komponen (gagal dulu)**

```ts
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { BoardPageComponent } from './board-page.component';

describe('BoardPageComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [BoardPageComponent],
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  function render() {
    const fixture = TestBed.createComponent(BoardPageComponent);
    fixture.detectChanges();
    http.match((r) => r.url.endsWith('/api/dashboard/boards')).forEach((r) => r.flush({ boards: [] }));
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({
      columns: [
        { status: 'To Do', cards: [{ issueKey: 'OMS-1', summary: 'Lock payments', status: 'To Do', issueURL: 'https://x/browse/OMS-1', boardId: 28, storyPoints: 2 }] },
        { status: 'Code Process', cards: [] },
      ],
    });
    fixture.detectChanges();
    return fixture;
  }

  it('renders one column per status with its cards', () => {
    const el: HTMLElement = render().nativeElement;
    expect(el.querySelectorAll('[data-column]').length).toBe(2);
    expect(el.querySelector('[data-card="OMS-1"]')?.textContent).toContain('Lock payments');
  });

  it('loads the Jira transitions when the status chip is opened and moves the ticket', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-card="OMS-1"] [data-status-chip]') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({
      issueKey: 'OMS-1', currentStatus: 'To Do',
      transitions: [{ id: '11', name: 'Start Development', toStatus: 'Code Process' }],
    });
    fixture.detectChanges();
    const option = el.querySelector('[data-transition="11"]') as HTMLButtonElement;
    expect(option.textContent).toContain('Code Process');
    option.click();
    const post = http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition'));
    expect(post.request.body).toEqual({ transitionId: '11' });
    post.flush({ message: 'ok', status: 'Code Process' });
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [] });
  });

  it('shows an empty-state when Jira offers no transition', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-card="OMS-1"] [data-status-chip]') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/transitions')).flush({ issueKey: 'OMS-1', currentStatus: 'To Do', transitions: [] });
    fixture.detectChanges();
    expect(el.textContent).toContain('Tidak ada transisi');
  });
});
```

- [ ] **Step 3: FAIL** — `cd frontend && npx ng test --watch=false --include=src/app/features/dashboard/board-page`

- [ ] **Step 4: Komponen**

`board-page.component.ts`:
```ts
import { Component, effect, inject, signal, untracked } from '@angular/core';
import { DashboardService, TicketCard, TransitionOption } from '../../../core/services/dashboard.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

interface Column {
  status: string;
  cards: TicketCard[];
}

type MenuState = { key: string; loading: boolean; error: string | null; options: TransitionOption[] };

/** Papan Tiket (Tech Spec "Papan Tiket & Cuti", US-1): move own sprint tickets along their Jira workflow. */
@Component({
  selector: 'app-board-page',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './board-page.component.html',
})
export class BoardPageComponent {
  private dashboard = inject(DashboardService);
  private boardContext = inject(BoardContextService);
  private toast = inject(ToastService);

  protected columns = signal<Column[]>([]);
  protected loading = signal(true);
  protected error = signal<string | null>(null);
  protected menu = signal<MenuState | null>(null);
  protected moving = signal<string | null>(null);

  constructor() {
    this.boardContext.load();
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => this.load(boardId));
    });
  }

  protected load(boardId: number | null = this.boardContext.selectedId()): void {
    this.loading.set(true);
    this.error.set(null);
    this.dashboard.getBoard(boardId).subscribe({
      next: (res) => {
        this.columns.set((res.columns ?? []).map((c) => ({ status: c.status, cards: c.cards ?? [] })));
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Gagal memuat tiket dari Jira, coba lagi');
        this.loading.set(false);
      },
    });
  }

  protected toggleMenu(card: TicketCard): void {
    if (this.menu()?.key === card.issueKey) {
      this.menu.set(null);
      return;
    }
    this.menu.set({ key: card.issueKey, loading: true, error: null, options: [] });
    this.dashboard.getTransitions(card.issueKey).subscribe({
      next: (res) => this.menu.set({ key: card.issueKey, loading: false, error: null, options: res.transitions ?? [] }),
      error: () => this.menu.set({ key: card.issueKey, loading: false, error: 'Gagal memuat transisi', options: [] }),
    });
  }

  protected move(card: TicketCard, option: TransitionOption): void {
    this.menu.set(null);
    this.moving.set(card.issueKey);
    this.dashboard.moveTicket(card.issueKey, option.id).subscribe({
      next: () => {
        this.moving.set(null);
        this.toast.success('Status dipindah', `${card.issueKey} → ${option.toStatus}`);
        this.load();
      },
      error: (err) => {
        this.moving.set(null);
        this.toast.error('Gagal memindah status', err?.error?.error ?? 'Coba lagi beberapa saat.');
      },
    });
  }
}
```
> Bila `boardContext.load()` sudah dipanggil oleh shell, hapus pemanggilan di constructor dan hapus `http.match(...boards)` di spec — cek `dashboard-shell.component.ts` dulu.

`board-page.component.html`:
```html
@if (loading()) {
  <div class="empty-inline"><span>Memuat tiket…</span></div>
} @else if (error()) {
  <div class="empty-inline"><span>{{ error() }}</span> <button type="button" class="btn btn-soft btn-sm" (click)="load()">Coba lagi</button></div>
} @else if (columns().length === 0) {
  <div class="empty-inline"><span>Belum ada tiket kamu di sprint aktif.</span></div>
} @else {
  <div class="kanban">
    @for (col of columns(); track col.status) {
      <section class="kanban-col" data-column>
        <header class="kanban-col-head"><span>{{ col.status }}</span><span class="sub-count">{{ col.cards.length }}</span></header>
        @for (card of col.cards; track card.issueKey) {
          <article class="kanban-card" [attr.data-card]="card.issueKey" [class.busy]="moving() === card.issueKey">
            <a class="ticket-key" [href]="card.issueURL" target="_blank" rel="noopener">{{ card.issueKey }}</a>
            <p class="kanban-summary">{{ card.summary }}</p>
            @if (card.parentKey) {
              <p class="kanban-parent">↳ {{ card.parentKey }}</p>
            }
            <div class="kanban-foot">
              @if (card.storyPoints) {
                <span class="sp-chip">{{ card.storyPoints }} SP</span>
              }
              <div class="status-menu">
                <button type="button" class="status-chip" data-status-chip [disabled]="moving() === card.issueKey" (click)="toggleMenu(card)">
                  {{ card.status }} <app-icon name="chevron-down" [size]="14" />
                </button>
                @if (menu()?.key === card.issueKey) {
                  <div class="status-dropdown" role="menu">
                    @if (menu()!.loading) {
                      <span class="status-dropdown-note">Memuat transisi…</span>
                    } @else if (menu()!.error) {
                      <span class="status-dropdown-note">{{ menu()!.error }}</span>
                    } @else if (menu()!.options.length === 0) {
                      <span class="status-dropdown-note">Tidak ada transisi</span>
                    } @else {
                      @for (opt of menu()!.options; track opt.id) {
                        <button type="button" role="menuitem" [attr.data-transition]="opt.id" (click)="move(card, opt)">
                          <span>{{ opt.name }}</span>
                          <span class="status-chip">→ {{ opt.toStatus }}</span>
                        </button>
                      }
                    }
                  </div>
                }
              </div>
            </div>
          </article>
        } @empty {
          <p class="kanban-empty">Kosong</p>
        }
      </section>
    }
  </div>
}
```

Route (`app.routes.ts`, children `dashboard`, setelah `''`):
```ts
      {
        path: 'board',
        loadComponent: () => import('./features/dashboard/board-page/board-page.component').then((m) => m.BoardPageComponent),
        title: 'Papan Tiket · Nudge',
        data: { heading: 'Papan Tiket', subheading: 'Pindahkan status tiket sprint aktif kamu sesuai workflow Jira' },
      },
```
Sidebar (`dashboard-shell.component.html`, setelah link Dashboard):
```html
      <a class="nav-item" routerLink="/dashboard/board" queryParamsHandling="preserve" routerLinkActive="active" (click)="closeSidebar()">
        <app-icon name="kanban" />
        <span>Papan Tiket</span>
      </a>
```
Ikon (`icon.component.ts`, map path):
```ts
  kanban: ['M3 3h18v18H3z', 'M9 3v18', 'M15 3v18'],
  calendar: ['M8 2v4', 'M16 2v4', 'M3 10h18', 'M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z'],
```
Gaya (`styles.scss`, pakai token warna yang sudah ada — cek nama variabel di file, mis. `var(--surface)`, `var(--border)`):
```scss
.kanban { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(260px, 1fr); gap: 12px; overflow-x: auto; padding-bottom: 8px; }
.kanban-col { background: var(--surface-muted, var(--surface)); border: 1px solid var(--border); border-radius: 12px; padding: 10px; display: flex; flex-direction: column; gap: 8px; min-height: 120px; }
.kanban-col-head { display: flex; justify-content: space-between; font-weight: 600; font-size: 13px; text-transform: uppercase; letter-spacing: .02em; }
.kanban-card { background: var(--surface); border: 1px solid var(--border); border-radius: 10px; padding: 10px; display: flex; flex-direction: column; gap: 6px; }
.kanban-card.busy { opacity: .6; pointer-events: none; }
.kanban-summary { margin: 0; font-size: 14px; }
.kanban-parent { margin: 0; font-size: 12px; color: var(--text-muted); }
.kanban-foot { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.kanban-empty { margin: 0; font-size: 13px; color: var(--text-muted); text-align: center; }
.status-menu { position: relative; margin-left: auto; }
.status-dropdown { position: absolute; right: 0; top: calc(100% + 4px); z-index: 20; min-width: 240px; background: var(--surface); border: 1px solid var(--border); border-radius: 10px; box-shadow: 0 8px 24px rgba(0,0,0,.12); padding: 4px; display: flex; flex-direction: column; }
.status-dropdown button { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 8px 10px; border: 0; background: none; border-radius: 6px; cursor: pointer; text-align: left; color: inherit; }
.status-dropdown button:hover { background: var(--surface-muted, rgba(0,0,0,.04)); }
.status-dropdown-note { padding: 8px 10px; font-size: 13px; color: var(--text-muted); }
@media (max-width: 640px) { .kanban { grid-auto-flow: row; grid-auto-columns: auto; } }
```
> `.status-chip`, `.sub-count`, `.ticket-key`, `.sp-chip`: pakai yang ada di `styles.scss`; bila `.sp-chip`/`.ticket-key` tidak ada, cek kelas yang dipakai Beranda untuk SP & nomor tiket dan pakai itu.

- [ ] **Step 5: PASS** — spec board-page, lalu `npx ng test --watch=false` (semua) dan `npx ng build`.

---

### Task 7: Frontend — Cuti

**Files:**
- Create: `frontend/src/app/core/services/leave.service.ts`
- Create: `frontend/src/app/features/dashboard/leave/leave.component.ts`, `.html`, `.spec.ts`
- Modify: `app.routes.ts`, `dashboard-shell.component.html`, `styles.scss`

**Interfaces:**
- Consumes: `GET/POST/DELETE /api/dashboard/leaves`.
- Produces: `LeaveService.list()`, `add(from, to, note)`, `remove(date)`; route `/dashboard/leave`.

- [ ] **Step 1: Service**

```ts
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface Leave {
  date: string;
  note: string;
}

export interface AddLeaveResult {
  added: string[];
  skipped: string[];
  recalculating: boolean;
}

/** The session user's own leave days (Tech Spec "Papan Tiket & Cuti", US-2). */
@Injectable({ providedIn: 'root' })
export class LeaveService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/leaves`;

  list(): Observable<{ leaves: Leave[] }> {
    return this.http.get<{ leaves: Leave[] }>(this.base);
  }

  add(from: string, to: string, note: string): Observable<AddLeaveResult> {
    return this.http.post<AddLeaveResult>(this.base, { from, to, note });
  }

  remove(date: string): Observable<{ deleted: string; recalculating: boolean }> {
    return this.http.delete<{ deleted: string; recalculating: boolean }>(`${this.base}/${date}`);
  }
}
```

- [ ] **Step 2: Spec (gagal dulu)**

```ts
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { LeaveComponent } from './leave.component';

describe('LeaveComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [LeaveComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  function render(leaves = [{ date: '2026-11-18', note: 'Keluarga' }]) {
    const fixture = TestBed.createComponent(LeaveComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/leaves') && r.method === 'GET').flush({ leaves });
    fixture.detectChanges();
    return fixture;
  }

  it('lists the leave days', () => {
    const el: HTMLElement = render().nativeElement;
    expect(el.querySelectorAll('[data-leave]').length).toBe(1);
    expect(el.textContent).toContain('Keluarga');
  });

  it('submits a range and reloads', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    const from = el.querySelector('[data-from]') as HTMLInputElement;
    const to = el.querySelector('[data-to]') as HTMLInputElement;
    from.value = '2026-11-23'; from.dispatchEvent(new Event('input'));
    to.value = '2026-11-24'; to.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ from: '2026-11-23', to: '2026-11-24', note: '' });
    post.flush({ added: ['2026-11-23', '2026-11-24'], skipped: [], recalculating: false });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });

  it('deletes a day after confirmation', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-leave="2026-11-18"] [data-delete]') as HTMLButtonElement).click();
    fixture.detectChanges();
    (el.querySelector('[data-leave="2026-11-18"] [data-confirm]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'DELETE' && r.url.endsWith('/leaves/2026-11-18')).flush({ deleted: '2026-11-18', recalculating: true });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });
});
```

- [ ] **Step 3: FAIL**

- [ ] **Step 4: Komponen**

`leave.component.ts`:
```ts
import { Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { LeaveService, Leave } from '../../../core/services/leave.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

function todayWIB(): string {
  return new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 10);
}

/** Cuti (Tech Spec "Papan Tiket & Cuti", US-2): leave days stop the clock and the DMs. */
@Component({
  selector: 'app-leave',
  standalone: true,
  imports: [IconComponent, DatePipe],
  templateUrl: './leave.component.html',
})
export class LeaveComponent {
  private api = inject(LeaveService);
  private toast = inject(ToastService);

  protected leaves = signal<Leave[]>([]);
  protected loading = signal(true);
  protected saving = signal(false);
  protected confirming = signal<string | null>(null);

  protected from = signal(todayWIB());
  protected to = signal('');
  protected note = signal('');

  protected valid = computed(() => {
    const from = this.from();
    const to = this.to();
    return !!from && (!to || to >= from) && this.note().length <= 200;
  });

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.api.list().subscribe({
      next: (res) => {
        this.leaves.set(res.leaves ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loading.set(false);
        this.toast.error('Gagal memuat cuti', 'Coba muat ulang halaman.');
      },
    });
  }

  protected submit(): void {
    if (!this.valid() || this.saving()) return;
    this.saving.set(true);
    this.api.add(this.from(), this.to(), this.note()).subscribe({
      next: (res) => {
        this.saving.set(false);
        const skipped = res.skipped.length ? ` (${res.skipped.length} dilewati: akhir pekan/libur)` : '';
        this.toast.success('Cuti disimpan', `${res.added.length} hari cuti disimpan${skipped}.`);
        this.to.set('');
        this.note.set('');
        this.load();
      },
      error: (err) => {
        this.saving.set(false);
        this.toast.error('Gagal menyimpan cuti', err?.error?.error ?? 'Coba lagi.');
      },
    });
  }

  protected remove(date: string): void {
    this.confirming.set(null);
    this.api.remove(date).subscribe({
      next: () => {
        this.toast.success('Cuti dihapus', date);
        this.load();
      },
      error: (err) => this.toast.error('Gagal menghapus cuti', err?.error?.error ?? 'Coba lagi.'),
    });
  }
}
```

`leave.component.html`:
```html
<section class="card" data-card="leave-form">
  <header class="card-header">
    <span class="card-icon"><app-icon name="calendar" [size]="20" /></span>
    <div>
      <h2>Tambah cuti</h2>
      <p>Hari cuti tidak dihitung ke waktu tiket dan Nudge tidak mengirim pengingat di hari itu.</p>
    </div>
  </header>
  <div class="leave-form">
    <label>Dari <input type="date" data-from [value]="from()" (input)="from.set($any($event.target).value)" /></label>
    <label>Sampai <input type="date" data-to [value]="to()" [min]="from()" (input)="to.set($any($event.target).value)" /></label>
    <label class="leave-note">Catatan <input type="text" maxlength="200" placeholder="Opsional" [value]="note()" (input)="note.set($any($event.target).value)" /></label>
    <button type="button" class="btn btn-primary" data-submit [class.loading]="saving()" [disabled]="!valid() || saving()" (click)="submit()">
      <span class="spinner" aria-hidden="true"></span><span class="btn-label">Simpan</span>
    </button>
  </div>
  <p class="leave-hint">Kosongkan "Sampai" untuk cuti satu hari. Akhir pekan dan libur nasional otomatis dilewati.</p>
</section>

<section class="card" data-card="leave-list">
  <header class="card-header"><div><h2>Cuti saya</h2></div></header>
  @if (loading()) {
    <div class="empty-inline"><span>Memuat…</span></div>
  } @else {
    <ul class="leave-list">
      @for (leave of leaves(); track leave.date) {
        <li [attr.data-leave]="leave.date">
          <span class="leave-date">{{ leave.date | date: 'EEEE, d MMM y' }}</span>
          <span class="leave-note-text">{{ leave.note || '–' }}</span>
          @if (confirming() === leave.date) {
            <button type="button" class="btn btn-sm btn-danger" data-confirm (click)="remove(leave.date)">Hapus</button>
            <button type="button" class="btn btn-sm btn-ghost" (click)="confirming.set(null)">Batal</button>
          } @else {
            <button type="button" class="icon-btn" data-delete aria-label="Hapus cuti" (click)="confirming.set(leave.date)">
              <app-icon name="trash" [size]="16" />
            </button>
          }
        </li>
      } @empty {
        <li class="leave-empty">Belum ada cuti.</li>
      }
    </ul>
  }
</section>
```
> Pakai `LOCALE_ID` yang sudah dikonfigurasi app (bila `id` terdaftar, nama hari tampil Indonesia). Cek kelas `btn-primary`/`btn-danger` ada di `styles.scss`; bila tidak, pakai kelas tombol yang dipakai Pengaturan.

Route & sidebar:
```ts
      {
        path: 'leave',
        loadComponent: () => import('./features/dashboard/leave/leave.component').then((m) => m.LeaveComponent),
        title: 'Cuti · Nudge',
        data: { heading: 'Cuti', subheading: 'Hari cuti tidak dihitung ke waktu tiket' },
      },
```
```html
      <a class="nav-item" routerLink="/dashboard/leave" queryParamsHandling="preserve" routerLinkActive="active" (click)="closeSidebar()">
        <app-icon name="calendar" />
        <span>Cuti</span>
      </a>
```
(sebelum link Pengaturan.) Gaya:
```scss
.leave-form { display: flex; flex-wrap: wrap; gap: 12px; align-items: flex-end; }
.leave-form label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; }
.leave-form input { padding: 8px 10px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); color: inherit; }
.leave-note { flex: 1 1 200px; }
.leave-hint { margin: 8px 0 0; font-size: 12px; color: var(--text-muted); }
.leave-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
.leave-list li { display: flex; align-items: center; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border); }
.leave-date { font-weight: 600; min-width: 190px; }
.leave-note-text { flex: 1; color: var(--text-muted); }
.leave-empty { color: var(--text-muted); justify-content: center; }
```

- [ ] **Step 5: PASS** — spec leave, semua spec, `npx ng build`.

---

### Task 8: Dokumentasi & verifikasi akhir

**Files:**
- Modify: `README.md` (bagian menu dashboard & daftar endpoint)
- Modify: `docs/tech-spec/papan-tiket-cuti/Tech Spec - Papan Tiket & Cuti.md` (`Document Status` → `Implemented`)

- [ ] **Step 1: README** — tambahkan ke daftar endpoint dashboard:
  - `GET /api/dashboard/tickets/:issueKey/transitions` — transisi yang diizinkan workflow Jira (tiket sendiri, sprint aktif).
  - `POST /api/dashboard/tickets/:issueKey/transition` — body `{ "transitionId": "11" }` (atau lama `{ "target": "todo|code-review" }`).
  - `GET/POST /api/dashboard/leaves`, `DELETE /api/dashboard/leaves/:date` — cuti hari penuh; hari cuti tidak dihitung & tanpa DM; cuti tanggal lampau memicu hitung ulang dari changelog.
  dan menu **Papan Tiket** / **Cuti** di bagian dashboard.
- [ ] **Step 2: Verifikasi penuh**

```bash
go build ./... && go vet ./... && go test ./...
cd frontend && npx ng test --watch=false && npx ng build
```
- [ ] **Step 3: Verifikasi live (manual)** — restart `air`; buka `/nudge/dashboard/board`: tiket OMS di To Do hanya menawarkan "Start Development → Code Process"; tiket CORE menampilkan daftar transisinya. Tambah cuti hari ini di `/nudge/dashboard/leave` → elapsed tiket aktif berhenti bertambah di Beranda; cek log `leave recalculation of ... rewrote N session(s)`.
- [ ] **Step 4:** Ubah status spec → `Implemented`. Commit hanya saat user meminta.
