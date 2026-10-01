package booking

import (
	"errors"
	"testing"
	"time"
)

var jakarta = time.FixedZone("WIB", 7*3600)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, jakarta)
}

func at(base time.Time, hour int) time.Time {
	return base.Add(time.Duration(hour) * time.Hour)
}

func TestSlots(t *testing.T) {
	d := day(2026, 10, 3)
	h := Hours{Open: 8 * time.Hour, Close: 12*time.Hour + 30*time.Minute}
	busy := []Interval{{Start: at(d, 10), End: at(d, 11)}}
	now := at(d, 8).Add(15 * time.Minute)

	slots := Slots(d, h, busy, now)

	want := []SlotState{SlotPast, SlotFree, SlotTaken, SlotFree}
	if len(slots) != len(want) {
		t.Fatalf("got %d slots, want %d (sisa 30 menit terakhir tidak dihitung)", len(slots), len(want))
	}
	for i, s := range slots {
		if s.State != want[i] {
			t.Errorf("slot %s: state %d, want %d", s.Start.Format("15:04"), s.State, want[i])
		}
	}
}

func TestSlotsUntilMidnight(t *testing.T) {
	d := day(2026, 10, 3)
	slots := Slots(d, Hours{Open: 22 * time.Hour, Close: 24 * time.Hour}, nil, at(d, 0))
	if len(slots) != 2 {
		t.Fatalf("got %d slots, want 2", len(slots))
	}
	if !slots[1].End.Equal(day(2026, 10, 4)) {
		t.Errorf("slot terakhir berakhir %v, want tengah malam", slots[1].End)
	}
}

func TestFreeRun(t *testing.T) {
	d := day(2026, 10, 3)
	h := Hours{Open: 8 * time.Hour, Close: 22 * time.Hour}
	busy := []Interval{{Start: at(d, 11), End: at(d, 12)}}
	slots := Slots(d, h, busy, at(d, 0))

	tests := []struct {
		start int
		want  int
	}{
		{8, 3},  // 08, 09, 10 lalu 11 terisi
		{11, 0}, // slot terisi
		{12, MaxHours},
		{21, 1},
		{7, 0}, // sebelum jam buka
	}
	for _, tt := range tests {
		if got := FreeRun(slots, at(d, tt.start)); got != tt.want {
			t.Errorf("FreeRun(%02d:00) = %d, want %d", tt.start, got, tt.want)
		}
	}
}

func TestCheck(t *testing.T) {
	d := day(2026, 10, 3)
	h := Hours{Open: 8 * time.Hour, Close: 22 * time.Hour}
	rules := Rules{Now: at(d, 9).Add(30 * time.Minute), WindowDays: 7}

	tests := []struct {
		name  string
		day   time.Time
		start time.Time
		hours int
		rules Rules
		want  error
	}{
		{"valid", d, at(d, 10), 2, rules, nil},
		{"durasi nol", d, at(d, 10), 0, rules, ErrDuration},
		{"durasi kepanjangan", d, at(d, 10), MaxHours + 1, rules, ErrDuration},
		{"sebelum buka", d, at(d, 7), 1, rules, ErrOutsideHours},
		{"lewat jam tutup", d, at(d, 21), 2, rules, ErrOutsideHours},
		{"tidak pas slot", d, at(d, 10).Add(30 * time.Minute), 1, rules, ErrMisaligned},
		{"sudah lewat", d, at(d, 9), 1, rules, ErrInPast},
		{"admin boleh jam lewat", d, at(d, 9), 1, Rules{Now: rules.Now, AllowPast: true}, nil},
		{"terlalu jauh", day(2026, 10, 11), at(day(2026, 10, 11), 10), 1, rules, ErrTooFar},
		{"batas terakhir", day(2026, 10, 10), at(day(2026, 10, 10), 10), 1, rules, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(tt.day, h, tt.start, tt.hours, tt.rules)
			if !errors.Is(err, tt.want) {
				t.Errorf("Check() = %v, want %v", err, tt.want)
			}
		})
	}
}
