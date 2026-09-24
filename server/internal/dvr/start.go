package dvr

import (
	"time"

	"broadwave/internal/store"
)

// StartDecision reports whether a pass should start recording this airing now,
// and for how many minutes. The tuner starts up to 90 seconds early, or earlier
// when the pass asks for padding. It does not join a show that is already more
// than 90 seconds underway.
func StartDecision(pass store.Pass, airing store.Airing, now time.Time) (bool, int) {
	lead := 90 * time.Second
	if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
		lead = extra
	}
	if now.Before(airing.Start.Add(-lead)) || !airing.End.After(now) {
		return false, 0
	}
	if now.After(airing.Start.Add(90 * time.Second)) {
		return false, 0
	}
	tail := time.Duration(pass.PadAfter)*time.Minute + SportsTail(airing)
	minutes := int(airing.End.Add(tail).Sub(now).Minutes()) + 1
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 8*60 {
		minutes = 8 * 60
	}
	return true, minutes
}
