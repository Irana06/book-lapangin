package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"lapangin/internal/booking"
	"lapangin/internal/store"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	venues, err := s.store.Venues(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, "home.html", data{
		"Title":  "Pilih tempat",
		"Venues": venues,
	})
}

// venueFromURL mengambil venue dari {slug}. Kalau tidak ada, halaman 404
// sudah ditulis dan ok bernilai false.
func (s *Server) venueFromURL(w http.ResponseWriter, r *http.Request) (store.Venue, bool) {
	v, err := s.store.VenueBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return v, false
	}
	if err != nil {
		s.serverError(w, err)
		return v, false
	}
	return v, true
}

// bookingWindow mengembalikan hari ini dan hari terakhir yang boleh dibooking customer.
func bookingWindow(v store.Venue) (today, last time.Time) {
	today = booking.DayStart(time.Now(), v.Loc())
	return today, today.AddDate(0, 0, v.BookingWindowDays)
}

type slotsView struct {
	Venue  store.Venue
	Court  store.Court
	Day    time.Time
	Slots  []booking.Slot
	Closed bool
	Notice string

	// Diisi oleh pickerView untuk halaman venue.
	Courts    []store.Court
	Days      []time.Time // pilihan cepat tanggal, maksimal 7 hari
	Today     time.Time
	Last      time.Time
	PastCount int
}

// AllPast bernilai true kalau semua slot di hari itu sudah lewat.
func (v slotsView) AllPast() bool {
	return len(v.Slots) > 0 && v.PastCount == len(v.Slots)
}

func (s *Server) loadSlots(ctx context.Context, v store.Venue, c store.Court, day time.Time) (slotsView, error) {
	view := slotsView{Venue: v, Court: c, Day: day}
	today, last := bookingWindow(v)
	if day.Before(today) || day.After(last) {
		view.Notice = "Booking online hanya untuk tanggal " + shortDate(today) + " sampai " + shortDate(last) + "."
		return view, nil
	}
	h, open, err := s.store.HoursOn(ctx, v.ID, day.Weekday())
	if err != nil {
		return view, err
	}
	if !open {
		view.Closed = true
		return view, nil
	}
	busy, err := s.store.BusyIntervals(ctx, c.ID, day.Add(h.Open), day.Add(h.Close))
	if err != nil {
		return view, err
	}
	view.Slots = booking.Slots(day, h, busy, time.Now())
	for i := range view.Slots {
		view.Slots[i].Start = view.Slots[i].Start.In(v.Loc())
		view.Slots[i].End = view.Slots[i].End.In(v.Loc())
	}
	return view, nil
}

// pickerView menyiapkan blok pemilih lapangan, tanggal, dan slot di halaman venue.
func (s *Server) pickerView(ctx context.Context, v store.Venue, courts []store.Court, c store.Court, day time.Time) (slotsView, error) {
	view, err := s.loadSlots(ctx, v, c, day)
	if err != nil {
		return view, err
	}
	view.Courts = courts
	view.Today, view.Last = bookingWindow(v)
	for d := view.Today; !d.After(view.Last) && len(view.Days) < 7; d = d.AddDate(0, 0, 1) {
		view.Days = append(view.Days, d)
	}
	for _, sl := range view.Slots {
		if sl.IsPast() {
			view.PastCount++
		}
	}
	return view, nil
}

func venueURL(v store.Venue, c store.Court, day time.Time) string {
	return fmt.Sprintf("/v/%s?court=%d&date=%s", v.Slug, c.ID, day.Format("2006-01-02"))
}

// pickCourt memilih lapangan dari query ?court=, default lapangan pertama.
func pickCourt(courts []store.Court, raw string) (store.Court, bool) {
	if id, ok := parseID(raw); ok {
		for _, c := range courts {
			if c.ID == id {
				return c, true
			}
		}
		return store.Court{}, false
	}
	if len(courts) == 0 {
		return store.Court{}, false
	}
	return courts[0], true
}

func (s *Server) handleVenue(w http.ResponseWriter, r *http.Request) {
	v, ok := s.venueFromURL(w, r)
	if !ok {
		return
	}
	courts, err := s.store.Courts(r.Context(), v.ID, true)
	if err != nil {
		s.serverError(w, err)
		return
	}
	today, last := bookingWindow(v)
	day, ok := parseDate(r.URL.Query().Get("date"), v.Loc())
	if !ok || day.Before(today) || day.After(last) {
		day = today
	}

	d := data{
		"Title":  v.Name,
		"Venue":  v,
		"Courts": courts,
	}
	c, ok := pickCourt(courts, r.URL.Query().Get("court"))
	if !ok && len(courts) > 0 {
		c, ok = courts[0], true
	}
	if ok {
		view, err := s.pickerView(r.Context(), v, courts, c, day)
		if err != nil {
			s.serverError(w, err)
			return
		}
		d["Picker"] = view
	}
	s.render(w, r, http.StatusOK, "venue.html", d)
}

// handleSlots dipanggil HTMX saat customer mengganti lapangan atau tanggal.
func (s *Server) handleSlots(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.VenueBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		s.partialError(w, err)
		return
	}
	courts, err := s.store.Courts(r.Context(), v.ID, true)
	if err != nil {
		s.serverError(w, err)
		return
	}
	c, ok := pickCourt(courts, r.URL.Query().Get("court"))
	if !ok {
		http.Error(w, "Lapangan tidak ditemukan.", http.StatusNotFound)
		return
	}
	day, ok := parseDate(r.URL.Query().Get("date"), v.Loc())
	if !ok {
		http.Error(w, "Format tanggal tidak valid.", http.StatusBadRequest)
		return
	}
	view, err := s.pickerView(r.Context(), v, courts, c, day)
	if err != nil {
		s.serverError(w, err)
		return
	}
	// Perbarui URL di browser supaya pilihan bisa dibagikan dan tombol back tetap jalan.
	w.Header().Set("HX-Push-Url", venueURL(v, c, day))
	s.renderPartial(w, http.StatusOK, "picker", view)
}

func (s *Server) partialError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Data tidak ditemukan.", http.StatusNotFound)
		return
	}
	s.serverError(w, err)
}

type durationOption struct {
	Hours int
	End   time.Time
	Price int64
}

type bookForm struct {
	Hours int
	Name  string
	Phone string
	Notes string
}

// bookPage menyiapkan data halaman form booking. Dipakai saat GET dan saat
// form dikirim ulang karena ada error. Data nil berarti parameter URL tidak valid.
func (s *Server) bookPage(r *http.Request, v store.Venue) (data, error) {
	q := r.Form
	courts, err := s.store.Courts(r.Context(), v.ID, true)
	if err != nil {
		return nil, err
	}
	c, okCourt := pickCourt(courts, q.Get("court"))
	day, okDay := parseDate(q.Get("date"), v.Loc())
	startOff, okStart := parseClock(q.Get("start"))
	if !okCourt || !okDay || !okStart || q.Get("court") == "" {
		return nil, nil
	}
	start := day.Add(startOff)

	view, err := s.loadSlots(r.Context(), v, c, day)
	if err != nil {
		return nil, err
	}
	run := booking.FreeRun(view.Slots, start)
	opts := make([]durationOption, 0, run)
	for n := 1; n <= run; n++ {
		opts = append(opts, durationOption{
			Hours: n,
			End:   start.Add(time.Duration(n) * booking.SlotLength),
			Price: booking.Price(n, c.PricePerHour),
		})
	}
	return data{
		"Title":   "Booking " + c.Name,
		"Venue":   v,
		"Court":   c,
		"Day":     day,
		"Start":   start,
		"Options": opts,
	}, nil
}

func (s *Server) handleBookForm(w http.ResponseWriter, r *http.Request) {
	v, ok := s.venueFromURL(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	d, err := s.bookPage(r, v)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if d == nil {
		http.Redirect(w, r, "/v/"+v.Slug, http.StatusSeeOther)
		return
	}
	d["Form"] = bookForm{Hours: 1}
	s.render(w, r, http.StatusOK, "book.html", d)
}

func (s *Server) handleBookSubmit(w http.ResponseWriter, r *http.Request) {
	v, ok := s.venueFromURL(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form tidak valid.", http.StatusBadRequest)
		return
	}
	d, err := s.bookPage(r, v)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if d == nil {
		http.Redirect(w, r, "/v/"+v.Slug, http.StatusSeeOther)
		return
	}

	hours, _ := strconv.Atoi(r.FormValue("hours"))
	form := bookForm{Hours: hours, Name: field(r, "name"), Phone: field(r, "phone"), Notes: field(r, "notes")}
	errs := formErrors{}
	checkLength(errs, "name", form.Name, "Nama", 2, 80)
	phone, okPhone := normalizePhone(form.Phone)
	if !okPhone {
		errs.add("phone", "Nomor HP tidak valid. Contoh: 081234567890.")
	}
	checkLength(errs, "notes", form.Notes, "Catatan", 0, 300)

	c := d["Court"].(store.Court)
	day := d["Day"].(time.Time)
	start := d["Start"].(time.Time)
	end := start.Add(time.Duration(hours) * booking.SlotLength)
	now := time.Now()

	if len(errs) == 0 {
		h, open, err := s.store.HoursOn(r.Context(), v.ID, day.Weekday())
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !open {
			errs.add("form", "Venue tutup pada tanggal ini.")
		} else if err := booking.Check(day, h, start, hours, booking.Rules{Now: now, WindowDays: v.BookingWindowDays}); err != nil {
			errs.add("form", bookingErrorMessage(err))
		}
	}

	if len(errs) == 0 {
		var hold *time.Time
		if v.HoldMinutes > 0 {
			t := now.Add(time.Duration(v.HoldMinutes) * time.Minute)
			hold = &t
		}
		code, err := s.store.CreateBooking(r.Context(), store.NewBooking{
			CourtID:       c.ID,
			CustomerName:  form.Name,
			CustomerPhone: phone,
			Notes:         form.Notes,
			Start:         start,
			End:           end,
			TotalPrice:    booking.Price(hours, c.PricePerHour),
			Status:        store.StatusPending,
			Source:        store.SourceOnline,
			HoldExpiresAt: hold,
		})
		switch {
		case err == nil:
			http.Redirect(w, r, "/b/"+code, http.StatusSeeOther)
			return
		case errors.Is(err, store.ErrSlotTaken):
			errs.add("form", "Jadwal ini baru saja diambil orang lain. Pilih jam atau durasi lain.")
			// Hitung ulang pilihan durasi karena slot sudah berubah.
			if d, err = s.bookPage(r, v); err != nil {
				s.serverError(w, err)
				return
			}
		default:
			s.serverError(w, err)
			return
		}
	}

	d["Form"] = form
	d["Errors"] = errs
	s.render(w, r, http.StatusUnprocessableEntity, "book.html", d)
}

func bookingErrorMessage(err error) string {
	switch {
	case errors.Is(err, booking.ErrInPast):
		return "Jam yang dipilih sudah lewat."
	case errors.Is(err, booking.ErrTooFar):
		return "Tanggal ini belum dibuka untuk booking."
	case errors.Is(err, booking.ErrDuration):
		return "Pilih durasi yang tersedia."
	case errors.Is(err, booking.ErrOutsideHours), errors.Is(err, booking.ErrMisaligned):
		return "Jam yang dipilih di luar jam buka."
	}
	return "Booking tidak bisa diproses."
}

func normalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func (s *Server) handleBookingStatus(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.BookingByCode(r.Context(), normalizeCode(chi.URLParam(r, "code")))
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	v, err := s.store.VenueByID(r.Context(), b.VenueID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, "booking.html", data{
		"Title":   "Booking " + b.Code,
		"Booking": b.In(v.Loc()),
		"Venue":   v,
	})
}

func (s *Server) handleCustomerCancel(w http.ResponseWriter, r *http.Request) {
	code := normalizeCode(chi.URLParam(r, "code"))
	err := s.store.CancelByCode(r.Context(), code)
	switch {
	case err == nil:
		s.setFlash(w, "Booking sudah dibatalkan.")
	case errors.Is(err, store.ErrInvalidTransition):
		s.setFlash(w, "Booking ini tidak bisa dibatalkan lewat website. Hubungi pengelola.")
	default:
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/b/"+code, http.StatusSeeOther)
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	code := normalizeCode(r.URL.Query().Get("code"))
	d := data{"Title": "Cek booking", "Code": code}
	if code != "" {
		_, err := s.store.BookingByCode(r.Context(), code)
		if err == nil {
			http.Redirect(w, r, "/b/"+code, http.StatusSeeOther)
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			s.serverError(w, err)
			return
		}
		d["Error"] = "Kode booking " + code + " tidak ditemukan. Periksa lagi hurufnya."
	}
	s.render(w, r, http.StatusOK, "lookup.html", d)
}
