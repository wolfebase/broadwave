package dvr

import (
	"context"
	"time"

	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

// RecordingNFO fills season, episode, and the original air date from the
// listing that was on when the recording started.
func RecordingNFO(ctx context.Context, st *store.Store, rec store.Recording) store.Recording {
	if st == nil || rec.StartedAt.IsZero() {
		return rec
	}
	from := rec.StartedAt.Add(-12 * time.Hour)
	to := rec.StartedAt.Add(12 * time.Hour)
	q := store.AiringQuery{From: from, To: to}
	if rec.ChannelID != 0 {
		q.Channels = []int64{rec.ChannelID}
	}
	airings, err := st.QueryAirings(ctx, q)
	if err != nil {
		return rec
	}
	return nfo.WithGuide(rec, airings)
}
