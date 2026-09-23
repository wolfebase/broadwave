package httpapi

import (
	"context"
	"net/http"
	"time"

	"waveguide/internal/sports"
)

func (s *Server) scoreboard(w http.ResponseWriter, r *http.Request) {
	provider := s.Sports
	if provider == nil {
		httpError(w, "scores are unavailable", http.StatusServiceUnavailable)
		return
	}
	day := time.Now()
	if raw := r.URL.Query().Get("date"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpError(w, "date needs to be YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		day = parsed
	}
	league := r.URL.Query().Get("league")
	var games []sports.Game
	var err error
	if league == "" {
		board, ok := provider.(interface {
			Boards(ctx context.Context, day time.Time) ([]sports.Game, error)
		})
		if !ok {
			httpError(w, "scores are unavailable", http.StatusServiceUnavailable)
			return
		}
		games, err = board.Boards(r.Context(), day)
	} else {
		if _, ok := sports.FindLeague(league); !ok {
			httpError(w, "unknown league", http.StatusBadRequest)
			return
		}
		games, err = provider.Scoreboard(r.Context(), league, day)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if games == nil {
		games = []sports.Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}
