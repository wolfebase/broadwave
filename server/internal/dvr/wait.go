package dvr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// waitWords is what a viewer reads about a refused start: on the plan line
// while the retry runs, and on the failed recording if it never starts.
func waitWords(err error) (reason, failure string) {
	var busy *live.BusyError
	var low *disk.LowError
	var write *disk.WriteError
	var missing *disk.MissingError
	switch {
	case errors.As(err, &busy):
		return "Waiting for a tuner", "Every tuner was busy."
	case errors.As(err, &low), errors.As(err, &write), errors.As(err, &missing):
		return "Waiting for the recordings disk", err.Error()
	}
	for _, words := range []error{live.ErrNoSignal, live.ErrStreamDown, live.ErrNoSource, live.ErrTunerSilent, live.ErrTunerRefused} {
		if errors.Is(err, words) {
			return "Waiting for the channel", words.Error()
		}
	}
	// Anything else names files or addresses, and goes to the log.
	return "Trying to start", "The recording could not start."
}

type waitKey struct {
	channel int64
	start   int64
}

func keyOfWait(channel int64, start time.Time) waitKey {
	return waitKey{channel, start.Unix()}
}

// MarkWaits marks the plan lines of airings that are waiting to start.
func MarkWaits(items []Planned, waits []store.RecordingWait) {
	open := make(map[waitKey]store.RecordingWait, len(waits))
	for _, w := range waits {
		open[keyOfWait(w.ChannelID, w.Start)] = w
	}
	for i := range items {
		w, ok := open[keyOfWait(items[i].Airing.ChannelID, items[i].Airing.Start)]
		if !ok || items[i].Skipped {
			continue
		}
		items[i].Waiting = true
		items[i].Reason = w.Reason
	}
}

// noteRefused keeps the retry's place. Activity hears on the second refusal: a
// guide scan being stopped for the recording refuses the first try, and the
// next tick starts it, so that one is not news.
func noteRefused(ctx context.Context, st *store.Store, open map[waitKey]store.RecordingWait, pass store.Pass, air store.Airing, err error, now time.Time) {
	key := keyOfWait(air.ChannelID, air.Start)
	prev, known := open[key]
	reason, failure := waitWords(err)
	w := store.RecordingWait{
		ChannelID: air.ChannelID, Start: air.Start, End: air.End,
		Until:  air.End.Add(time.Duration(pass.PadAfter)*time.Minute + SportsTail(air)),
		PassID: pass.ID, Title: air.Title, Subtitle: air.Subtitle, Description: air.Description,
		Category: air.Category, ProgramID: air.ProgramID, GameID: air.GameID,
		Reason: reason, Failure: failure, Since: now, Told: known,
	}
	if perr := st.PutRecordingWait(ctx, w); perr != nil {
		slog.Warn(fmt.Sprintf("recording wait: %v", perr))
		return
	}
	open[key] = w
	if !known {
		slog.Warn(fmt.Sprintf("pass record: %s: %v", air.Title, err))
		return
	}
	slog.Debug(fmt.Sprintf("pass record: %s: %v", air.Title, err))
	if prev.Told {
		return
	}
	message := fmt.Sprintf("%s is waiting for a tuner. Every tuner is busy.", air.Title)
	if reason != "Waiting for a tuner" {
		message = fmt.Sprintf("%s has not started. %s", air.Title, failure)
	}
	_ = st.AddEvent(ctx, "recording", message)
}

// noteStarted closes a wait now that its recording runs. A start that missed
// the beginning says so, and like any late start it is not the whole episode.
func noteStarted(ctx context.Context, st *store.Store, rec store.Recording, air store.Airing) {
	_ = st.DeleteRecordingWait(ctx, air.ChannelID, air.Start)
	late := rec.StartedAt.Sub(air.Start)
	if late <= 0 {
		return
	}
	if late > time.Minute {
		_ = st.MarkPartial(ctx, rec.ID, nil)
	}
	_ = st.AddEvent(ctx, "recording", fmt.Sprintf("%s started %s late.", air.Title, shortSpan(late)))
}

func shortSpan(d time.Duration) string {
	if d < 90*time.Second {
		return fmt.Sprintf("%d s", int(d.Seconds()))
	}
	return fmt.Sprintf("%d min", int(d.Round(time.Minute).Minutes()))
}

// settleWaits closes the waits that are over and returns the ones still open.
// A rule removed or an airing skipped while it waited is the viewer's choice.
// An airing that started some other way needs no more. An airing that ended
// without a recording leaves a failed one saying why.
func settleWaits(ctx context.Context, st *store.Store, waits []store.RecordingWait, passes []store.Pass, planned []Planned, recs []store.Recording, now time.Time) map[waitKey]store.RecordingWait {
	open := make(map[waitKey]store.RecordingWait, len(waits))
	if len(waits) == 0 {
		return open
	}
	wanted := make(map[waitKey]bool, len(planned))
	for _, item := range planned {
		if !item.Skipped {
			wanted[keyOfWait(item.Airing.ChannelID, item.Airing.Start)] = true
		}
	}
	for _, w := range waits {
		key := keyOfWait(w.ChannelID, w.Start)
		pass, ok := passByID(passes, w.PassID)
		switch {
		case !ok, startedSince(recs, w, pass), w.End.After(now) && !wanted[key]:
			_ = st.DeleteRecordingWait(ctx, w.ChannelID, w.Start)
		case !w.Until.After(now):
			failWait(ctx, st, w)
			_ = st.DeleteRecordingWait(ctx, w.ChannelID, w.Start)
		default:
			open[key] = w
		}
	}
	return open
}

func passByID(passes []store.Pass, id int64) (store.Pass, bool) {
	for _, pass := range passes {
		if pass.ID == id {
			return pass, true
		}
	}
	return store.Pass{}, false
}

// startedSince reports a recording of this airing that took: running, or
// finished as anything but a failure.
func startedSince(recs []store.Recording, w store.RecordingWait, pass store.Pass) bool {
	air := store.Airing{ChannelID: w.ChannelID, Title: w.Title, Start: w.Start, End: w.End}
	if pass.Kind == "manual" {
		air.ManualPass = pass.ID
	}
	floor := w.Start.Add(-max(recordLead, time.Duration(pass.PadBefore)*time.Minute))
	for _, rec := range recs {
		if rec.Status == "failed" || rec.Status == "interrupted" || !sameSlot(rec, air) {
			continue
		}
		if rec.StartedAt.Before(w.End) && !rec.StartedAt.Before(floor) {
			return true
		}
	}
	return false
}

func failWait(ctx context.Context, st *store.Store, w store.RecordingWait) {
	guide := ""
	if ch, err := st.SourceChannel(ctx, w.ChannelID); err == nil {
		guide = ch.GuideNumber
	}
	until := w.Until
	id, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: w.ChannelID, GuideNumber: guide, Title: w.Title, Status: "failed",
		Subtitle: w.Subtitle, Description: w.Description, Category: w.Category,
		ProgramID: w.ProgramID, GameID: w.GameID, PassID: w.PassID,
		StartedAt: w.Start, EndsAt: &until,
	})
	if err != nil {
		slog.Warn(fmt.Sprintf("recording wait: %v", err))
		return
	}
	failure := strings.TrimSpace(w.Failure)
	_ = st.FinishRecording(ctx, id, "failed", failure)
	_ = st.AddEvent(ctx, "recording", fmt.Sprintf("%s did not record. %s", w.Title, failure))
}
