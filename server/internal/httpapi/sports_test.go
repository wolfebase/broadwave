package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"waveguide/internal/sports"
	"waveguide/internal/store"
)

type stubSports struct{}

func (stubSports) Scoreboard(context.Context, string, time.Time) ([]sports.Game, error) {
	return []sports.Game{{ID: "1", League: "nfl", Name: "Chiefs at Bills", State: "pre", Start: time.Now()}}, nil
}

func TestFollowTeamRoute(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/teams", strings.NewReader(`{"name":"Kansas City Chiefs","short":"Chiefs","league":"nfl","record":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Chiefs") {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
}

func TestScoreboardReturnsGames(t *testing.T) {
	h := (&Server{Store: testStore(t), Sports: stubSports{}}).Handler()
	res := get(t, h, "/api/v1/sports/scoreboard?league=nfl")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Chiefs at Bills") {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/scoreboard?league=quidditch", nil)
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, req)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", bad.Code, bad.Body.String())
	}
}

type boardStub struct {
	games []sports.Game
}

func (b boardStub) Scoreboard(context.Context, string, time.Time) ([]sports.Game, error) {
	return b.games, nil
}

func (b boardStub) Boards(context.Context, time.Time) ([]sports.Game, error) {
	return b.games, nil
}

func TestLinkGamesStoresTheMatch(t *testing.T) {
	st := testStore(t)
	start := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := st.ReplaceAirings(t.Context(), []store.Airing{
		{ChannelID: 1, Title: "Chiefs at Bills", Start: start, End: start.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	api := &Server{Store: st, Sports: boardStub{games: []sports.Game{{
		ID: "401772971", League: "nfl", Name: "Kansas City Chiefs at Buffalo Bills", Start: start,
		Teams: []sports.Team{
			{Name: "Buffalo Bills", Short: "Bills", Abbr: "BUF", Home: true},
			{Name: "Kansas City Chiefs", Short: "Chiefs", Abbr: "KC"},
		},
	}}}}
	api.LinkGames(t.Context())
	rows, err := st.Airings(t.Context(), start.Add(-time.Minute), start.Add(4*time.Hour))
	if err != nil || len(rows) != 1 || rows[0].GameID != "401772971" {
		t.Fatalf("%v %+v", err, rows)
	}
}
