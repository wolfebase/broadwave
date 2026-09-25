package sports

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTheSportsDBEmptyKeyDoesNotDial(t *testing.T) {
	restore := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: denyNet{}}
	t.Cleanup(func() { http.DefaultClient = restore })

	provider, err := Open("thesportsdb")
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Scoreboard(t.Context(), "nfl", time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatal(err)
	}
}

func TestTheSportsDBParsesADay(t *testing.T) {
	const key = "user-typed-key"
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		if strings.Contains(r.URL.Path, "strHomeTeamBadge") {
			t.Errorf("asked for a badge: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[
			{"idEvent":"9","strEvent":"Kansas City Chiefs vs Buffalo Bills","strLeague":"NFL","dateEvent":"2026-09-25","strTime":"00:20:00","strStatus":"FT","intHomeScore":"24","intAwayScore":27,"strHomeTeam":"Buffalo Bills","strAwayTeam":"Kansas City Chiefs","strHomeTeamBadge":"https://www.thesportsdb.com/badge.png"},
			{"idEvent":"10","strEvent":"Other","strLeague":"CFL","dateEvent":"2026-09-25","strStatus":"NS","strHomeTeam":"A","strAwayTeam":"B"}
		]}`))
	}))
	t.Cleanup(srv.Close)
	provider := NewTheSportsDB(key)
	provider.Base = srv.URL
	provider.HTTP = srv.Client()
	day := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	games, err := provider.Scoreboard(t.Context(), "nfl", day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/"+key+"/eventsday.php") || strings.Contains(gotPath, "badge") {
		t.Fatalf("path %s", gotPath)
	}
	if len(games) != 1 {
		t.Fatalf("%+v", games)
	}
	game := games[0]
	if game.ID != "9" || game.State != "post" || !game.Completed || game.League != "nfl" {
		t.Fatalf("%+v", game)
	}
	home, ok := game.Home()
	if !ok || home.Name != "Buffalo Bills" || home.Score != "24" || home.Logo != "" {
		t.Fatalf("home %+v", home)
	}
	away, ok := game.Away()
	if !ok || away.Name != "Kansas City Chiefs" || away.Score != "27" || away.Logo != "" {
		t.Fatalf("away %+v", away)
	}
}

func TestTheSportsDBErrorHidesTheKey(t *testing.T) {
	const key = "user-typed-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	provider := NewTheSportsDB(key)
	provider.Base = srv.URL
	provider.HTTP = srv.Client()
	_, err := provider.Scoreboard(t.Context(), "nfl", time.Now())
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatal(err)
	}
}

func TestOffDoesNotUseTheNetwork(t *testing.T) {
	restore := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: denyNet{}}
	t.Cleanup(func() { http.DefaultClient = restore })
	games, err := Off{}.Boards(t.Context(), time.Now())
	if err != nil || games != nil {
		t.Fatal(err, games)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTheSportsDBUsesItsOwnClient(t *testing.T) {
	provider := NewTheSportsDB("k")
	provider.HTTP = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("stopped")
	})}
	_, err := provider.Scoreboard(t.Context(), "nfl", time.Now())
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatal(err)
	}
}
