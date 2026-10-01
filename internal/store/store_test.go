package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"lapangin/internal/booking"
	"lapangin/internal/database"
)

// Tes di file ini butuh PostgreSQL sungguhan. Set TEST_DATABASE_URL ke database
// khusus tes, karena isinya dihapus setiap tes jalan.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL belum di-set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `TRUNCATE venues, courts, opening_hours, bookings, users, sessions RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	return New(pool)
}

func createTestVenue(t *testing.T, st *Store, slug string) (venueID int64, court Court) {
	t.Helper()
	ctx := context.Background()
	id, err := st.CreateVenue(ctx, NewVenue{
		Slug:       slug,
		Name:       "Venue " + slug,
		Hours:      map[time.Weekday]booking.Hours{},
		Courts:     []Court{{Name: "A", Sport: "Futsal", PricePerHour: 100000}},
		OwnerName:  "Owner",
		OwnerEmail: slug + "@example.test",
		OwnerHash:  "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	courts, err := st.Courts(ctx, id, true)
	if err != nil || len(courts) != 1 {
		t.Fatalf("courts: %v %v", courts, err)
	}
	return id, courts[0]
}

func pending(courtID int64, start time.Time, hours int, hold time.Time) NewBooking {
	return NewBooking{
		CourtID:       courtID,
		CustomerName:  "Tes",
		CustomerPhone: "081200000000",
		Start:         start,
		End:           start.Add(time.Duration(hours) * time.Hour),
		TotalPrice:    int64(hours) * 100000,
		Status:        StatusPending,
		Source:        SourceOnline,
		HoldExpiresAt: &hold,
	}
}

func TestDoubleBookingRejected(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_, court := createTestVenue(t, st, "satu")
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)
	hold := time.Now().Add(time.Hour)

	if _, err := st.CreateBooking(ctx, pending(court.ID, start, 2, hold)); err != nil {
		t.Fatal(err)
	}
	// Mulai di tengah booking pertama.
	_, err := st.CreateBooking(ctx, pending(court.ID, start.Add(time.Hour), 1, hold))
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("booking bentrok: err = %v, want ErrSlotTaken", err)
	}
	// Tepat setelah booking pertama selesai harus boleh.
	if _, err := st.CreateBooking(ctx, pending(court.ID, start.Add(2*time.Hour), 1, hold)); err != nil {
		t.Fatalf("booking bersebelahan ditolak: %v", err)
	}
}

func TestExpiredHoldFreesSlot(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_, court := createTestVenue(t, st, "dua")
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)

	oldCode, err := st.CreateBooking(ctx, pending(court.ID, start, 1, time.Now().Add(-time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	busy, err := st.BusyIntervals(ctx, court.ID, start, start.Add(time.Hour))
	if err != nil || len(busy) != 0 {
		t.Fatalf("hold kedaluwarsa masih dianggap terisi: %v %v", busy, err)
	}
	if _, err := st.CreateBooking(ctx, pending(court.ID, start, 1, time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("slot dengan hold kedaluwarsa tidak bisa dibooking: %v", err)
	}
	old, err := st.BookingByCode(ctx, oldCode)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusExpired {
		t.Errorf("status booking lama = %s, want expired", old.Status)
	}
}

func TestTransitionsAreScopedToVenue(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	venueA, court := createTestVenue(t, st, "tiga")
	venueB, _ := createTestVenue(t, st, "empat")
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)

	code, err := st.CreateBooking(ctx, pending(court.ID, start, 1, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.BookingByCode(ctx, code)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.Confirm(ctx, venueB, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("venue lain bisa konfirmasi: err = %v", err)
	}
	got, err := st.Confirm(ctx, venueA, b.ID)
	if err != nil || got.Status != StatusConfirmed || got.HoldExpiresAt != nil {
		t.Fatalf("confirm: %+v %v", got, err)
	}
	if _, err := st.Confirm(ctx, venueA, b.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("confirm dua kali: err = %v", err)
	}
	if err := st.CancelByCode(ctx, code); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("customer bisa membatalkan booking lunas: err = %v", err)
	}
	if _, err := st.Cancel(ctx, venueA, b.ID); err != nil {
		t.Fatal(err)
	}
	// Setelah dibatalkan, slotnya bisa dipakai lagi.
	if _, err := st.CreateBooking(ctx, pending(court.ID, start, 1, time.Now().Add(time.Hour))); err != nil {
		t.Errorf("slot booking batal tidak bisa dipakai: %v", err)
	}
}
