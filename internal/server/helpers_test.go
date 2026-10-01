package server

import (
	"testing"
	"time"

	"lapangin/web"
)

func TestRupiah(t *testing.T) {
	tests := map[int64]string{
		0:       "Rp0",
		500:     "Rp500",
		1000:    "Rp1.000",
		150000:  "Rp150.000",
		1250000: "Rp1.250.000",
		-25000:  "-Rp25.000",
	}
	for in, want := range tests {
		if got := rupiah(in); got != want {
			t.Errorf("rupiah(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"081234567890", "081234567890", true},
		{"0812-3456-7890", "081234567890", true},
		{"+62 812 3456 7890", "081234567890", true},
		{"6281234567890", "081234567890", true},
		{"(021) 555 1234", "0215551234", true},
		{"12ab", "", false},
		{"0812", "", false},
		{"81234567890", "", false},
	}
	for _, tt := range tests {
		got, ok := normalizePhone(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("normalizePhone(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestWaLink(t *testing.T) {
	got := waLink("0812-3456-7890", "Halo, kode ABC")
	want := "https://wa.me/6281234567890?text=Halo%2C+kode+ABC"
	if got != want {
		t.Errorf("waLink = %q, want %q", got, want)
	}
}

func TestParseClock(t *testing.T) {
	if d, ok := parseClock("07:30"); !ok || d != 7*time.Hour+30*time.Minute {
		t.Errorf("07:30 = %v, %v", d, ok)
	}
	if d, ok := parseClock("24:00"); !ok || d != 24*time.Hour {
		t.Errorf("24:00 = %v, %v", d, ok)
	}
	if _, ok := parseClock("25:00"); ok {
		t.Error("25:00 harus ditolak")
	}
}

func TestEndClock(t *testing.T) {
	midnight := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	if got := endClock(midnight); got != "24:00" {
		t.Errorf("endClock(tengah malam) = %q, want 24:00", got)
	}
	if got := endClock(midnight.Add(21 * time.Hour)); got != "21:00" {
		t.Errorf("endClock(21:00) = %q", got)
	}
}

func TestTemplatesParse(t *testing.T) {
	if _, _, err := parseTemplates(web.FS); err != nil {
		t.Fatal(err)
	}
}
