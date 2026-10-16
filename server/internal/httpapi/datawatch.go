package httpapi

import (
	"context"
	"time"

	"broadwave/internal/store"
)

// WatchData tells the apps when what they cache changed: guide.changed,
// channels.changed, recordings.changed, schedule.changed, and
// settings.changed, each with the new version. The counters are kept by
// triggers, so every writer counts (a guide refresh, a restore, a tuner
// scan), and a burst of writes is one event per check.
func (s *Server) WatchData(ctx context.Context, every time.Duration) {
	if s.Bus == nil || s.Store == nil {
		return
	}
	last, err := s.Store.DataVersions(ctx)
	if err != nil {
		last = store.Versions{}
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		now, err := s.Store.DataVersions(ctx)
		if err != nil {
			continue
		}
		for _, change := range dataChanges(last, now) {
			s.Bus.Publish(change.kind, map[string]int64{"version": change.version})
		}
		last = now
	}
}

type dataChange struct {
	kind    string
	version int64
}

func dataChanges(was, now store.Versions) []dataChange {
	var out []dataChange
	add := func(kind string, a, b int64) {
		if a != b {
			out = append(out, dataChange{kind, b})
		}
	}
	add("guide.changed", was.Guide, now.Guide)
	add("channels.changed", was.Channels, now.Channels)
	add("recordings.changed", was.Recordings, now.Recordings)
	add("schedule.changed", was.Schedule, now.Schedule)
	add("settings.changed", was.Settings, now.Settings)
	return out
}

// DataVersions is the hello's versions: nil when the catalog cannot be read.
func (s *Server) DataVersions() any {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	v, err := s.Store.DataVersions(ctx)
	if err != nil {
		return nil
	}
	return v
}
