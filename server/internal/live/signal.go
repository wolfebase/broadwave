package live

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
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

// ErrNoPicture is a picture that did not start: the encode wrote nothing
// before the watch's wait ran out, or failed every restart. The words are
// the player's; the log says why.
var ErrNoPicture = errors.New("This channel didn't start. Try again.")

var errNoLock = errors.New("no lock")

// errDark is a probe whose tuner read the frequency and never locked.
var errDark = errors.New("no signal")

// darkProbe is how long a probe waits for a tuner on a frequency to lock.
// quickProbe is the wait for a channel that found no signal within
// noSignalFor: a station that comes back locks well inside it.
var (
	darkProbe  = 7 * time.Second
	quickProbe = 2 * time.Second
)

// noSignalFor is how long a tune that found no signal is remembered: the
// guide marks the channel, and a watch of it fails after quickProbe instead
// of darkProbe (or the 10 s a FLEX takes to refuse an open of the frequency).
const noSignalFor = 10 * time.Minute

// NoSignal is true when this channel's tuner is silent and reports no lock. A
// tuner that does not answer, and a feed with no tuner, are not. Silent means
// nothing in the last quietFor: a feed warmed or watched before the station
// went off the air has sent bytes, and still has no signal now.
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
	if m == nil || host == "" || tuner < 0 || heardWithin(m, quietFor) {
		return false
	}
	status, err := (hdhr.Control{Addr: controlAddr(host)}).Get("/tuner" + strconv.Itoa(tuner) + "/status")
	return err == nil && !hdhr.ParseStatus(status).Locked
}

// quietFor is how long a tuner has to send nothing before its "no lock" counts.
const quietFor = 2 * time.Second

func heardWithin(m *mux, d time.Duration) bool {
	if !m.got.Load() {
		return false
	}
	return time.Since(time.Unix(0, m.heard.Load())) < d
}

// DropDark gives back the tuner under a channel that never locked, without
// waiting for its renditions to idle out or its probe to give up. A channel
// someone still watches, records, or exports is left alone.
func (h *Hub) DropDark(channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil || f.recording != nil || f.heldOutside() {
		return
	}
	for _, r := range f.renditions {
		if r.viewers > 0 {
			return
		}
	}
	h.markDarkLocked(f.channel.ID, f.channel.FrequencyHz)
	if m := muxOf(h, f); m != nil {
		h.markDarkLocked(f.channel.ID, m.freq)
	}
	h.stopFeedLocked(f)
}

// darkFor is how long a channel whose tune found no signal still reads as
// lost after its tuner is given back, so a player asking why it stopped is
// told about the antenna. A player waits this out before it tunes again.
const darkFor = 20 * time.Second

type darkKey struct {
	channel int64
	freq    int
}

// noteDarkLocked remembers a tune that found no signal and forgets an older
// one when this tune worked.
func (h *Hub) noteDarkLocked(ch store.SourceChannel, err error) {
	switch {
	case errors.Is(err, ErrNoSignal):
		h.markDarkLocked(ch.ID, ch.FrequencyHz)
	case err == nil:
		delete(h.dark, darkKey{channel: ch.ID})
		if ch.FrequencyHz > 0 {
			delete(h.dark, darkKey{freq: ch.FrequencyHz})
		}
	}
}

func (h *Hub) markDarkLocked(channelID int64, freq int) {
	if h.dark == nil {
		h.dark = map[darkKey]time.Time{}
	}
	now := time.Now()
	h.dark[darkKey{channel: channelID}] = now
	if freq > 0 {
		h.dark[darkKey{freq: freq}] = now
	}
}

// RecentlyDark is true when a tune of this channel, or of another channel on
// its frequency, found no signal in the last darkFor.
func (h *Hub) RecentlyDark(channelID int64, freq int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.darkWithinLocked(channelID, freq, darkFor)
}

func (h *Hub) darkWithinLocked(channelID int64, freq int, d time.Duration) bool {
	now := time.Now()
	for key, at := range h.dark {
		if now.Sub(at) >= noSignalFor {
			delete(h.dark, key)
		}
	}
	if at, ok := h.dark[darkKey{channel: channelID}]; ok && now.Sub(at) < d {
		return true
	}
	at, ok := h.dark[darkKey{freq: freq}]
	return freq > 0 && ok && now.Sub(at) < d
}

// NoSignalChannels lists the channels whose last tune, in the last noSignalFor,
// found no signal.
func (h *Hub) NoSignalChannels() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	out := []int64{}
	for key, at := range h.dark {
		if now.Sub(at) >= noSignalFor {
			delete(h.dark, key)
			continue
		}
		if key.channel != 0 {
			out = append(out, key.channel)
		}
	}
	slices.Sort(out)
	return out
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
