package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/sports"
	"broadwave/internal/store"
)

func (s *Server) scoreboard(w http.ResponseWriter, r *http.Request) {
	provider := s.activeSports(r.Context())
	if provider == nil {
		httpError(w, "scores are unavailable", http.StatusServiceUnavailable)
		return
	}
	day := s.now()
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
		slog.Error(fmt.Sprintf("sports: %v", err))
		httpError(w, "Scores are unavailable right now.", http.StatusBadGateway)
		return
	}
	if games == nil {
		games = []sports.Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": s.withoutSpoilers(r.Context(), games)})
}

// activeSports is the ESPN cache, a TheSportsDB cache for a key the user
// typed, or Off when live scores are turned off. Off does not dial.
func (s *Server) activeSports(ctx context.Context) sports.Provider {
	if s != nil && s.Store != nil {
		if settings, err := s.Store.Settings(ctx); err == nil {
			if settings["liveScores"] == "0" {
				return sports.Off{}
			}
			if key := strings.TrimSpace(settings["sportsdbKey"]); key != "" {
				s.sportsMu.Lock()
				defer s.sportsMu.Unlock()
				if s.sportsDB == nil || s.sportsKey != key {
					s.sportsKey = key
					s.sportsDB = sports.NewCache(sports.NewTheSportsDB(key))
				}
				return s.sportsDB
			}
		}
	}
	if s == nil {
		return nil
	}
	return s.Sports
}

func (s *Server) withoutSpoilers(ctx context.Context, games []sports.Game) []sports.Game {
	games = withoutRemoteLogos(games)
	if s.Store == nil || len(games) == 0 {
		return games
	}
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		settings = map[string]string{}
	}
	hideAll := settings["hideScores"] == "1"
	hidden := map[string]bool{}
	if !hideAll {
		recs, recErr := s.Store.Recordings(ctx)
		if recErr == nil {
			for _, rec := range recs {
				if rec.GameID != "" && rec.Watched != 1 {
					hidden[rec.GameID] = true
				}
			}
		}
	}
	games = append([]sports.Game(nil), games...)
	for i := range games {
		if hideAll || hidden[games[i].ID] {
			games[i] = sports.HideScore(games[i])
		}
	}
	return games
}

func withoutRemoteLogos(games []sports.Game) []sports.Game {
	if len(games) == 0 {
		return games
	}
	out := append([]sports.Game(nil), games...)
	for i := range out {
		if len(out[i].Teams) == 0 {
			continue
		}
		teams := append([]sports.Team(nil), out[i].Teams...)
		for t := range teams {
			if strings.Contains(teams[t].Logo, "://") {
				teams[t].Logo = ""
			}
		}
		out[i].Teams = teams
	}
	return out
}

func (s *Server) boardsAround(ctx context.Context, now time.Time) []sports.Game {
	provider := s.activeSports(ctx)
	board, ok := provider.(interface {
		Boards(ctx context.Context, day time.Time) ([]sports.Game, error)
	})
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var games []sports.Game
	for _, day := range []time.Time{now, now.Add(24 * time.Hour), now.Add(48 * time.Hour)} {
		part, err := board.Boards(ctx, day)
		if err != nil {
			slog.Error(fmt.Sprintf("sports: %v", err))
			continue
		}
		for _, game := range part {
			if game.ID == "" || seen[game.ID] {
				continue
			}
			seen[game.ID] = true
			games = append(games, game)
		}
	}
	return games
}

// LinkGames writes a scoreboard id onto each listing that is a game.
func (s *Server) LinkGames(ctx context.Context) {
	if s == nil || s.Store == nil || s.Sports == nil {
		return
	}
	now := time.Now()
	from := now.Add(-3 * time.Hour)
	to := now.Add(48 * time.Hour)
	games := s.boardsAround(ctx, now)
	if len(games) == 0 {
		return
	}
	airings, err := s.Store.Airings(ctx, from, to)
	if err != nil {
		slog.Error(fmt.Sprintf("sports: %v", err))
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
		slog.Error(fmt.Sprintf("sports: %v", err))
		return
	}
	if len(links) > 0 {
		slog.Info(fmt.Sprintf("sports: matched %d listings", len(links)))
	}
	s.NoteTeams(ctx)
}

// ExtendRecordings keeps a matched game recording going until the game is over.
func (s *Server) ExtendRecordings(ctx context.Context) {
	if s == nil || s.Store == nil || s.Hub == nil || s.Sports == nil {
		return
	}
	recs, err := s.Store.Recordings(ctx)
	if err != nil {
		return
	}
	var open []store.Recording
	for _, rec := range recs {
		if rec.Status == "recording" && rec.GameID != "" && rec.EndsAt != nil {
			open = append(open, rec)
		}
	}
	if len(open) == 0 {
		return
	}
	now := time.Now()
	byID := map[string]sports.Game{}
	for _, game := range s.boardsAround(ctx, now) {
		byID[game.ID] = game
	}
	for _, rec := range open {
		game, found := byID[rec.GameID]
		ext, ok := dvr.NextExtension(rec.ID, *rec.EndsAt, game, found, now)
		if !ok {
			continue
		}
		if err := s.Hub.ExtendRecording(ctx, ext.ID, ext.Until); err != nil {
			slog.Error(fmt.Sprintf("recording: %v", err))
			continue
		}
		_ = s.Store.AddEvent(ctx, "recording", ext.Note)
	}
}
