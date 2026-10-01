# Lapangin

Sistem booking lapangan olahraga (futsal, badminton, mini soccer, padel) berbasis web.

## Tech stack

- **Go**: [chi](https://github.com/go-chi/chi) (router), [pgx](https://github.com/jackc/pgx) (PostgreSQL driver), [goose](https://github.com/pressly/goose) (migrasi)
- **Frontend**: `html/template` + [HTMX](https://htmx.org)
- **Database**: PostgreSQL

## Struktur folder

```
cmd/server/            entry point aplikasi
internal/config/       baca konfigurasi dari env
internal/database/     koneksi DB + file migrasi SQL
internal/server/       router, handler, helper render
web/templates/         layout, halaman, dan partial HTMX
web/static/            CSS, gambar, JS
```

## Menjalankan di lokal

1. Siapkan PostgreSQL dan buat database `lapangin`
   (atau jalankan `docker compose up -d` kalau pakai Docker).
2. Salin konfigurasi:
   ```bash
   cp .env.example .env
   ```
   Sesuaikan `DATABASE_URL` dengan user/password PostgreSQL kamu.
3. Jalankan server:
   ```bash
   go run ./cmd/server
   ```
   Migrasi database jalan otomatis saat server start.
4. Buka http://localhost:8080

## Build

```bash
go build -o bin/lapangin ./cmd/server
```
