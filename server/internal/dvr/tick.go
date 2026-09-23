package dvr

import (
	"context"
	"log"
	"strings"
	"time"

	"waveguide/internal/live"
	"waveguide/internal/store"
)

// Tick starts a recording when a series pass matches an airing that is about to start.
func Tick(ctx context.Context, st *store.Store, hub *live.Hub) {
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
	rows, err := st.Airings(ctx, now.Add(-time.Minute), now.Add(lead))
	if err != nil {
		return
	}
	active, _ := st.Recordings(ctx)
	seen, _ := st.SeenDeleted(ctx)
	skips, _ := st.Skips(ctx)
	planned := Plan(passes, rows, countTuners(ctx, st), now.Add(-time.Minute), now.Add(lead))
	planned = ApplyLibrary(planned, passes, active, seen, skips)
	for _, item := range planned {
		if item.Skipped || already(active, item.Airing) {
			continue
		}
		pass, ok := findPass(passes, item.Airing)
		if !ok {
			continue
		}
		start, minutes := StartDecision(pass, item.Airing, now)
		if !start {
			continue
		}
		if _, err := hub.RecordMeta(ctx, minutes, store.Recording{
			ChannelID: item.Airing.ChannelID, Title: item.Airing.Title, Subtitle: item.Airing.Subtitle,
			Description: item.Airing.Description, Category: item.Airing.Category, ProgramID: item.Airing.ProgramID, GameID: item.Airing.GameID,
		}); err != nil {
			log.Printf("pass record: %v", err)
		}
	}
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

func already(recs []store.Recording, airing store.Airing) bool {
	for _, rec := range recs {
		if rec.Status == "recording" && rec.ChannelID == airing.ChannelID && strings.EqualFold(rec.Title, airing.Title) {
			return true
		}
	}
	return false
}
