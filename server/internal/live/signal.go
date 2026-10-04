package live

import (
	"context"
	"errors"
	"strconv"
	"time"

	"broadwave/internal/hdhr"
)

// ErrNoSignal is a tune the tuner could not lock. The words are the player's.
var ErrNoSignal = errors.New("This channel isn't coming in. Check the antenna.")

// ErrStreamDown is a source that answered the stream request with an error or
// not at all, such as a playlist whose upstream stopped. The words are the player's.
var ErrStreamDown = errors.New("This channel's stream isn't answering. Trying again usually fixes it.")

// ErrNoSource is a channel no enabled source carries any more. The words are the player's.
var ErrNoSource = errors.New("No source has this channel now. Check Sources in Settings.")

// ErrTunerSilent is no tuner that answered at all. The clients key their
// tuner recovery on "did not answer". The words are the player's.
var ErrTunerSilent = errors.New("This tuner did not answer. Check that it is on.")

// ErrTunerRefused is a tuner that answered the stream request with an error.
// The status and body follow it for the log. The words are the player's.
var ErrTunerRefused = errors.New("The tuner would not start this channel. Try again.")

var errNoLock = errors.New("no lock")

// NoSignal is true when this channel's tuner has sent nothing and reports no
// lock. A tuner that does not answer, and a feed with no tuner, are not.
func (h *Hub) NoSignal(channelID int64) bool {
	h.mu.Lock()
	var m *mux
	host, tuner := "", -1
	if f := h.channels[channelID]; f != nil {
		if m = muxOf(h, f); m != nil {
			host, tuner = m.host, m.tuner
		}
	}
	h.mu.Unlock()
	if m == nil || host == "" || tuner < 0 || m.got.Load() {
		return false
	}
	status, err := (hdhr.Control{Addr: controlAddr(host)}).Get("/tuner" + strconv.Itoa(tuner) + "/status")
	return err == nil && !hdhr.ParseStatus(status).Locked
}

// DropDark gives back the tuner under a channel that never locked, without
// waiting for its renditions to idle out or its probe to give up. A channel
// someone still watches, records, or exports is left alone.
func (h *Hub) DropDark(channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil || f.recording != nil || f.exports > 0 {
		return
	}
	for _, r := range f.renditions {
		if r.viewers > 0 {
			return
		}
	}
	h.stopFeedLocked(f)
}

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
	f, err := h.tuneFeed(ctx, ch, nil)
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
