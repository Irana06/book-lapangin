package store

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"lapangin/internal/booking"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
	StatusCompleted = "completed"

	SourceOnline = "online"
	SourceAdmin  = "admin"
)

type Booking struct {
	ID            int64
	VenueID       int64
	CourtID       int64
	Code          string
	CourtName     string
	Sport         string
	CustomerName  string
	CustomerPhone string
	Notes         string
	Start         time.Time
	End           time.Time
	Status        string
	Source        string
	TotalPrice    int64
	HoldExpiresAt *time.Time
	CreatedAt     time.Time
}

func (b Booking) Hours() int {
	return int(b.End.Sub(b.Start) / booking.SlotLength)
}

func (b Booking) CanConfirm() bool { return b.Status == StatusPending }

func (b Booking) CanCancel() bool {
	return b.Status == StatusPending || b.Status == StatusConfirmed
}

// In mengonversi jam booking ke zona waktu venue untuk ditampilkan.
func (b Booking) In(loc *time.Location) Booking {
	b.Start = b.Start.In(loc)
	b.End = b.End.In(loc)
	if b.HoldExpiresAt != nil {
		t := b.HoldExpiresAt.In(loc)
		b.HoldExpiresAt = &t
	}
	return b
}

const bookingSelect = `
	SELECT b.id, c.venue_id, b.court_id, b.code, c.name, c.sport,
	       b.customer_name, b.customer_phone, coalesce(b.notes, ''),
	       lower(b.period), upper(b.period), b.status, b.source,
	       b.total_price, b.hold_expires_at, b.created_at
	FROM bookings b
	JOIN courts c ON c.id = b.court_id`

// activeBooking: booking yang masih menempati slot. Pending dengan hold yang
// sudah lewat dianggap kosong walaupun job expire belum sempat jalan.
const activeBooking = `(b.status IN ('confirmed', 'completed')
	OR (b.status = 'pending' AND (b.hold_expires_at IS NULL OR b.hold_expires_at > now())))`

func scanBooking(row pgx.Row) (Booking, error) {
	var b Booking
	err := row.Scan(&b.ID, &b.VenueID, &b.CourtID, &b.Code, &b.CourtName, &b.Sport,
		&b.CustomerName, &b.CustomerPhone, &b.Notes,
		&b.Start, &b.End, &b.Status, &b.Source,
		&b.TotalPrice, &b.HoldExpiresAt, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	return b, err
}

func collectBookings(rows pgx.Rows, err error) ([]Booking, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Booking, error) {
		return scanBooking(row)
	})
}

// BusyIntervals mengembalikan rentang waktu yang sudah terisi di satu lapangan.
func (s *Store) BusyIntervals(ctx context.Context, courtID int64, from, to time.Time) ([]booking.Interval, error) {
	rows, err := s.db.Query(ctx, `
		SELECT lower(b.period), upper(b.period)
		FROM bookings b
		WHERE b.court_id = $1
		  AND b.period && tstzrange($2::timestamptz, $3::timestamptz, '[)')
		  AND `+activeBooking, courtID, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (booking.Interval, error) {
		var iv booking.Interval
		err := row.Scan(&iv.Start, &iv.End)
		return iv, err
	})
}

type NewBooking struct {
	CourtID       int64
	CustomerName  string
	CustomerPhone string
	Notes         string
	Start         time.Time
	End           time.Time
	TotalPrice    int64
	Status        string
	Source        string
	HoldExpiresAt *time.Time
}

const expireHoldsSQL = `
	UPDATE bookings SET status = 'expired'
	WHERE status = 'pending' AND hold_expires_at IS NOT NULL AND hold_expires_at <= now()`

// CreateBooking menyimpan booking baru dan mengembalikan kodenya.
// ErrSlotTaken dikembalikan kalau jadwal bentrok dengan booking aktif lain.
func (s *Store) CreateBooking(ctx context.Context, nb NewBooking) (string, error) {
	for range 3 {
		code := newCode()
		err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			// Lepas dulu hold yang kedaluwarsa di lapangan ini, supaya slotnya
			// bisa langsung dipakai tanpa menunggu job expire.
			if _, err := tx.Exec(ctx, expireHoldsSQL+` AND court_id = $1`, nb.CourtID); err != nil {
				return err
			}
			var confirmedAt *time.Time
			if nb.Status == StatusConfirmed {
				now := time.Now()
				confirmedAt = &now
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO bookings (court_id, code, customer_name, customer_phone, notes, period,
				                      status, source, total_price, hold_expires_at, confirmed_at)
				VALUES ($1, $2, $3, $4, nullif($5, ''), tstzrange($6::timestamptz, $7::timestamptz, '[)'),
				        $8, $9, $10, $11, $12)`,
				nb.CourtID, code, nb.CustomerName, nb.CustomerPhone, nb.Notes, nb.Start, nb.End,
				nb.Status, nb.Source, nb.TotalPrice, nb.HoldExpiresAt, confirmedAt)
			return err
		})
		if err == nil {
			return code, nil
		}
		if e := pgError(err); e != nil && e.Code == codeExclusionViolation {
			return "", ErrSlotTaken
		}
		if !isUniqueViolation(err, "bookings_code_key") {
			return "", err
		}
	}
	return "", errors.New("gagal membuat kode booking unik")
}

// Huruf dan angka yang mudah dibaca: tanpa 0/O dan 1/I/L.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

func (s *Store) BookingByCode(ctx context.Context, code string) (Booking, error) {
	return scanBooking(s.db.QueryRow(ctx, bookingSelect+` WHERE b.code = $1`, code))
}

func (s *Store) Booking(ctx context.Context, venueID, id int64) (Booking, error) {
	return scanBooking(s.db.QueryRow(ctx, bookingSelect+` WHERE c.venue_id = $1 AND b.id = $2`, venueID, id))
}

type BookingFilter struct {
	Status string     // kosong berarti semua status
	From   *time.Time // rentang waktu main, opsional
	To     *time.Time
	Limit  int
	Oldest bool // urutkan dari jadwal paling awal
}

func (s *Store) Bookings(ctx context.Context, venueID int64, f BookingFilter) ([]Booking, error) {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	order := "DESC"
	if f.Oldest {
		order = "ASC"
	}
	return collectBookings(s.db.Query(ctx, bookingSelect+`
		WHERE c.venue_id = $1
		  AND ($2::text = '' OR b.status = $2)
		  AND ($3::timestamptz IS NULL OR b.period && tstzrange($3::timestamptz, $4::timestamptz, '[)'))
		ORDER BY lower(b.period) `+order+`
		LIMIT $5`, venueID, f.Status, f.From, f.To, f.Limit))
}

// ActiveBookings mengembalikan booking yang menempati jadwal di rentang waktu tertentu.
func (s *Store) ActiveBookings(ctx context.Context, venueID int64, from, to time.Time) ([]Booking, error) {
	return collectBookings(s.db.Query(ctx, bookingSelect+`
		WHERE c.venue_id = $1
		  AND b.period && tstzrange($2::timestamptz, $3::timestamptz, '[)')
		  AND `+activeBooking+`
		ORDER BY lower(b.period), c.name`, venueID, from, to))
}

// Confirm mengubah booking pending menjadi confirmed (pembayaran sudah diterima).
func (s *Store) Confirm(ctx context.Context, venueID, id int64) (Booking, error) {
	return s.transition(ctx, venueID, id, `status = 'confirmed', confirmed_at = now(), hold_expires_at = NULL`,
		`b.status = 'pending' AND (b.hold_expires_at IS NULL OR b.hold_expires_at > now())`)
}

func (s *Store) Cancel(ctx context.Context, venueID, id int64) (Booking, error) {
	return s.transition(ctx, venueID, id, `status = 'cancelled', cancelled_at = now()`,
		`b.status IN ('pending', 'confirmed')`)
}

func (s *Store) transition(ctx context.Context, venueID, id int64, set, allowed string) (Booking, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE bookings b SET `+set+`
		FROM courts c
		WHERE c.id = b.court_id AND c.venue_id = $1 AND b.id = $2 AND `+allowed, venueID, id)
	if err != nil {
		return Booking{}, err
	}
	b, err := s.Booking(ctx, venueID, id)
	if err != nil {
		return Booking{}, err
	}
	if tag.RowsAffected() == 0 {
		return b, ErrInvalidTransition
	}
	return b, nil
}

// CancelByCode dipakai customer untuk membatalkan booking yang belum dibayar.
func (s *Store) CancelByCode(ctx context.Context, code string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE bookings SET status = 'cancelled', cancelled_at = now()
		WHERE code = $1 AND status = 'pending'`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// ExpireHolds menandai booking pending yang melewati batas bayar sebagai expired.
func (s *Store) ExpireHolds(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, expireHoldsSQL)
	return tag.RowsAffected(), err
}

// CompleteFinished menandai booking confirmed yang jam mainnya sudah lewat sebagai completed.
func (s *Store) CompleteFinished(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE bookings SET status = 'completed'
		WHERE status = 'confirmed' AND upper(period) <= now()`)
	return tag.RowsAffected(), err
}

type Stats struct {
	TodayBookings int64
	Pending       int64
	MonthRevenue  int64
	MonthBookings int64
}

func (s *Store) Stats(ctx context.Context, venueID int64, dayStart, monthStart time.Time) (Stats, error) {
	dayEnd := dayStart.AddDate(0, 0, 1)
	monthEnd := monthStart.AddDate(0, 1, 0)
	paid := `b.status IN ('confirmed', 'completed') AND lower(b.period) >= $4 AND lower(b.period) < $5`

	var st Stats
	err := s.db.QueryRow(ctx, `
		SELECT
		    count(*) FILTER (WHERE b.period && tstzrange($2::timestamptz, $3::timestamptz, '[)') AND `+activeBooking+`),
		    count(*) FILTER (WHERE b.status = 'pending' AND (b.hold_expires_at IS NULL OR b.hold_expires_at > now())),
		    coalesce(sum(b.total_price) FILTER (WHERE `+paid+`), 0)::bigint,
		    count(*) FILTER (WHERE `+paid+`)
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		WHERE c.venue_id = $1`,
		venueID, dayStart, dayEnd, monthStart, monthEnd).
		Scan(&st.TodayBookings, &st.Pending, &st.MonthRevenue, &st.MonthBookings)
	return st, err
}
