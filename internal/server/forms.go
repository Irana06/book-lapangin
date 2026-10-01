package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// formErrors berisi pesan error per field, key "form" untuk error umum.
type formErrors map[string]string

func (e formErrors) add(field, msg string) {
	if _, exists := e[field]; !exists {
		e[field] = msg
	}
}

func field(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

func checkLength(errs formErrors, key, value, label string, min, max int) {
	n := utf8.RuneCountInString(value)
	switch {
	case n < min && min == 1:
		errs.add(key, label+" wajib diisi.")
	case n < min:
		errs.add(key, label+" minimal "+strconv.Itoa(min)+" karakter.")
	case n > max:
		errs.add(key, label+" maksimal "+strconv.Itoa(max)+" karakter.")
	}
}

// normalizePhone membersihkan nomor HP: buang spasi/tanda baca dan ubah
// awalan +62 / 62 menjadi 0. ok bernilai false kalau formatnya tidak masuk akal.
func normalizePhone(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')' || r == '+':
		default:
			return "", false
		}
	}
	num := b.String()
	if strings.HasPrefix(num, "62") {
		num = "0" + num[2:]
	}
	if !strings.HasPrefix(num, "0") || len(num) < 9 || len(num) > 15 {
		return "", false
	}
	return num, true
}

// parseClock membaca "HH:MM" menjadi durasi sejak tengah malam (00:00 sampai 24:00).
func parseClock(s string) (time.Duration, bool) {
	t, err := time.Parse("15:04", s)
	if err == nil {
		return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, true
	}
	if s == "24:00" {
		return 24 * time.Hour, true
	}
	return 0, false
}

func parseDate(s string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	return t, err == nil
}

// parseRupiah menerima "150000", "150.000", atau "Rp150.000".
func parseRupiah(s string) (int64, bool) {
	s = strings.NewReplacer("Rp", "", "rp", "", ".", "", ",", "", " ", "").Replace(s)
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n >= 0
}

func parseID(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n > 0
}
