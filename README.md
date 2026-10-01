# Lapangin

Sistem booking lapangan olahraga (futsal, badminton, mini soccer, padel) berbasis web.
Satu instalasi bisa dipakai banyak venue; data tiap venue terpisah.

## Fitur

**Customer**
- Pilih lapangan dan tanggal, lihat slot per jam (kosong, terisi, lewat)
- Pilih jam mulai dan durasi; harga dihitung otomatis
- Booking berstatus "menunggu pembayaran" dengan batas waktu bayar
- Halaman status booking per kode, info pembayaran, tombol kirim bukti via WhatsApp
- Cek booking dengan kode, batalkan booking yang belum dibayar

**Pengelola venue** (`/admin`)
- Ringkasan: booking hari ini, jumlah menunggu pembayaran, pendapatan bulan berjalan
- Grid jadwal harian per lapangan, klik sel kosong untuk booking manual
- Daftar booking dengan filter status dan tanggal; tandai lunas atau batalkan
- Booking manual (pesanan lewat WhatsApp, telepon, walk-in), langsung lunas
- Kelola lapangan: nama, jenis olahraga, harga per jam, aktif/nonaktif
- Pengaturan: profil, nomor WhatsApp, info pembayaran, batas waktu bayar, jam buka per hari

**Sistem**
- Anti double booking di level database (`EXCLUDE` constraint PostgreSQL)
- Booking yang tidak dibayar sampai batas waktu otomatis kedaluwarsa
- Booking lunas yang jamnya sudah lewat otomatis ditandai selesai
- Session login disimpan di database (hanya hash token), password di-hash bcrypt
- Batas 10 percobaan login per IP per 15 menit
- Proteksi CSRF bawaan Go (`http.CrossOriginProtection`)

## Tech stack

- **Go**: [chi](https://github.com/go-chi/chi) (router), [pgx](https://github.com/jackc/pgx) (driver PostgreSQL), [goose](https://github.com/pressly/goose) (migrasi)
- **Frontend**: `html/template` + [HTMX](https://htmx.org)
- **Database**: PostgreSQL

## Struktur folder

```
cmd/server/            entry point web server
cmd/manage/            perintah admin platform (seed, create-venue)
internal/booking/      aturan slot, validasi jam, harga (tanpa database)
internal/store/        semua query PostgreSQL
internal/auth/         password, token session, pembatas login
internal/jobs/         pekerjaan rutin di background
internal/server/       router, handler, middleware, helper template
internal/database/     koneksi DB + file migrasi SQL
web/templates/         layout, halaman, partial HTMX
web/static/            CSS dan JS
```

## Menjalankan di lokal

1. Buat database PostgreSQL bernama `lapangin` (atau `docker compose up -d` kalau pakai Docker).
2. Salin konfigurasi lalu sesuaikan `DATABASE_URL`:
   ```bash
   cp .env.example .env
   ```
3. Isi data demo (opsional):
   ```bash
   go run ./cmd/manage seed
   ```
4. Jalankan server (migrasi jalan otomatis):
   ```bash
   go run ./cmd/server
   ```
5. Buka http://localhost:3000

### Akun demo

Dibuat oleh `seed`, hanya untuk development lokal:

- Halaman publik: http://localhost:3000/v/demo
- Login admin: http://localhost:3000/admin/login
- Email `demo@lapangin.test`, password `demo12345`

## Mendaftarkan venue baru (klien)

```bash
go run ./cmd/manage create-venue -slug gor-maju -name "GOR Maju" -owner "Budi" -email budi@contoh.com
```

Password pemilik diminta saat perintah jalan. Venue dibuat dengan jam buka 08:00 sampai 22:00 setiap hari;
pemilik login lalu menambah lapangan dan mengatur info pembayaran di menu Pengaturan.
Halaman customer-nya ada di `/v/gor-maju`.

## Tes

```bash
go test ./...
```

Tes di `internal/store` butuh database khusus tes (isinya dihapus setiap tes jalan):

```bash
createdb lapangin_test
TEST_DATABASE_URL="postgres://user:pass@localhost:5432/lapangin_test?sslmode=disable" go test ./...
```

Tanpa `TEST_DATABASE_URL`, tes database dilewati.

## Build dan deploy

```bash
go build -o bin/lapangin ./cmd/server
```

Template dan file statis sudah ikut di dalam binary. Di server production set:

- `APP_ENV=production` (cookie hanya dikirim lewat HTTPS)
- `APP_ADDR`, `DATABASE_URL`

Jalankan di belakang reverse proxy dengan HTTPS (Caddy atau Nginx).

## Batasan saat ini

- Slot per 1 jam, jam buka maksimal sampai 24:00 (belum mendukung buka lewat tengah malam)
- Pembayaran dikonfirmasi manual oleh pengelola (belum ada payment gateway)
- Satu akun pengelola per venue
