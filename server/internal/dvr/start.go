package dvr

import (
	"time"

	"broadwave/internal/store"
)

// recordLead is how early a recording claims a tuner when the pass has no padding.
// The live-watch warning uses the same lead, so a viewer is asked before that claim.
const recordLead = 90 * time.Second

// StartDecision reports whether a pass should start recording this airing now,
// and for how many minutes. The tuner starts up to recordLead early, or earlier
// when the pass asks for padding. It does not join a show that is already more
// than recordLead underway.
func StartDecision(pass store.Pass, airing store.Airing, now time.Time) (bool, int) {
	lead := recordLead
	if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
		lead = extra
	}
	if now.Before(airing.Start.Add(-lead)) || !airing.End.After(now) {
		return false, 0
	}
	if now.After(airing.Start.Add(recordLead)) {
		return false, 0
	}
	return true, recordMinutes(pass, airing, now)
}

// JoinDecision starts an airing that is already more than recordLead
// underway when the tuner has been on its frequency since the airing began,
// so the recording can start from the beginning. held is how far back that
// frequency's buffer reaches; zero means it is not tuned.
func JoinDecision(pass store.Pass, airing store.Airing, now, held time.Time) (bool, int) {
	if held.IsZero() || held.After(airing.Start) || !airing.End.After(now) || !now.After(airing.Start.Add(recordLead)) {
		return false, 0
	}
	return true, recordMinutes(pass, airing, now)
}

func recordMinutes(pass store.Pass, airing store.Airing, now time.Time) int {
	tail := time.Duration(pass.PadAfter)*time.Minute + SportsTail(airing)
	minutes := int(airing.End.Add(tail).Sub(now).Minutes()) + 1
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 8*60 {
		minutes = 8 * 60
	}
	return minutes
}
