package sports

import (
	"strings"
)

// Alert is a game moment worth a toast: a followed team's game starting, or
// a close finish. Every alert names a channel this home gets.
type Alert struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	GameID    string `json:"gameId"`
	ChannelID int64  `json:"channelId"`
	Channel   string `json:"channel"`
	Text      string `json:"text"`
	Detail    string `json:"detail,omitempty"`
}

// OnAir is a game on a channel this home gets, as the alert rules see it.
type OnAir struct {
	Game      Game
	ChannelID int64
	Channel   string
	// Followed is a game a followed team plays in.
	Followed bool
	// Spoiler is a game whose score must not show: it is recording, or was
	// recorded and not watched, or every score is hidden.
	Spoiler bool
}

// Alerts is what changed since the last look. prev holds each game as last
// seen; sent holds alert ids already given, and is updated. A game seen for
// the first time can't have started since the last look, so it never alerts
// as starting. closeGames off keeps to followed teams.
func Alerts(prev map[string]Game, games []OnAir, sent map[string]bool, closeGames bool) []Alert {
	var out []Alert
	add := func(a Alert) {
		if sent[a.ID] {
			return
		}
		sent[a.ID] = true
		out = append(out, a)
	}
	for _, on := range games {
		g := on.Game
		before, seen := prev[g.ID]
		base := Alert{GameID: g.ID, ChannelID: on.ChannelID, Channel: on.Channel}
		if on.Followed && g.State == "in" && seen && before.State == "pre" {
			a := base
			a.ID, a.Kind, a.Text = g.ID+":start", "start", banner("Starting now", g)
			add(a)
		}
		// A close finish is a spoiler by itself, so a recording's game gets none.
		if (closeGames || on.Followed) && !on.Spoiler && g.State == "in" && closeFinish(g) {
			a := base
			a.ID, a.Kind, a.Text, a.Detail = g.ID+":close", "close", banner("Close game", g), scoreLine(g)
			add(a)
		}
	}
	return out
}

// scoreLine is "KC 21, LV 20 · 4th 3:12", away team first.
func scoreLine(g Game) string {
	away, okA := g.Away()
	home, okH := g.Home()
	teams := g.Teams
	if okA && okH {
		teams = []Team{away, home}
	}
	var parts []string
	for _, t := range teams {
		if t.Score == "" {
			return g.Detail
		}
		parts = append(parts, teamLabel(t)+" "+t.Score)
	}
	line := strings.Join(parts, ", ")
	if g.Detail != "" {
		line += " · " + g.Detail
	}
	return line
}

// Plays reports whether a followed team (name, short name, abbreviation,
// and league as a follow stores them) is in the game.
func Plays(g Game, name, short, abbr, league string) bool {
	// A follow from a listing stores the listing's league ("College
	// Football"), which only a scoreboard id can be checked against.
	if _, known := FindLeague(strings.ToLower(league)); known && g.League != "" && !strings.EqualFold(league, g.League) {
		return false
	}
	for _, t := range g.Teams {
		switch {
		case abbr != "" && strings.EqualFold(abbr, t.Abbr):
			return true
		case name != "" && strings.EqualFold(name, t.Name):
			return true
		case short != "" && strings.EqualFold(short, t.Short) && (abbr == "" || t.Abbr == ""):
			return true
		}
	}
	return false
}
