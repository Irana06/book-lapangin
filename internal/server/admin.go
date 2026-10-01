package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"lapangin/internal/auth"
	"lapangin/internal/booking"
	"lapangin/internal/store"
)

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "login.html", data{"Title": "Masuk pengelola"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(field(r, "email"))
	password := r.FormValue("password")
	d := data{"Title": "Masuk pengelola", "Email": email}

	ip := r.RemoteAddr
	if !s.logins.Allow(ip, time.Now()) {
		d["Error"] = "Terlalu banyak percobaan masuk. Coba lagi 15 menit lagi."
		s.render(w, r, http.StatusTooManyRequests, "login.html", d)
		return
	}

	u, hash, err := s.store.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, err)
		return
	}
	if err != nil {
		auth.SpendCheckTime(password)
	}
	if err != nil || !auth.CheckPassword(hash, password) {
		d["Error"] = "Email atau password salah."
		s.render(w, r, http.StatusUnauthorized, "login.html", d)
		return
	}

	token, tokenHash := auth.NewSessionToken()
	if err := s.store.CreateSession(r.Context(), tokenHash, u.ID, time.Now().Add(sessionTTL)); err != nil {
		s.serverError(w, err)
		return
	}
	s.logins.Reset(ip)
	s.setCookie(w, sessionCookie, token, sessionTTL)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			s.logger.Error("delete session", "err", err)
		}
	}
	s.clearCookie(w, sessionCookie)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// adminVenue mengambil venue milik user yang sedang login.
func (s *Server) adminVenue(w http.ResponseWriter, r *http.Request) (store.Venue, bool) {
	v, err := s.store.VenueByID(r.Context(), currentUser(r).VenueID)
	if err != nil {
		s.serverError(w, err)
		return v, false
	}
	return v, true
}

func localize(bs []store.Booking, loc *time.Location) []store.Booking {
	for i := range bs {
		bs[i] = bs[i].In(loc)
	}
	return bs
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	loc := v.Loc()
	today := booking.DayStart(time.Now(), loc)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)

	stats, err := s.store.Stats(ctx, v.ID, today, monthStart)
	if err != nil {
		s.serverError(w, err)
		return
	}
	pending, err := s.store.Bookings(ctx, v.ID, store.BookingFilter{Status: store.StatusPending, Oldest: true, Limit: 50})
	if err != nil {
		s.serverError(w, err)
		return
	}
	todays, err := s.store.ActiveBookings(ctx, v.ID, today, today.AddDate(0, 0, 1))
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin/dashboard.html", data{
		"Title":   "Dashboard",
		"Venue":   v,
		"Today":   today,
		"Stats":   stats,
		"Pending": localize(pending, loc),
		"Todays":  localize(todays, loc),
	})
}

type scheduleCell struct {
	Court   store.Court
	Start   time.Time
	Booking *store.Booking
	IsStart bool
	Past    bool
}

type scheduleRow struct {
	Start time.Time
	Cells []scheduleCell
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	loc := v.Loc()
	today := booking.DayStart(time.Now(), loc)
	day, ok := parseDate(r.URL.Query().Get("date"), loc)
	if !ok {
		day = today
	}

	d := data{
		"Title": "Jadwal",
		"Venue": v,
		"Day":   day,
		"Today": today,
		"Prev":  day.AddDate(0, 0, -1),
		"Next":  day.AddDate(0, 0, 1),
	}

	courts, err := s.store.Courts(ctx, v.ID, true)
	if err != nil {
		s.serverError(w, err)
		return
	}
	h, open, err := s.store.HoursOn(ctx, v.ID, day.Weekday())
	if err != nil {
		s.serverError(w, err)
		return
	}
	d["Courts"] = courts
	d["Open"] = open

	if open && len(courts) > 0 {
		bookings, err := s.store.ActiveBookings(ctx, v.ID, day.Add(h.Open), day.Add(h.Close))
		if err != nil {
			s.serverError(w, err)
			return
		}
		localize(bookings, loc)
		now := time.Now()
		var rows []scheduleRow
		for _, slot := range booking.Slots(day, h, nil, now) {
			row := scheduleRow{Start: slot.Start}
			for _, c := range courts {
				cell := scheduleCell{Court: c, Start: slot.Start, Past: slot.IsPast()}
				for i := range bookings {
					b := &bookings[i]
					if b.CourtID == c.ID && slot.Overlaps(booking.Interval{Start: b.Start, End: b.End}) {
						cell.Booking = b
						cell.IsStart = !b.Start.Before(slot.Start)
						break
					}
				}
				row.Cells = append(row.Cells, cell)
			}
			rows = append(rows, row)
		}
		d["Rows"] = rows
	}
	s.render(w, r, http.StatusOK, "admin/schedule.html", d)
}

func (s *Server) handleBookings(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.BookingFilter{Status: q.Get("status")}
	switch f.Status {
	case "", store.StatusPending, store.StatusConfirmed, store.StatusCompleted, store.StatusCancelled, store.StatusExpired:
	default:
		f.Status = ""
	}
	if day, ok := parseDate(q.Get("date"), v.Loc()); ok {
		end := day.AddDate(0, 0, 1)
		f.From, f.To = &day, &end
		f.Oldest = true
	}

	bookings, err := s.store.Bookings(r.Context(), v.ID, f)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin/bookings.html", data{
		"Title":    "Booking",
		"Venue":    v,
		"Bookings": localize(bookings, v.Loc()),
		"Status":   f.Status,
		"Date":     q.Get("date"),
		"Statuses": []string{store.StatusPending, store.StatusConfirmed, store.StatusCompleted, store.StatusCancelled, store.StatusExpired},
	})
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	s.changeStatus(w, r, s.store.Confirm, "Booking ditandai lunas.")
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.changeStatus(w, r, s.store.Cancel, "Booking dibatalkan.")
}

type transitionFunc func(ctx context.Context, venueID, id int64) (store.Booking, error)

// changeStatus menjalankan perubahan status. Untuk request HTMX, yang
// dikirim balik adalah kartu booking terbaru (termasuk kalau statusnya ternyata
// sudah berubah duluan), selain itu redirect ke halaman sebelumnya.
func (s *Server) changeStatus(w http.ResponseWriter, r *http.Request, fn transitionFunc, okMsg string) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	id, ok := parseID(chi.URLParam(r, "id"))
	if !ok {
		s.notFound(w, r)
		return
	}
	b, err := fn(r.Context(), v.ID, id)
	msg := okMsg
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, store.ErrInvalidTransition):
		msg = "Status booking " + b.Code + " sudah berubah menjadi " + strings.ToLower(statusLabel(b.Status)) + "."
	case err != nil:
		s.serverError(w, err)
		return
	}

	if isHTMX(r) {
		s.renderPartial(w, http.StatusOK, "booking_item", b.In(v.Loc()))
		return
	}
	s.setFlash(w, msg)
	back := r.Referer()
	if back == "" {
		back = "/admin/booking"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type adminBookingForm struct {
	CourtID int64
	Date    string
	Start   string
	Hours   int
	Name    string
	Phone   string
	Notes   string
}

func (s *Server) handleAdminBookingForm(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	courtID, _ := parseID(q.Get("court"))
	form := adminBookingForm{CourtID: courtID, Date: q.Get("date"), Start: q.Get("start"), Hours: 1}
	if form.Date == "" {
		form.Date = booking.DayStart(time.Now(), v.Loc()).Format("2006-01-02")
	}
	s.renderAdminBookingForm(w, r, v, form, nil, http.StatusOK)
}

func (s *Server) renderAdminBookingForm(w http.ResponseWriter, r *http.Request, v store.Venue, form adminBookingForm, errs formErrors, status int) {
	courts, err := s.store.Courts(r.Context(), v.ID, true)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, status, "admin/booking_form.html", data{
		"Title":  "Booking manual",
		"Venue":  v,
		"Courts": courts,
		"Form":   form,
		"Errors": errs,
	})
}

func (s *Server) handleAdminBookingCreate(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	courtID, _ := parseID(r.FormValue("court"))
	hours, _ := strconv.Atoi(r.FormValue("hours"))
	form := adminBookingForm{
		CourtID: courtID,
		Date:    field(r, "date"),
		Start:   field(r, "start"),
		Hours:   hours,
		Name:    field(r, "name"),
		Phone:   field(r, "phone"),
		Notes:   field(r, "notes"),
	}

	errs := formErrors{}
	c, err := s.store.Court(ctx, v.ID, courtID)
	if errors.Is(err, store.ErrNotFound) {
		errs.add("court", "Pilih lapangan.")
	} else if err != nil {
		s.serverError(w, err)
		return
	}
	day, okDay := parseDate(form.Date, v.Loc())
	if !okDay {
		errs.add("date", "Tanggal tidak valid.")
	}
	startOff, okStart := parseClock(form.Start)
	if !okStart {
		errs.add("start", "Pilih jam mulai.")
	}
	checkLength(errs, "name", form.Name, "Nama", 2, 80)
	phone, okPhone := normalizePhone(form.Phone)
	if !okPhone {
		errs.add("phone", "Nomor HP tidak valid. Contoh: 081234567890.")
	}
	checkLength(errs, "notes", form.Notes, "Catatan", 0, 300)

	if len(errs) == 0 {
		start := day.Add(startOff)
		h, open, err := s.store.HoursOn(ctx, v.ID, day.Weekday())
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !open {
			errs.add("form", "Venue tutup pada tanggal ini. Ubah jam buka di Pengaturan kalau ingin tetap menerima booking.")
		} else if err := booking.Check(day, h, start, hours, booking.Rules{Now: time.Now(), AllowPast: true}); err != nil {
			errs.add("form", bookingErrorMessage(err))
		} else {
			code, err := s.store.CreateBooking(ctx, store.NewBooking{
				CourtID:       c.ID,
				CustomerName:  form.Name,
				CustomerPhone: phone,
				Notes:         form.Notes,
				Start:         start,
				End:           start.Add(time.Duration(hours) * booking.SlotLength),
				TotalPrice:    booking.Price(hours, c.PricePerHour),
				Status:        store.StatusConfirmed,
				Source:        store.SourceAdmin,
			})
			switch {
			case err == nil:
				s.setFlash(w, "Booking "+code+" tersimpan.")
				http.Redirect(w, r, "/admin/jadwal?date="+form.Date, http.StatusSeeOther)
				return
			case errors.Is(err, store.ErrSlotTaken):
				errs.add("form", "Jadwal bentrok dengan booking lain di lapangan ini.")
			default:
				s.serverError(w, err)
				return
			}
		}
	}
	s.renderAdminBookingForm(w, r, v, form, errs, http.StatusUnprocessableEntity)
}

func (s *Server) handleCourts(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	courts, err := s.store.Courts(r.Context(), v.ID, false)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin/courts.html", data{
		"Title":  "Lapangan",
		"Venue":  v,
		"Courts": courts,
	})
}

type courtForm struct {
	Name     string
	Sport    string
	Price    string
	IsActive bool
}

// handleCourtForm menampilkan form tambah (tanpa {id}) atau edit lapangan.
func (s *Server) handleCourtForm(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	form := courtForm{Sport: sports[0], IsActive: true}
	var id int64
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, _ = parseID(raw)
		c, err := s.store.Court(r.Context(), v.ID, id)
		if errors.Is(err, store.ErrNotFound) {
			s.notFound(w, r)
			return
		}
		if err != nil {
			s.serverError(w, err)
			return
		}
		form = courtForm{Name: c.Name, Sport: c.Sport, Price: strconv.FormatInt(c.PricePerHour, 10), IsActive: c.IsActive}
	}
	s.renderCourtForm(w, r, v, id, form, nil, http.StatusOK)
}

func (s *Server) renderCourtForm(w http.ResponseWriter, r *http.Request, v store.Venue, id int64, form courtForm, errs formErrors, status int) {
	title := "Tambah lapangan"
	if id != 0 {
		title = "Ubah lapangan"
	}
	s.render(w, r, status, "admin/court_form.html", data{
		"Title":  title,
		"Venue":  v,
		"ID":     id,
		"Form":   form,
		"Errors": errs,
	})
}

func (s *Server) handleCourtSave(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	var id int64
	if raw := chi.URLParam(r, "id"); raw != "" {
		if id, ok = parseID(raw); !ok {
			s.notFound(w, r)
			return
		}
	}
	form := courtForm{
		Name:     field(r, "name"),
		Sport:    field(r, "sport"),
		Price:    field(r, "price"),
		IsActive: r.FormValue("active") == "on",
	}
	errs := formErrors{}
	checkLength(errs, "name", form.Name, "Nama lapangan", 1, 60)
	checkLength(errs, "sport", form.Sport, "Jenis olahraga", 1, 40)
	price, okPrice := parseRupiah(form.Price)
	if !okPrice || price == 0 {
		errs.add("price", "Isi harga per jam dalam rupiah, contoh 150000.")
	}
	if len(errs) > 0 {
		s.renderCourtForm(w, r, v, id, form, errs, http.StatusUnprocessableEntity)
		return
	}

	c := store.Court{ID: id, VenueID: v.ID, Name: form.Name, Sport: form.Sport, PricePerHour: price, IsActive: form.IsActive}
	var err error
	if id == 0 {
		_, err = s.store.CreateCourt(r.Context(), c)
	} else {
		err = s.store.UpdateCourt(r.Context(), c)
	}
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash(w, "Lapangan "+c.Name+" tersimpan.")
	http.Redirect(w, r, "/admin/lapangan", http.StatusSeeOther)
}

type hoursRow struct {
	Weekday time.Weekday
	Open    bool
	From    string
	To      string
}

// weekOrder: Senin di atas, Minggu paling bawah.
var weekOrder = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	rows, err := s.hoursRows(r.Context(), v.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderSettings(w, r, v, v, rows, nil, http.StatusOK)
}

func (s *Server) hoursRows(ctx context.Context, venueID int64) ([]hoursRow, error) {
	hours, err := s.store.OpeningHours(ctx, venueID)
	if err != nil {
		return nil, err
	}
	rows := make([]hoursRow, 0, 7)
	for _, wd := range weekOrder {
		row := hoursRow{Weekday: wd, From: "08:00", To: "22:00"}
		if h, ok := hours[wd]; ok {
			row.Open, row.From, row.To = true, clock(h.Open), clock(h.Close)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Server) renderSettings(w http.ResponseWriter, r *http.Request, v, form store.Venue, rows []hoursRow, errs formErrors, status int) {
	s.render(w, r, status, "admin/settings.html", data{
		"Title":  "Pengaturan",
		"Venue":  v,
		"Form":   form,
		"Hours":  rows,
		"Errors": errs,
	})
}

func (s *Server) handleVenueUpdate(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	form := v
	form.Name = field(r, "name")
	form.Phone = field(r, "phone")
	form.Address = field(r, "address")
	form.PaymentInfo = field(r, "payment_info")

	errs := formErrors{}
	checkLength(errs, "name", form.Name, "Nama tempat", 1, 100)
	if form.Phone != "" {
		if p, ok := normalizePhone(form.Phone); ok {
			form.Phone = p
		} else {
			errs.add("phone", "Nomor WhatsApp tidak valid. Contoh: 081234567890.")
		}
	}
	checkLength(errs, "address", form.Address, "Alamat", 0, 200)
	checkLength(errs, "payment_info", form.PaymentInfo, "Info pembayaran", 0, 1000)
	hold, err := strconv.Atoi(field(r, "hold_minutes"))
	if err != nil || hold < 0 || hold > 1440 {
		errs.add("hold_minutes", "Isi 0 sampai 1440 menit.")
	}
	window, err := strconv.Atoi(field(r, "booking_window_days"))
	if err != nil || window < 1 || window > 365 {
		errs.add("booking_window_days", "Isi 1 sampai 365 hari.")
	}
	form.HoldMinutes, form.BookingWindowDays = hold, window

	if len(errs) > 0 {
		rows, err := s.hoursRows(r.Context(), v.ID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		s.renderSettings(w, r, v, form, rows, errs, http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.UpdateVenue(r.Context(), form); err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash(w, "Profil tempat tersimpan.")
	http.Redirect(w, r, "/admin/pengaturan", http.StatusSeeOther)
}

func (s *Server) handleHoursUpdate(w http.ResponseWriter, r *http.Request) {
	v, ok := s.adminVenue(w, r)
	if !ok {
		return
	}
	errs := formErrors{}
	hours := make(map[time.Weekday]booking.Hours)
	rows := make([]hoursRow, 0, 7)
	for _, wd := range weekOrder {
		key := strconv.Itoa(int(wd))
		row := hoursRow{
			Weekday: wd,
			Open:    r.FormValue("open_"+key) == "on",
			From:    r.FormValue("from_" + key),
			To:      r.FormValue("to_" + key),
		}
		rows = append(rows, row)
		if !row.Open {
			continue
		}
		from, ok1 := parseClock(row.From)
		to, ok2 := parseClock(row.To)
		if !ok1 || !ok2 || to <= from {
			errs.add("hours", "Jam tutup hari "+dayNames[wd]+" harus setelah jam buka.")
			continue
		}
		hours[wd] = booking.Hours{Open: from, Close: to}
	}
	if len(errs) > 0 {
		s.renderSettings(w, r, v, v, rows, errs, http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.SetOpeningHours(r.Context(), v.ID, hours); err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash(w, "Jam buka tersimpan. Booking yang sudah ada tidak berubah.")
	http.Redirect(w, r, "/admin/pengaturan", http.StatusSeeOther)
}
