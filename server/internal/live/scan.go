package live

import (
	"context"
	"time"
)

// RunScan tunes each frequency in order. A watch or recording call cancels it
// before that call takes a tuner. tune is invoked only while the scan is still
// the active one.
func (h *Hub) RunScan(ctx context.Context, freqs []int, dwell time.Duration, tune func(ctx context.Context, freq int) error) {
	ctx, cancel := context.WithCancel(ctx)
	token := &struct{}{}
	h.mu.Lock()
	if h.scanCancel != nil {
		h.scanCancel()
	}
	h.scanCancel = cancel
	h.scanToken = token
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.scanToken == token {
			h.scanCancel = nil
			h.scanToken = nil
		}
		h.mu.Unlock()
		cancel()
	}()
	for _, freq := range freqs {
		if ctx.Err() != nil {
			return
		}
		tuneCtx, tuneCancel := context.WithTimeout(ctx, dwell)
		err := tune(tuneCtx, freq)
		tuneCancel()
		if err != nil || ctx.Err() != nil {
			return
		}
	}
}

func (h *Hub) preemptScan() {
	h.mu.Lock()
	cancel := h.scanCancel
	h.scanCancel = nil
	h.scanToken = nil
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
