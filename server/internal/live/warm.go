package live

import (
	"context"
	"time"
)

// Warm starts a channel's picture before anyone asks to watch it, so a
// channel change that follows starts on segments that already exist. It is a
// guess: it never tunes, only uses a frequency this server already has open,
// never frees another picture to fit the budget, and keeps one guess at a
// time. The picture stops after RenditionIdle unless a watch joins it.
// It reports whether the picture is running.
func (h *Hub) Warm(ctx context.Context, channelID int64, want Rendition, alternates bool) (bool, error) {
	want = want.normalized()
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[ch.ID]
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
