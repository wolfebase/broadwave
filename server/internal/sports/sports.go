// Package sports reads public scoreboards and turns them into games the guide can match.
package sports

import (
	"context"
	"time"
)

// League is one competition a provider knows how to read.
type League struct {
	ID           string
	Name         string
	Sport        string
	Slug         string
	ScheduleOnly bool
}

// Leagues is the set the scoreboard asks for. F1 and NASCAR are schedules:
// the feed lists the event, not a play-by-play.
var Leagues = []League{
	{ID: "nfl", Name: "NFL", Sport: "football", Slug: "nfl"},
	{ID: "ncaaf", Name: "College Football", Sport: "football", Slug: "college-football"},
	{ID: "nba", Name: "NBA", Sport: "basketball", Slug: "nba"},
	{ID: "wnba", Name: "WNBA", Sport: "basketball", Slug: "wnba"},
	{ID: "ncaab", Name: "College Basketball", Sport: "basketball", Slug: "mens-college-basketball"},
	{ID: "mlb", Name: "MLB", Sport: "baseball", Slug: "mlb"},
	{ID: "nhl", Name: "NHL", Sport: "hockey", Slug: "nhl"},
	{ID: "mls", Name: "MLS", Sport: "soccer", Slug: "usa.1"},
	{ID: "nwsl", Name: "NWSL", Sport: "soccer", Slug: "usa.nwsl"},
	{ID: "epl", Name: "Premier League", Sport: "soccer", Slug: "eng.1"},
	{ID: "f1", Name: "F1", Sport: "racing", Slug: "f1", ScheduleOnly: true},
	{ID: "nascar", Name: "NASCAR", Sport: "racing", Slug: "nascar-premier", ScheduleOnly: true},
}

func FindLeague(id string) (League, bool) {
	for _, league := range Leagues {
		if league.ID == id {
			return league, true
		}
	}
	return League{}, false
}

// Team is one side of a game, or one entry in a schedule-only event.
type Team struct {
	Name     string `json:"name"`
	Short    string `json:"short,omitempty"`
	Abbr     string `json:"abbr,omitempty"`
	Score    string `json:"score,omitempty"`
	Home     bool   `json:"home,omitempty"`
	Color    string `json:"color,omitempty"`
	AltColor string `json:"altColor,omitempty"`
	Logo     string `json:"logo,omitempty"`
}

// Game is one event on a scoreboard.
type Game struct {
	ID         string    `json:"id"`
	League     string    `json:"league"`
	Name       string    `json:"name"`
	ShortName  string    `json:"shortName,omitempty"`
	Start      time.Time `json:"start"`
	State      string    `json:"state"`
	Completed  bool      `json:"completed,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Clock      string    `json:"clock,omitempty"`
	Period     int       `json:"period,omitempty"`
	Broadcasts []string  `json:"broadcasts,omitempty"`
	Teams      []Team    `json:"teams,omitempty"`
}

// Home is the home side, when the feed labeled one.
func (g Game) Home() (Team, bool) {
	for _, team := range g.Teams {
		if team.Home {
			return team, true
		}
	}
	return Team{}, false
}

// Away is the side that is not home, when the feed labeled a home side.
func (g Game) Away() (Team, bool) {
	home, ok := g.Home()
	if !ok {
		return Team{}, false
	}
	for _, team := range g.Teams {
		if team.Abbr != home.Abbr || team.Name != home.Name {
			return team, true
		}
	}
	return Team{}, false
}

// Live is true while the event is in progress.
func (g Game) Live() bool { return g.State == "in" }

// Provider reads one league's scoreboard for a calendar day.
type Provider interface {
	Scoreboard(ctx context.Context, league string, day time.Time) ([]Game, error)
}
