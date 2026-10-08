package httpapi

import (
	"context"
	"strings"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/sports"
	"broadwave/internal/store"
)

// GameAlerts looks at the scoreboard and tells every screen, as a
// game.alert event, when a followed team's game starts or a game comes down
// to a close finish on a channel this home gets. The gameAlerts setting is
// all (the default), teams (followed teams only), or off.
func (s *Server) GameAlerts(ctx context.Context) {
	if s == nil || s.Store == nil || s.Bus == nil {
		return
	}
	for _, alert := range s.gameAlerts(ctx) {
		s.Bus.Publish("game.alert", alert)
	}
}

func (s *Server) gameAlerts(ctx context.Context) []sports.Alert {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return nil
	}
	if settings["gameAlerts"] == "off" || settings["liveScores"] == "0" {
		s.alertPrev = nil
		return nil
	}
	now := s.now()
	byID := map[string]sports.Game{}
	for _, g := range s.boardsAround(ctx, now) {
		byID[g.ID] = g
	}
	if len(byID) == 0 {
		return nil
	}
	onAir := s.gamesOnAir(ctx, byID, settings["hideScores"] == "1", now)
	if s.alertSent == nil {
		s.alertSent = map[string]bool{}
	}
	alerts := sports.Alerts(s.alertPrev, onAir, s.alertSent, settings["gameAlerts"] != "teams")
	s.alertPrev = byID
	for id := range s.alertSent {
		if _, ok := byID[id[:max(0, strings.LastIndex(id, ":"))]]; !ok {
			delete(s.alertSent, id)
		}
	}
	return alerts
}

// gamesOnAir pairs each game with the channel it airs on now: an enabled,
// shown channel, the one that records a simulcast pair.
func (s *Server) gamesOnAir(ctx context.Context, games map[string]sports.Game, hideAll bool, now time.Time) []sports.OnAir {
	airings, err := s.Store.RecordingAirings(ctx, now.Add(-6*time.Hour), now.Add(15*time.Minute))
	if err != nil {
		return nil
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return nil
	}
	shown := map[int64]store.Channel{}
	for _, ch := range channels {
		if ch.Enabled && !ch.Hidden && !ch.Protected {
			shown[ch.ID] = ch
		}
	}
	follows, _ := s.Store.TeamFollows(ctx)
	spoiler := map[string]bool{}
	if recs, err := s.Store.Recordings(ctx); err == nil {
		for _, rec := range recs {
			if rec.GameID != "" && rec.Watched != 1 {
				spoiler[rec.GameID] = true
			}
		}
	}
	var out []sports.OnAir
	taken := map[string]bool{}
	for _, airing := range airings {
		g, ok := games[airing.GameID]
		if !ok || taken[g.ID] {
			continue
		}
		// A game runs past its listing; one that has ended is off the air.
		if g.State == "post" || g.Completed || (g.State != "in" && !airing.End.After(now)) {
			continue
		}
		id := airing.ChannelID
		if airing.Simulcast != 0 {
			id = airing.Simulcast
		}
		ch, ok := shown[id]
		if !ok {
			continue
		}
		number := ch.DisplayNumber
		if number == "" {
			number = ch.GuideNumber
		}
		on := sports.OnAir{Game: g, ChannelID: ch.ID, Channel: number, Spoiler: hideAll || spoiler[g.ID]}
		for _, team := range follows {
			if dvr.TeamOn(team, airing) || sports.Plays(g, team.Name, team.Short, team.Abbr, team.League) {
				on.Followed = true
				break
			}
		}
		taken[g.ID] = true
		out = append(out, on)
	}
	return out
}
