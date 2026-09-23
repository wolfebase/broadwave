package httpapi

import (
	"context"
	"log"
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

// LinkGames writes a scoreboard id onto each listing that is a game.
func (s *Server) LinkGames(ctx context.Context) {
	if s == nil || s.Store == nil || s.Sports == nil {
		return
	}
	board, ok := s.Sports.(interface {
		Boards(ctx context.Context, day time.Time) ([]sports.Game, error)
	})
	if !ok {
		return
	}
	now := time.Now()
	from := now.Add(-3 * time.Hour)
	to := now.Add(48 * time.Hour)
	seen := map[string]bool{}
	var games []sports.Game
	for _, day := range []time.Time{now, now.Add(24 * time.Hour), now.Add(48 * time.Hour)} {
		part, err := board.Boards(ctx, day)
		if err != nil {
			log.Printf("sports: %v", err)
			continue
		}
		for _, game := range part {
			if seen[game.ID] {
				continue
			}
			seen[game.ID] = true
			games = append(games, game)
		}
	}
	if len(games) == 0 {
		return
	}
	airings, err := s.Store.Airings(ctx, from, to)
	if err != nil {
		log.Printf("sports: %v", err)
		return
	}
	listings := make([]sports.Listing, 0, len(airings))
	for _, airing := range airings {
		listings = append(listings, sports.Listing{
			ID: airing.ID, Title: airing.Title, Subtitle: airing.Subtitle,
			Description: airing.Description, Category: airing.Category, Start: airing.Start,
		})
	}
	links := sports.Link(listings, games)
	if err := s.Store.SetAiringGames(ctx, from, to, links); err != nil {
		log.Printf("sports: %v", err)
		return
	}
	if len(links) > 0 {
		log.Printf("sports: matched %d listings", len(links))
	}
}
