// Package booking berisi aturan penjadwalan (slot, validasi jam, harga) yang
// tidak bergantung pada database, supaya mudah dites.
package booking

import (
	"errors"
	"time"
)

// SlotLength adalah durasi satu slot. Booking selalu kelipatan slot.
const SlotLength = time.Hour

// MaxHours adalah durasi maksimal satu booking.
const MaxHours = 6

var (
	ErrOutsideHours = errors.New("jam di luar jam operasional")
	ErrMisaligned   = errors.New("jam mulai tidak sesuai slot")
	ErrInPast       = errors.New("jam sudah lewat")
	ErrTooFar       = errors.New("tanggal melebihi batas booking")
	ErrDuration     = errors.New("durasi tidak valid")
)

type Interval struct {
	Start time.Time
	End   time.Time
}

func (a Interval) Overlaps(b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// Hours adalah jam operasional dalam satu hari, dihitung dari tengah malam.
// Close boleh 24 jam (tutup tepat tengah malam).
type Hours struct {
	Open  time.Duration
	Close time.Duration
}

type SlotState int

const (
	SlotFree SlotState = iota
	SlotTaken
	SlotPast
)

type Slot struct {
	Interval
	State SlotState
}

func (s Slot) IsFree() bool  { return s.State == SlotFree }
func (s Slot) IsTaken() bool { return s.State == SlotTaken }
func (s Slot) IsPast() bool  { return s.State == SlotPast }

// DayStart mengembalikan jam 00:00 dari tanggal t di zona waktu loc.
func DayStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Slots membagi jam operasional menjadi slot per SlotLength dan menandai
// slot yang sudah lewat atau bentrok dengan booking lain. Sisa waktu yang
// kurang dari satu slot di akhir hari tidak ditampilkan.
func Slots(day time.Time, h Hours, busy []Interval, now time.Time) []Slot {
	var slots []Slot
	for off := h.Open; off+SlotLength <= h.Close; off += SlotLength {
		s := Slot{Interval: Interval{Start: day.Add(off), End: day.Add(off + SlotLength)}}
		switch {
		case s.Start.Before(now):
			s.State = SlotPast
		case overlapsAny(s.Interval, busy):
			s.State = SlotTaken
		}
		slots = append(slots, s)
	}
	return slots
}

func overlapsAny(iv Interval, busy []Interval) bool {
	for _, b := range busy {
		if iv.Overlaps(b) {
			return true
		}
	}
	return false
}

// FreeRun menghitung berapa slot kosong berturut-turut mulai dari start,
// dibatasi MaxHours. Hasil 0 berarti slot start tidak tersedia.
func FreeRun(slots []Slot, start time.Time) int {
	n := 0
	started := false
	for _, s := range slots {
		if !started {
			if !s.Start.Equal(start) {
				continue
			}
			started = true
		}
		if s.State != SlotFree || n == MaxHours {
			break
		}
		n++
	}
	return n
}

type Rules struct {
	Now        time.Time
	WindowDays int // 0 berarti tanpa batas
	AllowPast  bool
}

// Check memvalidasi permintaan booking selama hours jam mulai dari start
// pada hari day (jam 00:00 di zona waktu venue). Bentrok dengan booking lain
// tidak dicek di sini; itu dijaga oleh constraint database.
func Check(day time.Time, h Hours, start time.Time, hours int, r Rules) error {
	if hours < 1 || hours > MaxHours {
		return ErrDuration
	}
	off := start.Sub(day)
	end := off + time.Duration(hours)*SlotLength
	if off < h.Open || end > h.Close {
		return ErrOutsideHours
	}
	if (off-h.Open)%SlotLength != 0 {
		return ErrMisaligned
	}
	if !r.AllowPast && start.Before(r.Now) {
		return ErrInPast
	}
	if r.WindowDays > 0 {
		last := DayStart(r.Now, day.Location()).AddDate(0, 0, r.WindowDays)
		if day.After(last) {
			return ErrTooFar
		}
	}
	return nil
}

func Price(hours int, perHour int64) int64 {
	return int64(hours) * perHour
}
