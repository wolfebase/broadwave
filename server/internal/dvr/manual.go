package dvr

import (
	"strconv"
	"time"

	"broadwave/internal/store"
)

// MaxManualMinutes is the longest a manual pass records in one go.
const MaxManualMinutes = 8 * 60

// manualAirings lays each manual pass onto the clock between from and to:
// one slot per chosen weekday, or the one start of a one-time pass. A slot
// takes the title and details of the listing it overlaps most on its
// channel, when the guide has one, and keeps its own start and length.
func manualAirings(passes []store.Pass, airings []store.Airing, from, to time.Time) []store.Airing {
	var out []store.Airing
	for _, pass := range passes {
		if pass.Kind != "manual" || pass.ChannelID == 0 || pass.Duration < 1 {
			continue
		}
		for _, start := range ManualStarts(pass, from, to) {
			out = append(out, manualAiring(pass, start, airings))
		}
	}
	return out
}

// ManualStarts are the starts of a manual pass's slots that are on at some
// point between from and to.
func ManualStarts(pass store.Pass, from, to time.Time) []time.Time {
	length := time.Duration(pass.Duration) * time.Minute
	on := func(start time.Time) bool { return start.Add(length).After(from) && start.Before(to) }
	if len(pass.Days) == 0 {
		if pass.AiringStart.IsZero() || !on(pass.AiringStart) {
			return nil
		}
		return []time.Time{pass.AiringStart}
	}
	var out []time.Time
	// Walk local days at noon, so a DST change never lands the walk on the
	// wrong date. A slot that began the day before can still be on.
	first := from.In(time.Local).Add(-length - 24*time.Hour)
	day := time.Date(first.Year(), first.Month(), first.Day(), 12, 0, 0, 0, time.Local)
	for ; !day.After(to.In(time.Local).Add(24 * time.Hour)); day = time.Date(day.Year(), day.Month(), day.Day()+1, 12, 0, 0, 0, time.Local) {
		if !onDay(pass.Days, int(day.Weekday())) {
			continue
		}
		start, ok := store.ManualStart(pass, day)
		if ok && on(start) {
			out = append(out, start)
		}
	}
	return out
}

// manualID is a slot's airing id: negative, so it never meets a listing's,
// and the same for the same pass and start on every plan.
func manualID(passID int64, start time.Time) int64 {
	// An unsaved pass being previewed is id -1.
	if passID < 0 {
		passID = -passID
	}
	return -(passID*100_000_000 + start.Unix()/60)
}

func manualAiring(pass store.Pass, start time.Time, airings []store.Airing) store.Airing {
	end := start.Add(time.Duration(pass.Duration) * time.Minute)
	air := store.Airing{ID: manualID(pass.ID, start), ChannelID: pass.ChannelID, Title: pass.Title, Start: start, End: end, ManualPass: pass.ID}
	listing, ok := overlapping(airings, pass.ChannelID, start, end)
	if !ok {
		return air
	}
	if air.Title == "" {
		air.Title = listing.Title
	}
	air.Subtitle, air.Description, air.Category, air.ProgramID = listing.Subtitle, listing.Description, listing.Category, listing.ProgramID
	air.New, air.ImageURL, air.ImageWidth, air.ImageHeight = listing.New, listing.ImageURL, listing.ImageWidth, listing.ImageHeight
	air.Season, air.Episode, air.EpisodeLabel, air.SeriesID = listing.Season, listing.Episode, listing.EpisodeLabel, listing.SeriesID
	air.Rating, air.GuideSource = listing.Rating, listing.GuideSource
	return air
}

// overlapping is the listing on the channel that shares the most time with
// the slot.
func overlapping(airings []store.Airing, channelID int64, start, end time.Time) (store.Airing, bool) {
	var best store.Airing
	var most time.Duration
	for _, air := range airings {
		if air.ChannelID != channelID || air.ManualPass != 0 || !air.End.After(start) || !air.Start.Before(end) {
			continue
		}
		shared := minTime(air.End, end).Sub(maxTime(air.Start, start))
		if shared > most {
			best, most = air, shared
		}
	}
	return best, most > 0
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// ManualSkipKey is the skip row key for a manual pass's slot. It names the
// pass, so skipping one slot never skips the listing's episode elsewhere.
func ManualSkipKey(passID int64) string {
	return "manual:" + strconv.FormatInt(passID, 10)
}
