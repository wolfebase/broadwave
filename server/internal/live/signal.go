package live

import (
	"context"
	"strconv"
	"time"

	"broadwave/internal/hdhr"
)

// Measure tunes a channel long enough to read /tunerN/status, then releases it
// when nobody else is using it.
func (h *Hub) Measure(ctx context.Context, channelID int64) (hdhr.Lock, error) {
	var zero hdhr.Lock
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return zero, err
	}
	h.mu.Lock()
	f, err := h.ensureFeedLocked(ctx, ch, nil)
	if err != nil {
		h.mu.Unlock()
		return zero, err
	}
	f.probing = true
	host, tuner := "", -1
	if m := muxOf(h, f); m != nil {
		host, tuner = m.host, m.tuner
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		f.probing = false
		h.dropIfUnusedLocked(f)
		h.mu.Unlock()
	}()
	if host == "" || tuner < 0 {
		return zero, nil
	}
	deadline := time.Now().Add(8 * time.Second)
	var last hdhr.Lock
	for {
		if ctx.Err() != nil {
			return last, ctx.Err()
		}
		status, err := (hdhr.Control{Addr: controlAddr(host)}).Get("/tuner" + strconv.Itoa(tuner) + "/status")
		if err == nil {
			last = hdhr.ParseStatus(status)
			if last.Locked && (last.Symbol > 0 || time.Now().After(deadline)) {
				return last, nil
			}
		}
		if time.Now().After(deadline) {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// TunedLocks reads /tunerN/status for frequencies this server already has open.
func (h *Hub) TunedLocks() map[int]hdhr.Lock {
	type snap struct {
		freq, tuner int
		host        string
	}
	h.mu.Lock()
	var open []snap
	for _, m := range h.muxes {
		if m.freq > 0 && m.tuner >= 0 && m.host != "" {
			open = append(open, snap{m.freq, m.tuner, m.host})
		}
	}
	h.mu.Unlock()
	out := map[int]hdhr.Lock{}
	for _, m := range open {
		status, err := (hdhr.Control{Addr: controlAddr(m.host)}).Get("/tuner" + strconv.Itoa(m.tuner) + "/status")
		if err != nil {
			continue
		}
		out[m.freq] = hdhr.ParseStatus(status)
	}
	return out
}
