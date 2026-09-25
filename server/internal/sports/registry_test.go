package sports

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type denyNet struct{}

func (denyNet) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("sports test dialed the network")
}

func TestOpenESPNDoesNotDial(t *testing.T) {
	restore := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: denyNet{}}
	t.Cleanup(func() { http.DefaultClient = restore })

	for _, name := range []string{"", "espn", "ESPN"} {
		provider, err := Open(name)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		espn, ok := provider.(*ESPN)
		if !ok || espn.Base != "https://site.api.espn.com/apis/site/v2/sports" {
			t.Fatalf("%q returned %#v", name, provider)
		}
	}
	if _, err := Open("no-such-provider"); err == nil {
		t.Fatal("expected an unknown provider to fail")
	}
}

func TestRegisteredProviderFeedsTheCache(t *testing.T) {
	restore := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: denyNet{}}
	t.Cleanup(func() { http.DefaultClient = restore })

	const want = "Fixture United at Registry City"
	Register("fixture", func() Provider {
		return &fake{games: []Game{{ID: "fixture-1", League: "nfl", Name: want, State: "pre"}}}
	})
	provider, err := Open("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, espn := provider.(*ESPN); espn {
		t.Fatal("fixture resolved to ESPN")
	}
	day := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	games, err := NewCache(provider).Scoreboard(t.Context(), "nfl", day)
	if err != nil || len(games) != 1 || games[0].Name != want || games[0].ID != "fixture-1" {
		t.Fatal(err, games)
	}
}
