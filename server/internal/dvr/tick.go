package dvr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// Tick starts a recording when a series pass matches an airing that is about to start.
func Tick(ctx context.Context, st *store.Store, hub *live.Hub) {
	// A game can run hours past its listing; a day later the once pass is done.
	if err := st.DeleteEndedOncePasses(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		slog.Debug(fmt.Sprintf("once passes: %v", err))
	}
	if err := st.DeleteEndedManualPasses(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		slog.Debug(fmt.Sprintf("manual passes: %v", err))
	}
	passes, err := st.Passes(ctx)
	if err != nil || len(passes) == 0 || hub == nil {
		return
	}
	now := time.Now()
	lead := 2 * time.Minute
	for _, pass := range passes {
		if extra := time.Duration(pass.PadBefore)*time.Minute + time.Minute; extra > lead {
			lead = extra
		}
	}
	// The other copy of a simulcast, or of a channel two tuners carry, is recorded once.
	rows, err := st.RecordingAirings(ctx, now.Add(-time.Minute), now.Add(lead))
	if err != nil {
		return
	}
	active, _ := st.Recordings(ctx)
	seen, _ := st.SeenDeleted(ctx)
	skips, _ := st.Skips(ctx)
	planned := PlanLibrary(passes, rows, TunerCount(ctx, st, now), now.Add(-time.Minute), now.Add(lead), active, seen, skips)
	hold := 0
	for _, item := range planned {
		if item.Skipped || already(active, item.Airing) {
			continue
		}
		pass, ok := findPass(passes, item.Airing)
		if !ok {
			continue
		}
		start, minutes := StartDecision(pass, item.Airing, now)
		retry := false
		if !start && !attempted(active, pass, item.Airing) {
			start, minutes = JoinDecision(pass, item.Airing, now, hub.BufferedSince(ctx, item.Airing.ChannelID))
		}
		if !start && RetryDue(active, pass, item.Airing, now) {
			start, minutes, retry = true, recordMinutes(pass, item.Airing, now), true
		}
		if !start {
			hold++
			continue
		}
		// A recording that starts late begins at the airing, padding
		// included, when the tuner's buffer still holds it.
		rec, err := hub.RecordMeta(ctx, minutes, store.Recording{
			ChannelID: item.Airing.ChannelID, Title: item.Airing.Title, Subtitle: item.Airing.Subtitle,
			Description: item.Airing.Description, Category: item.Airing.Category, ProgramID: item.Airing.ProgramID, GameID: item.Airing.GameID,
			StartedAt: item.Airing.Start.Add(-time.Duration(pass.PadBefore) * time.Minute), PassID: pass.ID,
		})
		if err != nil {
			slog.Error(fmt.Sprintf("pass record: %v", err))
			continue
		}
		if retry {
			slog.Info(fmt.Sprintf("recording: trying %s on channel %d again for the %d min left", item.Airing.Title, item.Airing.ChannelID, minutes))
			// The buffer can still hold the start; then the retry is whole.
			if rec.StartedAt.After(item.Airing.Start.Add(time.Minute)) {
				_ = st.MarkPartial(ctx, rec.ID, nil)
			}
		}
	}
	hub.SetHold(hold)
}

func findPass(passes []store.Pass, airing store.Airing) (store.Pass, bool) {
	return matchPass(passes, airing)
}

// attempted reports a recording of this airing in any state, so a show the
// viewer stopped, or one that failed, is not joined again from the buffer.
// The window is the same lead StartDecision uses. An earlier episode of the
// same title started before that lead, and must not count as this one.
func attempted(recs []store.Recording, pass store.Pass, airing store.Airing) bool {
	lead := recordLead
	if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
		lead = extra
	}
	floor := airing.Start.Add(-lead)
	for _, rec := range recs {
		if sameSlot(rec, airing) && rec.StartedAt.Before(airing.End) && !rec.StartedAt.Before(floor) {
			return true
		}
	}
	return false
}

// sameSlot reports a recording made for this airing's channel and show: by
// title for a listing, by pass for a manual slot, whose title the guide can
// change and whose listing another pass may be recording.
func sameSlot(rec store.Recording, airing store.Airing) bool {
	if rec.ChannelID != airing.ChannelID {
		return false
	}
	if airing.ManualPass != 0 {
		return rec.PassID == airing.ManualPass
	}
	return strings.EqualFold(rec.Title, airing.Title)
}

// maxRetries is how many times one airing is tried again after a failure.
const maxRetries = 3

// RetryDue reports an airing whose recording failed partway (a tuner that
// dropped, a disk that filled, a restart nothing resumed) and that is still
// on: it records again for the time left. A recording the viewer stopped is
// complete and is not tried again. Each retry waits a minute longer than the
// last, and after maxRetries failures the airing is left alone.
func RetryDue(recs []store.Recording, pass store.Pass, airing store.Airing, now time.Time) bool {
	lead := recordLead
	if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
		lead = extra
	}
	floor := airing.Start.Add(-lead)
	tries := 0
	var last time.Time
	for _, rec := range recs {
		if !sameSlot(rec, airing) || !rec.StartedAt.Before(airing.End) || rec.StartedAt.Before(floor) {
			continue
		}
		if rec.Status != "failed" && rec.Status != "interrupted" {
			return false
		}
		tries++
		ended := rec.StartedAt
		if rec.EndedAt != nil {
			ended = *rec.EndedAt
		}
		if ended.After(last) {
			last = ended
		}
	}
	if tries == 0 || tries > maxRetries || now.Sub(last) < time.Duration(tries)*time.Minute {
		return false
	}
	end := airing.End.Add(time.Duration(pass.PadAfter)*time.Minute + SportsTail(airing))
	return end.Sub(now) >= time.Minute
}

func already(recs []store.Recording, airing store.Airing) bool {
	for _, rec := range recs {
		if rec.Status != "recording" || !sameSlot(rec, airing) {
			continue
		}
		// A manual slot runs at most 8 hours, so a slot still recording that
		// started before this one ends is this one.
		if airing.ManualPass == 0 || rec.StartedAt.Before(airing.End) {
			return true
		}
	}
	return false
}
