package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"waveguide/internal/sports"
)

type stubSports struct{}

func (stubSports) Scoreboard(context.Context, string, time.Time) ([]sports.Game, error) {
	return []sports.Game{{ID: "1", League: "nfl", Name: "Chiefs at Bills", State: "pre", Start: time.Now()}}, nil
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
