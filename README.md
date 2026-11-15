# Nudge

<img src="frontend/public/favicon.svg" width="56" alt="Nudge logo" />

**Nudge** (sebelumnya *Jira U Bot*) — bot Go yang memantau tiket Jira berstatus **Code Process**, menghitung budget waktu dari Story Point, lalu mengirim reminder otomatis ke Google Chat saat mendekati, melewati, atau menyelesaikan budget lebih awal.

## Fitur

- Polling tiket Jira (project OMS & CORE) tiap 10 detik, hanya untuk assignee yang sudah registrasi dan hanya **selama tiket ada di sprint aktif** (subtask ikut sprint parent-nya); session tiket yang keluar dari sprint aktif ditutup tanpa menambah waktu
- **Backfill dari changelog Jira** (`modules/sprint_backfill`): tiket sprint aktif (+ subtask/linked milik user) yang tidak sempat tercatat poller dihitung ulang dari riwayat status (jam kerja, aturan per project) dan disimpan sebagai session tertutup (`createdBy = 'backfill'`) — otomatis saat start & tiap hari 07:00 WIB untuk semua user, atau lewat tombol "Hitung ulang dari Jira" (`POST /api/dashboard/report/backfill`)
- Reminder bertingkat yang bisa diatur per user: default T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-5 menit, T-1 menit, lalu notifikasi "budget habis" dan overage berulang tiap 5 menit
- **Notifikasi perubahan status tiket** (`modules/status_watch`): tiap 10 detik satu JQL (`sprint in openSprints() AND assignee in (<user terdaftar>) AND updated >= <cursor>`) dibandingkan dengan snapshot `tr_issuestatus`; setiap status tiket sprint aktif milik user berubah — digeser di Jira, Papan Tiket, atau `/n-geser` — user dapat DM "To Do → Code Process · oleh <pelaku> · jam" (pelaku dari changelog Jira). Bila tiket keluar dari status yang dihitung, pesan ditambah info waktu ("Selesai X lebih awal dari budget" / "Waktu terpakai X dari budget Y"), menggantikan notif lama "selesai lebih awal" & "balik ke To Do". Tiket yang baru pertama terlihat tidak dinotif; tidak dikirim saat saklar utama OFF, mute, jam tenang, atau cuti
- Aturan tracking per project (`jira_client_dto/tracking_rules.go`): OMS dihitung hanya saat **Code Process**; CORE dihitung saat **Code Process + CODE REVIEW** dan berhenti saat keluar dari keduanya (reminder tetap dikirim selama review, pesannya ditandai "CODE REVIEW")
- Elapsed time dihitung berbasis jam kerja (08:30–17:30 WIB, istirahat 12:00–13:00) dan kalender **per user** = libur nasional + hari cuti user itu (`modules/user_leave`, tabel `tr_user_leave`). Hari cuti tidak dihitung ke waktu tiket, deadline mundur, dan reminder tidak dikirim ke user itu; menambah/menghapus cuti di tanggal yang sudah lewat menghitung ulang `accumulatedSeconds` session sprint aktif user dari changelog Jira (`editedBy = 'leave-recalc'`)
- Routing pesan: tiket milik sendiri → webhook space bersama, tiket orang lain → DM pribadi (via OAuth)
- Command interaktif (`/n-mytickets`, `/n-budget <issueKey>`, `/n-overdue`, `/n-team`, `/n-help`) — dikirim langsung di DM Google Chat biasa (bukan lewat Chat App terpisah), dideteksi lewat polling `spaces.messages.list` memakai OAuth yang sama dengan fitur DM reminder, dibalas atas nama akun yang sama
- Command `/n-geser<issueKey>:todo` dan `/n-geser<issueKey>:code-review` — geser status tiket Jira langsung dari chat (hanya untuk tiket milik si pengirim sendiri), dengan validasi transisi yang tersedia di workflow project sebelum eksekusi (tidak lagi diiklankan di pesan reminder; tetap tercantum di `/n-help`)
- **Dashboard web** (frontend Angular di `frontend/`, disajikan di `/nudge/`; backend hanya JSON API di `/nudge/api/...`, login via Sign in with Google, domain `@esb.co.id` saja):
  - **Per board Jira**: pemilih board di navbar (ikon project dari Jira, via proxy `GET /api/dashboard/boards/:id/avatar`) berisi "Semua board" + board yang sprint aktifnya berisi tiket kamu. Board tiket = board dari sprint **aktif**-nya (field sprint Jira). Dashboard & Pengaturan mengikuti board terpilih (`?board=` di URL, diingat di browser)
  - Pengaturan per board: "Semua board" = pengaturan default; board tanpa pengaturan khusus mengikuti default (tombol **Atur khusus board ini** / **Kembalikan ke default**); mute dua level (semua board + per board). Disimpan di `ms_user_setting` dengan kunci `(userID, boardID)`, `boardID = 0` = default; cache board di `ms_board`
  - Layout: sidebar kiri (menu **Dashboard**, **Papan Tiket**, **Cuti**, dan **Pengaturan**), topbar dengan judul halaman dan dropdown user (foto profil, Admin khusus admin, Logout)
  - **Dashboard** (halaman awal) — sapaan + status notif, 4 statistik (sedang dikerjakan, lewat budget, total tiket, achievement sprint), tiket Code Process dengan progress bar budget (hijau/oranye ≥80%/merah lewat budget) dan tombol geser ke Code Review / To Do, semua tiket per status (link ke Jira), dan ringkasan sprint aktif (achievement, dalam budget, estimasi vs aktual, plus **semua tiket sprint** urut deviasi terbesar dengan link ke Jira; "–" bila belum ada tiket tercatat). Untuk board CORE ada card **Sedang di-review** (tiket CODE REVIEW beserta budget-nya) di bawah "Sedang dikerjakan". **Subtask & linked item** (semua tipe link, milik kamu) dari tiket sprint aktif tampil sebagai cabang di bawah parent-nya: di card Sedang dikerjakan/di-review (yang berstatus dihitung, lengkap dengan budget) dan di "Tiket saya per status" (semua status, tidak menambah jumlah) — diambil dengan satu JQL tambahan `key in (…) AND assignee = kamu`
  - **Papan Tiket** — tiket kamu di sprint aktif per kolom status (ikut pemilih board). Klik chip status di card → daftar transisi yang **diizinkan workflow Jira tiket itu** (OMS dan CORE berbeda, mis. OMS di To Do hanya "Start Development → Code Process"), dijalankan dengan Jira credential kamu. Endpoint: `GET /api/dashboard/tickets/:issueKey/transitions`, `POST /api/dashboard/tickets/:issueKey/transition` body `{ "transitionId": "11" }` (body lama `{ "target": "todo|code-review" }` tetap didukung). Tiket di luar sprint aktif kamu → 403; transisi yang sudah tidak tersedia → 422
  - **Cuti** — isi cuti hari penuh per rentang tanggal (maks 31 hari, paling lama 60 hari ke belakang; akhir pekan & libur nasional dilewati) dan hapus per hari. Endpoint: `GET/POST /api/dashboard/leaves`, `DELETE /api/dashboard/leaves/:date`
  - **Pengaturan** — setting notifikasi per user (tabel `ms_user_setting`; tanpa baris = default yang sama dengan perilaku lama):
    - **Reminder sebelum budget habis** — pilih tier preset atau tambah tier custom (1 menit–8 jam, maks. 10)
    - **Reminder setelah lewat budget** — tier `T+` (mis. +15 menit, +1 jam), lalu ulang tiap 5/10/15/30/60 menit atau berhenti
    - **Perubahan status tiket** — on/off notif perubahan status (`ms_user_setting.notifyStatusChange`) + pilihan status tujuan yang dikirim (`statusChangeStatuses`, kosong = semua status); daftar status mengikuti workflow project board terpilih. Juga tersedia di Admin › Detail User
    - **Saklar notifikasi** — saklar utama semua notif otomatis (`ms_user.notificationsEnabled`). User baru mulai **OFF**; user lama tetap ON. Selama belum pernah memilih (`notificationChoiceDate` NULL), pop up muncul setelah login dengan toggle awal OFF dan hanya bisa ditutup lewat Simpan (`PUT /api/dashboard/settings/notifications`). OFF diperlakukan seperti mute oleh reminder engine; broadcast admin & balasan command tetap jalan
    - **Mute & jam tenang** — mute 1 jam / 2 jam / sampai besok 08:30, dan jam tenang harian (WIB); reminder yang jatuh tempo saat itu dilewati (`tr_reminderlog.deliveryStatus = skipped`), notif lewat budget langsung lanjut setelahnya. Badge 🔕 muncul di topbar saat mute aktif
    - Kalau beberapa tier terlewati sekaligus, hanya tier paling relevan yang dikirim
  - **Jira API key pribadi** (opsional; begitu di-set, aksi transisi `/n-geser` tercatat atas nama akun sendiri; token disimpan terenkripsi AES-256-GCM)
  - **Foto profil** = foto akun Google (claim `picture` Sign-In, disimpan di `ms_user_picture` tiap login; proxy `GET /api/dashboard/me/avatar`, cache 24 jam; belum pernah login / tanpa foto → huruf inisial)
  - **Admin panel** (khusus `ADMIN_EMAIL`, route `/admin` dengan shell & sidebar sendiri, di-lazy-load; `/dashboard/admin` lama di-redirect). Tombol **Dashboard Utama** di topbar kembali ke `/dashboard`:
    - **Dashboard** — statistik user + daftar pintas user tanpa credential & user nonaktif
    - **User** — cari user, aktifkan/nonaktifkan, edit Jira account ID, reset credential pribadi, hapus user; klik baris → **detail user**: semua pengaturan dinamis milik user itu (notifikasi default & per board, mute global & per board, isi/ganti/hapus Jira credential) memakai komponen Pengaturan yang sama. Endpoint: `/api/dashboard/admin/users/:userID/{settings,settings/notification,settings/mute,credential,boards,avatar}`, bentuk request/respons identik dengan endpoint user (`SettingsHandler` bersama)
    - Setiap perubahan admin pada pengaturan/credential user lain dikirim ke user tsb lewat **DM Google Chat** (best-effort; tidak dikirim bila admin mengubah akunnya sendiri)
    - **Broadcast Message** — pengumuman lewat DM ke semua user aktif
  - Data dashboard & pengaturan user **hanya milik user yang login** (di-scope di server lewat email sesi); hanya endpoint `/api/dashboard/admin/**` (di balik `AdminMiddleware`) yang bisa menyentuh user lain

### Brand & UI

- Nama aplikasi: **Nudge**; base path `/nudge` (env `BASE_PATH`), Go module `nudge`
- Ikon: `frontend/public/favicon.svg` (sumber), plus `favicon.ico`, `apple-touch-icon.png`, `icon-192.png`, `icon-512.png` hasil render-nya, dan `manifest.webmanifest`
- Design system di `frontend/src/styles.scss` (token warna indigo `--primary` + aksen amber `--accent`, font Plus Jakarta Sans); komponen UI bersama: `shared/ui/icon.component.ts` (ikon SVG inline), `core/ui/toast.service.ts` + `core/ui/confirm.service.ts` (toast & dialog konfirmasi, di-render oleh `shared/ui/overlays.component.ts`), dropdown custom `shared/ui/select.component.ts` dan pemilih jam `shared/ui/time-picker.component.ts` (pengganti `<select>`/`<input type="time">` native)
- Command bot Google Chat memakai prefix `/n-` (Nudge; dulu `/u-`, prefix lama tidak dikenali lagi)

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

## Deploy

Produksi: `https://labcup.tech/nudge/` (Ubuntu, nginx, systemd unit `nudge`, MySQL lokal).

- **CI** (`.github/workflows/ci.yml`) — tiap PR dan push ke `feat/**`: `go vet`, `go test` (MySQL 8 service), build backend; `ng test` (ChromeHeadless) dan `ng build` frontend.
- **Deploy** (`.github/workflows/deploy.yml`) — push ke `main` atau manual (*Run workflow*): jalankan CI → build binary Linux + bundle Angular jadi satu `nudge.tar.gz` → `scp` ke server → `deploy/remote-deploy.sh` membuka release baru, memindah symlink `current`, restart service, dan **rollback otomatis** kalau `/nudge/ping` tidak menjawab. Lima release terakhir disimpan.
- **Secrets GitHub** (environment `production`): `SSH_HOST`, `SSH_USER`, `SSH_PRIVATE_KEY`, `SSH_KNOWN_HOSTS` (output `ssh-keyscan <host>`), opsional `SSH_PORT` (default 22).

Layout server:

```
/var/www/nudge/
  current -> releases/<waktu>-<sha>   binary nudge, config/, public/nudge/ (Angular)
  releases/                           5 release terakhir
  shared/.env, shared/data/           konfigurasi & token OAuth — tidak ikut release
```

File server di repo: `deploy/nudge.service` (`/etc/systemd/system/`), `deploy/nginx-nudge.conf` (`/etc/nginx/snippets/nudge.conf`, di-`include` di server block labcup.tech). nginx menyajikan Angular langsung dan meneruskan `/nudge/api|auth|dashboard/admin/` serta `/nudge/ping` ke Go (port `PORT` di `shared/.env`).

## Struktur

- `modules/poller` — loop polling Jira & deteksi perubahan status tiket
- `modules/reminder_engine` — logika tier reminder, overage, early completion, dan filter preference per-user
- `modules/business_hours` — kalkulasi elapsed/deadline berbasis jam kerja & kalender libur
- `modules/google_chat_notifier` / `modules/chat_dm_notifier` — pengirim pesan ke webhook space & DM
- `modules/chat_interactive` — logika dispatch command (`/n-mytickets`, dll) ke jawaban teks
- `modules/chat_command_poller` — loop polling pesan DM tiap 10 detik, mendeteksi & membalas command
- `modules/chat_command_poll_cursor` — penyimpanan cursor (createTime pesan terakhir diproses) per DM space
- `modules/registration` — repository & service data user (`ms_user`) — dipakai auto-registrasi, bukan lagi form manual
- `modules/jira_client` — client Jira Cloud REST API, termasuk varian `*As` yang jalan atas nama credential pribadi user
- `modules/jira_credential` — penyimpanan & resolusi Jira API key pribadi user, terenkripsi AES-256-GCM
- `modules/auth` — Google Sign-In, session, auto-registrasi, middleware auth/admin
- `modules/user_dashboard` — board, overdue, team, preference & credential setup
- `modules/admin_dashboard` — manajemen user untuk admin, termasuk pengaturan user lain (`AdminTarget`) dan DM pemberitahuan perubahan
- `modules/user_avatar` — foto profil Google user: disimpan saat login, di-proxy (hanya `https://*.googleusercontent.com`, tanpa kredensial)
- `modules/sprint_report` — generate report Estimated vs Actual Time per sprint
