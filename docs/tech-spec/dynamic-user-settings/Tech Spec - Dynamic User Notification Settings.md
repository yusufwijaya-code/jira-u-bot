# Tech Spec — Dynamic User Notification Settings (Fase 1)

> **Date:** 2026-11-14
> **PRD Link:** — (hasil brainstorming di sesi Claude Code, disetujui user)
> **STP Link:** —
> **Epic / Task:** Nudge — setting dinamis per user (Fase 1)
> **Document Status:** `Implemented`
> **Document Owner:** Yusuf Wijaya
> **Need Review:** —

---

## 📋 Background, Problem & Solution

| | |
|---|---|
| **Why** | Semua perilaku notifikasi Nudge di-hardcode: tier reminder tetap 6 buah, notif overage berulang tiap 5 menit tanpa batas, notif "selesai lebih awal" & "balik ke To Do" selalu terkirim, dan tidak ada cara meredam notif. User ingin mengatur sendiri kapan dan seberapa sering dicolek. |
| **Current condition** | `modules/reminder_engine/reminder_engine_service/reminder_engine_service_impl.go` — `tierDurations` (6 tier tetap), `overageRepeatInterval = 5 * time.Minute`; preferensi tier hanya berupa subset CSV di `ms_user.reminderTierPreference` (migration `0006`); dedup kirim lewat `tr_reminderlog (issueKey, tier)` per tiket sepanjang umurnya; repeat overage memakai `tr_issuesession.lastOverageReminderDate`. |
| **Solution** | Tabel baru `ms_user_setting` (kolom bertipe, satu baris per user, tanpa baris = default yang identik dengan perilaku sekarang). Reminder engine membaca setting user: tier sebelum budget custom, tier setelah budget (`+Xm`), interval ulang atau berhenti, toggle jenis notif, mute & jam tenang. Halaman **Pengaturan** di dashboard Angular menjadi 5 kartu + badge mute di topbar. |

---

## 🗺️ Domain Mapping

| User Story | Domain | Sub-module |
|------------|--------|------------|
| US-1 Tier reminder sebelum budget custom | Notification | `user_setting`, `reminder_engine` |
| US-2 Tier reminder setelah budget + ulang/berhenti | Notification | `user_setting`, `reminder_engine`, `issue_session` |
| US-3 Toggle notif selesai lebih awal / balik ke To Do | Notification | `user_setting`, `reminder_engine` |
| US-4 Mute sementara & jam tenang | Notification | `user_setting`, `reminder_engine`, `reminder_log` |
| US-5 Halaman Pengaturan & badge mute | Dashboard | `user_dashboard`, frontend `settings`, `dashboard-shell` |

**Acceptance Criteria**

- **AC1.1** User bisa memilih/menambah tier sebelum budget berformat `<n>m` / `<n>h`, rentang 1m–8h, maksimal 10, tanpa duplikat.
- **AC1.2** Tier yang ≥ budget tiket diabaikan (perilaku sekarang).
- **AC1.3** Bila beberapa tier sudah terlewati sekaligus, hanya tier terdekat dengan deadline yang dikirim; sisanya dicatat `skipped`.
- **AC1.4** Input tier tidak valid (format salah, < 1m, > 8h, > 10 tier, duplikat) → `400` dengan pesan field.
- **AC2.1** Saat budget habis, notif T+0 (`overage`) tetap dikirim sekali.
- **AC2.2** Tiap tier overage `+Xm` dikirim sekali saat `elapsed − budget ≥ X` (jam kerja).
- **AC2.3** Setelah semua tier overage terlewati, notif diulang tiap `overageRepeatMinutes` (5/10/15/30/60) sejak notif overage terakhir; `null` = berhenti.
- **AC2.4** Default (tanpa tier overage, ulang 5 menit) = perilaku sekarang.
- **AC2.5** `overageRepeatMinutes` di luar 5/10/15/30/60 → `400`.
- **AC3.1** `notifyEarlyFinish = false` → notif selesai lebih awal tidak dikirim.
- **AC3.2** `notifyReturnedToTodo = false` → notif balik ke To Do tidak dikirim.
- **AC4.1** Saat mute aktif (`mutedUntil > now`) atau dalam jam tenang: tier sebelum budget & tier overage yang jatuh tempo dicatat `skipped`, tidak dikirim belakangan.
- **AC4.2** Saat mute/jam tenang: notif T+0 dan ulangan overage tidak dikirim dan **tidak** memajukan jadwal → begitu mute selesai, notif overage langsung terkirim bila tiket masih lewat budget.
- **AC4.3** Saat mute/jam tenang: notif selesai lebih awal & balik ke To Do dilewati.
- **AC4.4** Mute cepat: 1 jam, 2 jam, sampai besok (08:30 WIB hari berikutnya); bisa dibatalkan.
- **AC4.5** Jam tenang: start & end keduanya diisi atau keduanya kosong; boleh melewati tengah malam; start ≠ end.
- **AC5.1** `GET /settings` mengembalikan semua setting + status mute + credential.
- **AC5.2** Identitas selalu dari sesi, tidak pernah dari body/query.
- **AC5.3** Halaman Pengaturan: 5 kartu (tier sebelum budget, tier setelah budget, jenis notif, mute & jam tenang, Jira API key).
- **AC5.4** Badge 🔕 di topbar saat mute aktif.
- **AC6.1** User tanpa baris `ms_user_setting` / assignee belum registrasi / gagal baca DB → default; reminder tidak pernah berhenti karena setting.

---

## 🔀 Cross-repo Impact

| Repo | Impact | Detail | Priority |
|------|--------|--------|----------|
| `jira-u-bot` (Go) | `YES` | modul baru `user_setting`, `reminder_engine`, `reminder_log`, `user_dashboard`, migration `0008` | `HIGH` |
| `jira-u-bot/frontend` (Angular) | `YES` | halaman Pengaturan, `board.service.ts`, shell badge | `HIGH` |
| `fnb-backend` / `go-core-api` / `phantom-lancer` | `NO` | tidak terkait | — |

---

## 🗄️ DB Schema Changes

```sql
-- Table   : ms_user_setting
-- Action  : NEW TABLE
CREATE TABLE IF NOT EXISTS ms_user_setting (
  userID                 BIGINT       NOT NULL,
  reminderTiers          VARCHAR(200) NOT NULL DEFAULT '2h,1h,30m,15m,5m,1m' COMMENT 'tier sebelum budget habis, CSV kode <n>m/<n>h',
  overageTiers           VARCHAR(200) NOT NULL DEFAULT ''                    COMMENT 'tier setelah budget lewat, CSV kode <n>m/<n>h (tanpa tanda +)',
  overageRepeatMinutes   SMALLINT     NULL     DEFAULT 5                     COMMENT 'ulang overage tiap N menit setelah tier overage terakhir; NULL = berhenti',
  notifyEarlyFinish      TINYINT(1)   NOT NULL DEFAULT 1,
  notifyReturnedToTodo   TINYINT(1)   NOT NULL DEFAULT 1,
  mutedUntil             DATETIME     NULL,
  quietHoursStart        TIME         NULL,
  quietHoursEnd          TIME         NULL,
  updatedDate            DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (userID),
  CONSTRAINT fk_ms_user_setting_user FOREIGN KEY (userID) REFERENCES ms_user (userID) ON DELETE CASCADE
);

-- Backfill preferensi lama (idempotent)
INSERT IGNORE INTO ms_user_setting (userID, reminderTiers)
SELECT userID, reminderTierPreference
FROM ms_user
WHERE reminderTierPreference IS NOT NULL;

-- Table   : tr_reminderlog
-- Action  : ADD COLUMN
ALTER TABLE tr_reminderlog
  ADD COLUMN deliveryStatus VARCHAR(10) NOT NULL DEFAULT 'sent' COMMENT 'sent | skipped' AFTER tier;

-- Migration files: migrations/0008_add_user_setting.sql (tabel + backfill) dan
-- migrations/0009_add_reminderlog_delivery_status.sql (ADD COLUMN, dibungkus toleransi
-- duplicate-column seperti 0002/0006/0007); keduanya di-apply di migrations/migrate.go
```

- `ms_user.reminderTierPreference` **tidak** di-drop di fase ini (rollback aman); di-drop di migration terpisah setelah stabil.
- Kode tier overage di `tr_reminderlog.tier` memakai prefiks `+` (contoh `+15m`, ≤ 20 karakter) agar tidak bentrok dengan tier sebelum budget (`15m`). Notif T+0 & ulangan tetap `overage`.
- Dedup `HasTierBeenSent` menganggap `sent` dan `skipped` sama-sama "sudah ditangani".

---

## 📡 API Contract

Semua di bawah grup `dashboardAPI` (`{BASE_PATH}/api/dashboard`, `AuthMiddlewareAPI`). Identitas dari `auth_middleware.CurrentUserKey`.

### `GET /api/dashboard/settings` *(diperluas)*

**Response 200:**
```json
{
  "notification": {
    "reminderTiers": ["2h", "1h", "30m", "15m", "5m", "1m"],
    "overageTiers": ["15m", "1h"],
    "overageRepeatMinutes": 30,
    "notifyEarlyFinish": true,
    "notifyReturnedToTodo": true,
    "quietHours": { "start": "12:00", "end": "13:00" }
  },
  "mute": { "active": true, "until": "2026-11-14T15:30:00+07:00" },
  "tierPreference": { "tiers": ["2h", "1h", "30m", "15m", "5m", "1m"] },
  "credential": { "hasCredential": true, "maskedPreview": "••••ab3f" }
}
```
`quietHours` = `null` bila tidak aktif; `mute.until` = `null` bila tidak mute. `tierPreference` dipertahankan sementara untuk kompatibilitas.

### `PUT /api/dashboard/settings/notification`

**Request:**
```json
{
  "reminderTiers": ["2h", "45m", "15m"],
  "overageTiers": ["15m", "1h"],
  "overageRepeatMinutes": 30,
  "notifyEarlyFinish": true,
  "notifyReturnedToTodo": false,
  "quietHours": { "start": "12:00", "end": "13:00" }
}
```
`overageRepeatMinutes: null` = berhenti; `quietHours: null` = nonaktif.

**Response 200:** sama dengan objek `notification` pada `GET /settings`.

**Error Responses:**
| Code | Condition |
|------|-----------|
| `400` | `{"error": "...", "fields": {"reminderTiers": "..."}}` — format tier salah / di luar 1m–8h / > 10 / duplikat; `overageRepeatMinutes` bukan 5/10/15/30/60/null; jam tenang hanya satu sisi, format bukan `HH:MM`, atau start = end |
| `401` | Tidak ada sesi |
| `500` | Gagal simpan |

### `POST /api/dashboard/settings/mute`

**Request:** `{ "minutes": 60 }` (1–1440) **atau** `{ "until": "tomorrow" }` (besok 08:30 WIB).

**Response 200:** `{ "active": true, "until": "2026-11-14T15:30:00+07:00" }`

| Code | Condition |
|------|-----------|
| `400` | keduanya/tak satu pun diisi, `minutes` di luar 1–1440, `until` bukan `tomorrow` |

### `DELETE /api/dashboard/settings/mute`

**Response 200:** `{ "active": false, "until": null }`

### Dihapus
`POST /api/dashboard/preference` — digantikan `PUT /settings/notification` (satu-satunya pemakai adalah frontend kita).

---

## 🔐 Access Control Matrix

| Action | Rule | Behavior if Denied |
|--------|------|--------------------|
| Lihat / ubah setting sendiri | Login (`AuthMiddlewareAPI`), domain `@esb.co.id` | `401` → interceptor redirect ke login |
| Ubah setting user lain | Tidak ada endpoint; `userID` selalu dari sesi | — |

> Tidak ada entri `lk_accesscontrol` (bukan ESB Core).

---

## 🔄 Sequence Diagram

```mermaid
sequenceDiagram
  participant Poller
  participant Engine as ReminderEngine
  participant Setting as UserSettingService
  participant Log as ReminderLogRepo
  participant Chat as Notifier

  Poller->>Engine: EvaluateAndSend(issue, session)
  Engine->>Setting: GetForEmail(assigneeEmail)
  Setting-->>Engine: UserSetting (atau default)
  alt elapsed <= budget
    Engine->>Engine: tier sebelum budget yang jatuh tempo & belum ditangani
  else elapsed > budget
    Engine->>Engine: T+0 / tier overage +Xm / ulangan
  end
  alt muted / quiet hours
    Engine->>Log: LogReminder(tier, skipped) (tier saja; ulangan tidak dicatat)
  else normal
    Engine->>Chat: SendReminder(tier paling relevan)
    Engine->>Log: LogReminder(tier, sent) + skipped utk tier lama yang terlewati
  end
```

---

## 🧠 State Management & UI Logic

```
Kartu: Reminder sebelum budget habis
  Default state : 6 tile preset (dicentang sesuai reminderTiers) + tier custom sebagai tile yang bisa dihapus
  Tambah tier   → input angka + select menit/jam; tolak format/rentang/duplikat/>10 di client (server tetap validasi)
  On save       : PUT /settings/notification (seluruh objek notification), toast sukses / error per field

Kartu: Reminder setelah lewat budget
  Default state : preset T+5m, T+15m, T+30m, T+1j, T+2j (tidak dicentang) + tier custom
  Dropdown      : "Setelah itu" = ulang tiap 5/10/15/30/60 menit | Berhenti
  Preview       : "Budget habis → +15m → +1j → tiap 30 menit"

Kartu: Jenis notifikasi
  Toggle        : Selesai lebih awal, Tiket balik ke To Do

Kartu: Mute & jam tenang
  Mute aktif    → banner "🔕 Dimute sampai HH:MM · Batalkan"; tombol cepat disembunyikan
  Mute nonaktif → tombol 1 jam / 2 jam / Sampai besok (aksi instan, POST /settings/mute)
  Jam tenang    : toggle; saat on → dua input time (start, end), wajib keduanya

Topbar
  mute.active   → badge 🔕 di samping email (SettingsStateService dibagi shell & halaman)
```

Kartu 1–3 + jam tenang disimpan bersama oleh satu `PUT` (state di-hold di `SettingsComponent`), tiap kartu punya tombol Simpan sendiri yang mengirim seluruh objek terbaru.

---

## ✅ Breakdown Task & Effort

| # | Task | Repo | AC Coverage | 🤖 AI | 👨‍💻 Dev Verify | Blocked By |
|---|------|------|-------------|-------|----------------|------------|
| 1 | Migration `0008` + apply di `migrate.go` + test | `jira-u-bot` | AC6.1 | `~1m` | `~15m` | — |
| 2 | Modul `user_setting` (dto + parse/validasi tier, repository, service GetForEmail/Save/Mute) + test | `jira-u-bot` | AC1.1, AC1.4, AC2.5, AC4.4, AC4.5, AC6.1 | `~3m` | `~1h` | #1 |
| 3 | `reminder_log`: `deliveryStatus` + `LogReminder(issueKey, tier, status)` | `jira-u-bot` | AC1.3, AC4.1 | `~1m` | `~15m` | #1 |
| 4 | `reminder_engine`: tier dari setting, satu notif paling relevan, overage tiers + repeat, mute/quiet, toggle + test | `jira-u-bot` | AC1.2, AC1.3, AC2.1–2.4, AC3.1–3.2, AC4.1–4.3 | `~3m` | `~2h` | #2, #3 |
| 5 | `user_dashboard`: GET diperluas, PUT notification, POST/DELETE mute, hapus `/preference` + test | `jira-u-bot` | AC5.1, AC5.2 | `~2m` | `~45m` | #2 |
| 6 | Wiring `main.go` / `router.go` | `jira-u-bot` | — | `~30s` | `~10m` | #4, #5 |
| 7 | Frontend: service + Pengaturan 5 kartu + badge mute + spec | `frontend` | AC5.3, AC5.4 | `~5m` | `~1.5h` | #5 |
| 8 | README + verifikasi end-to-end | — | semua | `~1m` | `~30m` | #6, #7 |

> **🤖 AI generates code: ~0.3h**
>
> **👨‍💻 Dev verify & fix: ~6.5h**
>
> **Combined: ~6.8h** (~1 working day)

---

## 🔗 Dependencies

**Internal:** `registration_repository.FindUserByEmail` (resolve userID by email), `business_hours` (elapsed jam kerja, WIB), `issue_session.UpdateLastOverageReminder`.

**External:** Tidak ada.

---

## 🚀 Milestone / Deployment Strategy

1. Migration jalan otomatis saat service start (`migrations.Apply`), idempotent.
2. Tanpa baris setting = default identik dengan perilaku sekarang → aman di-deploy tanpa komunikasi ke user.
3. Piloting flag: tidak diperlukan.
4. **Rollback plan:** deploy build sebelumnya; tabel `ms_user_setting` & kolom `deliveryStatus` dibiarkan (tidak dibaca build lama), `ms_user.reminderTierPreference` masih utuh.

---

## ⚠️ Drawbacks / Risks

| Category | Risk |
|----------|------|
| **Performance** | `GetForEmail` dipanggil per tiket aktif per siklus poll (10 detik) — satu query PK/unique join, jumlah tiket aktif kecil; tidak perlu cache di fase 1. |
| **Behavior change** | Aturan "satu notif paling relevan" mengubah perilaku saat banyak tier terlewati sekaligus (dulu terkirim beruntun tiap 10 detik) — disengaja. |
| **Tier custom berubah di tengah tiket** | Tier yang baru ditambah dan sudah terlewati ikut aturan "paling relevan" → paling banyak 1 notif, bukan banjir. |
| **Zona waktu** | Mute & jam tenang dievaluasi di `Asia/Jakarta`, konsisten dengan business hours. |

---

## ❓ Unresolved / Pending Clarification

| # | Location | Contradiction / Ambiguity | Assumption Made |
|---|----------|---------------------------|-----------------|
| 1 | AC4.4 | "Sampai besok" saat besok hari libur/weekend | Besok 08:30 WIB kalender, tidak melompati hari libur (gate jam kerja tetap menahan notif) |
| 2 | AC4.1 | Mute berlaku untuk tiket orang lain yang dikirim ke DM user? | Setting yang dipakai selalu milik **assignee** tiket |
| 3 | AC2.2 | Tier overage > 8h | Ditolak (rentang sama 1m–8h) |

> **Future possibilities (Fase 2+):** command `/u-mute`, setting global admin (jam kerja, hari libur, SP→jam, project, nama status, interval polling, footer pesan), tujuan kirim (DM vs space) per user.

---

## 📁 Files to Modify

| File | Change |
|------|--------|
| `migrations/0008_add_user_setting.sql` | Baru — tabel `ms_user_setting` + backfill |
| `migrations/0009_add_reminderlog_delivery_status.sql` | Baru — kolom `tr_reminderlog.deliveryStatus` |
| `migrations/migrate.go` | Apply `0008` + `0009` |
| `modules/user_setting/user_setting_dto/*.go` | Baru — `UserSetting`, default, parse & validasi tier/jam tenang |
| `modules/user_setting/user_setting_repository/*.go` | Baru — Find/Upsert/SetMute by userID |
| `modules/user_setting/user_setting_service/*.go` | Baru — `GetForEmail`, `Get/Save/Mute/Unmute` by userID, `IsSilenced` |
| `modules/reminder_log/reminder_log_repository/*.go` | `LogReminder(issueKey, tier, status)` |
| `modules/reminder_log/reminder_log_dto/reminder_log_dto.go` | Konstanta `DeliverySent`, `DeliverySkipped`, `OverageTierPrefix` |
| `modules/reminder_engine/reminder_engine_service/*.go` | Logika tier dinamis, overage tiers, mute/quiet, toggle |
| `modules/user_dashboard/user_dashboard_handler/*.go`, `router.go` | Endpoint settings baru, hapus `/preference` |
| `modules/user_dashboard/user_dashboard_service/*.go` | Hapus preferensi tier lama (pindah ke `user_setting`) |
| `main.go` | Wiring `user_setting` |
| `frontend/src/app/core/services/board.service.ts` | Tipe & method settings baru |
| `frontend/src/app/core/services/mute-state.service.ts` | Baru — state mute bersama shell & halaman |
| `frontend/src/app/features/dashboard/settings/*` | 5 kartu; komponen baru `tier-picker`, `mute-panel`, util `tier-codes.ts`; `reminder-preference` dihapus |
| `frontend/src/app/layouts/dashboard-shell/*` | Badge mute |
| `modules/google_chat_notifier/google_chat_notifier_service/google_chat_notifier_service_impl.go` | `tierLabel` mendukung kode tier custom (`45m` → "T-45 menit") |
| `README.md` | Dokumentasi fitur |
