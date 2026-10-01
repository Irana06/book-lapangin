package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"lapangin/internal/auth"
	"lapangin/internal/store"
	"lapangin/web"
)

type Options struct {
	// SecureCookies menambahkan flag Secure di cookie. Aktifkan di production (HTTPS).
	SecureCookies bool
}

type Server struct {
	logger   *slog.Logger
	store    *store.Store
	pages    map[string]*template.Template
	partials *template.Template
	secure   bool
	logins   *auth.Limiter
}

func New(logger *slog.Logger, st *store.Store, opts Options) (*Server, error) {
	pages, partials, err := parseTemplates(web.FS)
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:   logger,
		store:    st,
		pages:    pages,
		partials: partials,
		secure:   opts.SecureCookies,
		logins:   auth.NewLimiter(10, 15*time.Minute),
	}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Tolak POST dari website lain (proteksi CSRF bawaan Go).
	r.Use(http.NewCrossOriginProtection().Handler)
	r.Use(s.loadUser)
	r.NotFound(s.notFound)

	static, _ := fs.Sub(web.FS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(static)))

	r.Get("/", s.handleHome)
	r.Get("/healthz", s.handleHealth)
	r.Get("/cek", s.handleLookup)
	r.Get("/b/{code}", s.handleBookingStatus)
	r.Post("/b/{code}/batal", s.handleCustomerCancel)

	r.Get("/v/{slug}", s.handleVenue)
	r.Get("/v/{slug}/slots", s.handleSlots)
	r.Get("/v/{slug}/booking", s.handleBookForm)
	r.Post("/v/{slug}/booking", s.handleBookSubmit)

	r.Get("/admin/login", s.handleLoginForm)
	r.Post("/admin/login", s.handleLogin)

	r.Group(func(r chi.Router) {
		r.Use(s.requireUser)
		r.Post("/admin/logout", s.handleLogout)
		r.Get("/admin", s.handleDashboard)
		r.Get("/admin/jadwal", s.handleSchedule)
		r.Get("/admin/booking", s.handleBookings)
		r.Get("/admin/booking/baru", s.handleAdminBookingForm)
		r.Post("/admin/booking/baru", s.handleAdminBookingCreate)
		r.Post("/admin/booking/{id}/konfirmasi", s.handleConfirm)
		r.Post("/admin/booking/{id}/batal", s.handleCancel)
		r.Get("/admin/lapangan", s.handleCourts)
		r.Get("/admin/lapangan/baru", s.handleCourtForm)
		r.Post("/admin/lapangan/baru", s.handleCourtSave)
		r.Get("/admin/lapangan/{id}", s.handleCourtForm)
		r.Post("/admin/lapangan/{id}", s.handleCourtSave)
		r.Get("/admin/pengaturan", s.handleSettings)
		r.Post("/admin/pengaturan", s.handleVenueUpdate)
		r.Post("/admin/pengaturan/jam", s.handleHoursUpdate)
	})

	return r
}

// parseTemplates membuat satu template set per halaman (layout + partials + halaman),
// supaya block "content" di tiap halaman tidak saling menimpa. Halaman di
// templates/admin memakai admin_layout, sisanya memakai layout publik.
func parseTemplates(fsys fs.FS) (map[string]*template.Template, *template.Template, error) {
	partials, err := template.New("").Funcs(funcs).ParseFS(fsys, "templates/partials/*.html")
	if err != nil {
		return nil, nil, fmt.Errorf("parse partials: %w", err)
	}

	var files []string
	for _, pattern := range []string{"templates/pages/*.html", "templates/admin/*.html"} {
		matches, err := fs.Glob(fsys, pattern)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, matches...)
	}

	pages := make(map[string]*template.Template, len(files))
	for _, file := range files {
		t, err := template.New("").Funcs(funcs).ParseFS(fsys,
			"templates/layout.html", "templates/admin_layout.html", "templates/partials/*.html", file)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", file, err)
		}
		name := strings.TrimPrefix(file, "templates/")
		name = strings.TrimPrefix(name, "pages/")
		pages[name] = t
	}
	return pages, partials, nil
}

type data map[string]any

// render merender ke buffer dulu, jadi kalau template error
// user tidak menerima setengah halaman.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, d data) {
	t, ok := s.pages[page]
	if !ok {
		s.serverError(w, fmt.Errorf("page %q not found", page))
		return
	}
	if d == nil {
		d = data{}
	}
	d["User"] = currentUser(r)
	d["Flash"] = s.popFlash(w, r)
	d["Path"] = r.URL.Path

	layout := "layout"
	if strings.HasPrefix(page, "admin/") {
		layout = "admin_layout"
	}
	s.write(w, status, func(buf *bytes.Buffer) error {
		return t.ExecuteTemplate(buf, layout, d)
	})
}

func (s *Server) renderPartial(w http.ResponseWriter, status int, name string, d any) {
	s.write(w, status, func(buf *bytes.Buffer) error {
		return s.partials.ExecuteTemplate(buf, name, d)
	})
}

func (s *Server) write(w http.ResponseWriter, status int, exec func(*bytes.Buffer) error) {
	var buf bytes.Buffer
	if err := exec(&buf); err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.logger.Error("internal error", "err", err)
	http.Error(w, "Terjadi kesalahan di server. Coba lagi sebentar lagi.", http.StatusInternalServerError)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, "error.html", data{
		"Title":   "Halaman tidak ditemukan",
		"Message": "Halaman yang kamu cari tidak ada atau sudah dipindah.",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}
