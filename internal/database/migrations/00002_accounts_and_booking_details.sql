-- +goose Up
ALTER TABLE venues
    ADD COLUMN payment_info TEXT NOT NULL DEFAULT '',
    ADD COLUMN hold_minutes INT NOT NULL DEFAULT 60 CHECK (hold_minutes BETWEEN 0 AND 1440),
    ADD COLUMN booking_window_days INT NOT NULL DEFAULT 30 CHECK (booking_window_days BETWEEN 1 AND 365);

ALTER TABLE bookings
    ADD COLUMN code TEXT,
    ADD COLUMN source TEXT NOT NULL DEFAULT 'online' CHECK (source IN ('online', 'admin')),
    ADD COLUMN confirmed_at TIMESTAMPTZ,
    ADD COLUMN cancelled_at TIMESTAMPTZ;

UPDATE bookings SET code = upper(substr(md5(id::text || random()::text), 1, 8)) WHERE code IS NULL;
ALTER TABLE bookings ALTER COLUMN code SET NOT NULL;
ALTER TABLE bookings ADD CONSTRAINT bookings_code_key UNIQUE (code);

-- expired: booking pending yang tidak dibayar sampai hold_expires_at.
ALTER TABLE bookings DROP CONSTRAINT bookings_status_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_status_check
    CHECK (status IN ('pending', 'confirmed', 'cancelled', 'expired', 'completed'));

-- Booking completed juga tetap menempati jadwal, jadi ikut dijaga dari bentrok.
ALTER TABLE bookings DROP CONSTRAINT bookings_no_overlap;
ALTER TABLE bookings ADD CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
    court_id WITH =,
    period WITH &&
) WHERE (status IN ('pending', 'confirmed', 'completed'));

CREATE INDEX bookings_pending_hold_idx ON bookings (hold_expires_at) WHERE status = 'pending';

-- Akun pemilik/admin venue. Satu user terikat ke satu venue.
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    venue_id      BIGINT NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Yang disimpan hanya hash SHA-256 dari token cookie, bukan token aslinya.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
DROP INDEX bookings_pending_hold_idx;

ALTER TABLE bookings DROP CONSTRAINT bookings_no_overlap;
ALTER TABLE bookings ADD CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
    court_id WITH =,
    period WITH &&
) WHERE (status IN ('pending', 'confirmed'));

UPDATE bookings SET status = 'cancelled' WHERE status = 'expired';
ALTER TABLE bookings DROP CONSTRAINT bookings_status_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_status_check
    CHECK (status IN ('pending', 'confirmed', 'cancelled', 'completed'));

ALTER TABLE bookings
    DROP COLUMN cancelled_at,
    DROP COLUMN confirmed_at,
    DROP COLUMN source,
    DROP COLUMN code;

ALTER TABLE venues
    DROP COLUMN booking_window_days,
    DROP COLUMN hold_minutes,
    DROP COLUMN payment_info;
