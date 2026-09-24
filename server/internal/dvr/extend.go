package dvr

import (
	"fmt"
	"strings"
	"time"

	"broadwave/internal/sports"
	"broadwave/internal/store"
)

const (
	liveExtend    = 10 * time.Minute
	postTail      = 8 * time.Minute
	overtimeAfter = 3*time.Hour + 15*time.Minute
	sportsPad     = 60 * time.Minute
)

// Extend is a new stop time for a recording that is following a game.
type Extend struct {
	ID    int64
	Until time.Time
	Note  string
}

// NextExtension decides whether a live recording should run longer or stop
// soon. A game still in progress, or late to start, gains ten minutes.
// A finished game stops eight minutes later. An unknown game is left alone.
func NextExtension(id int64, ends time.Time, game sports.Game, found bool, now time.Time) (Extend, bool) {
	if !found {
		return Extend{}, false
	}
	if game.State == "post" || game.Completed {
		stop := now.Add(postTail)
		if !ends.IsZero() && ends.After(now) && !ends.After(stop.Add(time.Minute)) {
			return Extend{}, false
		}
		return Extend{ID: id, Until: stop, Note: "The game is over. Stopping in 8 min."}, true
	}
	if game.State != "in" && game.State != "pre" {
		return Extend{}, false
	}
	if game.State == "pre" && !now.After(game.Start) {
		return Extend{}, false
	}
	horizon := now.Add(liveExtend)
	if !ends.IsZero() && !ends.Before(horizon) {
		return Extend{}, false
	}
	added := int(liveExtend / time.Minute)
	if !ends.IsZero() && ends.After(now) {
		added = int(horizon.Sub(ends) / time.Minute)
		if added < 1 {
			added = 1
		}
	}
	note := fmt.Sprintf("Extended %d min, the game is still on.", added)
	switch {
	case game.State == "pre":
		note = fmt.Sprintf("Extended %d min, the game has not started.", added)
	case now.After(game.Start.Add(overtimeAfter)):
		note = fmt.Sprintf("Extended %d min for overtime.", added)
	}
	return Extend{ID: id, Until: horizon, Note: note}, true
}

// SportsTail is an hour after the listing when a sports show is not a matched game.
func SportsTail(airing store.Airing) time.Duration {
	if airing.GameID != "" || !sportsCategory(airing.Category) {
		return 0
	}
	return sportsPad
}

func sportsCategory(category string) bool {
	cat := strings.ToLower(category)
	for _, word := range []string{"sport", "football", "basketball", "baseball", "hockey", "soccer"} {
		if strings.Contains(cat, word) {
			return true
		}
	}
	return false
}
