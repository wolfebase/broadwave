package live

import (
	"context"
	"time"
)

// Dwell tunes a channel's full mux long enough to read the broadcast guide,
// then releases it when nobody else is using it. It does not start a transcode.
func (h *Hub) Dwell(ctx context.Context, channelID int64, dwell time.Duration) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return 0, err
	}
	h.mu.Lock()
	f, err := h.ensureFeedLocked(ctx, ch, nil)
	if err != nil {
		h.mu.Unlock()
		return 0, err
	}
	f.probing = true
	freq := 0
	if m := muxOf(h, f); m != nil {
		freq = m.freq
	}
	h.mu.Unlock()

	timer := time.NewTimer(dwell)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}

	h.mu.Lock()
	f.probing = false
	h.dropIfUnusedLocked(f)
	h.mu.Unlock()
	if freq < 0 {
		return 0, nil
	}
	return freq, nil
}

// Idle reports whether any viewer, recording, or export is using a tuner.
func (h *Hub) Idle() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range h.feedsLocked() {
		if f.recording != nil || f.exports > 0 || f.probing {
			return false
		}
		for _, r := range f.renditions {
			if r.viewers > 0 {
				return false
			}
		}
	}
	return true
}
