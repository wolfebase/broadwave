package live

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestASilenceMovesTheClockOn(t *testing.T) {
	start := time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return start }
	at := tl.Wall(0)
	// The tuner was silent for 70 s, then carried on with the next timestamp.
	tl.Shift(70 * time.Second)
	if got := tl.Wall(90000).Sub(at); got != 71*time.Second {
		t.Fatalf("one second of picture after 70 s of silence is %v after the last, want 71s", got)
	}
	tl.Settle()
	// A station clock break later is pinned to the last segment, as before.
	lastEnd := at.Add(80 * time.Second)
	tl.Reanchor(5_000_000, lastEnd)
	if got := tl.Wall(5_000_000); !got.Equal(lastEnd) {
		t.Fatalf("a break without silence moved %v", got.Sub(lastEnd))
	}
}

func TestABreakAfterASilenceLandsOnTheWallClock(t *testing.T) {
	tl := NewTimeline()
	tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
	at := tl.Wall(0)
	lastEnd := at.Add(2 * time.Second)
	// The silence, then a frame with a stray timestamp: the break keeps the silence.
	tl.Shift(70 * time.Second)
	tl.Reanchor(7_000_000, lastEnd)
	if got := tl.Wall(7_000_000).Sub(lastEnd); got != 70*time.Second {
		t.Fatalf("the break after the silence is %v after the last segment, want 70s", got)
	}
	// The silence is counted once: the next break is pinned again.
	tl.Reanchor(180_000, lastEnd.Add(73*time.Second))
	if got := tl.Wall(180_000).Sub(lastEnd); got != 73*time.Second {
		t.Fatalf("second break %v, want 73s", got)
	}
	// Before the first anchor there is no clock to move.
	fresh := NewTimeline()
	fresh.Shift(time.Minute)
	if _, _, set := fresh.Anchor(); set {
		t.Fatal("a shift anchored a clock")
	}
}

func TestEveryEncodeOnAMuxMovesTogether(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	m := &mux{freq: 539000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	other := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.mu.Lock()
	h.muxes[m.freq] = m
	h.muxes[other.freq] = other
	f1 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "104.1"}})
	f2 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "138.1"}})
	f3 := h.addFeedLocked(other, store.SourceChannel{Channel: store.Channel{ID: 3, GuideNumber: "4.1"}})
	clocks := map[string]*Timeline{}
	for name, f := range map[string]*feed{"copy": f1, "tile": f2, "other": f3} {
		tl := NewTimeline()
		tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
		tl.Wall(0)
		clocks[name] = tl
		f.renditions[name] = &rendition{clock: tl}
	}
	f1.renditions["none"] = &rendition{}
	h.mu.Unlock()
	h.shiftClocks(m, 70*time.Second)
	base := time.Date(2026, 10, 2, 15, 50, 56, 0, time.UTC)
	for name, want := range map[string]time.Duration{"copy": 70 * time.Second, "tile": 70 * time.Second, "other": 0} {
		if got := clocks[name].Wall(0).Sub(base); got != want {
			t.Errorf("%s moved %v, want %v", name, got, want)
		}
	}
}

// The first part after a break stays listed while its segment is open and
// for three targets after. Anchoring it on every stamp counted the silence
// once and then dropped it, so later frames went back by the silence.
func TestAListedPartAfterABreakKeepsTheSilence(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"init.mp4":      videoInit(),
		"seg00000.m4s":  keyframeFragment(0, 90000),
		"part00001.m4s": keyframeFragment(9_000_000, 90000),
		"seg00001.m4s":  keyframeFragment(9_000_000, 90000),
		"part00002.m4s": keyframeFragment(9_090_000, 90000),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tl := NewTimeline()
	tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
	var st playlistStamper
	head := "#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1.000,\nseg00000.m4s\n"
	st.stamp(dir, []byte(head), tl)
	lastEnd := tl.Wall(90000)
	tl.Shift(70 * time.Second)
	after := head + "#EXT-X-DISCONTINUITY\n#EXT-X-PART:DURATION=1.000,INDEPENDENT=YES,URI=\"part00001.m4s\"\n#EXTINF:1.000,\nseg00001.m4s\n#EXT-X-PART:DURATION=1.000,INDEPENDENT=YES,URI=\"part00002.m4s\"\n"
	for i := range 3 {
		st.stamp(dir, []byte(after), tl)
		if got := tl.Wall(9_090_000).Sub(lastEnd); got != 71*time.Second {
			t.Fatalf("stamp %d: the part after the break is %v after the last segment, want 71s", i+1, got)
		}
	}
}
