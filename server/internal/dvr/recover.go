package dvr

import (
	"context"
	"time"

	"waveguide/internal/store"
)

// Recover closes recordings that were in progress when the process died.
// A show that is still on is started again for the time that is left. The
// file from the crashed process stays, marked failed, because it was cut off.
func Recover(ctx context.Context, st *store.Store, now time.Time, resume func(store.Recording, time.Duration) error) error {
	recs, err := st.Recordings(ctx)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.Status != "recording" {
			continue
		}
		var left time.Duration
		if rec.EndsAt != nil && rec.EndsAt.After(now) {
			left = rec.EndsAt.Sub(now)
		}
		if err := st.FinishRecording(ctx, rec.ID, "failed", "Stopped when the server restarted."); err != nil {
			return err
		}
		if left >= time.Minute && resume != nil {
			if err := resume(rec, left); err != nil {
				return err
			}
		}
	}
	return nil
}
