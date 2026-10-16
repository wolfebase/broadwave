package dvr

import (
	"context"
	"time"

	"broadwave/internal/store"
)

// TunerQuietAfter is three missed rounds of the five-minute tuner refresh.
// A tuner quiet that long is offline: it records nothing.
const TunerQuietAfter = 15 * time.Minute

// DeviceOffline reports a tuner that has not answered the refresh lately. A
// device never seen yet, or with no tuners (a playlist), is not offline.
func DeviceOffline(d store.Device, now time.Time) bool {
	if d.TunerCount <= 0 {
		return false
	}
	seen, err := time.Parse(time.RFC3339, d.LastSeen)
	if err != nil {
		return false
	}
	return now.Sub(seen) > TunerQuietAfter
}

// LiveTuners is how many tuners the planner can count on: those that
// answered lately and that the owner has not retired. A two-tuner box that
// went quiet two weeks ago still counted, so the plan promised recordings
// nothing could make. When every tuner is quiet the plan keeps the ones not
// retired, so a short outage does not turn every recording into a conflict.
func LiveTuners(devices []store.Device, now time.Time) int {
	live, kept := 0, 0
	for _, d := range devices {
		if d.TunerCount <= 0 || d.Retired {
			continue
		}
		kept += d.TunerCount
		if !DeviceOffline(d, now) {
			live += d.TunerCount
		}
	}
	switch {
	case live > 0:
		return live
	case kept > 0:
		return kept
	default:
		return 2
	}
}

// TunerCount reads the devices and counts the live tuners.
func TunerCount(ctx context.Context, st *store.Store, now time.Time) int {
	devices, err := st.Devices(ctx)
	if err != nil {
		return 2
	}
	return LiveTuners(devices, now)
}
