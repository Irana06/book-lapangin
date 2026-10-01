package server

import (
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"

	"lapangin/internal/booking"
	"lapangin/internal/store"
)

var (
	dayNames   = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	monthNames = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	monthShort = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
		"Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
)

var funcs = template.FuncMap{
	"rupiah":      rupiah,
	"tanggal":     longDate,
	"tglPendek":   shortDate,
	"bulan":       func(t time.Time) string { return monthNames[t.Month()-1] + " " + strconv.Itoa(t.Year()) },
	"jam":         func(t time.Time) string { return t.Format("15:04") },
	"jamSelesai":  endClock,
	"isoDate":     func(t time.Time) string { return t.Format("2006-01-02") },
	"namaHari":    func(wd time.Weekday) string { return dayNames[wd] },
	"clock":       clock,
	"statusLabel": statusLabel,
	"waLink":      waLink,
	"hourOptions": hourOptions,
	"durations":   func() []int { return seq(1, booking.MaxHours) },
	"sports":      func() []string { return sports },
}

// sports adalah pilihan jenis olahraga di form lapangan.
var sports = []string{"Futsal", "Mini soccer", "Badminton", "Padel", "Basket", "Tenis", "Voli", "Lainnya"}

func rupiah(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-Rp" + b.String()
	}
	return "Rp" + b.String()
}

func longDate(t time.Time) string {
	return fmt.Sprintf("%s, %d %s %d", dayNames[t.Weekday()], t.Day(), monthNames[t.Month()-1], t.Year())
}

func shortDate(t time.Time) string {
	return fmt.Sprintf("%s, %d %s", dayNames[t.Weekday()][:3], t.Day(), monthShort[t.Month()-1])
}

// endClock menampilkan jam selesai. Booking yang berakhir tepat tengah malam
// ditulis "24:00", bukan "00:00", supaya tidak terbaca sebagai jam awal hari.
func endClock(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 {
		return "24:00"
	}
	return t.Format("15:04")
}

func clock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func statusLabel(status string) string {
	switch status {
	case store.StatusPending:
		return "Menunggu pembayaran"
	case store.StatusConfirmed:
		return "Lunas"
	case store.StatusCancelled:
		return "Dibatalkan"
	case store.StatusExpired:
		return "Kedaluwarsa"
	case store.StatusCompleted:
		return "Selesai"
	}
	return status
}

// waLink membuat link wa.me dengan pesan yang sudah terisi.
// Nomor lokal 08xx diubah ke format internasional 628xx.
func waLink(phone, text string) string {
	num := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if strings.HasPrefix(num, "0") {
		num = "62" + num[1:]
	}
	return "https://wa.me/" + num + "?text=" + url.QueryEscape(text)
}

// hourOptions mengembalikan pilihan jam "00:00" sampai "24:00".
func hourOptions() []string {
	opts := make([]string, 0, 25)
	for h := 0; h <= 24; h++ {
		opts = append(opts, fmt.Sprintf("%02d:00", h))
	}
	return opts
}

func seq(from, to int) []int {
	s := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		s = append(s, i)
	}
	return s
}
