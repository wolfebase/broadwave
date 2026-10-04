package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// tuned is a 1.0 tune opened without h.mu. wait is set instead when another
// watch is already tuning the same frequency.
type tuned struct {
	wait chan struct{}

	err      error
	body     io.ReadCloser
	auto     *autoGuess
	ch       store.SourceChannel
	host     string
	base     string
	root     string
	tuner    int
	freq     int
	marked   int
	programs []hdhr.Program
	known    bool
	began    time.Time
	status   time.Time
	locked   time.Time
}

// openTuned tunes a 1.0 channel on its own device without h.mu. A channel
// with no signal holds its open for 15 s or more, and every playlist and
// watch waits on h.mu. It opens only when that device has the free tuner
// ensureFeedLocked would pick, and otherwise leaves the tune (failover,
// freeing a warm tuner) to it.
func (h *Hub) openTuned(ctx context.Context, ch store.SourceChannel) *tuned {
	need := NeedFor(ch.VideoCodec, ch.AudioCodec, ch.ATSC3)
	if need.ATSC3 || ch.TunerCount == 0 || ch.BaseURL == "" || hlsStream(ch) {
		return nil
	}
	h.mu.Lock()
	if h.channels[ch.ID] != nil || h.muxes == nil || time.Now().Before(h.statusDown[ch.BaseURL]) {
		h.mu.Unlock()
		return nil
	}
	if t, done := h.siblingLocked(ch); done {
		h.mu.Unlock()
		return t
	}
	h.mu.Unlock()

	// With no other device to fail over to, a 1.0 channel takes a 3.0
	// tuner when no other is free, as ensureFeedLocked does.
	alone := true
	if h.Store != nil {
		if more, err := h.Store.OtherDevices(ctx, ch.GuideNumber, ch.DeviceID); err != nil {
			alone = false
		} else {
			for _, base := range more {
				alone = alone && (base == "" || base == ch.BaseURL)
			}
		}
	}
	began := time.Now()
	raw, err := fetchTunerStatus(ctx, ch.BaseURL)
	if err != nil {
		h.mu.Lock()
		if ctx.Err() == nil && h.statusDown != nil {
			h.statusDown[ch.BaseURL] = time.Now().Add(statusDownFor)
		}
		h.mu.Unlock()
		return nil
	}
	status := time.Now()

	h.mu.Lock()
	if h.channels[ch.ID] != nil {
		h.mu.Unlock()
		return nil
	}
	if t, done := h.siblingLocked(ch); done {
		h.mu.Unlock()
		return t
	}
	device := DeviceTuners{Host: hostOf(ch.BaseURL), Base: ch.BaseURL, Tuners: h.tunersFromLocked(ctx, ch.BaseURL, raw)}
	held := h.heldLocked(device.Host, device.Tuners)
	used := h.usedTunersLocked(device.Host)
	tuner, ok := firstFree(nonATSC3(device.Tuners), used, held)
	if !ok && alone {
		tuner, ok = firstFree(device.Tuners, used, held)
	}
	if !ok {
		h.mu.Unlock()
		return nil
	}
	if h.pending == nil {
		h.pending = map[string]bool{}
	}
	h.pending[pendingKey(device.Host, tuner)] = true
	if ch.FrequencyHz > 0 {
		if h.tuning == nil {
			h.tuning = map[int]chan struct{}{}
		}
		h.tuning[ch.FrequencyHz] = make(chan struct{})
	}
	h.mu.Unlock()

	t := &tuned{ch: ch, host: device.Host, base: ch.BaseURL, tuner: tuner, freq: ch.FrequencyHz, marked: ch.FrequencyHz, began: began, status: status}
	defer func() {
		// The watch never reaches attachTunedLocked after a panic here, and
		// the tuner and the frequency would stay marked.
		if r := recover(); r != nil {
			h.mu.Lock()
			h.unmarkLocked(t)
			h.dropTunedLocked(t)
			h.mu.Unlock()
			panic(r)
		}
	}()
	h.openTunedStream(ctx, t, device.Tuners)
	return t
}

// opened is what openUnlocked did for feedLocked to attach.
type opened struct {
	opener bool
	auto   *autoGuess
	tune   *tuned
}

// openUnlocked does the slow device steps of a tune without h.mu, or waits
// for another caller's open of the same channel. It opens nothing when the
// tune needs the locked path (failover, freeing a warm tuner).
func (h *Hub) openUnlocked(ctx context.Context, ch store.SourceChannel) (opened, error) {
	var o opened
	h.mu.Lock()
	_, isTuned := h.channels[ch.ID]
	wait := h.opening[ch.ID]
	if !isTuned && wait == nil {
		h.beginOpenLocked(ch.ID)
		o.opener = true
	}
	h.mu.Unlock()
	if wait != nil {
		select {
		case <-wait:
			return o, nil
		case <-ctx.Done():
			return o, ctx.Err()
		}
	}
	if !o.opener {
		return o, nil
	}
	if o.auto = h.openAuto(ctx, ch, false); o.auto != nil {
		return o, nil
	}
	o.tune = h.openTuned(ctx, ch)
	// A sibling's open that failed leaves this one to tune for itself, still
	// without h.mu. Two waits at most, then the locked path.
	for i := 0; o.tune != nil && o.tune.wait != nil; i++ {
		select {
		case <-o.tune.wait:
		case <-ctx.Done():
			h.mu.Lock()
			h.endOpenLocked(ch.ID)
			h.mu.Unlock()
			return opened{}, ctx.Err()
		}
		o.tune = nil
		if i < 2 {
			o.tune = h.openTuned(ctx, ch)
		}
	}
	return o, nil
}

// feedLocked attaches what openUnlocked opened, or tunes under h.mu when it
// opened nothing. res is a playlist stream the caller opened instead.
func (h *Hub) feedLocked(ctx context.Context, ch store.SourceChannel, o opened, res *http.Response) (*feed, error) {
	if o.opener {
		h.endOpenLocked(ch.ID)
	}
	f := h.channels[ch.ID]
	if o.auto != nil {
		if f != nil {
			o.auto.body.Close()
		} else {
			f = h.attachAutoLocked(ch, o.auto)
		}
	}
	if o.tune != nil {
		var err error
		if f, err = h.attachTunedLocked(o.tune); err != nil {
			return nil, err
		}
	}
	if f == nil {
		return h.ensureFeedLocked(ctx, ch, res)
	}
	if res != nil {
		res.Body.Close()
	}
	return f, nil
}

// tuneFeed is the feed for a channel, tuned without holding h.mu through
// the device's answer. It returns with h.mu held, also on an error.
func (h *Hub) tuneFeed(ctx context.Context, ch store.SourceChannel, res *http.Response) (*feed, error) {
	var o opened
	if res == nil {
		var err error
		if o, err = h.openUnlocked(ctx, ch); err != nil {
			h.mu.Lock()
			return nil, err
		}
	}
	h.mu.Lock()
	return h.feedLocked(ctx, ch, o, res)
}

// siblingLocked is a watch whose station is already tuned or being tuned:
// a mux to join (nil, done), or another watch's open to wait for.
func (h *Hub) siblingLocked(ch store.SourceChannel) (*tuned, bool) {
	if ch.FrequencyHz <= 0 {
		return nil, false
	}
	if h.muxes[ch.FrequencyHz] != nil {
		return nil, true
	}
	if wait := h.tuning[ch.FrequencyHz]; wait != nil {
		return &tuned{wait: wait}, true
	}
	return nil, false
}

// openTunedStream is the slow part of openTuned: the probe and the device's answer.
func (h *Hub) openTunedStream(ctx context.Context, t *tuned, tuners []Tuner) {
	ch := t.ch
	t.root = h.streamRootFor(ctx, ch.BaseURL, ch.GuideNumber)
	if ch.FrequencyHz > 0 && ch.ProgramNum > 0 {
		if body, err := openMux(t.root, t.tuner, ch.FrequencyHz); err == nil {
			t.body, t.known, t.locked = body, true, time.Now()
			return
		}
	}
	freq, programs, err := probe(t.host, t.tuner, ch.GuideNumber)
	if err != nil {
		url := h.autoURL(ch, t.root)
		res, openErr := openStream(url, "", "")
		if openErr != nil {
			t.err = autoError(openErr, tuners, errors.Is(err, errNoLock))
			return
		}
		t.auto = &autoGuess{body: res.Body, host: t.host, url: url, began: t.began, status: t.status, answered: time.Now()}
		return
	}
	if h.Store != nil {
		for _, p := range programs {
			_ = h.Store.RememberProgram(ctx, ch.DeviceID, p.GuideNumber, freq, p.Number)
		}
	}
	t.ch.ProgramNum = programFor(programs, ch.GuideNumber)
	t.ch.FrequencyHz = freq
	t.freq, t.programs = freq, programs
	body, err := openMux(t.root, t.tuner, freq)
	if err != nil {
		releaseTuner(t.host, t.tuner)
		t.err = err
		return
	}
	t.body, t.locked = body, time.Now()
}

// attachTunedLocked starts the feed on what openTuned opened. A feed another
// watch started meanwhile wins, and this open is closed.
func (h *Hub) attachTunedLocked(t *tuned) (*feed, error) {
	h.unmarkLocked(t)
	if f := h.channels[t.ch.ID]; f != nil {
		h.dropTunedLocked(t)
		return f, nil
	}
	if t.err != nil || t.auto != nil {
		// The device holds an /auto tune by its channel, not by this tuner.
		delete(h.pending, pendingKey(t.host, t.tuner))
		if t.err != nil {
			return nil, t.err
		}
		return h.attachAutoLocked(t.ch, t.auto), nil
	}
	if m := h.muxes[t.freq]; m != nil {
		h.dropTunedLocked(t)
		return h.addFeedLocked(m, t.ch), nil
	}
	delete(h.pending, pendingKey(t.host, t.tuner))
	f := h.beginMuxLocked(t.ch, t.host, t.base, t.tuner, t.freq, t.programs, t.body)
	m := muxOf(h, f)
	m.reopen = muxURL(t.root, t.tuner, t.freq)
	noteTune(m, t.began, t.status, t.locked)
	if t.known {
		go h.rememberSiblings(t.host, t.tuner, t.freq)
	}
	return f, nil
}

// unmarkLocked lets the siblings waiting on this open in.
func (h *Hub) unmarkLocked(t *tuned) {
	if c := h.tuning[t.marked]; c != nil {
		close(c)
		delete(h.tuning, t.marked)
	}
}

// dropTunedLocked lets go of an open nobody uses. A tuner the hub opened
// stays pending until the device has set it free, so no tune picks it first.
func (h *Hub) dropTunedLocked(t *tuned) {
	if t.auto != nil {
		t.auto.body.Close()
	}
	key := pendingKey(t.host, t.tuner)
	if t.body == nil {
		delete(h.pending, key)
		return
	}
	t.body.Close()
	go func() {
		releaseTuner(t.host, t.tuner)
		h.mu.Lock()
		delete(h.pending, key)
		h.mu.Unlock()
	}()
}

// heldLocked is the tuners on one device a new tune must leave alone: one
// another tune is opening there, and the ones held back for other apps.
func (h *Hub) heldLocked(host string, tuners []Tuner) map[int]bool {
	held := maps.Clone(h.reserved)
	if held == nil {
		held = map[int]bool{}
	}
	for key := range h.pending {
		if k, n, ok := strings.Cut(key, "#"); ok && k == hostOf(host) {
			var i int
			if _, err := fmt.Sscan(n, &i); err == nil {
				held[i] = true
			}
		}
	}
	if h.hold > 0 {
		maps.Copy(held, HoldBack(tuners, h.hold))
	}
	return held
}

func pendingKey(host string, tuner int) string {
	return fmt.Sprintf("%s#%d", hostOf(host), tuner)
}

func nonATSC3(tuners []Tuner) []Tuner {
	out := make([]Tuner, 0, len(tuners))
	for _, t := range tuners {
		if !t.ATSC3 {
			out = append(out, t)
		}
	}
	return out
}
