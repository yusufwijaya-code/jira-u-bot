# Jira U Bot

Bot Go yang memantau tiket Jira berstatus **Code Process**, menghitung budget waktu dari Story Point, lalu mengirim reminder otomatis ke Google Chat saat mendekati, melewati, atau menyelesaikan budget lebih awal.

## Fitur

- Polling tiket Jira (project OMS & CORE) tiap 30 detik, hanya untuk assignee yang sudah registrasi
- Reminder bertingkat: T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-1 menit, plus notifikasi overage berulang
- Notifikasi "selesai lebih awal" saat tiket pindah ke Code Review sebelum budget habis
- Elapsed time dihitung berbasis jam kerja (08:30–17:30 WIB, istirahat 12:00–13:00) dan kalender libur
- Routing pesan: tiket milik sendiri → webhook space bersama, tiket orang lain → DM pribadi (via OAuth)
- Halaman registrasi web (`/jira-notif/register`) untuk memetakan email ke akun Jira

## Tech Stack

- Go
- MySQL
- Jira Cloud REST API v3
- Google Chat API (incoming webhook & Chat API untuk DM)

## Menjalankan

1. Copy `.env.example` ke `.env`, isi kredensial Jira, Google Chat, dan MySQL.
2. (Opsional, untuk fitur DM) `go run . --setup-oauth` sekali untuk otorisasi OAuth Google Chat secara interaktif.
3. `go run .` untuk menjalankan service — migrasi database diterapkan otomatis saat start.

## Struktur

- `modules/poller` — loop polling Jira & deteksi perubahan status tiket
- `modules/reminder_engine` — logika tier reminder, overage, dan early completion
- `modules/business_hours` — kalkulasi elapsed/deadline berbasis jam kerja & kalender libur
- `modules/google_chat_notifier` / `modules/chat_dm_notifier` — pengirim pesan ke webhook space & DM
- `modules/registration` — halaman dan service registrasi user
- `modules/jira_client` — client Jira Cloud REST API
