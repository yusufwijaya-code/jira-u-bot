# Jira U Bot

Bot Go yang memantau tiket Jira berstatus **Code Process**, menghitung budget waktu dari Story Point, lalu mengirim reminder otomatis ke Google Chat saat mendekati, melewati, atau menyelesaikan budget lebih awal.

## Fitur

- Polling tiket Jira (project OMS & CORE) tiap 10 detik, hanya untuk assignee yang sudah registrasi
- Reminder bertingkat: T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-5 menit, T-1 menit, plus notifikasi overage berulang
- Notifikasi "selesai lebih awal" saat tiket pindah ke Code Review sebelum budget habis
- Elapsed time dihitung berbasis jam kerja (08:30–17:30 WIB, istirahat 12:00–13:00) dan kalender libur
- Routing pesan: tiket milik sendiri → webhook space bersama, tiket orang lain → DM pribadi (via OAuth)
- Command interaktif (`/u-mytickets`, `/u-budget <issueKey>`, `/u-overdue`, `/u-team`, `/u-help`) — dikirim langsung di DM Google Chat biasa (bukan lewat Chat App terpisah), dideteksi lewat polling `spaces.messages.list` memakai OAuth yang sama dengan fitur DM reminder, dibalas atas nama akun yang sama
- Command `/u-geser<issueKey>:todo` dan `/u-geser<issueKey>:code-review` — geser status tiket Jira langsung dari chat (hanya untuk tiket milik si pengirim sendiri), dengan validasi transisi yang tersedia di workflow project sebelum eksekusi; hint-nya otomatis muncul di tiap pesan reminder & overdue
- **Dashboard web** (`/jira-notif/dashboard`, login via Sign in with Google, domain `@esb.co.id` saja):
  - **My Board** — semua tiket Jira milik user yang login, lintas status (bukan cuma Code Process), plus detail budget untuk yang sedang di-track
  - **Reminder preference** — pilih sendiri subset tier reminder (T-2 jam s/d T-1 menit) yang mau diterima
  - **Jira API key pribadi** (opsional) — begitu di-set, aksi transisi status (baik dari dashboard maupun command `/u-geser`) tercatat di Jira atas nama akun sendiri, bukan akun pusat; token disimpan terenkripsi (AES-256-GCM)
  - **Team** / **Overdue** — versi web dari `/u-team` dan `/u-overdue`, data sama persis
  - **Admin Dashboard** (`/jira-notif/dashboard/admin`, khusus `yusuf.wijaya@esb.co.id`) — kelola user terdaftar: aktifkan/nonaktifkan, edit mapping Jira account ID, reset credential pribadi user, hapus user
  - **Sprint Report** (`/jira-notif/dashboard/report`) — pilih satu atau beberapa sprint, lihat Aggregate Deviation, Achievement %, dan perbandingan Estimated vs Actual Time per tiket

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
- `modules/user_dashboard` — My Board, preference, credential setup, Team/Overdue, aksi transisi status
- `modules/admin_dashboard` — manajemen user untuk admin
- `modules/sprint_report` — generate report Estimated vs Actual Time per sprint
