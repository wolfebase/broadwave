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
	if err != nil || hub == nil {
		return
	}
	now := time.Now()
	// Recordings still start without this. A wait not read is a wait not tried.
	waits, err := st.RecordingWaits(ctx)
	if err != nil {
		slog.Error(fmt.Sprintf("recording waits: %v", err))
	}
	if len(passes) == 0 {
		settleWaits(ctx, st, hub, waits, nil, nil, nil, nil, now)
		return
	}
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
	planned := PlanOnAir(passes, rows, TunerCount(ctx, st, now), now.Add(-time.Minute), now.Add(lead), active, seen, skips, OnAir{
		Now: now, Waits: waits,
		Held: func(id int64) time.Time { return hub.BufferedSince(ctx, id) },
	})
	open := settleWaits(ctx, st, hub, waits, passes, planned, active, skips, now)
	tunes := map[int64]string{}
	for _, row := range rows {
		tunes[row.ChannelID] = row.Tune
	}
	busy := map[string]bool{}
	for _, rec := range active {
		if rec.Status == "recording" {
			busy[tuneOf(store.Airing{ChannelID: rec.ChannelID, Tune: tunes[rec.ChannelID]})] = true
		}
	}
	var onTime, late []startPlan
	soon := map[string]bool{}
	for _, item := range planned {
		if item.Skipped {
			continue
		}
		pass, ok := findPass(passes, item.Airing)
		if !ok || already(active, pass, item.Airing) {
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
		// Refused on time, it keeps trying while the airing is on. The padding
		// after it has none of the show to save.
		_, waiting := open[keyOfWait(item.Airing.ChannelID, item.Airing.Start)]
		if !start && waiting && item.Airing.End.After(now) {
			start, minutes = true, recordMinutes(pass, item.Airing, now)
		}
		if !start {
			soon[tuneOf(item.Airing)] = true
			continue
		}
		plan := startPlan{item: item, pass: pass, minutes: minutes, retry: retry, waiting: waiting}
		if now.After(item.Airing.Start.Add(recordLead)) {
			late = append(late, plan)
			continue
		}
		onTime = append(onTime, plan)
		busy[tuneOf(item.Airing)] = true
	}
	// The tuners held back last tick are for these. A recording is refused by
	// its own hold otherwise, and starts a tick late.
	if len(onTime) > 0 {
		hub.SetHold(0)
	}
	for _, plan := range onTime {
		runStart(ctx, st, hub, open, plan, now, hub.RecordMeta)
	}
	hub.SetHold(tunersToHold(soon, busy))
	// A late retry comes after the hold, so it can't take a tuner from a
	// recording about to start.
	for _, plan := range late {
		runStart(ctx, st, hub, open, plan, now, hub.RecordIfFree)
	}
	tryPastListing(ctx, st, hub, open, passes, now)
}

// startPlan is one airing Tick has decided to start.
type startPlan struct {
	item    Planned
	pass    store.Pass
	minutes int
	retry   bool
	waiting bool
}

// tunersToHold is the tuners the recordings about to start will need: a
// frequency once, and none for one a recording already has.
func tunersToHold(soon, busy map[string]bool) int {
	n := 0
	for tune := range soon {
		if !busy[tune] {
			n++
		}
	}
	return n
}

func runStart(ctx context.Context, st *store.Store, hub *live.Hub, open map[waitKey]store.RecordingWait, plan startPlan, now time.Time, record func(context.Context, int, store.Recording) (store.Recording, error)) {
	air := plan.item.Airing
	// A recording that starts late begins at the airing, padding included,
	// when the tuner's buffer still holds it.
	rec, err := record(ctx, plan.minutes, store.Recording{
		ChannelID: air.ChannelID, Title: air.Title, Subtitle: air.Subtitle,
		Description: air.Description, Category: air.Category, ProgramID: air.ProgramID, GameID: air.GameID,
		StartedAt: air.Start.Add(-time.Duration(plan.pass.PadBefore) * time.Minute), PassID: plan.pass.ID,
	})
	if err != nil {
		noteRefused(ctx, st, open, plan.pass, air, err, now)
		return
	}
	if plan.waiting {
		noteStarted(ctx, st, rec, air)
	}
	if plan.retry {
		slog.Info(fmt.Sprintf("recording: trying %s on channel %d again for the %d min left", air.Title, air.ChannelID, plan.minutes))
		// The buffer can still hold the start; then the retry is whole.
		if rec.StartedAt.After(air.Start.Add(time.Minute)) {
			_ = st.MarkPartial(ctx, rec.ID, nil)
		}
	}
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
	tries, last := failedTries(recs, pass, airing)
	if tries == 0 || tries > maxRetries || now.Sub(last) < time.Duration(tries)*time.Minute {
		return false
	}
	end := airing.End.Add(time.Duration(pass.PadAfter)*time.Minute + SportsTail(airing))
	return end.Sub(now) >= time.Minute
}

// retriesLeft reports an airing RetryDue will start, now or after its wait.
func retriesLeft(recs []store.Recording, pass store.Pass, airing store.Airing, now time.Time) bool {
	tries, _ := failedTries(recs, pass, airing)
	if tries == 0 || tries > maxRetries {
		return false
	}
	end := airing.End.Add(time.Duration(pass.PadAfter)*time.Minute + SportsTail(airing))
	return end.Sub(now) >= time.Minute
}

// failedTries counts this airing's failed attempts and when the last one
// ended. Zero tries means there is none to repeat: any other attempt took.
func failedTries(recs []store.Recording, pass store.Pass, airing store.Airing) (int, time.Time) {
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
			return 0, time.Time{}
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
	return tries, last
}

// already reports this airing's recording in progress. One of the airing
// before (the last episode in its padding, a game that ran long) is not it:
// the two record side by side. The window is the one attempted uses.
func already(recs []store.Recording, pass store.Pass, airing store.Airing) bool {
	lead := recordLead
	if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
		lead = extra
	}
	floor := airing.Start.Add(-lead)
	for _, rec := range recs {
		if rec.Status == "recording" && sameSlot(rec, airing) && rec.StartedAt.Before(airing.End) && !rec.StartedAt.Before(floor) {
			return true
		}
	}
	return false
}
