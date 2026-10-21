package dvr

import (
	"strings"
	"testing"
	"time"

	"broadwave/internal/sports"
	"broadwave/internal/store"
)

func TestNextExtensionFollowsTheGame(t *testing.T) {
	start := time.Date(2026, 9, 24, 0, 20, 0, 0, time.UTC)
	now := start.Add(time.Hour)
	ends := now.Add(2 * time.Minute)
	game := sports.Game{ID: "1", State: "in", Start: start}

	ext, ok := NextExtension(4, ends, game, true, now)
	if !ok || !ext.Until.Equal(now.Add(10*time.Minute)) || !strings.Contains(ext.Note, "still on") {
		t.Fatalf("%v %+v", ok, ext)
	}
	if _, ok := NextExtension(4, now.Add(30*time.Minute), game, true, now); ok {
		t.Fatal("a recording that already runs past the next check should stay")
	}

	late := now.Add(4 * time.Hour)
	ext, ok = NextExtension(4, late.Add(2*time.Minute), sports.Game{ID: "1", State: "in", Start: start}, true, late)
	if !ok || !strings.Contains(ext.Note, "overtime") {
		t.Fatalf("%v %+v", ok, ext)
	}

	waiting := sports.Game{ID: "1", State: "pre", Start: start}
	ext, ok = NextExtension(4, ends, waiting, true, now)
	if !ok || !strings.Contains(ext.Note, "has not started") {
		t.Fatalf("%v %+v", ok, ext)
	}
	if _, ok := NextExtension(4, ends, waiting, true, start.Add(-time.Minute)); ok {
		t.Fatal("a game that is not late yet should keep its scheduled stop")
	}

	done := sports.Game{ID: "1", State: "post", Completed: true, Start: start}
	ext, ok = NextExtension(4, now.Add(2*time.Hour), done, true, now)
	if !ok || !ext.Until.Equal(now.Add(8*time.Minute)) || !strings.Contains(ext.Note, "over") {
		t.Fatalf("%v %+v", ok, ext)
	}
	if _, ok := NextExtension(4, now.Add(5*time.Minute), done, true, now); ok {
		t.Fatal("a recording already stopping soon should stay")
	}
	if _, ok := NextExtension(4, ends, game, false, now); ok {
		t.Fatal("a missing score should leave the recording alone")
	}
}

func TestUnmatchedSportsGetsAnExtraHour(t *testing.T) {
	start := time.Date(2026, 9, 24, 0, 20, 0, 0, time.UTC)
	airing := store.Airing{Title: "Bears at Bills", Category: "Sports", Start: start, End: start.Add(3 * time.Hour)}
	ok, minutes := StartDecision(store.Pass{}, airing, start.Add(-30*time.Second))
	if !ok || minutes < 230 || minutes > 245 {
		t.Fatalf("unmatched sports %v %d", ok, minutes)
	}
	airing.GameID = "1"
	ok, minutes = StartDecision(store.Pass{}, airing, start.Add(-30*time.Second))
	if !ok || minutes > 190 {
		t.Fatalf("matched game %v %d", ok, minutes)
	}
}

func TestUnmatchedSportsTailFitsTheSport(t *testing.T) {
	cases := []struct {
		category, title string
		want            time.Duration
	}{
		{"Sports event, Basketball", "Lakeside at Ridge", 30 * time.Minute},
		{"Sports, Soccer", "Liga MX", 30 * time.Minute},
		{"Sports event", "College Football", time.Hour},
		{"Sports event", "PGA Tour Golf", 90 * time.Minute},
		{"Auto racing", "IndyCar Series", 90 * time.Minute},
		{"Sports event", "Lakeside at Ridge", time.Hour},
		{"Sports talk", "Football Night", 0},
		{"Sports non-event", "Inside the Huddle", 0},
		{"Movie", "Football Dreams", 0},
		{"News", "Evening News", 0},
	}
	for _, tc := range cases {
		if got := SportsTail(store.Airing{Category: tc.category, Title: tc.title}); got != tc.want {
			t.Errorf("%s / %s: %v, want %v", tc.category, tc.title, got, tc.want)
		}
	}
	if got := SportsTail(store.Airing{Category: "Sports event, Golf", GameID: "9"}); got != 0 {
		t.Fatalf("a matched game follows the scoreboard, got %v", got)
	}
}
