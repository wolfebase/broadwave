package sports

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func eventFixture(t *testing.T, league, id string) Game {
	t.Helper()
	raw, err := os.ReadFile("testdata/scoreboard-situation.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, ev := range doc.Events {
		var probe struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(ev, &probe) != nil || probe.ID != id {
			continue
		}
		body, err := json.Marshal(map[string]any{"events": []json.RawMessage{ev}})
		if err != nil {
			t.Fatal(err)
		}
		games, err := ParseScoreboard(league, body)
		if err != nil || len(games) != 1 {
			t.Fatal(err, games)
		}
		return games[0]
	}
	t.Fatalf("missing event %s", id)
	return Game{}
}

func withScores(g Game, away, home string) Game {
	teams := append([]Team(nil), g.Teams...)
	g.Teams = teams
	for i := range g.Teams {
		if g.Teams[i].Home {
			g.Teams[i].Score = home
		} else {
			g.Teams[i].Score = away
		}
	}
	return g
}

func TestSituationFixtureNeedsNoLogos(t *testing.T) {
	raw, err := os.ReadFile("testdata/scoreboard-situation.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "://") || strings.Contains(strings.ToLower(string(raw)), "logo") {
		t.Fatal("fixture should not carry a logo url")
	}
	games, err := ParseScoreboard("nfl", raw)
	if err != nil || len(games) != 6 {
		t.Fatal(err, len(games))
	}
	for _, game := range games {
		for _, team := range game.Teams {
			if team.Logo != "" {
				t.Fatalf("%s kept logo %q", game.ID, team.Logo)
			}
		}
	}
	zone := eventFixture(t, "nfl", "nfl-redzone")
	if !zone.RedZone || zone.PowerPlay || zone.Situation != "Red zone" || zone.Clock != "8:12" || zone.Period != 3 {
		t.Fatalf("%+v", zone)
	}
	power := eventFixture(t, "nhl", "nhl-pp")
	if !power.PowerPlay || power.RedZone || power.Situation != "Power play" || power.League != "nhl" {
		t.Fatalf("%+v", power)
	}
	quiet := eventFixture(t, "nfl", "nfl-quiet")
	if quiet.RedZone || quiet.Situation != "GB, 2nd & 5" {
		t.Fatalf("%+v", quiet)
	}
	for _, team := range zone.Teams {
		if team.Name == "" || team.Abbr == "" {
			t.Fatalf("%+v", zone.Teams)
		}
	}
}

func TestPickFocusRedZoneWins(t *testing.T) {
	now := time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC)
	zone := eventFixture(t, "nfl", "nfl-redzone")
	closeGame := eventFixture(t, "nfl", "nfl-close")
	quiet := eventFixture(t, "nfl", "nfl-quiet")
	got := PickFocus(now, time.Time{}, []Game{closeGame, quiet, zone}, nil)
	if got.KeepManual || got.GameID != "nfl-redzone" || got.Banner != "Red zone: KC at LV" {
		t.Fatalf("%+v", got)
	}
	both := closeGame
	both.RedZone = true
	got = PickFocus(now, time.Time{}, []Game{both}, nil)
	if got.GameID != both.ID || got.Banner != "Red zone: BUF at MIA" {
		t.Fatalf("red zone outranks final minutes: %+v", got)
	}
	done := zone
	done.State = "post"
	got = PickFocus(now, time.Time{}, []Game{done, quiet}, nil)
	if got.GameID != "" || got.Banner != "" || got.KeepManual {
		t.Fatalf("finished game: %+v", got)
	}
}

func TestPickFocusPowerPlayWins(t *testing.T) {
	now := time.Date(2026, 9, 25, 23, 0, 0, 0, time.UTC)
	power := eventFixture(t, "nhl", "nhl-pp")
	lead := eventFixture(t, "nfl", "nfl-lead")
	prev := withScores(lead, "7", "14")
	got := PickFocus(now, time.Time{}, []Game{lead, power}, []Game{prev})
	if got.KeepManual || got.GameID != "nhl-pp" || got.Banner != "Power play: BOS at NYR" {
		t.Fatalf("%+v", got)
	}
	late := power
	late.Period = 3
	late.Clock = "1:00"
	late.Detail = "3rd 1:00"
	got = PickFocus(now, time.Time{}, []Game{late}, nil)
	if got.Banner != "Power play: BOS at NYR" {
		t.Fatalf("power play outranks final minutes: %+v", got)
	}
}

func TestPickFocusLeadChange(t *testing.T) {
	now := time.Date(2026, 9, 25, 20, 30, 0, 0, time.UTC)
	lead := eventFixture(t, "nfl", "nfl-lead")
	quiet := eventFixture(t, "nfl", "nfl-quiet")
	closeGame := eventFixture(t, "nfl", "nfl-close")
	prevLead := withScores(lead, "7", "14")
	got := PickFocus(now, time.Time{}, []Game{closeGame, quiet, lead}, []Game{quiet, prevLead})
	if got.KeepManual || got.GameID != "nfl-lead" || got.Banner != "Lead change: DAL at PHI" {
		t.Fatalf("%+v", got)
	}
	same := PickFocus(now, time.Time{}, []Game{lead, quiet}, []Game{lead, quiet})
	if same.GameID != "" {
		t.Fatalf("same leader: %+v", same)
	}
	first := PickFocus(now, time.Time{}, []Game{withScores(lead, "7", "0"), quiet}, []Game{withScores(lead, "0", "0"), quiet})
	if first.GameID != "" {
		t.Fatalf("first lead is not a lead change: %+v", first)
	}
}

func TestPickFocusCloseFinish(t *testing.T) {
	now := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	closeGame := eventFixture(t, "nfl", "nfl-close")
	quiet := eventFixture(t, "nfl", "nfl-quiet")
	got := PickFocus(now, time.Time{}, []Game{quiet, closeGame}, []Game{quiet, closeGame})
	if got.KeepManual || got.GameID != "nfl-close" || got.Banner != "Final minutes: BUF at MIA" {
		t.Fatalf("%+v", got)
	}

	t.Run("eight points", func(t *testing.T) {
		g := withScores(closeGame, "20", "28")
		got := PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.GameID != g.ID {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("nine points", func(t *testing.T) {
		g := withScores(closeGame, "20", "29")
		got := PickFocus(now, time.Time{}, []Game{g, quiet}, nil)
		if got.GameID != "" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("five minutes left", func(t *testing.T) {
		g := closeGame
		g.Clock = "5:00"
		g.Detail = "4th 5:00"
		got := PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.GameID != "" {
			t.Fatalf("%+v", got)
		}
		g.Clock = "4:59"
		g.Detail = "4th 4:59"
		got = PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.Banner != "Final minutes: BUF at MIA" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("detail names the period", func(t *testing.T) {
		g := closeGame
		g.Period = 0
		g.Clock = ""
		g.Detail = "4th 2:05"
		got := PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.Banner != "Final minutes: BUF at MIA" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("third quarter", func(t *testing.T) {
		g := closeGame
		g.Period = 3
		g.Clock = "2:00"
		g.Detail = "3rd 2:00"
		got := PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.GameID != "" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("halftime", func(t *testing.T) {
		g := closeGame
		g.Period = 2
		g.Clock = "0:00"
		g.Detail = "Halftime"
		got := PickFocus(now, time.Time{}, []Game{g}, nil)
		if got.GameID != "" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("hockey", func(t *testing.T) {
		g := eventFixture(t, "nhl", "nhl-pp")
		g.PowerPlay = false
		g.Period = 3
		g.Clock = "4:00"
		g.Detail = "3rd 4:00"
		got := PickFocus(now, time.Time{}, []Game{g, quiet}, nil)
		if got.Banner != "Final minutes: BOS at NYR" {
			t.Fatalf("%+v", got)
		}
		wide := withScores(g, "4", "1")
		got = PickFocus(now, time.Time{}, []Game{wide}, nil)
		if got.GameID != "" {
			t.Fatalf("two-goal game: %+v", got)
		}
	})
	t.Run("soccer", func(t *testing.T) {
		g := eventFixture(t, "mls", "mls-late")
		got := PickFocus(now, time.Time{}, []Game{quiet, g}, nil)
		if got.Banner != "Final minutes: LAFC at SEA" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("first of two", func(t *testing.T) {
		other := closeGame
		other.ID = "nfl-close-2"
		got := PickFocus(now, time.Time{}, []Game{closeGame, other}, nil)
		if got.GameID != closeGame.ID {
			t.Fatalf("%+v", got)
		}
	})
}

func TestPickFocusManualHold(t *testing.T) {
	now := time.Date(2026, 9, 25, 20, 2, 0, 0, time.UTC)
	zone := eventFixture(t, "nfl", "nfl-redzone")
	closeGame := eventFixture(t, "nfl", "nfl-close")
	games := []Game{closeGame, zone}
	held := PickFocus(now, now.Add(-2*time.Minute+time.Millisecond), games, nil)
	if !held.KeepManual || held.GameID != "" || held.Banner != "" {
		t.Fatalf("under 2 minutes: %+v", held)
	}
	free := PickFocus(now, now.Add(-2*time.Minute), games, nil)
	if free.KeepManual || free.GameID != "nfl-redzone" || free.Banner != "Red zone: KC at LV" {
		t.Fatalf("at 2 minutes: %+v", free)
	}
	if PickFocus(now, time.Time{}, games, nil).KeepManual {
		t.Fatal("zero manual time should not hold")
	}
	if got := PickFocus(now, now.Add(time.Minute), games, nil); got.KeepManual || got.GameID != "nfl-redzone" {
		t.Fatalf("future manual time: %+v", got)
	}
}
