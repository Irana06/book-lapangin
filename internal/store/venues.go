package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"lapangin/internal/booking"
)

type Venue struct {
	ID                int64
	Slug              string
	Name              string
	Phone             string
	Address           string
	Timezone          string
	PaymentInfo       string
	HoldMinutes       int
	BookingWindowDays int
}

var locations sync.Map

// Loc mengembalikan zona waktu venue. Kalau nama zona tidak dikenal,
// dipakai Asia/Jakarta.
func (v Venue) Loc() *time.Location {
	if l, ok := locations.Load(v.Timezone); ok {
		return l.(*time.Location)
	}
	loc, err := time.LoadLocation(v.Timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Jakarta")
	}
	locations.Store(v.Timezone, loc)
	return loc
}

const venueColumns = `id, slug, name, coalesce(phone, ''), coalesce(address, ''), timezone,
	payment_info, hold_minutes, booking_window_days`

func scanVenue(row pgx.Row) (Venue, error) {
	var v Venue
	err := row.Scan(&v.ID, &v.Slug, &v.Name, &v.Phone, &v.Address, &v.Timezone,
		&v.PaymentInfo, &v.HoldMinutes, &v.BookingWindowDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return Venue{}, ErrNotFound
	}
	return v, err
}

func (s *Store) Venues(ctx context.Context) ([]Venue, error) {
	rows, err := s.db.Query(ctx, `SELECT `+venueColumns+` FROM venues ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Venue, error) {
		return scanVenue(row)
	})
}

func (s *Store) VenueBySlug(ctx context.Context, slug string) (Venue, error) {
	return scanVenue(s.db.QueryRow(ctx, `SELECT `+venueColumns+` FROM venues WHERE slug = $1`, slug))
}

func (s *Store) VenueByID(ctx context.Context, id int64) (Venue, error) {
	return scanVenue(s.db.QueryRow(ctx, `SELECT `+venueColumns+` FROM venues WHERE id = $1`, id))
}

func (s *Store) UpdateVenue(ctx context.Context, v Venue) error {
	_, err := s.db.Exec(ctx, `
		UPDATE venues
		SET name = $2, phone = $3, address = $4, payment_info = $5,
		    hold_minutes = $6, booking_window_days = $7
		WHERE id = $1`,
		v.ID, v.Name, v.Phone, v.Address, v.PaymentInfo, v.HoldMinutes, v.BookingWindowDays)
	return err
}

// OpeningHours mengembalikan jam buka per hari. Hari yang tidak ada di map berarti tutup.
func (s *Store) OpeningHours(ctx context.Context, venueID int64) (map[time.Weekday]booking.Hours, error) {
	rows, err := s.db.Query(ctx, `
		SELECT weekday, extract(epoch FROM open_time)::bigint, extract(epoch FROM close_time)::bigint
		FROM opening_hours WHERE venue_id = $1`, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hours := make(map[time.Weekday]booking.Hours)
	for rows.Next() {
		var wd int
		var open, closeAt int64
		if err := rows.Scan(&wd, &open, &closeAt); err != nil {
			return nil, err
		}
		hours[time.Weekday(wd)] = booking.Hours{
			Open:  time.Duration(open) * time.Second,
			Close: time.Duration(closeAt) * time.Second,
		}
	}
	return hours, rows.Err()
}

// HoursOn mengembalikan jam buka pada satu hari. ok bernilai false kalau venue tutup.
func (s *Store) HoursOn(ctx context.Context, venueID int64, wd time.Weekday) (h booking.Hours, ok bool, err error) {
	var open, closeAt int64
	err = s.db.QueryRow(ctx, `
		SELECT extract(epoch FROM open_time)::bigint, extract(epoch FROM close_time)::bigint
		FROM opening_hours WHERE venue_id = $1 AND weekday = $2`, venueID, int(wd)).Scan(&open, &closeAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return booking.Hours{}, false, nil
	}
	if err != nil {
		return booking.Hours{}, false, err
	}
	return booking.Hours{Open: time.Duration(open) * time.Second, Close: time.Duration(closeAt) * time.Second}, true, nil
}

func (s *Store) SetOpeningHours(ctx context.Context, venueID int64, hours map[time.Weekday]booking.Hours) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		return insertHours(ctx, tx, venueID, hours, true)
	})
}

func insertHours(ctx context.Context, tx pgx.Tx, venueID int64, hours map[time.Weekday]booking.Hours, replace bool) error {
	if replace {
		if _, err := tx.Exec(ctx, `DELETE FROM opening_hours WHERE venue_id = $1`, venueID); err != nil {
			return err
		}
	}
	for wd, h := range hours {
		_, err := tx.Exec(ctx, `
			INSERT INTO opening_hours (venue_id, weekday, open_time, close_time)
			VALUES ($1, $2, $3::time, $4::time)`,
			venueID, int(wd), clock(h.Open), clock(h.Close))
		if err != nil {
			return err
		}
	}
	return nil
}

// clock memformat durasi sejak tengah malam jadi "HH:MM". 24 jam jadi "24:00",
// yang diterima PostgreSQL sebagai tipe TIME.
func clock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

type NewVenue struct {
	Slug       string
	Name       string
	Phone      string
	Address    string
	Hours      map[time.Weekday]booking.Hours
	Courts     []Court
	OwnerName  string
	OwnerEmail string
	OwnerHash  string
}

// CreateVenue membuat venue beserta jam buka, lapangan, dan akun pemiliknya
// dalam satu transaksi.
func (s *Store) CreateVenue(ctx context.Context, nv NewVenue) (int64, error) {
	var venueID int64
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO venues (slug, name, phone, address) VALUES ($1, $2, $3, $4) RETURNING id`,
			nv.Slug, nv.Name, nv.Phone, nv.Address).Scan(&venueID)
		if err != nil {
			return err
		}
		if err := insertHours(ctx, tx, venueID, nv.Hours, false); err != nil {
			return err
		}
		for _, c := range nv.Courts {
			_, err := tx.Exec(ctx, `
				INSERT INTO courts (venue_id, name, sport, price_per_hour) VALUES ($1, $2, $3, $4)`,
				venueID, c.Name, c.Sport, c.PricePerHour)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO users (venue_id, name, email, password_hash) VALUES ($1, $2, $3, $4)`,
			venueID, nv.OwnerName, nv.OwnerEmail, nv.OwnerHash)
		return err
	})
	switch {
	case isUniqueViolation(err, "venues_slug_key"):
		return 0, ErrSlugTaken
	case isUniqueViolation(err, "users_email_key"):
		return 0, ErrEmailTaken
	}
	return venueID, err
}
