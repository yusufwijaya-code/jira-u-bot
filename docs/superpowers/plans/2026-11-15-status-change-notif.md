# Notifikasi Perubahan Status Tiket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** DM ke assignee setiap status tiket sprint aktifnya berubah (di Jira maupun di Nudge), menggantikan notif "selesai lebih awal" / "balik ke To Do" dan card "Jenis notifikasi".

**Architecture:** Modul baru `status_watch` berjalan tiap 10 detik: satu JQL (`sprint in openSprints() AND assignee in (...) AND updated >= cursor`) → bandingkan dengan snapshot `tr_issuestatus` → DM lewat notifier yang ada (`ReminderMessage.Text` = teks jadi). Poller berhenti mengirim dua notif lama. Toggle tunggal `notifyStatusChange` di `ms_user_setting`.

**Tech Stack:** Go + Gin + MySQL, Angular 19.

**Spec:** `docs/tech-spec/status-change-notif/Tech Spec - Notifikasi Perubahan Status Tiket.md`

## Global Constraints

- Branch: `feat/nudge` (jangan buat branch baru). Commit hanya saat user minta. Stage hanya file sendiri.
- Interval watcher 10 detik; cursor = mulai cycle sukses − 1 menit; cycle pertama tanpa filter `updated`.
- Tiket baru di snapshot → tanpa notif. Status berubah → update snapshot dulu, lalu DM sekali (tidak diulang bila gagal).
- Gate: saklar utama OFF / `NotifyStatusChange` false / mute / jam tenang (`IsSilenced`) / hari cuti (kalender user) → lewati. Tidak dibatasi jam kerja.
- Routing: email = `selfEmail` (JIRA_EMAIL) atau DM notifier nil → webhook; selain itu DM.
- Teks UI Bahasa Indonesia; kode English.

---

## File Structure

| File | Tanggung jawab |
|---|---|
| `migrations/0016_status_change_notif.sql`, `migrations/migrate.go` | tabel `tr_issuestatus`, kolom `notifyStatusChange` (+ isi dari dua kolom lama) |
| `modules/user_setting/...` | `NotifyStatusChange` menggantikan `NotifyEarlyFinish`/`NotifyReturnedToTodo` |
| `modules/jira_client/jira_client_dto/jira_client_dto.go` | `ChangelogEntry.Author` |
| `modules/jira_client/jira_client_service/jira_agile_client.go` | `SearchSprintIssuesUpdatedSince` |
| `modules/google_chat_notifier/google_chat_notifier_dto`, `.../BuildMessageText` | `ReminderMessage.Text` (pesan jadi) |
| `modules/status_watch/status_watch_repository/*` | snapshot CRUD |
| `modules/status_watch/status_watch_service/*` | cycle, gate, pesan, Run |
| `modules/poller/...`, `modules/reminder_engine/...` | hapus notif lama |
| `main.go` | worker |
| `frontend/.../settings/*`, `core/services/board.service.ts` | card "Perubahan status tiket" |

---

### Task 1: Migrasi + `NotifyStatusChange` di user_setting

**Files:** `migrations/0016_status_change_notif.sql` (baru), `migrations/migrate.go`, `modules/user_setting/user_setting_dto/user_setting_dto.go`, `modules/user_setting/user_setting_repository/user_setting_repository_impl.go`, `modules/user_setting/user_setting_service/user_setting_service_impl.go`, `modules/user_dashboard/user_dashboard_handler/user_dashboard_handler_impl.go:64-65`, semua test yang memakai dua field lama.

**Interfaces — Produces:** `user_setting_dto.UserSetting.NotifyStatusChange bool` (default `true`), `NotificationInput.NotifyStatusChange bool` JSON `notifyStatusChange`; JSON settings `notification.notifyStatusChange`.

- [ ] **Step 1:** Migrasi — dua file SQL karena pola `addColumnIfNotExists` satu ALTER:

`migrations/0016_status_change_notif.sql`:
```sql
-- Tech Spec "Notifikasi Perubahan Status Tiket": last status seen per ticket.
CREATE TABLE IF NOT EXISTS tr_issuestatus (
  issueKey      VARCHAR(32)  NOT NULL PRIMARY KEY,
  status        VARCHAR(100) NOT NULL,
  assigneeEmail VARCHAR(255) NOT NULL,
  lastSeenDate  DATETIME     NOT NULL,
  editedDate    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_issuestatus_lastseen (lastSeenDate)
);
```
`migrations/0016b_user_setting_status_change.sql`:
```sql
-- One toggle replaces notifyEarlyFinish / notifyReturnedToTodo.
ALTER TABLE ms_user_setting
  ADD COLUMN notifyStatusChange TINYINT(1) NOT NULL DEFAULT 1 AFTER notifyReturnedToTodo;
```
`migrate.go`: embed keduanya; di akhir `apply` setelah `addUserLeaveSQL`:
```go
	if _, err := db.Exec(statusSnapshotSQL); err != nil {
		return err
	}
	// The UPDATE only runs together with the ALTER that adds the column, so
	// later starts never overwrite what users saved.
	added, err := addColumnIfMissing(db, userSettingStatusChangeSQL)
	if err != nil || !added {
		return err
	}
	_, err = db.Exec(`UPDATE ms_user_setting SET notifyStatusChange = (notifyEarlyFinish OR notifyReturnedToTodo)`)
	return err
```
Bila `addColumnIfNotExists` tidak melaporkan "ditambahkan", buat helper `addColumnIfMissing(db, sql) (bool, error)` di file yang sama: jalankan ALTER; error MySQL 1060 (duplicate column) → `(false, nil)`; sukses → `(true, nil)`. Cek dulu implementasi `addColumnIfNotExists` dan pakai cara deteksi 1060 yang sama.

- [ ] **Step 2:** Ganti field di DTO: hapus `NotifyEarlyFinish`/`NotifyReturnedToTodo` dari `UserSetting` & `NotificationInput`, tambah `NotifyStatusChange bool` (`json:"notifyStatusChange"` di input); `Default()` → `NotifyStatusChange: true`; `Apply` → `out.NotifyStatusChange = in.NotifyStatusChange`.
- [ ] **Step 3:** Repo: `rowColumns` ganti `notifyEarlyFinish, notifyReturnedToTodo` → `notifyStatusChange`; `scanRow` scan ke `&s.NotifyStatusChange`; `UpsertNotification` kolom & argumen ganti satu `notifyStatusChange` (placeholder `?` berkurang satu). Kolom lama tetap di tabel, tidak ditulis lagi.
- [ ] **Step 4:** Service (`grep NotifyEarlyFinish` di `user_setting_service_impl.go`) dan handler JSON: `"notifyStatusChange": s.NotifyStatusChange`.
- [ ] **Step 5:** Perbarui test (`user_setting_dto_test.go`, `user_setting_repository_test.go`, `settings_handler_test.go`, `user_dashboard_handler_test.go`) — ganti dua field lama dengan `NotifyStatusChange`. Tambah test repo: simpan `NotifyStatusChange: false` → baca kembali false.
- [ ] **Step 6:** `go build ./... && go vet ./... && go test ./modules/user_setting/... ./modules/user_dashboard/... ./migrations/` → PASS (poller/reminder_engine belum dikompilasi ulang bila masih memakai field lama — dibereskan di Task 4; jalankan `go build ./...` untuk memastikan tidak ada pemakaian non-test lain).

### Task 2: Jira client — pencarian sprint terbaru + author changelog; `ReminderMessage.Text`

**Files:** `modules/jira_client/jira_client_dto/jira_client_dto.go`, `modules/jira_client/jira_client_service/jira_agile_client.go`, test `jira_agile_client_test.go` (atau test client yang ada), `modules/google_chat_notifier/google_chat_notifier_dto/*.go`, `google_chat_notifier_service_impl.go` (`BuildMessageText`), test-nya.

**Interfaces — Produces:**
- `jira_client_dto.ChangelogEntry.Author *ChangelogAuthor` (`DisplayName string json:"displayName"`)
- `JiraAgileClient.SearchSprintIssuesUpdatedSince(ctx, accountIDs []string, since *time.Time) ([]jira_client_dto.Issue, error)` — kredensial bot; `since == nil` → tanpa filter; JQL `project in (CORE, OMS) AND sprint in openSprints() AND assignee in ("a","b") [AND updated >= "2006/01/02 15:04"] ORDER BY updated DESC`, waktu dalam zona WIB (Jira memakai zona user bot = Asia/Jakarta); fields `summary,status,assignee`; paginasi `nextPageToken` sampai habis (maks 10 halaman × 100).
- `ReminderMessage.Text string` — bila tidak kosong, `BuildMessageText` mengembalikannya apa adanya.

- [ ] **Step 1:** Test `BuildMessageText` dengan `Text: "halo"` → `"halo"`. FAIL → tambah field + `if msg.Text != "" { return msg.Text }` di awal fungsi → PASS.
- [ ] **Step 2:** Test client dengan `httptest.Server` (pola test client yang ada): memeriksa JQL berisi `assignee in ("acc-1","acc-2")`, `sprint in openSprints()`, dan `updated >= "2026/11/15 10:14"` untuk since `2026-11-15T10:14:00+07:00`; tanpa since tidak ada `updated`. Respons dua halaman (`nextPageToken`) → 2 issue total. Implementasi; `SearchResponse` tambah `NextPageToken string json:"nextPageToken"` bila belum ada.
- [ ] **Step 3:** Tambah `Author` ke `ChangelogEntry`; test decode JSON changelog dengan `"author":{"displayName":"Anwar"}`.
- [ ] **Step 4:** `go test ./modules/jira_client/... ./modules/google_chat_notifier/...` PASS.

### Task 3: Modul `status_watch`

**Files:**
- `modules/status_watch/status_watch_dto/status_watch_dto.go`
- `modules/status_watch/status_watch_repository/status_watch_repository.go` (+ `_test.go`, DB nyata `testutil.OpenTestDB`)
- `modules/status_watch/status_watch_service/status_watch_service.go`, `status_watch_service_impl.go`, `message.go`, `status_watch_service_impl_test.go`

**Interfaces:**
- DTO: `Snapshot{IssueKey, Status, AssigneeEmail string; LastSeen time.Time}`.
- Repo: `Find(ctx, keys []string) (map[string]Snapshot, error)`, `Upsert(ctx, s Snapshot) error`, `DeleteNotSeenSince(ctx, t time.Time) (int64, error)`.
- Service deps (interfaces kecil di package service):
  - `issueSource{ SearchSprintIssuesUpdatedSince(...); GetChangelog(ctx, cred, key) }`
  - `userDirectory{ FindAll(ctx) ([]registration_dto.User, error) }`
  - `settingsSource{ GetEffectiveForEmail(ctx, email string, boardID int64) user_setting_dto.UserSetting }`
  - `sessionSource{ GetSessionByIssueKey(ctx, key) (*issue_session_dto.IssueSession, bool, error) }`
  - `business_hours_service.BusinessHoursService`, `business_hours_dto.CalendarSource`
  - `notifier google_chat_notifier_service.GoogleChatNotifierService` (webhook), `dm google_chat_notifier_service.GoogleChatNotifierService` (bisa nil), `selfEmail string`, `jiraBaseURL string`
- Produces: `NewStatusWatchService(deps Deps) StatusWatchService` dengan `RunOnce(ctx) error`, `Run(ctx)` (tiap 10 detik + cleanup 30 hari sekali sehari).

- [ ] **Step 1:** Repo + test DB (insert → Find → Upsert status baru → DeleteNotSeenSince).
- [ ] **Step 2:** Test service (fake semua deps, `now` tetap):
  1. Cycle pertama: issue OMS-1 "To Do" belum ada di snapshot → snapshot tersimpan, 0 pesan; `since` yang dikirim = nil.
  2. Cycle kedua: OMS-1 jadi "Code Process" → 1 pesan berisi `*To Do* → *Code Process*`, `oleh Anwar`, link; `since` = mulai cycle 1 − 1 menit.
  3. Gate: `NotificationsOff`, `NotifyStatusChange=false`, `MutedUntil` di masa depan, kalender user memuat hari ini → 0 pesan tapi snapshot tetap diperbarui.
  4. Keluar tracked: CORE-1 "CODE REVIEW" → "Code Completed" dengan session (SP 2, accumulated 1 jam) → baris `⏱ Selesai 1 jam lebih awal dari budget 2 jam`; OMS-2 "Code Process" → "To Do" (SP 1, accumulated 2 jam) → `⏱ Waktu terpakai 2 jam dari budget 1 jam (lewat 1 jam)`.
  5. CORE Code Process → CODE REVIEW: tanpa baris ⏱.
  6. Assignee tidak terdaftar / beda dari snapshot: snapshot diperbarui ke assignee baru, tidak ada pesan untuk perubahan itu.
  7. Routing: assignee = selfEmail → webhook; lainnya → dm.
  8. Search error → `RunOnce` mengembalikan error dan cursor tidak maju (cycle berikut memakai since yang sama).
- [ ] **Step 3:** Implementasi. Inti `RunOnce`:
```go
func (s *service) RunOnce(ctx context.Context) error {
	started := s.now()
	users, accountIDs := s.registeredUsers(ctx) // cache 5 menit; map accountId→User
	if len(accountIDs) == 0 {
		return nil
	}
	issues, err := s.jira.SearchSprintIssuesUpdatedSince(ctx, accountIDs, s.cursor)
	if err != nil {
		return err // cursor stays
	}
	keys := make([]string, 0, len(issues))
	for _, is := range issues {
		keys = append(keys, is.Key)
	}
	known, err := s.snapshots.Find(ctx, keys)
	if err != nil {
		return err
	}
	for _, is := range issues {
		user, ok := users[accountIDOf(is)]
		if !ok {
			continue
		}
		status := is.Fields.Status.Name
		prev, seen := known[is.Key]
		snap := status_watch_dto.Snapshot{IssueKey: is.Key, Status: status, AssigneeEmail: user.Email, LastSeen: started}
		if err := s.snapshots.Upsert(ctx, snap); err != nil {
			log.Printf("status_watch: snapshot %s: %v", is.Key, err)
			continue
		}
		if !seen || strings.EqualFold(prev.Status, status) || prev.AssigneeEmail != user.Email {
			continue
		}
		s.notify(ctx, is, user, prev.Status, status)
	}
	c := started.Add(-time.Minute)
	s.cursor = &c
	return nil
}
```
`notify`: board = `issue.Fields.ActiveSprint()` boardID (0 bila tidak ada) → setting = `GetEffectiveForEmail(email, boardID)`; lewati bila `setting.NotificationsOff || !setting.NotifyStatusChange || setting.IsSilenced(now) || cal.ForEmail(email).IsHoliday(now)`. Pelaku = entri changelog terakhir yang punya item `status`; gagal → kosong. Bangun teks (`message.go`):
```go
// statusChangeText renders the DM (Tech Spec AC1.3, AC2.2).
func statusChangeText(key, summary, url, from, to, actor string, at time.Time, timing string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 *%s* — %s\n*%s* → *%s*", key, summary, from, to)
	if actor != "" {
		fmt.Fprintf(&b, " · oleh %s", actor)
	}
	fmt.Fprintf(&b, " · %s WIB", at.In(wib).Format("15:04"))
	if timing != "" {
		b.WriteString("\n" + timing)
	}
	if url != "" {
		b.WriteString("\n" + url)
	}
	return b.String()
}
```
Baris waktu (`timingLine`): hanya bila `rule.IsTracked(from) && !rule.IsTracked(to)` dan session ada dengan SP > 0: `elapsed := issue_session_service.CalculateElapsedSeconds(*session, bh, cal)`, `budget := int64(SP*3600)`; `elapsed > budget` → `⏱ Waktu terpakai X dari budget Y (lewat Z)`; `rule.IsDone(to)` → `⏱ Selesai X lebih awal dari budget Y`; lainnya → `⏱ Waktu terpakai X dari budget Y`. Durasi pakai `google_chat_notifier_service.FormatDuration`.
Kirim: `ReminderMessage{IssueKey, IssueURL, AssigneeEmail: email, Text: text}` lewat webhook bila `email == selfEmail || dm == nil`, selain itu dm.
`Run`: ticker 10 detik memanggil `RunOnce` (error di-log); sekali per 24 jam `DeleteNotSeenSince(now − 30 hari)`.
- [ ] **Step 4:** `go test ./modules/status_watch/...` PASS; `go vet`.

### Task 4: Lebur notif lama + wiring

**Files:** `modules/poller/poller_service/poller_service_impl.go` (±200–235) + test, `modules/reminder_engine/reminder_engine_service/reminder_engine_service.go`, `_impl.go`, `_test.go`, `main.go`.

- [ ] **Step 1:** Poller: hapus blok `GetIssueStatus` + switch `NotifyIfEarlyCompletion`/`NotifyReturnedToTodo` setelah `CloseSession` (beserta variabel yang jadi tidak terpakai). Hapus test poller yang menguji notif itu; tambah test: session keluar tracked → `CloseSession` dipanggil, tidak ada panggilan notif (fake reminder engine tidak lagi punya method itu).
- [ ] **Step 2:** Reminder engine: hapus `NotifyIfEarlyCompletion` & `NotifyReturnedToTodo` dari interface, impl, dan test; perbarui fake di package lain (`grep -rn "NotifyIfEarlyCompletion\|NotifyReturnedToTodo"`).
- [ ] **Step 3:** `main.go`: setelah `reminderEngineSvc`:
```go
	// Tech Spec "Notifikasi Perubahan Status Tiket": DM on every status
	// change of a registered user's active-sprint ticket.
	statusWatchSvc := status_watch_service.NewStatusWatchService(status_watch_service.Deps{
		Jira: jiraAgileClient, Users: registrationRepo, Settings: userSettingSvc,
		Sessions: issueSessionRepo, BusinessHours: businessHoursSvc, Calendar: userLeaveSvc,
		Snapshots: status_watch_repository.NewStatusWatchRepository(db),
		Webhook: notifierSvc, DM: dmNotifierSvc, SelfEmail: cfg.JiraEmail, JiraBaseURL: cfg.JiraBaseURL,
	})
```
dan worker baru dengan pola `shutdownMgr.AddWorker()` + `go func(){ defer shutdownMgr.WorkerDone(); statusWatchSvc.Run(shutdownMgr.Context()) }()`. Perhatikan `dmNotifierSvc` bertipe interface yang bisa nil — oper hanya bila non-nil (hindari nil interface berisi pointer nil).
- [ ] **Step 4:** `go build ./... && go vet ./... && go test ./...` PASS.

### Task 5: Frontend — card "Perubahan status tiket"

**Files:** `frontend/src/app/core/services/board.service.ts:19-20`, `features/dashboard/settings/settings.component.ts:37` (+ hint functions `earlyFinishHint`/`returnedToTodoHint` dihapus), `settings.component.html` (card `events`), `settings.component.spec.ts`; cek juga admin user detail yang memakai komponen yang sama.

- [ ] **Step 1:** `board.service.ts`: ganti dua field dengan `notifyStatusChange: boolean`. `settings.component.ts`: `events: ['notifyStatusChange']`; hapus hint lama.
- [ ] **Step 2:** HTML card `events`:
```html
    <!-- 3. Status change notice -->
    <section class="card" data-card="events" [class.readonly]="readOnly()" [attr.inert]="readOnly() ? '' : null">
      <header class="card-header">
        <span class="card-icon"><app-icon name="repeat" [size]="20" /></span>
        <div>
          <h2>Perubahan status tiket</h2>
          <p>DM setiap status tiket kamu di sprint aktif berubah — digeser di Jira maupun di Nudge.</p>
        </div>
      </header>
      <div class="toggle-list">
        <label class="toggle-row">
          <span class="toggle-text">
            <strong>Kirim notif perubahan status</strong>
            <small data-hint="status-change">Contoh: "To Do → Code Process · oleh Anwar". Saat tiket keluar dari {{ trackedLabel() }}, ikut info waktu terhadap budget.</small>
          </span>
          <input
            type="checkbox"
            class="switch"
            data-toggle="status-change"
            [checked]="d.notifyStatusChange"
            (change)="patch({ notifyStatusChange: $any($event.target).checked })"
          />
        </label>
      </div>
      <ng-container *ngTemplateOutlet="footer; context: { card: 'events' }" />
    </section>
```
`trackedLabel()`: "Code Process/Code Review" untuk board CORE, "Code Process" untuk OMS, "status yang dihitung" untuk Semua board — pakai data board terpilih (`reviewTracked`) yang sudah ada di komponen; bila tidak tersedia, teks statis "status yang dihitung waktunya".
- [ ] **Step 3:** Spec: ganti assertion dua toggle lama dengan satu toggle `[data-toggle="status-change"]` yang mengirim `notifyStatusChange` di body PUT; hapus test hint lama.
- [ ] **Step 4:** `npx ng test --watch=false && npx ng build` PASS.

### Task 6: README, status spec, verifikasi

- [ ] README: ganti poin "Notifikasi selesai lebih awal" & "Jenis notifikasi" dengan notif perubahan status (watcher 10 detik, snapshot `tr_issuestatus`, gate, toggle `notifyStatusChange`).
- [ ] Spec → `Implemented`.
- [ ] `go build ./... && go vet ./... && go test ./...`; `ng test`; `ng build`; screenshot Pengaturan (mock server) desktop & mobile.
