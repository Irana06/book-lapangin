package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Court struct {
	ID           int64
	VenueID      int64
	Name         string
	Sport        string
	PricePerHour int64
	IsActive     bool
}

const courtColumns = `id, venue_id, name, sport, price_per_hour, is_active`

func scanCourt(row pgx.Row) (Court, error) {
	var c Court
	err := row.Scan(&c.ID, &c.VenueID, &c.Name, &c.Sport, &c.PricePerHour, &c.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Court{}, ErrNotFound
	}
	return c, err
}

func (s *Store) Courts(ctx context.Context, venueID int64, activeOnly bool) ([]Court, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+courtColumns+` FROM courts
		WHERE venue_id = $1 AND (is_active OR NOT $2)
		ORDER BY name`, venueID, activeOnly)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Court, error) {
		return scanCourt(row)
	})
}

func (s *Store) Court(ctx context.Context, venueID, courtID int64) (Court, error) {
	return scanCourt(s.db.QueryRow(ctx,
		`SELECT `+courtColumns+` FROM courts WHERE venue_id = $1 AND id = $2`, venueID, courtID))
}

func (s *Store) CreateCourt(ctx context.Context, c Court) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO courts (venue_id, name, sport, price_per_hour, is_active)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		c.VenueID, c.Name, c.Sport, c.PricePerHour, c.IsActive).Scan(&id)
	return id, err
}

func (s *Store) UpdateCourt(ctx context.Context, c Court) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE courts SET name = $3, sport = $4, price_per_hour = $5, is_active = $6
		WHERE venue_id = $1 AND id = $2`,
		c.VenueID, c.ID, c.Name, c.Sport, c.PricePerHour, c.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
