// Package store berisi semua query ke PostgreSQL. Query untuk panel admin
// selalu difilter dengan venue_id supaya data antar klien tidak tercampur.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("data tidak ditemukan")
	ErrSlotTaken         = errors.New("jadwal sudah terisi")
	ErrInvalidTransition = errors.New("status booking tidak bisa diubah")
	ErrEmailTaken        = errors.New("email sudah dipakai")
	ErrSlugTaken         = errors.New("slug sudah dipakai")
)

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

const (
	codeExclusionViolation = "23P01"
	codeUniqueViolation    = "23505"
)

func pgError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr
	}
	return nil
}

func isUniqueViolation(err error, constraint string) bool {
	e := pgError(err)
	return e != nil && e.Code == codeUniqueViolation && e.ConstraintName == constraint
}
