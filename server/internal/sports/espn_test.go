package sports

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const chiefsAtBills = `{
  "events": [{
    "id": "401772971",
    "name": "Kansas City Chiefs at Buffalo Bills",
    "shortName": "KC @ BUF",
    "date": "2026-09-25T00:15:00Z",
    "status": {"type": {"state": "pre", "completed": false, "shortDetail": "9/24 - 8:15 PM EDT"}},
    "competitions": [{
      "status": {"period": 0, "displayClock": "0:00", "type": {"state": "pre", "completed": false, "shortDetail": "9/24 - 8:15 PM EDT"}},
      "broadcasts": [{"names": ["Prime Video"]}],
      "competitors": [
        {"homeAway": "home", "score": "0", "team": {"displayName": "Buffalo Bills", "shortDisplayName": "Bills", "abbreviation": "BUF", "color": "00338d", "alternateColor": "c60c30", "logo": "https://a.espncdn.com/buf.png"}},
        {"homeAway": "away", "score": "0", "team": {"displayName": "Kansas City Chiefs", "shortDisplayName": "Chiefs", "abbreviation": "KC", "color": "e31837", "alternateColor": "ffb81c", "logo": "https://a.espncdn.com/kc.png"}}
      ]
    }]
  }]
}`

func TestParseScoreboardReadsTeamsAndBroadcast(t *testing.T) {
	games, err := ParseScoreboard("nfl", []byte(chiefsAtBills))
	if err != nil || len(games) != 1 {
		t.Fatal(err, games)
	}
	game := games[0]
	home, ok := game.Home()
	away, ok2 := game.Away()
	if !ok || !ok2 || home.Abbr != "BUF" || away.Abbr != "KC" || home.Color != "#00338d" {
		t.Fatalf("%+v home %+v away %+v", game, home, away)
	}
	if game.State != "pre" || len(game.Broadcasts) != 1 || game.Broadcasts[0] != "Prime Video" || game.Live() {
		t.Fatalf("%+v", game)
	}
	if game.Start.Format(time.RFC3339) != "2026-09-25T00:15:00Z" {
		t.Fatal(game.Start)
	}
	short, err := ParseScoreboard("nfl", []byte(strings.Replace(chiefsAtBills, "2026-09-25T00:15:00Z", "2026-09-25T00:15Z", 1)))
	if err != nil || len(short) != 1 || short[0].Start.Format(time.RFC3339) != "2026-09-25T00:15:00Z" {
		t.Fatal(err, short)
	}
}

func TestESPNAsksForTheLeagueAndDay(t *testing.T) {
	var path, date string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		date = r.URL.Query().Get("dates")
		_, _ = w.Write([]byte(chiefsAtBills))
	}))
	defer srv.Close()
	client := &ESPN{Base: srv.URL, HTTP: srv.Client()}
	day := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	games, err := client.Scoreboard(t.Context(), "nfl", day)
	if err != nil || len(games) != 1 {
		t.Fatal(err, games)
	}
	if !strings.HasSuffix(path, "/football/nfl/scoreboard") || date != "20260924" {
		t.Fatalf("path %s date %s", path, date)
	}
	if _, err := client.Scoreboard(t.Context(), "quidditch", day); err == nil {
		t.Fatal("expected an unknown league to fail")
	}
}
