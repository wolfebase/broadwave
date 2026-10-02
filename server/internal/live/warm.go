package live

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"time"

	"broadwave/internal/store"
)

// Warm starts a channel's picture before anyone asks to watch it, so a
// channel change that follows starts on segments that already exist. It is a
// guess: it uses a frequency this server already has open, never frees
// another picture to fit the budget, and keeps one guess at a time. An ATSC
// 3.0 channel always needs its own tune and takes 5-7 s cold, so a guess may
// tune one for a player that takes it as sent, when a second tuner that can
// carry it stays free. The picture stops after RenditionIdle unless a watch
// joins it, and its tuner with it. It reports whether the picture is running.
func (h *Hub) Warm(ctx context.Context, channelID int64, want Rendition, alternates bool) (bool, error) {
	want = want.normalized()
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	h.mu.Lock()
	f := h.channels[ch.ID]
	untuned := f == nil && (ch.FrequencyHz <= 0 || h.muxes[ch.FrequencyHz] == nil)
	h.mu.Unlock()
	var opened *autoGuess
	// Only a picture sent as broadcast is cheap enough to start on a guess.
	if untuned && want.Video == "copy" {
		if opened = h.openGuess(ctx, ch); opened == nil {
			return false, nil
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f = h.channels[ch.ID]
	if opened != nil {
		if f != nil {
			opened.body.Close()
		} else {
			f = h.attachAutoLocked(ch, opened.body, opened.host, opened.url)
			slog.Info("live: tuned " + ch.GuideNumber + " ahead of a channel change")
		}
	}
	if f == nil && (ch.FrequencyHz <= 0 || h.muxes[ch.FrequencyHz] == nil) {
		return false, nil
	}
	if f == nil {
		if f, err = h.ensureFeedLocked(ctx, ch, nil); err != nil {
			return false, err
		}
	}
	if alternates && want.Track != "" {
		main := want
		main.Track = ""
		main = main.normalized()
		if h.mainServesLocked(f, main, want.Track) {
			want = main
		}
	}
	if want.Codec == "hevc" && !h.HEVC {
		want.Codec = ""
	}
	key := want.Key()
	h.dropGuessesLocked(f, key)
	if r := f.renditions[key]; r != nil {
		if r.viewers == 0 {
			h.idleLocked(channelID, key, r)
		}
		return true, nil
	}
	if want.Video != "copy" && h.Host.Tiles > 0 && h.transcodesLocked() >= h.Host.Tiles {
		h.dropIfUnusedLocked(f)
		return false, nil
	}
	r, err := h.ensureRenditionLocked(f, want)
	if err != nil {
		h.dropIfUnusedLocked(f)
		return false, err
	}
	// A guess is the first picture a real watch frees, and it does not make
	// its tuner look watched.
	r.guess = true
	r.seen = time.Time{}
	h.idleLocked(channelID, key, r)
	h.changed()
	return true, nil
}

// dropGuessesLocked stops every guessed picture nobody joined, except the one
// named, so browsing a list never runs more than one.
func (h *Hub) dropGuessesLocked(keep *feed, keepKey string) {
	for _, f := range h.feedsLocked() {
		for key, r := range f.renditions {
			if r.guess && r.viewers == 0 && (f != keep || key != keepKey) {
				h.stopRenditionLocked(f, key)
			}
		}
		if f != keep {
			h.dropIfUnusedLocked(f)
		}
	}
}

func (h *Hub) idleLocked(channelID int64, key string, r *rendition) {
	stopTimer(&r.idle)
	r.idle = time.AfterFunc(h.RenditionIdle, func() { h.idleStop(channelID, key) })
}

type autoGuess struct {
	body      io.ReadCloser
	host, url string
}

// openGuess opens a device-tuned (ATSC 3.0) channel for a guess. It runs
// without h.mu: the device takes about 2 s to answer, and every playlist and
// watch waits on h.mu. It opens nothing unless another tuner that can carry
// the channel stays free for a recording or another app on the device.
func (h *Hub) openGuess(ctx context.Context, ch store.SourceChannel) *autoGuess {
	need := NeedFor(ch.VideoCodec, ch.AudioCodec, ch.ATSC3)
	if !need.ATSC3 || ch.TunerCount == 0 || ch.BaseURL == "" || hlsStream(ch) {
		return nil
	}
	raw, err := fetchTunerStatus(ctx, ch.BaseURL)
	if err != nil {
		return nil
	}
	tuners := make([]Tuner, len(raw))
	for i, row := range raw {
		tuners[i] = Tuner{Index: i, Guide: row.VctNumber, Target: row.TargetIP}
	}
	h.markATSC3(ctx, ch.BaseURL, tuners)
	h.mu.Lock()
	used := h.usedTunersLocked(hostOf(ch.BaseURL))
	replaced := h.guessGuidesLocked()
	held := maps.Clone(h.reserved)
	if held == nil {
		held = map[int]bool{}
	}
	if h.hold > 0 {
		maps.Copy(held, HoldBack(tuners, h.hold))
	}
	h.mu.Unlock()
	free := 0
	for _, t := range tuners {
		// A guess nobody joined is stopped when this one starts.
		idle := (t.Target == "" && t.Guide == "") || replaced[t.Guide]
		if t.ATSC3 && idle && !used[t.Index] && !held[t.Index] {
			free++
		}
	}
	if free < 2 {
		return nil
	}
	url := h.autoURL(ch, h.streamRootFor(ctx, ch.BaseURL, ch.GuideNumber))
	res, err := openStream(url, "", "")
	if err != nil {
		return nil
	}
	return &autoGuess{body: res.Body, host: hostOf(ch.BaseURL), url: url}
}

// guessGuidesLocked is the guide numbers of device-tuned streams that carry
// only a guess nobody joined.
func (h *Hub) guessGuidesLocked() map[string]bool {
	out := map[string]bool{}
	for _, m := range h.muxes {
		if m.tuner >= 0 || m.input != "" || len(m.feeds) != 1 {
			continue
		}
		for g, f := range m.feeds {
			if f.recording != nil || f.probing || f.exports > 0 || len(f.renditions) == 0 {
				continue
			}
			guess := true
			for _, r := range f.renditions {
				guess = guess && r.guess && r.viewers == 0
			}
			out[g] = guess
		}
	}
	return out
}
