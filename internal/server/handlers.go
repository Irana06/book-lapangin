package server

import (
	"net/http"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "home.html", map[string]any{
		"Title": "Booking Lapangan",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

type courtRow struct {
	VenueName    string
	Name         string
	Sport        string
	PricePerHour int64
}

// handleCourtList dipanggil HTMX dan hanya mengembalikan potongan HTML.
func (s *Server) handleCourtList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT v.name, c.name, c.sport, c.price_per_hour
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		WHERE c.is_active
		ORDER BY v.name, c.name`)
	if err != nil {
		s.serverError(w, err)
		return
	}
	defer rows.Close()

	var courts []courtRow
	for rows.Next() {
		var c courtRow
		if err := rows.Scan(&c.VenueName, &c.Name, &c.Sport, &c.PricePerHour); err != nil {
			s.serverError(w, err)
			return
		}
		courts = append(courts, c)
	}
	if err := rows.Err(); err != nil {
		s.serverError(w, err)
		return
	}

	s.renderPartial(w, http.StatusOK, "court_list", courts)
}
