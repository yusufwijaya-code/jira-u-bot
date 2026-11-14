# Nudge

<img src="frontend/public/favicon.svg" width="56" alt="Nudge logo" />

**Nudge** (sebelumnya *Jira U Bot*) — bot Go yang memantau tiket Jira berstatus **Code Process**, menghitung budget waktu dari Story Point, lalu mengirim reminder otomatis ke Google Chat saat mendekati, melewati, atau menyelesaikan budget lebih awal.

## Fitur

- Polling tiket Jira (project OMS & CORE) tiap 10 detik, hanya untuk assignee yang sudah registrasi
- Reminder bertingkat: T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-5 menit, T-1 menit, plus notifikasi overage berulang
- Notifikasi "selesai lebih awal" saat tiket pindah ke Code Review sebelum budget habis
- Elapsed time dihitung berbasis jam kerja (08:30–17:30 WIB, istirahat 12:00–13:00) dan kalender libur
- Routing pesan: tiket milik sendiri → webhook space bersama, tiket orang lain → DM pribadi (via OAuth)
- Command interaktif (`/u-mytickets`, `/u-budget <issueKey>`, `/u-overdue`, `/u-team`, `/u-help`) — dikirim langsung di DM Google Chat biasa (bukan lewat Chat App terpisah), dideteksi lewat polling `spaces.messages.list` memakai OAuth yang sama dengan fitur DM reminder, dibalas atas nama akun yang sama
- Command `/u-geser<issueKey>:todo` dan `/u-geser<issueKey>:code-review` — geser status tiket Jira langsung dari chat (hanya untuk tiket milik si pengirim sendiri), dengan validasi transisi yang tersedia di workflow project sebelum eksekusi; hint-nya otomatis muncul di tiap pesan reminder & overdue
- **Dashboard web** (frontend Angular di `frontend/`, disajikan di `/nudge/`; backend hanya JSON API di `/nudge/api/...`, login via Sign in with Google, domain `@esb.co.id` saja):
  - Layout: sidebar kiri (menu **Setting**), topbar dengan judul halaman dan dropdown user (Admin khusus admin, Logout)
  - **Setting** — **Reminder preference** (pilih subset tier T-2 jam s/d T-1 menit) dan **Jira API key pribadi** (opsional; begitu di-set, aksi transisi `/u-geser` tercatat atas nama akun sendiri; token disimpan terenkripsi AES-256-GCM)
  - **Admin** (khusus `ADMIN_EMAIL`) — statistik user, cari user, aktifkan/nonaktifkan, edit Jira account ID, reset credential pribadi, hapus user, dan broadcast pengumuman lewat DM
  - Semua data **hanya milik user yang login** (di-scope di server lewat email sesi, termasuk admin)

### Brand & UI

- Nama aplikasi: **Nudge**; base path `/nudge` (env `BASE_PATH`), Go module `nudge`
- Ikon: `frontend/public/favicon.svg` (sumber), plus `favicon.ico`, `apple-touch-icon.png`, `icon-192.png`, `icon-512.png` hasil render-nya, dan `manifest.webmanifest`
- Design system di `frontend/src/styles.scss` (token warna indigo `--primary` + aksen amber `--accent`, font Plus Jakarta Sans); komponen UI bersama: `shared/ui/icon.component.ts` (ikon SVG inline), `core/ui/toast.service.ts` + `core/ui/confirm.service.ts` (toast & dialog konfirmasi, di-render oleh `shared/ui/overlays.component.ts`)
- Command bot Google Chat tetap memakai prefix `/u-`

## Tech Stack

- Go
- MySQL
- Jira Cloud REST API v3
- Google Chat API (incoming webhook & Chat API untuk DM + polling command)
- Google Sign-In (OAuth 2.0, domain-restricted)

## Menjalankan

1. Copy `.env.example` ke `.env`, isi kredensial Jira, Google Chat, dan MySQL.
2. (Opsional, untuk fitur DM & command interaktif) `go run . --setup-oauth` sekali untuk otorisasi OAuth Google Chat secara interaktif, lalu isi `GOOGLE_CHAT_SELF_USER_ID` (format `users/<id>`, didapat dari field `sender.name` pada pesan yang pernah dikirim akun tersebut lewat Chat API) di `.env`.
3. Untuk dashboard: buat OAuth Client baru tipe **Web application** di GCP Console (project yang sama dengan OAuth DM), isi `GOOGLE_SIGNIN_CLIENT_ID`/`GOOGLE_SIGNIN_CLIENT_SECRET`/`GOOGLE_SIGNIN_REDIRECT_URL`, `ADMIN_EMAIL`, dan `CREDENTIAL_ENCRYPTION_KEY` (32-byte base64, `openssl rand -base64 32`) di `.env`.
4. `go run .` untuk menjalankan service — migrasi database diterapkan otomatis saat start.

## Struktur

- `modules/poller` — loop polling Jira & deteksi perubahan status tiket
- `modules/reminder_engine` — logika tier reminder, overage, early completion, dan filter preference per-user
- `modules/business_hours` — kalkulasi elapsed/deadline berbasis jam kerja & kalender libur
- `modules/google_chat_notifier` / `modules/chat_dm_notifier` — pengirim pesan ke webhook space & DM
- `modules/chat_interactive` — logika dispatch command (`/u-mytickets`, dll) ke jawaban teks
- `modules/chat_command_poller` — loop polling pesan DM tiap 10 detik, mendeteksi & membalas command
- `modules/chat_command_poll_cursor` — penyimpanan cursor (createTime pesan terakhir diproses) per DM space
- `modules/registration` — repository & service data user (`ms_user`) — dipakai auto-registrasi, bukan lagi form manual
- `modules/jira_client` — client Jira Cloud REST API, termasuk varian `*As` yang jalan atas nama credential pribadi user
- `modules/jira_credential` — penyimpanan & resolusi Jira API key pribadi user, terenkripsi AES-256-GCM
- `modules/auth` — Google Sign-In, session, auto-registrasi, middleware auth/admin
- `modules/user_dashboard` — board, overdue, team, preference & credential setup
- `modules/admin_dashboard` — manajemen user untuk admin
- `modules/sprint_report` — generate report Estimated vs Actual Time per sprint
