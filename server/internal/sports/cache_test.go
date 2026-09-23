package sports

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fake struct {
	games []Game
	err   error
	calls int
}

func (f *fake) Scoreboard(context.Context, string, time.Time) ([]Game, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.games, nil
}

func TestCacheRefreshesLiveGamesSooner(t *testing.T) {
	now := time.Date(2026, 9, 23, 20, 0, 0, 0, time.UTC)
	src := &fake{games: []Game{{ID: "1", State: "in"}}}
	cache := NewCache(src)
	cache.now = func() time.Time { return now }
	if _, err := cache.Scoreboard(t.Context(), "nfl", now); err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	if _, err := cache.Scoreboard(t.Context(), "nfl", now); err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 {
		t.Fatalf("calls %d, a live board should still be fresh at 20s", src.calls)
	}
	now = now.Add(20 * time.Second)
	if _, err := cache.Scoreboard(t.Context(), "nfl", now); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 {
		t.Fatalf("calls %d, a live board should refresh after 30s", src.calls)
	}
}

func TestCacheKeepsTheLastBoardWhenTheFeedFails(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	src := &fake{games: []Game{{ID: "9", State: "pre", Name: "Chiefs at Bills"}}}
	cache := NewCache(src)
	cache.now = func() time.Time { return now }
	if _, err := cache.Scoreboard(t.Context(), "nfl", now); err != nil {
		t.Fatal(err)
	}
	src.err = errors.New("down")
	now = now.Add(3 * time.Hour)
	games, err := cache.Scoreboard(t.Context(), "nfl", now)
	if err != nil || len(games) != 1 || games[0].Name != "Chiefs at Bills" {
		t.Fatal(err, games)
	}
	calls := src.calls
	now = now.Add(10 * time.Second)
	if _, err := cache.Scoreboard(t.Context(), "nfl", now); err != nil {
		t.Fatal(err)
	}
	if src.calls != calls {
		t.Fatal("a failed feed should back off before trying again")
	}
}

func TestHideScoreLeavesTheBoardAlone(t *testing.T) {
	original := Game{
		ID: "1", State: "post", Completed: true, Detail: "Final",
		Teams: []Team{{Name: "Chiefs", Score: "27"}, {Name: "Bills", Score: "24"}},
	}
	hidden := HideScore(original)
	if original.Teams[0].Score != "27" || original.Detail != "Final" {
		t.Fatalf("board was changed: %+v", original)
	}
	if hidden.Teams[0].Score != "" || hidden.Teams[1].Score != "" || hidden.Detail != "" {
		t.Fatalf("score still visible: %+v", hidden)
	}
	if hidden.Teams[0].Name != "Chiefs" {
		t.Fatalf("team name lost: %+v", hidden)
	}
}
