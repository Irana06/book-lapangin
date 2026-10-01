package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"lapangin/internal/auth"
	"lapangin/internal/store"
)

type ctxKey int

const userKey ctxKey = 0

const (
	sessionCookie = "lapangin_session"
	sessionTTL    = 30 * 24 * time.Hour
	flashCookie   = "lapangin_flash"
)

// loadUser membaca cookie session dan menaruh user yang login ke context.
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.store.SessionUser(r.Context(), auth.HashToken(c.Value))
		switch {
		case err == nil:
			r = r.WithContext(context.WithValue(r.Context(), userKey, &u))
		case !errors.Is(err, store.ErrNotFound):
			s.logger.Error("load session", "err", err)
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) != nil {
			next.ServeHTTP(w, r)
			return
		}
		if isHTMX(r) {
			w.Header().Set("HX-Redirect", "/admin/login")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// setFlash menyimpan pesan singkat yang ditampilkan sekali di halaman berikutnya
// (setelah redirect).
func (s *Server) setFlash(w http.ResponseWriter, msg string) {
	s.setCookie(w, flashCookie, base64.RawURLEncoding.EncodeToString([]byte(msg)), time.Minute)
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return ""
	}
	s.clearCookie(w, flashCookie)
	msg, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	return string(msg)
}
