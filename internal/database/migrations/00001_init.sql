-- +goose Up
-- btree_gist dibutuhkan untuk EXCLUDE constraint (court_id =, period &&).
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Satu venue = satu klien (pemilik GOR / lapangan).
CREATE TABLE venues (
    id         BIGSERIAL PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    phone      TEXT,
    address    TEXT,
    timezone   TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE courts (
    id             BIGSERIAL PRIMARY KEY,
    venue_id       BIGINT NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    sport          TEXT NOT NULL,
    price_per_hour BIGINT NOT NULL CHECK (price_per_hour >= 0),
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX courts_venue_id_idx ON courts (venue_id);

-- weekday: 0 = Minggu ... 6 = Sabtu (sama dengan time.Weekday di Go).
CREATE TABLE opening_hours (
    venue_id   BIGINT NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    weekday    SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    open_time  TIME NOT NULL,
    close_time TIME NOT NULL CHECK (close_time > open_time),
    PRIMARY KEY (venue_id, weekday)
);

CREATE TABLE bookings (
    id              BIGSERIAL PRIMARY KEY,
    court_id        BIGINT NOT NULL REFERENCES courts (id) ON DELETE RESTRICT,
    customer_name   TEXT NOT NULL,
    customer_phone  TEXT NOT NULL,
    period          TSTZRANGE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'confirmed', 'cancelled', 'completed')),
    total_price     BIGINT NOT NULL CHECK (total_price >= 0),
    -- Booking pending yang belum dibayar sampai waktu ini akan dibatalkan otomatis.
    hold_expires_at TIMESTAMPTZ,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Anti double booking di level database: dua booking aktif di lapangan
    -- yang sama tidak boleh punya rentang waktu yang beririsan.
    CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
        court_id WITH =,
        period WITH &&
    ) WHERE (status IN ('pending', 'confirmed'))
);

CREATE INDEX bookings_court_period_idx ON bookings USING gist (court_id, period);

-- +goose Down
DROP TABLE bookings;
DROP TABLE opening_hours;
DROP TABLE courts;
DROP TABLE venues;
