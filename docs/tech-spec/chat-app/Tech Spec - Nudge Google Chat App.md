# Tech Spec — Nudge sebagai Google Chat App

> **Date:** 2026-11-15
> **PRD Link:** — (request langsung dari user, 2026-11-15; desain disetujui lewat brainstorming 4 bagian)
> **STP Link:** —
> **Epic / Task:** Nudge — Pengiriman Pesan
> **Document Status:** `Not Started`
> **Document Owner:** Yusuf Wijaya
> **Need Review:** —

---

## 📋 Background, Problem & Solution

| | |
|---|---|
| **Why** | Semua DM Nudge (reminder, notif status, balasan command, broadcast admin) tampil **atas nama akun pribadi Yusuf Wijaya**, sehingga user mengira Yusuf yang mengirim. Tiket milik Yusuf sendiri dikirim ke space **JIRA U BOT** lewat webhook. User ingin semua pesan datang dari **Nudge** sendiri, tanpa lewat akun Yusuf maupun space. Akun ESB tidak bisa membuat akun bot baru (bukan admin, dan akun baru memakai lisensi berbayar). |
| **Current condition** | DM dikirim lewat **user OAuth akun Yusuf** (`modules/chat_oauth`, scope `chat.messages`/`chat.spaces.create`/`chat.spaces`) oleh `modules/chat_dm_notifier/chat_dm_notifier_service/chat_dm_notifier_service_impl.go` (`spaces:setup` + `spaces/{id}/messages`). Routing "tiket sendiri → webhook, lainnya → DM" ada di `google_chat_notifier_service.PickNotifier`, dipakai `reminder_engine` (`pickNotifier`, ±baris 79) dan `status_watch` (±baris 224). Command `/n-*` dibaca lewat polling `spaces.messages.list` tiap 10 detik (`modules/chat_command_poller`) lalu dijawab `chat_interactive.RouteCommand`. `registration_service` (pesan terima kasih) dan `admin_dashboard_service` (broadcast + change notice) memanggil `SendPlainTextDM` langsung. |
| **Solution** | Daftarkan Nudge sebagai **Google Chat App** (sudah dikonfigurasi di GCP project `jira-u-bot`; terverifikasi bisa di-install di domain ESB tanpa admin). Pesan dikirim dengan **service account** (scope `chat.bot`), sehingga tampil sebagai "Nudge (App)". Event dari Google (install/uninstall, pesan, slash command, klik tombol) diterima di `POST /api/chat/events`. Router baru `chat_delivery` mengirim lewat **pengirim pilihan user** di Pengaturan: **Nudge** (app) atau **Yusuf Wijaya** (DM lama lewat akun operator). Hanya satu yang aktif, dan akun operator sendiri selalu memakai Nudge. Aktivasi Nudge bisa **1-klik dari dashboard** (`spaces.setup` + `singleUserBotDm`) atau manual dari Google Chat. Webhook space JIRA U BOT tidak dipakai lagi. Dikerjakan dalam 3 fase: (1) pengirim + events + aktivasi + pilihan pengirim, (2) slash command native, (3) tombol aksi di card. |

### Keputusan

| # | Pertanyaan | Keputusan |
|---|---|---|
| 1 | Cara bot punya identitas sendiri | **Google Chat App** di GCP project `jira-u-bot` (milik Gmail pribadi; `yusuf.wijaya@esb.co.id` = Admin/Owner). Akun ESB baru ditolak karena memakai lisensi berbayar. Project di org ESB juga tidak bisa dibuat (tidak ada `resourcemanager.projects.create`). |
| 2 | Tipe app | Chat app klasik (**bukan** Workspace add-on), HTTP endpoint, **Authentication Audience = Project Number** (`275006361614`), hanya DM 1:1. |
| 3 | Jalur pengirim | **Pilihan per user di Pengaturan** (permanen, bukan fallback sementara): **Nudge** atau **Yusuf Wijaya**, hanya salah satu. Akun operator (`JIRA_EMAIL`) **hanya Nudge** (tidak bisa DM diri sendiri). User yang belum memilih = otomatis: Nudge jika sudah aktivasi, Yusuf Wijaya jika belum. *(Revisi 2026-11-15, menggantikan "fallback sementara" + `CHAT_LEGACY_FALLBACK`.)* |
| 4 | Space JIRA U BOT / webhook | **Langsung tidak dipakai.** Tiket milik operator juga lewat DM Nudge. |
| 5 | Aktivasi | **1-klik dari dashboard** dan **manual** (cari "Nudge" di Chat lalu Install). Didahului spike (task #0). |
| 6 | Cakupan interaktif | **Semua**: command teks `/n-*` lewat event, slash command native (fase 2), tombol aksi di card (fase 3). |
| 7 | Arsitektur | Modul baru `chat_app` + `chat_app_activation` + router `chat_delivery`. Modul lama tidak diubah isinya, hanya wiring, supaya pembersihan setelah transisi tinggal menghapus modul. |
| 8 | Branch | Tetap di `feat/nudge` (permintaan user). |

---

## 🗺️ Domain Mapping

| User Story | Domain | Sub-module |
|------------|--------|------------|
| US-1 Pesan dikirim sebagai Nudge App | Nudge Bot | baru `chat_app`, baru `chat_delivery`; wiring `reminder_engine`, `status_watch`, `registration`, `admin_dashboard` |
| US-2 Menerima event & command lewat app | Nudge Bot | baru `chat_app` (handler + service), `chat_interactive` |
| US-3 Aktivasi 1-klik & manual | Nudge Bot + Frontend | baru `chat_app_activation`, `registration` (repo), dashboard shell |
| US-4 Pemantauan aktivasi oleh admin | Nudge Bot + Frontend | `admin_dashboard`, admin panel |
| US-7 Pilihan pengirim pesan | Nudge Bot + Frontend | `registration` (kolom `ms_user`), `user_dashboard` settings handler, halaman Pengaturan + Admin › Detail User |
| US-5 Slash command native (fase 2) | Nudge Bot | `chat_app`, `chat_interactive` |
| US-6 Tombol aksi di pesan (fase 3) | Nudge Bot | `chat_app`, `google_chat_notifier` (builder pesan), `chat_interactive` (geser) |

### Acceptance Criteria

**US-1 — Pesan dikirim sebagai Nudge App** *(fase 1)*
- **AC1.1** Semua pengiriman (reminder, overage, notif perubahan status, pesan registrasi, broadcast admin, change notice admin) lewat satu router `chat_delivery`, yang mengimplementasikan `SendReminder(ctx, ReminderMessage)` dan `SendPlainTextDM(ctx, email, text)`.
- **AC1.1a** Pengirim efektif per penerima (lihat US-7):
  - penerima = akun operator → `nudge`;
  - `ms_user.chatSender` diisi → nilai itu;
  - `chatSender` NULL → `nudge` bila `chatAppSpaceID` ada, `operator` bila tidak.
- **AC1.2** Jika pengirim efektif = `nudge` dan penerima punya `ms_user.chatAppSpaceID`, pesan dikirim dengan `POST https://chat.googleapis.com/v1/{chatAppSpaceID}/messages` memakai token service account (scope `https://www.googleapis.com/auth/chat.bot`), body `{"text": ...}`. Format teks sama dengan sekarang (`BuildMessageText`, markup Chat).
- **AC1.3** Jika pengiriman app mendapat **403/404**, app dianggap sudah dicopot tanpa event: `chatAppSpaceID`, `chatAppUserID`, `chatAppActivatedDate` dikosongkan dan `chatSender` di-reset ke NULL (otomatis). Pesan itu dikirim lewat cadangan AC1.4b.
- **AC1.4** Jika pengirim efektif = `operator`, pesan dikirim lewat `chat_dm_notifier` lama (akun operator). Footer `\n\n💡 Pesan ini bisa datang langsung dari app Nudge — aktifkan di https://nudge.labcup.tech` **hanya** ditambahkan bila `chatSender` NULL (user belum pernah memilih) **dan** belum aktivasi. User yang memilih Yusuf Wijaya secara sadar tidak diberi footer.
- **AC1.4a** Router mengembalikan `ErrRecipientNotActivated` bila pengirim efektif = `nudge` tapi `chatAppSpaceID` kosong **dan** penerima adalah akun operator (operator yang belum aktivasi), atau bila pengirim efektif = `operator` tapi OAuth operator tidak tersedia (`chat_oauth` nil).
- **AC1.4b** **Cadangan saat Nudge error** *(revisi user 2026-11-15)*: bila pengirim efektif = `nudge` dan pengiriman lewat app **gagal karena error apa pun** (5xx, timeout, gagal ambil token, 403/404, atau `chatAppSpaceID` kosong), pesan yang sama langsung dikirim sekali lewat akun Yusuf (`chat_dm_notifier`). Teksnya diberi footer `\n\n_(dikirim lewat akun Yusuf Wijaya karena app Nudge sedang gangguan)_`. Error app di-log (`chat_delivery: app send to <email> failed, falling back to operator: <err>`).
  - Tidak berlaku untuk **akun operator**: error app → `failed`.
  - Bila cadangan juga gagal → error gabungan dikembalikan (`failed`).
  - Pilihan `chatSender` user **tidak** diubah oleh cadangan ini (kecuali reset di AC1.3 untuk 403/404).
- **AC1.5** `reminder_engine` dan `status_watch` mencatat `ErrRecipientNotActivated` sebagai `deliveryStatus = skipped` (tidak di-retry), bukan `failed`. Pengiriman yang sukses lewat cadangan dicatat `sent`.
- **AC1.6** Webhook space (`GOOGLE_CHAT_WEBHOOK_URL`) tidak lagi dipakai untuk routing. `PickNotifier` tidak dipanggil lagi.
- **AC1.7** Jika `GOOGLE_CHAT_APP_CREDENTIALS_FILE` kosong atau tidak bisa dibaca saat start, fitur app dinonaktifkan dengan log warning. Semua non-operator dikirim lewat Yusuf Wijaya (pilihan `nudge` diperlakukan sebagai `operator`), dan pilihan Nudge di Pengaturan disembunyikan. Dengan begitu deploy tanpa key tetap jalan seperti sebelumnya, minus webhook.

**US-2 — Event & command lewat app** *(fase 1)*
- **AC2.1** `POST /api/chat/events` menolak (**401**) request tanpa `Authorization: Bearer <JWT>` yang valid: signature RS256 terhadap sertifikat `https://www.googleapis.com/service_accounts/v1/metadata/x509/chat@system.gserviceaccount.com` (di-cache sesuai `Cache-Control` `max-age`, di-refresh bila `kid` tidak dikenal), `iss = chat@system.gserviceaccount.com`, `aud = GOOGLE_CHAT_APP_PROJECT_NUMBER`, `exp` belum lewat (toleransi 60 detik).
- **AC2.2** `ADDED_TO_SPACE` dengan `space.type = DM`/`spaceType = DIRECT_MESSAGE`: jika `user.email` terdaftar dan aktif di `ms_user`, simpan `chatAppSpaceID = space.name`, `chatAppUserID = user.name`, `chatAppActivatedDate = NOW()` (idempoten; tanggal tidak ditimpa bila space sama), lalu balas sambutan berisi daftar command. Jika email tidak terdaftar: balas "Kamu belum terdaftar di Nudge. Login dulu ke https://nudge.labcup.tech lalu kirim pesan apa saja di sini." tanpa menyimpan apa pun.
- **AC2.3** `REMOVED_FROM_SPACE`: kosongkan `chatAppSpaceID`, `chatAppUserID`, `chatAppActivatedDate` untuk user dengan `chatAppSpaceID = space.name`. Respons body kosong.
- **AC2.4** `MESSAGE` di DM dari user terdaftar & aktif: teks (`message.argumentText` bila ada, jika tidak `message.text`) diteruskan ke `chat_interactive.RouteCommand(ctx, user.email, text)`, dan hasilnya dikembalikan sebagai `{"text": ...}`. Teks yang bukan command dibalas petunjuk `/n-help` (perilaku RouteCommand saat ini). Jika user mengirim pesan tapi `chatAppSpaceID`-nya kosong (mis. install terlewat), space disimpan dulu seperti AC2.2.
- **AC2.5** `MESSAGE` dari email tidak terdaftar dibalas seperti AC2.2. Dari user nonaktif dibalas "Akun Nudge kamu sedang dinonaktifkan admin."
- **AC2.6** Command diproses dengan context timeout 25 detik. Jika lewat, endpoint langsung membalas "⏳ Sedang diproses…", dan hasilnya dikirim belakangan lewat `CreateMessage` ke space yang sama.
- **AC2.7** Event dari space non-DM diabaikan (respons `{}`). Panic/error internal tetap merespons 200 dengan `{"text":"Terjadi kesalahan, coba lagi."}` dan error di-log.
- **AC2.8** `chat_command_poller` tetap berjalan seperti sekarang (permanen, untuk user yang memakai Yusuf Wijaya). Balasan command selalu dikirim lewat jalur tempat command itu diketik: command di DM Nudge dibalas Nudge, command di DM Yusuf dibalas lewat akun Yusuf, apa pun pilihan pengirimnya.

**US-3 — Aktivasi** *(fase 1)*
- **AC3.1** `GET /api/dashboard/chat-app/status` → `{ "enabled": bool, "activated": bool, "activatedDate": "RFC3339|null", "oneClick": bool, "sender": "nudge|operator|null", "effectiveSender": "nudge|operator", "operatorAccount": bool, "operatorName": "Yusuf Wijaya" }`. `enabled = false` bila fitur app nonaktif (AC1.7), dan banner tidak tampil. `operatorName` = nama tampilan akun operator (dari `ms_user` email `JIRA_EMAIL`).
- **AC3.2** `GET /auth/chat-app/activate` (butuh session login) membuat `state` acak yang terikat ke session (cookie `HttpOnly`, 10 menit), lalu redirect ke layar izin Google memakai OAuth client Sign-In: scope `https://www.googleapis.com/auth/chat.spaces.create`, `login_hint=<email session>`, `access_type=online`, `prompt=consent`, `include_granted_scopes=false`.
- **AC3.3** `GET /auth/chat-app/callback`: validasi `state`. Tukar `code` dengan token, lalu pastikan email pemilik token (`userinfo`/ID token) sama dengan email session. Panggil `POST /v1/spaces:setup` dengan body `{"space":{"spaceType":"DIRECT_MESSAGE","singleUserBotDm":true}}`, simpan `space.name` seperti AC2.2, lalu **buang token** (tidak disimpan di mana pun). Redirect ke `/dashboard?chatApp=activated`.
- **AC3.4** Error callback → redirect `/dashboard?chatApp=<kode>` dengan kode:
  - `cancelled` (`error=access_denied`)
  - `blocked` (`admin_policy_enforced` / `org_internal` / `disallowed_useragent`)
  - `not_allowed` (`spaces.setup` 403/404: user tidak ada di daftar Visibility)
  - `mismatch` (email token ≠ session)
  - `failed` (lainnya)

  Frontend menampilkan toast sesuai kode, plus tautan panduan manual.
- **AC3.4a** Aktivasi (1-klik maupun manual lewat `ADDED_TO_SPACE`) **tidak** mengubah `chatSender` yang sudah dipilih eksplisit. User dengan `chatSender` NULL otomatis pindah ke Nudge (AC1.1a). Aktivasi yang dimulai dari pilihan "Nudge" di Pengaturan (US-7) menyimpan `chatSender = 'nudge'` setelah berhasil.
- **AC3.5** Banner di dashboard shell (semua halaman user) selama `enabled && !activated && sender == null && !operatorAccount`: teks "Notifikasi bisa datang langsung dari app **Nudge** di Google Chat, bukan dari akun Yusuf Wijaya.", tombol **Aktifkan Nudge di Google Chat** (ke AC3.2), tautan **Cara manual** (dialog: Google Chat → New chat → ketik "Nudge" → Install app), dan tombol **Nanti saja**. "Nanti saja" menyembunyikan banner 7 hari (localStorage, try/catch). Untuk **akun operator** yang belum aktivasi, banner selalu tampil dan tidak bisa ditutup, dengan teks "Akunmu hanya bisa menerima notifikasi lewat app Nudge. Aktifkan sekarang." Setelah `chatApp=activated`, status di-refetch dan banner hilang.
- **AC3.6** Spike (task #0) dengan akun operator: catat apakah muncul layar "Google hasn't verified this app", apakah lolos kebijakan ESB, dan apakah `singleUserBotDm` berhasil. **Jika spike gagal**, AC3.2–AC3.4 ditunda. Banner tetap tampil dengan cara manual saja (tombol diganti "Buka Google Chat" → `https://mail.google.com/chat/`).

**US-4 — Pemantauan admin** *(fase 1)*
- **AC4.1** Admin › User: kolom **Nudge App** (✓ + tanggal aktivasi, atau "—") dan filter "Belum aktivasi". Data dari kolom `ms_user.chatAppSpaceID`/`chatAppActivatedDate` pada respons list user admin.
- **AC4.2** Admin › Dashboard: statistik "Belum aktivasi Nudge App" (user aktif dengan `chatAppSpaceID IS NULL`) yang bisa diklik menuju daftar terfilter.
- **AC4.3** Admin › Detail User: baris status Nudge App dan card **Pengirim pesan** (komponen yang sama dengan US-7). Admin bisa mengubah pilihan pengirim user lain **hanya** ke pengirim yang tersedia untuk user itu (Nudge hanya bila user sudah aktivasi). Perubahan dikirim ke user lewat change notice DM seperti setting lain.
- **AC4.4** Admin › User: kolom **Pengirim** (Nudge / Yusuf Wijaya, berdasarkan pengirim efektif).

**US-7 — Pilihan pengirim pesan** *(fase 1)*
- **AC7.1** Halaman Pengaturan, view **Semua board** (setting ini global per user, bukan per board): card **Pengirim pesan** dengan dua pilihan radio:
  - **Nudge (app)**: "Pesan datang dari app Nudge di Google Chat. Mendukung command & tombol aksi."
  - **Yusuf Wijaya**: "Pesan datang dari akun Yusuf Wijaya seperti sebelumnya." (nama dari `operatorName`)

  Hanya satu yang bisa dipilih. Pilihan terpilih awal = `effectiveSender`. Pada view board tertentu, card ini tidak tampil.
- **AC7.2** `PUT /api/dashboard/settings/chat-sender` body `{ "sender": "nudge" | "operator" }` → simpan `ms_user.chatSender`. Validasi:
  - `operator` untuk akun operator → `422` "Akun ini hanya bisa memakai Nudge".
  - `nudge` saat belum aktivasi → `409` `{ "error": "chat_app_not_activated" }`.
  - `nudge` saat fitur app nonaktif → `422`.
- **AC7.3** Memilih **Nudge** saat belum aktivasi: frontend tidak memanggil PUT, tetapi membuka dialog "Aktifkan Nudge dulu" dengan tombol 1-klik (ke `GET /auth/chat-app/activate?then=sender`) dan cara manual. Callback dengan `then=sender` menyimpan `chatSender = 'nudge'` setelah aktivasi berhasil. Untuk cara manual, user memilih ulang setelah aktivasi (status di-refetch saat jendela kembali fokus).
- **AC7.4** Akun operator: card menampilkan **Nudge (app)** saja, terpilih dan non-aktif, dengan keterangan "Akun operator tidak bisa menerima DM dari dirinya sendiri." Kalau belum aktivasi, ditambah tombol aktivasi.
- **AC7.5** Perubahan pilihan berlaku untuk pesan berikutnya (tidak ada pesan yang dikirim ulang). Pesan konfirmasi dikirim lewat pengirim **baru**: "✅ Mulai sekarang notifikasi Nudge dikirim dari sini."
- **AC7.6** Mute, jam tenang, saklar utama, dan cuti berlaku sama untuk kedua pengirim.
- **AC7.7** Fitur app nonaktif (AC1.7) → card tidak tampil.

**US-5 — Slash command native** *(fase 2)*
- **AC5.1** Slash command didaftarkan di Configuration GCP:

  | ID | Command | Argumen |
  |---|---|---|
  | 1 | `/n-mytickets` | — |
  | 2 | `/n-budget` | `<issueKey>` |
  | 3 | `/n-overdue` | — |
  | 4 | `/n-team` | — |
  | 5 | `/n-help` | — |
  | 6 | `/n-geser` | `<issueKey> <todo\|code-review>` |

  Bila console menolak tanda hubung, nama tanpa tanda hubung dipakai (`/nmytickets`, dst.) dan ID tetap sama.
- **AC5.2** `MESSAGE` dengan `message.slashCommand.commandId` dipetakan ke teks command baku lalu ke `RouteCommand`. Untuk ID 6, `argumentText` `CORE-123 todo` diubah menjadi `/n-geser<CORE-123>:todo` (format teks lama yang dikenali `chat_interactive`, dengan kurung siku). Argumen salah dibalas format yang benar.
- **AC5.3** Command teks lama (`/n-geser<CORE-123>:todo`, dll.) tetap diterima.
- **AC5.4** `/n-help` dan pesan sambutan menampilkan format slash command baru.

**US-6 — Tombol aksi** *(fase 3)*
- **AC6.1** Reminder (tier, overage, budget habis) yang dikirim lewat app memakai `cardsV2` (+ `text` ringkas sebagai cadangan notifikasi):
  - header `<issueKey> — <judul reminder>`
  - widget berisi baris info yang sama dengan `BuildMessageText`
  - tombol **Geser ke Code Review** dan **Geser ke To Do** (hanya target yang valid untuk project & status tiket, aturan sama dengan `/n-geser`), serta **Buka di Jira** (`openLink`)
- **AC6.2** Notif perubahan status lewat app memakai card dengan tombol **Buka di Jira** saja.
- **AC6.3** `CARD_CLICKED` dengan `action.actionMethodName = "geser"`, parameter `issueKey` & `target`:
  - validasi pengklik adalah assignee tiket (sama dengan `handleGeser`);
  - jalankan transisi (credential pribadi bila ada);
  - balas `actionResponse.type = UPDATE_MESSAGE` dengan card yang tombol geser-nya dihapus dan berisi baris `✅ Digeser ke <status> oleh <nama> · HH:mm WIB`.
- **AC6.4** Bila transisi tidak tersedia / tiket di luar sprint aktif / bukan milik pengklik: card di-update dengan `⚠️ Tidak bisa digeser: <alasan>` dan tombol geser dihapus (kecuali alasan "bukan milik kamu": card **tidak** diubah, dan balasan berupa pesan privat `privateMessageViewer`).
- **AC6.5** Pesan lewat akun Yusuf (pilihan maupun cadangan AC1.4b) tetap teks biasa tanpa tombol.

---

## 🔀 Cross-repo Impact

| Repo | Impact | Detail | Priority |
|------|--------|--------|----------|
| `nudge` (Go backend) | `YES` | modul baru `chat_app`, `chat_app_activation`, `chat_delivery`; wiring `main.go`, `router.go`; `reminder_engine`, `status_watch` (map `ErrRecipientNotActivated` → skipped); `registration` repo; `admin_dashboard`; config | `HIGH` |
| `nudge/frontend` (Angular) | `YES` | banner aktivasi di dashboard shell, toast hasil callback, card **Pengirim pesan** di Pengaturan & Admin › Detail User, kolom/filter/statistik admin | `HIGH` |
| GCP project `jira-u-bot` | `YES` | redirect URI baru `https://nudge.labcup.tech/auth/chat-app/callback` di OAuth client Sign-In; slash command (fase 2); daftar Visibility | `HIGH` |
| Server produksi | `YES` | file key `shared/data/chat-app-sa.json` (chmod 600), env baru di `shared/.env` | `HIGH` |
| `fnb-backend` / `go-core-api` / `phantom-lancer` | `NO` | — | — |

---

## 🗄️ DB Schema Changes

```sql
-- migrations/0017_chat_app.sql
-- Table   : ms_user
-- Action  : ADD COLUMN
ALTER TABLE ms_user
  ADD COLUMN chatAppSpaceID VARCHAR(255) NULL COMMENT 'Space DM user <-> Nudge Chat App (spaces/xxx); NULL = belum aktivasi',
  ADD COLUMN chatAppUserID VARCHAR(64) NULL COMMENT 'users/<id> Google Chat dari event',
  ADD COLUMN chatAppActivatedDate DATETIME NULL COMMENT 'Waktu aktivasi Nudge App',
  ADD COLUMN chatSender VARCHAR(16) NULL COMMENT 'Pilihan pengirim: nudge | operator; NULL = otomatis (nudge bila aktif)';

ALTER TABLE ms_user
  ADD INDEX idx_ms_user_chatAppSpaceID (chatAppSpaceID);
```

- `googleChatDMSpaceID` (DM lama dengan akun Yusuf) **tidak** diubah, karena dipakai permanen oleh pengirim "Yusuf Wijaya" dan poller.
- `chatSender` sengaja di `ms_user` (global per user), bukan `ms_user_setting` (per board).
- Migrasi diterapkan otomatis saat start (`migrations/migrate.go`). Tidak ada `down` (pola repo); rollback = kolom dibiarkan, karena kolom NULL tidak berdampak ke kode lama.

---

## 📡 API Contract

### `POST /api/chat/events` *(baru, dipanggil Google Chat)*

**Auth:** `Authorization: Bearer <JWT chat@system.gserviceaccount.com>` (AC2.1). Tidak memakai session.

**Request (contoh `MESSAGE`):**
```json
{
  "type": "MESSAGE",
  "eventTime": "2026-11-15T07:00:00Z",
  "space": { "name": "spaces/AAAA", "type": "DM", "spaceType": "DIRECT_MESSAGE" },
  "user": { "name": "users/1234", "email": "yusuf.wijaya@esb.co.id", "displayName": "Yusuf Wijaya" },
  "message": { "name": "spaces/AAAA/messages/BBB", "text": "/n-mytickets", "argumentText": "/n-mytickets",
               "slashCommand": { "commandId": "1" } }
}
```

**Response 200:**
```json
{ "text": "string — balasan, markup Chat" }
```
`CARD_CLICKED` (fase 3):
```json
{ "actionResponse": { "type": "UPDATE_MESSAGE" }, "cardsV2": [ { "cardId": "reminder", "card": { } } ] }
```

**Error Responses:**
| Code | Condition |
|------|-----------|
| `401` | JWT tidak ada / signature salah / `aud` ≠ project number / kedaluwarsa |
| `400` | Body bukan JSON event yang valid |

### `GET /api/dashboard/chat-app/status` *(baru, session user)*

**Response 200:**
```json
{ "enabled": true, "activated": false, "activatedDate": null, "oneClick": true,
  "sender": null, "effectiveSender": "operator", "operatorAccount": false, "operatorName": "Yusuf Wijaya" }
```

### `PUT /api/dashboard/settings/chat-sender` *(baru, session user; juga `PUT /api/dashboard/admin/users/:userID/settings/chat-sender`)*

**Request:**
```json
{ "sender": "nudge | operator" }
```

**Response 200:** sama dengan respons status di atas (sudah ter-update).

**Error Responses:**
| Code | Condition |
|------|-----------|
| `400` | `sender` bukan `nudge`/`operator` |
| `409` | `nudge` dipilih tapi user belum aktivasi (`chat_app_not_activated`) |
| `422` | `operator` untuk akun operator, atau `nudge` saat fitur app nonaktif |

### `GET /auth/chat-app/activate` · `GET /auth/chat-app/callback` *(baru, session user, redirect)*

- `activate[?then=sender]` → `302` ke `accounts.google.com/o/oauth2/v2/auth?...` (AC3.2). `then` disimpan bersama `state`. Tanpa session → `302` ke login.
- `callback?code&state` → `302 /dashboard?chatApp=activated|cancelled|blocked|not_allowed|mismatch|failed` (AC3.3–AC3.4).

### Respons admin yang diubah

- `GET /api/dashboard/admin/users` & detail user: tambah `chatAppActivated: bool`, `chatAppActivatedDate: string|null`, `chatSender: string|null`, `effectiveChatSender: string`. Filter "Belum aktivasi" dilakukan di frontend (daftar user sudah dimuat penuh); `?chatApp=inactive` di URL halaman Admin › User hanya mengisi awal checkbox filter.
- `GET /api/dashboard/admin/users/:userID/chat-app/status` dan `PUT /api/dashboard/admin/users/:userID/settings/chat-sender`: bentuk sama dengan versi user; perubahan memicu change notice DM ("Admin X mengubah pengirim pesan Nudge kamu.").
- `/auth/chat-app/*` dipasang di balik `AuthMiddlewareAPI`, sehingga tanpa session menjawab `401` JSON (bukan redirect ke login). Ini hanya dibuka dari dashboard oleh user yang sudah login.
- Statistik admin dashboard: tambah `chatAppInactiveCount: number`.

---

## 🔐 Access Control Matrix

| Action | Siapa | Behavior if Denied |
|--------|-------|--------------------|
| `POST /api/chat/events` | Google Chat (JWT valid) | `401` |
| Status & aktivasi Nudge App | User login (`@esb.co.id`), hanya untuk dirinya sendiri (email dari session) | redirect login / `401` |
| Lihat status aktivasi user lain, filter & statistik | `ADMIN_EMAIL` (`AdminMiddleware`) | `403` |
| Menggunakan app di Google Chat | Email yang ada di daftar **Visibility** Configuration GCP | App tidak bisa ditemukan / `spaces.setup` 403 |
| Geser tiket lewat tombol | Assignee tiket saja | pesan privat "bukan tiket kamu" |

> Tidak ada `lk_accesscontrol` (bukan ESB Core).

---

## 🔄 Sequence Diagram

```mermaid
sequenceDiagram
  actor User
  participant UI as Dashboard (Angular)
  participant API as Nudge (Go)
  participant G as Google OAuth
  participant Chat as Google Chat API
  participant DB as MySQL

  Note over User,DB: Aktivasi 1-klik (fase 1)
  User->>UI: klik "Aktifkan Nudge di Google Chat"
  UI->>API: GET /auth/chat-app/activate
  API->>G: redirect consent (chat.spaces.create, login_hint)
  G-->>API: GET /auth/chat-app/callback?code&state
  API->>G: tukar code → access token (online)
  API->>Chat: POST spaces:setup {singleUserBotDm:true} (token user)
  Chat-->>API: spaces/AAAA
  API->>DB: UPDATE ms_user SET chatAppSpaceID...
  Chat-)API: POST /api/chat/events ADDED_TO_SPACE (idempoten)
  API-->>User: redirect /dashboard?chatApp=activated

  Note over User,DB: Reminder (fase 1)
  API->>DB: chatSender, chatAppSpaceID penerima
  API->>API: pengirim efektif (operator → nudge; pilihan; otomatis)
  alt nudge & aktif
    API->>Chat: POST spaces/AAAA/messages (token service account)
  else operator
    API->>Chat: DM lama (OAuth Yusuf) [+ footer bila belum pernah memilih]
  else nudge tapi app error / belum aktif (bukan operator)
    API->>Chat: cadangan DM lama (OAuth Yusuf) + footer "app Nudge sedang gangguan"
  else operator account belum aktif
    API->>DB: tr_reminderlog deliveryStatus=skipped
  end

  Note over User,DB: Command (fase 1/2)
  User->>Chat: /n-mytickets
  Chat->>API: POST /api/chat/events MESSAGE (JWT)
  API->>API: verifikasi JWT → RouteCommand
  API-->>Chat: {"text": "..."}
```

---

## 🧠 State Management & UI Logic

```
Banner aktivasi (dashboard shell)
  Default state : disembunyikan sampai GET /api/dashboard/chat-app/status selesai
  enabled=false atau activated=true → tidak tampil
  operatorAccount & !activated → tampil, tidak bisa ditutup
  sender=null & !activated → tampil, "Nanti saja" = sembunyi 7 hari (localStorage)
  sender='operator' (pilihan sadar) → tidak tampil
  Query ?chatApp=<kode> → toast sesuai kode, hapus query dari URL, refetch status
  oneClick=false → tombol diganti "Buka Google Chat"

Card "Pengirim pesan" (Pengaturan › Semua board, Admin › Detail User)
  Default state : radio terpilih = effectiveSender
  operatorAccount → hanya "Nudge (app)", disabled (+ tombol aktivasi bila belum aktif)
  Pilih "Yusuf Wijaya" → PUT sender=operator → toast
  Pilih "Nudge" & activated → PUT sender=nudge → toast
  Pilih "Nudge" & !activated → radio kembali ke nilai lama, dialog "Aktifkan Nudge dulu"
                               (1-klik ?then=sender / cara manual)
  Jendela kembali fokus → refetch status
  enabled=false → card tidak tampil

Router chat_delivery (per pesan)
  pengirim efektif = operator account ? nudge : (chatSender ?? (aktif ? nudge : operator))
  nudge & chatAppSpaceID ada → kirim lewat app
    403/404 → kosongkan space + chatSender=NULL → cadangan
    error lain → cadangan
  nudge & chatAppSpaceID kosong → cadangan (operator account → ErrRecipientNotActivated/skipped)
  cadangan = bukan operator account ? DM lama + footer "app Nudge sedang gangguan" : failed
  operator → DM lama (+ footer bila chatSender NULL & belum aktif)
           → OAuth operator tidak ada → ErrRecipientNotActivated (skipped)

Admin › User
  Kolom Nudge App: ✓ dd MMM yyyy | —
  Filter "Belum aktivasi" → ?chatApp=inactive
```

`oneClick` di respons status berasal dari env `GOOGLE_CHAT_APP_ONE_CLICK` (default `true`, diset `false` bila spike gagal).

---

## ✅ Breakdown Task & Effort

> Development workflow: AI generates code → Developer verifies and fixes.

| # | Task | Repo | AC Coverage | 🤖 AI | 👨‍💻 Dev Verify | Blocked By |
|---|------|------|-------------|-------|----------------|------------|
| 0 | Spike aktivasi 1-klik: script kecil (throwaway) consent `chat.spaces.create` + `spaces.setup singleUserBotDm` dengan akun operator; catat hasil | `nudge` | AC3.6 | `~2m` | `~30m` | — |
| 1 | Migration `0017_chat_app.sql` + repo `registration`: `SetChatAppSpace`, `ClearChatAppSpaceBySpace`, `ClearChatAppSpaceByEmail` (sekaligus reset `chatSender`), `SetChatSender`, field di DTO user | `nudge` | AC2.2, AC2.3, AC1.3, AC7.2 | `~1m` | `~15m` | — |
| 2 | Config: `GOOGLE_CHAT_APP_CREDENTIALS_FILE`, `GOOGLE_CHAT_APP_PROJECT_NUMBER`, `GOOGLE_CHAT_APP_ONE_CLICK`, `GOOGLE_CHAT_APP_ACTIVATION_REDIRECT_URL` + `.env.example` + test | `nudge` | AC1.7 | `~1m` | `~10m` | — |
| 3 | `chat_app_client`: token source service account (`google.CredentialsFromJSON`, scope `chat.bot`), `CreateMessage(space, text)`, `CreateCardMessage`, error bertipe untuk 403/404 + test httptest | `nudge` | AC1.2, AC1.3 | `~2m` | `~30m` | #2 |
| 4 | Verifier JWT Chat (sertifikat x509, cache, `iss`/`aud`/`exp`) + test (valid, aud salah, kedaluwarsa, signature salah, kid baru) | `nudge` | AC2.1 | `~2m` | `~45m` | #2 |
| 5 | `chat_app_service` + `chat_app_handler`: dispatch `ADDED_TO_SPACE`/`REMOVED_FROM_SPACE`/`MESSAGE`, user tidak terdaftar/nonaktif, timeout 25 detik + async, recover; route `POST /api/chat/events` | `nudge` | AC2.2–AC2.7 | `~3m` | `~1h` | #1, #3, #4 |
| 6 | `chat_delivery` router (`SendReminder`, `SendPlainTextDM`, pengirim efektif per penerima, footer, reset saat 403/404, cadangan ke Yusuf saat app error, `ErrRecipientNotActivated`) + tabel test | `nudge` | AC1.1–AC1.4b | `~2m` | `~45m` | #1, #3 |
| 7 | Wiring: `reminder_engine` & `status_watch` memakai router (hapus `PickNotifier`/webhook), map `ErrRecipientNotActivated` → `skipped`; `registration_service` & `admin_dashboard_service` memakai router; poller tetap jalan; update test | `nudge` | AC1.5, AC1.6, AC2.8 | `~3m` | `~1h` | #6 |
| 8 | `chat_app_activation`: `GET /auth/chat-app/activate[?then=sender]` + `callback` (state cookie, exchange, cek email, `spaces.setup`, simpan sender bila `then=sender`, redirect kode) + `GET /api/dashboard/chat-app/status` + test | `nudge` | AC3.1–AC3.4a | `~3m` | `~1h` | #0, #1 |
| 9 | Frontend: service status, banner di dashboard shell (aturan operator / "Nanti saja"), dialog cara manual, toast `?chatApp=` + spec | `frontend` | AC3.5 | `~3m` | `~45m` | #8 |
| 10 | Admin: field aktivasi & pengirim di list/detail user, filter `?chatApp=inactive`, statistik; frontend kolom/filter/stat + spec | `nudge` + `frontend` | AC4.1–AC4.4 | `~3m` | `~45m` | #1 |
| 10a | Pilihan pengirim: `PUT /api/dashboard/settings/chat-sender` (+ versi admin, change notice), validasi 409/422, pesan konfirmasi lewat pengirim baru + test; frontend card **Pengirim pesan** di Pengaturan (Semua board) & Admin › Detail User, dialog "Aktifkan Nudge dulu" + spec | `nudge` + `frontend` | AC7.1–AC7.7, AC4.3 | `~3m` | `~1h` | #6, #8 |
| 11 | README + deploy notes (key di `shared/data`, env, redirect URI, daftar Visibility) | `nudge` | — | `~1m` | `~10m` | #7, #8, #10a |
| 12 | **Fase 2** — slash command: mapping `commandId` → teks baku, parser argumen `/n-geser`, update `/n-help` & sambutan + test; daftar command di GCP | `nudge` | AC5.1–AC5.4 | `~2m` | `~45m` | #5 |
| 13 | **Fase 3** — builder card reminder & status (`cardsV2`), tombol berdasarkan target valid; router memakai card untuk jalur app | `nudge` | AC6.1, AC6.2, AC6.5 | `~3m` | `~1h` | #6 |
| 14 | **Fase 3** — `CARD_CLICKED` "geser": validasi assignee, transisi, `UPDATE_MESSAGE`, pesan privat + test | `nudge` | AC6.3, AC6.4 | `~3m` | `~1h` | #5, #13 |
| 15 | Uji manual produksi per fase (aktivasi, pilihan pengirim, command di kedua DM, slash command, tombol, uninstall) | — | semua | — | `~1h` | #11, #12, #14 |

> **🤖 AI generates code: ~0.7h**
>
> **👨‍💻 Dev verify & fix: ~12h**
>   — setup GCP (redirect URI, slash command, Visibility), menaruh key di server, uji event nyata dari Google, menyesuaikan bentuk event/card yang tidak sesuai dokumentasi
>
> **Combined: ~12.7h** (~1.6 hari kerja; fase 1 ≈ 8h, fase 2 ≈ 1h, fase 3 ≈ 2.5h)
>
> ⚠️ Uncovered ACs: —

### Dependency Chain
```
#0 ─────────────────────────┐
#1 ─┬─────────────┬─────────┼─► #8 ─► #9
    │             │         │
#2 ─┼─► #3 ─┬─► #6 ─► #7 ─┐ │
    └─► #4 ─┤             ├─► #11 ─► #15
            └─► #5 ─► #12 ┘
#1 ─► #10                 #6 ─► #13 ─► #14 (dengan #5)
#6 + #8 ─► #10a ─► #11
```

---

## 🔗 Dependencies

**Internal:**
- `chat_interactive.RouteCommand` dan logika `handleGeser` (dipakai ulang, tidak diubah perilakunya kecuali `/n-help`)
- `registration_repository` (lookup user by email/space)
- `chat_dm_notifier` + `chat_oauth` + `chat_command_poller` (pengirim "Yusuf Wijaya", permanen)
- `auth` session (aktivasi) dan OAuth client Sign-In (`GOOGLE_SIGNIN_CLIENT_ID/SECRET`)

**Cross-repo:** —

**External:**
- Google Chat API (`chat.googleapis.com/v1`): `spaces.messages.create`, `spaces.setup` (`singleUserBotDm`), events HTTP
- Sertifikat publik `chat@system.gserviceaccount.com`
- GCP project `jira-u-bot`: Chat App config (nama Nudge, avatar `https://nudge.labcup.tech/icon-512.png`, endpoint `https://nudge.labcup.tech/api/chat/events`, audience Project Number, Visibility), service account `nudge-app@jira-u-bot.iam.gserviceaccount.com`
- Library: `golang.org/x/oauth2`, `golang.org/x/oauth2/google` (sudah ada di `go.mod`); verifikasi JWT memakai `crypto/*` stdlib

---

## 🚀 Milestone / Deployment Strategy

1. **Pre-deploy (sekali):**
   - Upload key ke server `/var/www/nudge/shared/data/chat-app-sa.json` (`chmod 600`, owner user service).
   - Tambah ke `shared/.env`:
     - `GOOGLE_CHAT_APP_CREDENTIALS_FILE=/var/www/nudge/shared/data/chat-app-sa.json`
     - `GOOGLE_CHAT_APP_PROJECT_NUMBER=275006361614`
     - `GOOGLE_CHAT_APP_ACTIVATION_REDIRECT_URL=https://nudge.labcup.tech/auth/chat-app/callback`
     - `GOOGLE_CHAT_APP_ONE_CLICK=true|false` sesuai hasil spike
   - Tambah redirect URI tersebut di OAuth client Sign-In (GCP › Credentials).
   - Ubah Markup syntax Chat App ke syntax Google Chat (bila belum).
2. **Fase 1 deploy** (push `main` → workflow deploy). Migrasi `0017` otomatis.
3. Operator aktivasi lebih dulu, lalu uji: reminder, notif status, command, uninstall/install ulang.
4. Tambah email tim ke **Visibility** Chat App, lalu kirim broadcast ajakan aktivasi dari Admin (terkirim lewat Yusuf Wijaya dengan footer bagi yang belum memilih).
5. User bebas memilih pengirim di Pengaturan. Tidak ada tanggal pematian; jalur Yusuf Wijaya permanen.
6. **Fase 2** lalu **fase 3** di-deploy terpisah. Slash command didaftarkan di GCP tepat sebelum deploy fase 2.
7. **Pembersihan (di luar spec ini):** hanya `GOOGLE_CHAT_WEBHOOK_URL` dan `google_chat_notifier_service.PickNotifier`/webhook sender. `chat_dm_notifier`, `chat_command_poller`, `chat_oauth` **tetap** dipakai.
8. **Rollback plan:** deploy release sebelumnya (`deploy/remote-deploy.sh` menyimpan 5 release). Kolom baru NULL-able, jadi tidak perlu revert migrasi. Webhook tetap tersedia di config lama selama pembersihan belum jalan.

---

## ⚠️ Drawbacks / Risks

| Category | Risk |
|----------|------|
| **Ketergantungan akun pribadi** | Project GCP dimiliki Gmail pribadi (`diosysdev@gmail.com`). Jika akun itu hilang/terkunci, Chat App ikut mati. Mitigasi: `yusuf.wijaya@esb.co.id` sudah Owner/Admin. Pindahkan ke project org ESB jika suatu saat diberi akses. |
| **Unverified app** | Consent 1-klik (scope Chat, sensitif) di project External bisa menampilkan "Google hasn't verified this app", dibatasi 100 user, atau diblok kebijakan ESB. Mitigasi: spike #0 dan cara manual sebagai jalur utama cadangan. |
| **Visibility manual** | Setiap user baru harus ditambahkan ke daftar Visibility di GCP oleh operator. Jika lupa, aktivasi gagal (`not_allowed`). Mitigasi: pesan error jelas. Bisa diganti satu Google Group bila tersedia. |
| **Keamanan key** | Key service account bisa mengirim pesan atas nama Nudge ke semua DM-nya. Mitigasi: hanya di `shared/data` (600), tidak di repo/CI, rotasi bila bocor (hapus key di GCP). |
| **Spoofing event** | Endpoint publik. Mitigasi: verifikasi JWT wajib sebelum parsing (AC2.1). |
| **Pesan ganda** | Cadangan dijalankan untuk semua error app (AC1.4b), termasuk timeout/5xx. Kalau Google sebenarnya sudah menerima pesan sebelum timeout, user bisa menerima pesan **dua kali** (dari Nudge dan dari Yusuf). Ini diterima sesuai keputusan user (lebih baik ganda daripada hilang). Mitigasi: timeout kirim app 10 detik, dan cadangan hanya sekali per pesan. |
| **Gangguan Nudge tidak terlihat** | Karena ada cadangan, gangguan app bisa tidak disadari. Mitigasi: log per kegagalan + footer "app Nudge sedang gangguan" di pesan cadangan. |
| **Dua jalur permanen** | OAuth akun Yusuf (refresh token di `shared/data`) dan poller tetap harus dirawat selamanya. Jika token OAuth dicabut/kedaluwarsa, user yang memilih Yusuf Wijaya tidak dapat notif. Mitigasi: log error kirim, dan pengirim Nudge tetap tersedia. |
| **Tiket operator** | Akun operator hanya lewat Nudge. Bila operator belum aktivasi, reminder tiketnya di-skip. Mitigasi: banner wajib untuk operator, dan operator aktivasi pertama kali (langkah deploy 3). |
| **Batas 30 detik event** | Jira lambat saat geser → balasan async (AC2.6). |
| **Bentuk event berbeda dari dokumentasi** | Struktur `space.type`/`spaceType`, `slashCommand`, `CARD_CLICKED` bisa bervariasi. Mitigasi: log body event (tanpa token) saat development, test dengan payload nyata. |

---

## ❓ Unresolved / Pending Clarification

| # | Location | Contradiction / Ambiguity | Assumption Made |
|---|----------|---------------------------|-----------------|
| 1 | AC3.2–AC3.4 | Belum diuji apakah consent `chat.spaces.create` dari project External lolos kebijakan ESB dan apakah `singleUserBotDm` berhasil. | Diputuskan oleh spike #0. Jika gagal, `GOOGLE_CHAT_APP_ONE_CLICK=false` dan hanya cara manual. |
| 2 | AC5.1 | Belum dipastikan nama slash command boleh mengandung `-`. | Coba `/n-…` dulu; jika ditolak, nama tanpa tanda hubung dengan ID sama. |
| 3 | AC1.1a | Default untuk user yang belum pernah memilih pengirim. | Otomatis: Nudge bila sudah aktivasi, Yusuf Wijaya bila belum. Pilihan eksplisit tidak pernah diubah sistem, kecuali reset saat app dicopot. |
| 4 | Visibility | Apakah ESB punya Google Group tim yang bisa dipakai di Visibility? | Pakai daftar email satu per satu. |
| 5 | Markup syntax | Configuration Chat App masih `MARKDOWN`. | Diubah ke syntax Google Chat. Bila tidak tersedia, pesan app dikirim dengan `formattedText`-kompatibel (dicek saat task #3). |

> **Future possibilities:** App Home tab (ringkasan tiket), dialog `/n-cuti` untuk isi cuti dari Chat, link preview tiket Jira, publikasi ke Marketplace internal bila project dipindah ke org ESB.

---

## 📁 Files to Modify

| File | Change |
|------|--------|
| `migrations/0017_chat_app.sql` | **Baru** — kolom `chatAppSpaceID`, `chatAppUserID`, `chatAppActivatedDate` + index |
| `config/config.go`, `config/config_test.go`, `.env.example` | Env baru (task #2) |
| `modules/chat_app/chat_app_client/*` | **Baru** — client service account + create message/card |
| `modules/chat_app/chat_app_auth/*` | **Baru** — verifier JWT event Chat |
| `modules/chat_app/chat_app_dto/*` | **Baru** — struct event & respons (text, cardsV2, actionResponse) |
| `modules/chat_app/chat_app_service/*` | **Baru** — dispatch event, slash command, card click |
| `modules/chat_app/chat_app_handler/*` | **Baru** — `POST /api/chat/events` |
| `modules/chat_app_activation/*` | **Baru** — activate/callback/status, `PUT .../settings/chat-sender` |
| `modules/chat_delivery/*` | **Baru** — router (pengirim efektif) + `ErrRecipientNotActivated` |
| `modules/user_dashboard/user_dashboard_handler/*` (SettingsHandler) | Route chat-sender untuk user & admin (`AdminTarget`, change notice) |
| `frontend/src/app/features/dashboard/settings/*` | Card **Pengirim pesan** + dialog aktivasi |
| `modules/registration/registration_repository/*`, `registration_dto` | Method & field aktivasi Chat App |
| `modules/reminder_engine/reminder_engine_service/reminder_engine_service_impl.go` (+test) | Satu notifier (router), hapus `pickNotifier`/webhook, skipped untuk belum aktivasi |
| `modules/status_watch/status_watch_service/status_watch_service_impl.go` (+test) | Sama dengan reminder_engine |
| `modules/registration/registration_service/registration_service_impl.go` | Pakai router |
| `modules/admin_dashboard/admin_dashboard_service/*`, `admin_dashboard_handler/*`, dto | Pakai router; field/filter/statistik aktivasi |
| `modules/google_chat_notifier/google_chat_notifier_service/*` | Builder card (fase 3); `PickNotifier` tidak dipakai lagi |
| `modules/chat_interactive/chat_interactive_service/chat_interactive_service_impl.go` | Teks `/n-help` (fase 2); expose hasil geser terstruktur untuk card (fase 3) |
| `main.go` | Wiring app client, router, handler, aktivasi; poller tetap jalan |
| `router.go` | Route `/api/chat/events`, `/auth/chat-app/*`, `/api/dashboard/chat-app/status` |
| `frontend/src/app/layouts/dashboard-shell/*` | Banner aktivasi + dialog cara manual + toast `?chatApp=` |
| `frontend/src/app/core/services/*` | Service status Nudge App |
| `frontend/src/app/features/admin/*` | Kolom/filter/statistik aktivasi |
| `README.md` | Fitur, setup Chat App, env, deploy |
