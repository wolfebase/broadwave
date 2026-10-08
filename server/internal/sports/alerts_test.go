package sports

import (
	"os"
	"testing"
)

func situationBoard(t *testing.T) map[string]Game {
	t.Helper()
	body, err := os.ReadFile("testdata/scoreboard-situation.json")
	if err != nil {
		t.Fatal(err)
	}
	games, err := ParseScoreboard("nfl", body)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Game{}
	for _, g := range games {
		switch g.ID {
		case "nhl-pp":
			g.League = "nhl"
		case "mls-late":
			g.League = "mls"
		}
		out[g.ID] = g
	}
	return out
}

func onAir(board map[string]Game, followed, spoiler map[string]bool) []OnAir {
	var out []OnAir
	for _, id := range []string{"nfl-redzone", "nhl-pp", "nfl-close", "nfl-lead", "nfl-quiet", "mls-late"} {
		out = append(out, OnAir{Game: board[id], ChannelID: 4, Channel: "4.1", Followed: followed[id], Spoiler: spoiler[id]})
	}
	return out
}

func TestAlertsForAStartAndCloseFinishes(t *testing.T) {
	board := situationBoard(t)
	prev := map[string]Game{}
	for id, g := range board {
		prev[id] = g
	}
	kickoff := board["nfl-redzone"]
	kickoff.State = "pre"
	prev["nfl-redzone"] = kickoff
	sent := map[string]bool{}
	alerts := Alerts(prev, onAir(board, map[string]bool{"nfl-redzone": true}, nil), sent, true)
	got := map[string]Alert{}
	for _, a := range alerts {
		got[a.ID] = a
	}
	if len(got) != 3 {
		t.Fatalf("alerts %+v", alerts)
	}
	if a := got["nfl-redzone:start"]; a.Text != "Starting now: CHI at LV" || a.Detail != "" || a.ChannelID != 4 || a.Channel != "4.1" {
		t.Fatalf("start %+v", a)
	}
	if a := got["nfl-close:close"]; a.Text != "Close game: BUF at MIA" || a.Detail != "BUF 21, MIA 24 · 4th 3:20" {
		t.Fatalf("close %+v", a)
	}
	if a := got["mls-late:close"]; a.Text != "Close game: LAFC at SEA" {
		t.Fatalf("soccer %+v", a)
	}
	if again := Alerts(board, onAir(board, map[string]bool{"nfl-redzone": true}, nil), sent, true); len(again) != 0 {
		t.Fatalf("said twice: %+v", again)
	}
}

func TestAlertsKeepSpoilersAndQuiet(t *testing.T) {
	board := situationBoard(t)
	prev := map[string]Game{}
	for id, g := range board {
		g.State = "pre"
		prev[id] = g
	}
	// A recording's game: its start still alerts (no score in it), its
	// close finish does not.
	alerts := Alerts(prev, onAir(board, map[string]bool{"nfl-close": true}, map[string]bool{"nfl-close": true, "mls-late": true}), map[string]bool{}, true)
	if len(alerts) != 1 || alerts[0].ID != "nfl-close:start" || alerts[0].Detail != "" {
		t.Fatalf("spoiler alerts %+v", alerts)
	}
	// Followed teams only: no close finish for a game nobody follows.
	if alerts := Alerts(board, onAir(board, nil, nil), map[string]bool{}, false); len(alerts) != 0 {
		t.Fatalf("teams-only alerts %+v", alerts)
	}
	if alerts := Alerts(board, onAir(board, map[string]bool{"mls-late": true}, nil), map[string]bool{}, false); len(alerts) != 1 || alerts[0].ID != "mls-late:close" {
		t.Fatalf("followed close %+v", alerts)
	}
	// A game seen for the first time is not "starting now": the server may
	// have just started in the middle of it.
	if alerts := Alerts(nil, onAir(board, map[string]bool{"nfl-redzone": true}, nil), map[string]bool{}, false); len(alerts) != 0 {
		t.Fatalf("first sight %+v", alerts)
	}
}

func TestPlaysMatchesAFollowedTeam(t *testing.T) {
	g := situationBoard(t)["nfl-close"]
	for _, c := range []struct {
		name, short, abbr, league string
		want                      bool
	}{
		{"", "", "buf", "nfl", true},
		{"Miami Dolphins", "", "", "", true},
		{"", "Bills", "", "nfl", true},
		{"", "", "BUF", "nba", false},
		{"Buffalo Sabres", "", "BUF2", "nhl", false},
		{"", "Bills", "BUFX", "", false},
		{"Bills", "Bills", "", "College Football", true},
		{"Bills", "Bills", "", "NFL", true},
	} {
		if got := Plays(g, c.name, c.short, c.abbr, c.league); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}
