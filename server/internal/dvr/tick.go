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
	planned := PlanLibrary(passes, rows, countTuners(ctx, st), now.Add(-time.Minute), now.Add(lead), active, seen, skips)
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
		if !start && !attempted(active, item.Airing) {
			start, minutes = JoinDecision(pass, item.Airing, now, hub.BufferedSince(ctx, item.Airing.ChannelID))
		}
		if !start {
			hold++
			continue
		}
		// A recording that starts late begins at the airing, padding
		// included, when the tuner's buffer still holds it.
		if _, err := hub.RecordMeta(ctx, minutes, store.Recording{
			ChannelID: item.Airing.ChannelID, Title: item.Airing.Title, Subtitle: item.Airing.Subtitle,
			Description: item.Airing.Description, Category: item.Airing.Category, ProgramID: item.Airing.ProgramID, GameID: item.Airing.GameID,
			StartedAt: item.Airing.Start.Add(-time.Duration(pass.PadBefore) * time.Minute), PassID: pass.ID,
		}); err != nil {
			slog.Error(fmt.Sprintf("pass record: %v", err))
		}
	}
	hub.SetHold(hold)
}

func countTuners(ctx context.Context, st *store.Store) int {
	devices, err := st.Devices(ctx)
	if err != nil || len(devices) == 0 {
		return 2
	}
	n := 0
	for _, device := range devices {
		n += device.TunerCount
	}
	if n < 1 {
		return 2
	}
	return n
}

func findPass(passes []store.Pass, airing store.Airing) (store.Pass, bool) {
	return matchPass(passes, airing)
}

// attempted reports a recording of this airing in any state, so a show the
// viewer stopped, or one that failed, is not joined again from the buffer.
func attempted(recs []store.Recording, airing store.Airing) bool {
	for _, rec := range recs {
		if rec.ChannelID == airing.ChannelID && strings.EqualFold(rec.Title, airing.Title) &&
			rec.StartedAt.Before(airing.End) && !rec.StartedAt.Before(airing.Start.Add(-time.Hour)) {
			return true
		}
	}
	return false
}

func already(recs []store.Recording, airing store.Airing) bool {
	for _, rec := range recs {
		if rec.Status == "recording" && rec.ChannelID == airing.ChannelID && strings.EqualFold(rec.Title, airing.Title) {
			return true
		}
	}
	return false
}
