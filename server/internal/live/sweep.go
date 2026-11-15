package live

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/store"
)

// sweepPause is the wait between two muxes, so the sweep never looks like a
// viewer flipping through channels to the tuner or the network; sweepIdle is
// the wait before the gates are read again when one is shut.
var sweepPause, sweepIdle = 20 * time.Second, 30 * time.Second

const (
	// sweepDarkFor is how long a mux that found no signal is left alone.
	sweepDarkFor = time.Hour
	// sweepRecordingLead keeps the sweep off the tuners before a recording.
	sweepRecordingLead = 3 * time.Minute
	// SweepStaleAfter is how long a swept still is shown. A pass over a big
	// lineup takes longer than FrameStaleAfter.
	SweepStaleAfter = 45 * time.Minute
	// sweepRefresh is the age at which a still is swept again: inside
	// SweepStaleAfter, so a swept still is replaced before it stops being
	// listed, and long enough that an idle server is not tuning all day.
	sweepRefresh = 30 * time.Minute
	// sweepFreeTuners is the free tuners a device needs before the sweep
	// takes one: it never takes the last, or one another app holds.
	sweepFreeTuners = 2
	// sweepProgramGap is the youngest still a new program replaces early.
	sweepProgramGap = 10 * time.Minute
	// sweepDailyTunes is the most a target is tuned in a day: what
	// sweepRefresh alone allows. A new program moves a tune earlier and
	// never adds one past it.
	sweepDailyTunes = int(24 * time.Hour / sweepRefresh)
	// sweepHourlyTunes is the most a target is tuned in any hour. A day's
	// tunes spent in its first hours left the channels on it with no still
	// for the rest.
	sweepHourlyTunes = 2
	// sweepEarlyGap is the shortest wait after a tune before a new program
	// tunes again. With two tunes an hour, no still gets older than an hour
	// less this, inside SweepStaleAfter.
	sweepEarlyGap = 20 * time.Minute
	// sweepGiveUp is the tunes a channel that never had a still gets before
	// it no longer makes its target due (a sound-only service), until the
	// lineup changes.
	sweepGiveUp = 3
)

// StillSettle is the start of a program a still from is counted as the
// program before: a station often ends the last show or runs a break over
// the boundary.
const StillSettle = 2 * time.Minute

// sweepGrabWait is the longest a swept tune waits for its stills.
var sweepGrabWait = 30 * time.Second

// SettingStillSweep turns the idle still sweep off with "0". Missing means on.
const SettingStillSweep = "stillSweep"

// sweepTarget is one tune of the sweep: the channels a frequency carries, or
// one ATSC 3.0 channel the device tunes on its own.
type sweepTarget struct {
	key      string
	first    store.SourceChannel
	channels []int64
	// rank is how likely a viewer opens it, lowest first (sweepRank).
	rank int
	// lead are its channels a viewer opens (rank under sweepRest), and
	// program the start of the latest of their programs on now. A still
	// from before it shows the program before.
	lead    []int64
	program time.Time
}

// Sweep ranks: a game, live show, or new episode on now; a favorite; a
// station's main channel; the rest.
const (
	sweepOnNow = iota
	sweepFavorite
	sweepMain
	sweepRest
)

// sweepState is the running sweep's tune, for a watch to stop.
type sweepState struct {
	cancel context.CancelFunc
	feed   *feed
}

// RunStillSweep keeps a recent still for every channel by tuning, one at a
// time, the frequencies no tune is on. Each tune goes to the first target
// due in sweepTargets' order, so the channels a viewer opens are refreshed
// first and again as soon as a new program starts on them. It tunes only
// while nothing on that device is watched,
// recorded, or warm, no recording starts within three minutes, the device
// reports at least two free tuners, and the stillSweep setting is not "0".
// A watch, recording, or warm stops it at once (StopSweep), and the tuner
// is let go before anything else tunes. It returns when ctx ends.
func (h *Hub) RunStillSweep(ctx context.Context) {
	if os.Getenv("BROADWAVE_STILL_SWEEP") == "0" || h.Store == nil || h.FFmpeg == "" {
		return
	}
	// A pass ends when no target is due; its length is logged.
	var passStart time.Time
	passTunes := 0
	lineup := ""
	for ctx.Err() == nil {
		targets := h.sweepTargets(ctx)
		// A channel given up on gets another try when the lineup changes.
		if now := sweepLineup(targets); now != lineup {
			h.forgetSweepMisses()
			lineup = now
		}
		swept, blocked := false, false
		// A device that is busy is passed over; another may be free.
		shut := map[string]bool{}
		for _, t := range targets {
			if ctx.Err() != nil {
				return
			}
			if !h.sweepDue(t, time.Now()) {
				continue
			}
			host := hostOf(t.first.BaseURL)
			if shut[host] {
				continue
			}
			if !h.sweepAllowed(ctx, t.first) {
				shut[host], blocked = true, true
				continue
			}
			if passStart.IsZero() {
				passStart = time.Now()
			}
			h.sweepOne(ctx, t)
			passTunes++
			swept = true
			break
		}
		if !swept && !blocked && passTunes > 0 {
			slog.Info(fmt.Sprintf("stills: swept %d tunes in %s", passTunes, time.Since(passStart).Round(time.Second)))
			passStart, passTunes = time.Time{}, 0
		}
		wait := sweepIdle
		if swept {
			wait = sweepPause
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// sweepTargets is every tune the sweep may make, in sweepRank order, then
// guide order. Hidden, disabled, absent, duplicate, and copy-protected channels (an
// encrypted ATSC 3.0 channel's clear twin is swept instead) are left out,
// and so are links and playlists, which have no tuner.
func (h *Hub) sweepTargets(ctx context.Context) []sweepTarget {
	list, err := h.Store.Channels(ctx, false)
	if err != nil {
		return nil
	}
	// A tuner that is retired or not answering is never swept: its status read
	// failed, the pass counted as blocked, and it never ended, so the lineup
	// was tuned round and round all day.
	gone := map[string]bool{}
	if devices, err := h.Store.Devices(ctx); err == nil {
		now := time.Now()
		for _, d := range devices {
			if d.Retired || d.Quiet(now) {
				gone[d.DeviceID] = true
			}
		}
	}
	now := time.Now()
	onNow := map[int64]store.Airing{}
	if airings, err := h.Store.Airings(ctx, now, now.Add(time.Second)); err == nil {
		for _, a := range airings {
			onNow[a.ChannelID] = a
		}
	}
	// A shown 3.0 channel can carry its 1.0 twin's listings.
	airingOf := func(ch store.Channel) (store.Airing, bool) {
		if a, ok := onNow[ch.ID]; ok {
			return a, true
		}
		a, ok := onNow[ch.TwinID]
		return a, ok && ch.TwinID != 0
	}
	var shown []store.Channel
	for _, ch := range list {
		// A SameAs row is the same channel on another tuner; the shown row is swept.
		if ch.Hidden || !ch.Enabled || !ch.Present || ch.Protected || ch.SameAs != 0 || gone[ch.DeviceID] {
			continue
		}
		shown = append(shown, ch)
	}
	ranks := make(map[int64]int, len(shown))
	for _, ch := range shown {
		a, ok := airingOf(ch)
		ranks[ch.ID] = sweepRank(ch, a, ok)
	}
	slices.SortStableFunc(shown, func(a, b store.Channel) int { return ranks[a.ID] - ranks[b.ID] })
	byKey := map[string]int{}
	var out []sweepTarget
	for _, ch := range shown {
		src, err := h.Store.SourceChannel(ctx, ch.ID)
		if err != nil || src.TunerCount == 0 || src.BaseURL == "" || hlsStream(src) {
			continue
		}
		// An emulator's channels are a provider's streams, not idle tuners.
		if h.NoStatus(ctx, src.BaseURL) {
			continue
		}
		key := "id:" + strconv.FormatInt(src.ID, 10)
		if !NeedFor(src.VideoCodec, src.AudioCodec, src.ATSC3).ATSC3 && src.FrequencyHz > 0 {
			key = hostOf(src.BaseURL) + ":" + strconv.Itoa(src.FrequencyHz)
		}
		i, ok := byKey[key]
		if !ok {
			i = len(out)
			byKey[key] = i
			out = append(out, sweepTarget{key: key, first: src, rank: ranks[ch.ID]})
		}
		t := &out[i]
		t.channels = append(t.channels, src.ID)
		if ranks[ch.ID] < sweepRest {
			t.lead = append(t.lead, src.ID)
			if a, ok := airingOf(ch); ok && a.Start.After(t.program) {
				t.program = a.Start
			}
		}
	}
	return out
}

// sweepRank is how likely a viewer opens ch, with a its program on now when
// on is set: a game, live show, sports, or new episode first, then a
// favorite, then a station's main channel (its network is known, or it is
// the .1), then the rest.
func sweepRank(ch store.Channel, a store.Airing, on bool) int {
	switch {
	case on && (a.GameID != "" || a.Live || a.New || a.Premiere || strings.Contains(strings.ToLower(a.Category), "sport")):
		return sweepOnNow
	case ch.Favorite:
		return sweepFavorite
	case ch.Network != "" || strings.HasSuffix(ch.GuideNumber, ".1"):
		return sweepMain
	}
	return sweepRest
}

// sweepDue reports a target that no tune is on now, that did not find dark
// within the hour, that has tunes left this hour and today, and that was not
// tuned in the last sweepProgramGap. It is due for a channel whose still is
// missing or older than sweepRefresh, or, sweepEarlyGap after the last tune,
// for a lead channel whose still is from before its program and
// sweepProgramGap old. A channel the last tune brought back no still for
// waits for the refresh, and one that never had a still stops counting after
// sweepGiveUp tunes.
func (h *Hub) sweepDue(t sweepTarget, now time.Time) bool {
	h.mu.Lock()
	if at, ok := h.sweepDark[t.key]; ok && time.Since(at) < sweepDarkFor {
		h.mu.Unlock()
		return false
	}
	if h.darkWithinLocked(t.first.ID, t.first.FrequencyHz, sweepDarkFor) {
		h.mu.Unlock()
		return false
	}
	for _, id := range t.channels {
		if h.channels[id] != nil {
			h.mu.Unlock()
			return false
		}
	}
	tunes, hour := 0, 0
	var last time.Time
	for _, at := range h.sweepTunes[t.key] {
		if now.Sub(at) < 24*time.Hour {
			tunes++
		}
		if now.Sub(at) < time.Hour {
			hour++
		}
		if at.After(last) {
			last = at
		}
	}
	misses := make(map[int64]int, len(t.channels))
	for _, id := range t.channels {
		if n := h.sweepMisses[id]; n > 0 {
			misses[id] = n
		}
	}
	h.mu.Unlock()
	// A tune that brought back no new still (a slate kept the last one, the
	// grab missed) waits sweepProgramGap like any other, not one pause.
	if tunes >= sweepDailyTunes || hour >= sweepHourlyTunes || now.Sub(last) < sweepProgramGap {
		return false
	}
	for _, id := range t.channels {
		info, err := os.Stat(FramePath(h.Dir, id, 0))
		stale := err != nil || now.Sub(info.ModTime()) > sweepRefresh
		switch n := misses[id]; {
		case !stale:
		case n == 0:
			return true
		case err != nil && n >= sweepGiveUp:
			// Never a picture: a sound-only service, or one the grab cannot read.
		case now.Sub(last) >= sweepRefresh:
			return true
		}
	}
	settled := t.program.Add(StillSettle)
	if t.program.IsZero() || now.Before(settled) || now.Sub(last) < sweepEarlyGap {
		return false
	}
	for _, id := range t.lead {
		if misses[id] > 0 {
			continue
		}
		if at, ok := frameTime(h.Dir, id); ok && at.Before(settled) && now.Sub(at) >= sweepProgramGap {
			return true
		}
	}
	return false
}

// noteSweepResult counts, for each channel of a tune that began then, a tune
// that brought back no still, and forgets the count of one that got one.
func (h *Hub) noteSweepResult(t sweepTarget, began time.Time) {
	got := make(map[int64]bool, len(t.channels))
	for _, id := range t.channels {
		if at, ok := frameTime(h.Dir, id); ok && !at.Before(began) {
			got[id] = true
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sweepMisses == nil {
		h.sweepMisses = map[int64]int{}
	}
	for _, id := range t.channels {
		if got[id] {
			delete(h.sweepMisses, id)
		} else {
			h.sweepMisses[id]++
		}
	}
}

// sweepLineup names every target and its channels.
func sweepLineup(targets []sweepTarget) string {
	var b strings.Builder
	for _, t := range targets {
		b.WriteString(t.key)
		for _, id := range t.channels {
			b.WriteByte(',')
			b.WriteString(strconv.FormatInt(id, 10))
		}
		b.WriteByte(';')
	}
	return b.String()
}

// forgetSweepMisses starts every channel's count again, for a new lineup.
func (h *Hub) forgetSweepMisses() {
	h.mu.Lock()
	h.sweepMisses = nil
	h.mu.Unlock()
}

// noteSweepTune counts a tune of target key against its day.
func (h *Hub) noteSweepTune(key string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sweepTunes == nil {
		h.sweepTunes = map[string][]time.Time{}
	}
	tunes := slices.DeleteFunc(h.sweepTunes[key], func(t time.Time) bool { return at.Sub(t) >= 24*time.Hour })
	h.sweepTunes[key] = append(tunes, at)
}

// sweepAllowed reads every gate for a tune on ch's device.
func (h *Hub) sweepAllowed(ctx context.Context, ch store.SourceChannel) bool {
	ok, _ := h.tunerGateOpen(ctx, ch, tunerGate.sweepOpen)
	return ok
}

// tunerGate is what a background tune on one device (the still sweep, a
// sound check's twin) is allowed by.
type tunerGate struct {
	// off is the stillSweep setting "0".
	off bool
	// held is a hold, a channel scan, or a guessed tune anywhere.
	held bool
	// opened is a tuner on the device this server opened by number.
	opened bool
	// recording is a recording on the device.
	recording bool
	// used is a viewer, an export, a mosaic, a guide dwell, or a warm
	// picture on the device.
	used bool
	// soon is a recording due within sweepRecordingLead.
	soon bool
	// free is the tuners the device reports free.
	free int
}

// sweepOpen is the still sweep's gate: nothing on the device is in use.
func (g tunerGate) sweepOpen() bool {
	return !g.off && !g.opened && !g.used && g.backgroundOpen()
}

// backgroundOpen is the gate a sound check's twin tune shares with the
// sweep: a viewer may be watching, but nothing is recorded or about to be,
// no scan or hold is on, and the tune never takes the last free tuner.
func (g tunerGate) backgroundOpen() bool {
	return !g.held && !g.recording && !g.soon && g.free >= sweepFreeTuners
}

// why names the first shut gate, for a background tune that did not run.
func (g tunerGate) why() string {
	switch {
	case g.held:
		return "a channel scan or a hold is on"
	case g.recording:
		return "a recording is on that tuner"
	case g.soon:
		return "a recording starts within 3 minutes"
	case g.free < 0:
		return "the tuner did not answer"
	case g.free < sweepFreeTuners:
		return fmt.Sprintf("%d tuner free, and the last one is never taken", g.free)
	}
	return "open"
}

// tunerGateOpen reads the gates for a tune on ch's device and asks open,
// returning what it read. The schedule and the device are asked only while
// the gates read so far leave it open.
func (h *Hub) tunerGateOpen(ctx context.Context, ch store.SourceChannel, open func(tunerGate) bool) (bool, tunerGate) {
	g := tunerGate{free: sweepFreeTuners}
	if values, err := h.Store.Settings(ctx); err != nil || strings.TrimSpace(values[SettingStillSweep]) == "0" {
		g.off = true
	}
	host := hostOf(ch.BaseURL)
	h.mu.Lock()
	g.held = h.hold > 0 || h.scanCancel != nil || len(h.guessTunes) > 0
	g.opened = len(h.tunerMuxesLocked(host)) > 0
	for _, f := range h.feedsLocked() {
		// A recording or a viewer on another tuner leaves this one to
		// its own free-tuner count: a recording all evening on one box
		// stopped every still on the other.
		if m := muxOf(h, f); m != nil && hostOf(m.host) != host && hostOf(m.base) != host {
			continue
		}
		if f.recordingNow() {
			g.recording = true
		}
		if f.heldOutside() || f.tuneHeld() {
			g.used = true
		}
		for _, r := range f.renditions {
			if r.viewers > 0 || r.guess {
				g.used = true
			}
		}
	}
	h.mu.Unlock()
	if !open(g) {
		return false, g
	}
	if h.RecordingSoon != nil && h.RecordingSoon(ctx, sweepRecordingLead) {
		g.soon = true
		return false, g
	}
	raw, err := fetchTunerStatus(ctx, ch.BaseURL)
	if err != nil {
		g.free = -1
		return false, g
	}
	g.free = 0
	for _, row := range raw {
		if row.VctNumber == "" && row.TargetIP == "" {
			g.free++
		}
	}
	return open(g), g
}

// sweepOne tunes one target, waits for the stills of every channel on it,
// and lets the tuner go at once.
func (h *Hub) sweepOne(ctx context.Context, t sweepTarget) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	state := &sweepState{cancel: cancel}
	h.mu.Lock()
	h.sweep = state
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.sweep == state {
			h.sweep = nil
		}
		h.mu.Unlock()
	}()
	began := time.Now()
	h.noteSweepTune(t.key, began)
	f, err := h.tuneFeed(ctx, t.first, nil)
	if err != nil {
		// tuneFeed returns holding h.mu.
		if errors.Is(err, ErrNoSignal) || h.darkWithinLocked(t.first.ID, t.first.FrequencyHz, sweepDarkFor) {
			if h.sweepDark == nil {
				h.sweepDark = map[string]time.Time{}
			}
			h.sweepDark[t.key] = time.Now()
		}
		h.mu.Unlock()
		slog.Debug(fmt.Sprintf("stills: %s: %v", t.first.GuideNumber, err))
		return
	}
	if ctx.Err() != nil {
		// Stopped while it tuned: StopSweep runs under h.mu, so this sees it.
		h.dropIfUnusedLocked(f)
		h.mu.Unlock()
		return
	}
	// Held, so nothing else drops it, until the stills land or a watch
	// takes over.
	f.probing = true
	state.feed = f
	h.mu.Unlock()
	h.waitStills(ctx, t.channels, began)
	// A watch that stopped the tune says nothing about the channels' pictures.
	if ctx.Err() == nil {
		h.noteSweepResult(t, began)
	}
	// Every program on the tune got a still, not only the target's, and
	// the tables say each one's captions and sound.
	ids := t.channels
	h.mu.Lock()
	m := muxOf(h, f)
	program := f.program
	h.mu.Unlock()
	if m != nil {
		data := m.pictureBytes()
		for _, job := range h.frameJobs(m) {
			if !slices.Contains(ids, job.ChannelID) {
				ids = append(ids, job.ChannelID)
			}
			p := job.Program
			if p == 0 {
				p = program
			}
			if tracks := AudioTracks(data, p); len(tracks) > 0 {
				_ = h.Store.SetChannelStreamInfo(ctx, job.ChannelID, HasCaptions(data, p), AudioLayout(tracks))
			}
		}
	}
	h.mu.Lock()
	if state.feed == f {
		state.feed = nil
		f.probing = false
		h.dropIfUnusedLocked(f)
	}
	h.mu.Unlock()
	n := 0
	for _, id := range ids {
		if at, ok := frameTime(h.Dir, id); ok && !at.Before(began) {
			h.noteSwept(id, at)
			n++
		}
	}
	slog.Debug(fmt.Sprintf("stills: %s: %d of %d channels in %s", t.first.GuideNumber, n, len(ids), time.Since(began).Round(100*time.Millisecond)))
}

// waitStills waits until every channel has a still written since began, the
// grab gives up, or ctx ends. The tune's own grab (watchFrames) writes them.
func (h *Hub) waitStills(ctx context.Context, ids []int64, began time.Time) {
	deadline := time.Now().Add(sweepGrabWait)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(deadline) {
		done := true
		for _, id := range ids {
			if at, ok := frameTime(h.Dir, id); !ok || at.Before(began) {
				done = false
				break
			}
		}
		if done {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func frameTime(dir string, id int64) (time.Time, bool) {
	info, err := os.Stat(FramePath(dir, id, 0))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func (h *Hub) noteSwept(id int64, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.swept == nil {
		h.swept = map[int64]time.Time{}
	}
	h.swept[id] = at
}

// SweptAt reports when the sweep wrote a channel's still, when the still on
// disk is still that one.
func (h *Hub) SweptAt(id int64) (time.Time, bool) {
	h.mu.Lock()
	at, ok := h.swept[id]
	h.mu.Unlock()
	if !ok {
		return time.Time{}, false
	}
	if now, ok := frameTime(h.Dir, id); !ok || !now.Equal(at) {
		return time.Time{}, false
	}
	return at, true
}

// StopSweep stops the still sweep's tune, and a sound check's twin tune, for
// a watch, recording, or warm that is about to tune. A watch of a channel on
// the tuned frequency keeps the tune and joins it; any other gets the tuner
// back before it tunes.
func (h *Hub) StopSweep(freq int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sweep, sound := h.sweep, h.soundTune
	h.sweep, h.soundTune = nil, nil
	h.stopBackgroundLocked(sweep, freq)
	h.stopBackgroundLocked(sound, freq)
}

func (h *Hub) stopBackgroundLocked(s *sweepState, freq int) {
	if s == nil {
		return
	}
	s.cancel()
	f := s.feed
	if f == nil {
		return
	}
	s.feed = nil
	f.probing = false
	if m := muxOf(h, f); freq > 0 && m != nil && m.freq == freq {
		// The watch joins it; a watch that fails leaves it to go soon.
		h.lingerLocked(f, guessLinger)
		return
	}
	h.dropIfUnusedLocked(f)
}
