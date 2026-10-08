package httpapi

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/sports"
	"broadwave/internal/store"
)

type stateBoard struct{ games *[]sports.Game }

func (b stateBoard) Scoreboard(context.Context, string, time.Time) ([]sports.Game, error) {
	return *b.games, nil
}

func (b stateBoard) Boards(context.Context, time.Time) ([]sports.Game, error) {
	return *b.games, nil
}

func TestGameAlertsNameTheChannelAndKeepSpoilers(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV", StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "5.1", GuideName: "WTST", StreamURL: "http://flex:5004/auto/v5.1"},
	}); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	chs, _ := st.Channels(ctx, false)
	for _, ch := range chs {
		ids[ch.GuideNumber] = ch.ID
	}
	now := time.Date(2026, 10, 11, 19, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: ids["4.1"], Title: "NFL Football", Subtitle: "Bills at Dolphins", Start: start, End: now.Add(-5 * time.Minute)},
		{ChannelID: ids["5.1"], Title: "NFL Football", Subtitle: "Bears at Raiders", Start: now.Add(5 * time.Minute), End: now.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	airings, _ := st.Airings(ctx, start.Add(-time.Minute), now.Add(4*time.Hour))
	links := map[int64]string{}
	for _, a := range airings {
		links[a.ID] = map[string]string{"Bills at Dolphins": "close", "Bears at Raiders": "kick"}[a.Subtitle]
	}
	if err := st.SetAiringGames(ctx, start.Add(-time.Minute), now.Add(4*time.Hour), links); err != nil {
		t.Fatal(err)
	}
	// Shaped as the Sports page follows a team: the side's name and the
	// listing's league, no abbreviation.
	if err := st.FollowTeam(ctx, store.TeamFollow{Name: "Bears", Short: "Bears", League: "NFL Football"}); err != nil {
		t.Fatal(err)
	}
	team := func(name, abbr, score string, home bool) sports.Team {
		return sports.Team{Name: name, Abbr: abbr, Score: score, Home: home}
	}
	games := []sports.Game{
		{ID: "close", League: "nfl", State: "in", Period: 4, Clock: "2:10", Detail: "4th 2:10", Start: start,
			Teams: []sports.Team{team("Miami Dolphins", "MIA", "24", true), team("Buffalo Bills", "BUF", "21", false)}},
		{ID: "kick", League: "nfl", State: "pre", Start: now.Add(5 * time.Minute),
			Teams: []sports.Team{team("Las Vegas Raiders", "LV", "", true), team("Chicago Bears", "CHI", "", false)}},
	}
	api := &Server{Store: st, Sports: stateBoard{&games}, Clock: func() time.Time { return now }}

	// The first look only learns the board; the close game, running past
	// its listing's end, alerts at once on its channel.
	alerts := api.gameAlerts(ctx)
	if len(alerts) != 1 || alerts[0].ID != "close:close" || alerts[0].ChannelID != ids["4.1"] || alerts[0].Channel != "4.1" ||
		alerts[0].Detail != "BUF 21, MIA 24 · 4th 2:10" {
		t.Fatalf("first look %+v", alerts)
	}
	now = now.Add(6 * time.Minute)
	games[1].State = "in"
	alerts = api.gameAlerts(ctx)
	if len(alerts) != 1 || alerts[0].Text != "Starting now: CHI at LV" || alerts[0].Channel != "5.1" {
		t.Fatalf("kickoff %+v", alerts)
	}
	if again := api.gameAlerts(ctx); len(again) != 0 {
		t.Fatalf("said twice %+v", again)
	}

	// Each rule on a server that has not seen the close game yet: it would
	// alert at first look unless the rule holds it back.
	firstLook := func(settings map[string]string) []sports.Alert {
		t.Helper()
		if err := st.PutSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		return (&Server{Store: st, Sports: stateBoard{&games}, Clock: func() time.Time { return now }}).gameAlerts(ctx)
	}
	if alerts := firstLook(map[string]string{}); len(alerts) != 1 || alerts[0].ID != "close:close" {
		t.Fatalf("control %+v", alerts)
	}
	for _, c := range []struct {
		name     string
		set, put map[string]string
	}{
		{"alerts off", map[string]string{"gameAlerts": "off"}, map[string]string{"gameAlerts": "all"}},
		{"teams only", map[string]string{"gameAlerts": "teams"}, map[string]string{"gameAlerts": "all"}},
		{"scores hidden", map[string]string{"hideScores": "1"}, map[string]string{"hideScores": "0"}},
	} {
		if alerts := firstLook(c.set); len(alerts) != 0 {
			t.Fatalf("%s: %+v", c.name, alerts)
		}
		if err := st.PutSettings(ctx, c.put); err != nil {
			t.Fatal(err)
		}
	}
	hidden := true
	if _, err := st.PatchChannel(ctx, ids["4.1"], store.ChannelPatch{Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	if alerts := firstLook(map[string]string{}); len(alerts) != 0 {
		t.Fatalf("a hidden channel: %+v", alerts)
	}
	hidden = false
	if _, err := st.PatchChannel(ctx, ids["4.1"], store.ChannelPatch{Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	// A game this home is recording and hasn't watched gets no close finish.
	if _, err := st.CreateRecording(ctx, store.Recording{ChannelID: ids["4.1"], Title: "NFL Football", Path: "/r/x.ts", Status: "recording", StartedAt: start, GameID: "close"}); err != nil {
		t.Fatal(err)
	}
	if alerts := firstLook(map[string]string{}); len(alerts) != 0 {
		t.Fatalf("spoiled a recording %+v", alerts)
	}
}
