package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangin/web"
)

type Server struct {
	logger   *slog.Logger
	db       *pgxpool.Pool
	pages    map[string]*template.Template
	partials *template.Template
}

func New(logger *slog.Logger, db *pgxpool.Pool) (*Server, error) {
	pages, partials, err := parseTemplates(web.FS)
	if err != nil {
		return nil, err
	}
	return &Server{logger: logger, db: db, pages: pages, partials: partials}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	static, _ := fs.Sub(web.FS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(static)))

	r.Get("/", s.handleHome)
	r.Get("/healthz", s.handleHealth)
	r.Get("/partials/courts", s.handleCourtList)

	return r
}

// parseTemplates membuat satu template set per halaman (layout + partials + halaman),
// supaya block "content" di tiap halaman tidak saling menimpa.
func parseTemplates(fsys fs.FS) (map[string]*template.Template, *template.Template, error) {
	partials, err := template.ParseFS(fsys, "templates/partials/*.html")
	if err != nil {
		return nil, nil, fmt.Errorf("parse partials: %w", err)
	}

	pageFiles, err := fs.Glob(fsys, "templates/pages/*.html")
	if err != nil {
		return nil, nil, err
	}

	pages := make(map[string]*template.Template, len(pageFiles))
	for _, file := range pageFiles {
		t, err := template.ParseFS(fsys, "templates/layout.html", "templates/partials/*.html", file)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", file, err)
		}
		pages[path.Base(file)] = t
	}
	return pages, partials, nil
}

// render merender ke buffer dulu, jadi kalau template error
// user tidak menerima setengah halaman.
func (s *Server) render(w http.ResponseWriter, status int, page string, data any) {
	t, ok := s.pages[page]
	if !ok {
		s.serverError(w, fmt.Errorf("page %q not found", page))
		return
	}
	s.write(w, status, func(buf *bytes.Buffer) error {
		return t.ExecuteTemplate(buf, "layout", data)
	})
}

func (s *Server) renderPartial(w http.ResponseWriter, status int, name string, data any) {
	s.write(w, status, func(buf *bytes.Buffer) error {
		return s.partials.ExecuteTemplate(buf, name, data)
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
	http.Error(w, "Terjadi kesalahan di server", http.StatusInternalServerError)
}
