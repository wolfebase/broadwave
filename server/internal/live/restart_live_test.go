package live

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// segmentDates is the program date of each segment in a stamped playlist,
// and whether a discontinuity came before it.
func segmentDates(t *testing.T, body []byte) (dates []time.Time, broke []bool) {
	t.Helper()
	gap := false
	for _, line := range strings.Split(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "#EXT-X-DISCONTINUITY"):
			gap = true
		case strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"):
			at, err := time.Parse("2006-01-02T15:04:05.000Z", strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"))
			if err != nil {
				t.Fatal(err)
			}
			dates = append(dates, at)
			broke = append(broke, gap)
			gap = false
		}
	}
	return dates, broke
}

func mediaSequence(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if v, ok := strings.CutPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"); ok {
			return v
		}
	}
	return ""
}

// A second ffmpeg crash used to start a new playlist at sequence 0, and the
// players on it lost their place. The same command line now continues the
// same playlist after a discontinuity, and the frames after it carry the
// dates every other encode of the channel gives them: a room with a copy and
// a transcode stays on one frame.
func TestACrashedEncodeKeepsItsPlaylistAndItsDates(t *testing.T) {
	h, st, _ := liveHub(t)
	id := idOf(t, st, "4.1")
	ctx := context.Background()
	cp, err := h.Watch(ctx, id, Rendition{Video: "copy", Audio: "copy"}, false)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := h.Watch(ctx, id, Rendition{Video: "360", Audio: "aac2", Mode: "broadcast"}, false)
	if err != nil {
		t.Fatal(err)
	}
	playlist := func(key string) []byte {
		body, err := h.Playlist(id, key)
		if err != nil {
			return nil
		}
		return body
	}
	for _, key := range []string{cp.Rendition, tr.Rendition} {
		waitUntil(t, 20*time.Second, key+" segments", func() bool {
			dates, _ := segmentDates(t, playlist(key))
			return len(dates) >= 3
		})
	}
	before := playlist(cp.Rendition)
	h.mu.Lock()
	r := h.channels[id].renditions[cp.Rendition]
	gate, cmd := r.gate, r.cmd
	h.mu.Unlock()
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	var after []byte
	waitUntil(t, 20*time.Second, "segments after the break", func() bool {
		after = playlist(cp.Rendition)
		_, broke := segmentDates(t, after)
		for i, b := range broke {
			if b {
				return len(broke)-i >= 3
			}
		}
		return false
	})
	h.mu.Lock()
	same := r.gate == gate && h.channels[id].renditions[cp.Rendition] == r && len(r.restarts) == 1
	h.mu.Unlock()
	if !same {
		t.Fatal("the crash replaced the rendition or its playlist")
	}
	if a, b := mediaSequence(before), mediaSequence(after); a != b {
		t.Fatalf("media sequence moved from %s to %s", a, b)
	}
	oldDates, _ := segmentDates(t, before)
	newDates, broke := segmentDates(t, after)
	for i, d := range oldDates {
		if !newDates[i].Equal(d) {
			t.Fatalf("segment %d was dated %v and is now %v", i, d, newDates[i])
		}
	}
	// The transcode never stopped. Each copy segment after the break starts
	// on a frame it also starts a segment on, with the same date.
	from := 0
	for i, b := range broke {
		if b {
			from = i
		}
	}
	waitUntil(t, 10*time.Second, "the transcode past the break", func() bool {
		dates, _ := segmentDates(t, playlist(tr.Rendition))
		return len(dates) > 0 && !dates[len(dates)-1].Before(newDates[len(newDates)-1])
	})
	trDates, _ := segmentDates(t, playlist(tr.Rendition))
	for _, d := range newDates[from:] {
		best := time.Hour
		for _, o := range trDates {
			best = min(best, max(d.Sub(o), o.Sub(d)))
		}
		if best > 60*time.Millisecond {
			t.Fatalf("after the restart, %v is %v from every transcode segment\ncopy:\n%s\ntranscode:\n%s", d, best, after, playlist(tr.Rendition))
		}
	}
	if gap := newDates[from].Sub(oldDates[len(oldDates)-1]); gap <= 0 || gap > 10*time.Second {
		t.Fatalf("the first picture after the restart is %v after the last one before it", gap)
	}
}

// An encode that reads its input and writes nothing never trips the output
// watchdog, which waits for a playlist that grew once. libx265 on a silent channel did
// that. A start that writes nothing in startStall is ended and started
// again, and the budget still gives the tuner back.
func TestAStartThatWritesNothingIsStartedAgain(t *testing.T) {
	h, st, _ := liveHub(t)
	prev := startStall
	startStall = 500 * time.Millisecond
	t.Cleanup(func() { startStall = prev })
	dir := t.TempDir()
	mark := filepath.Join(dir, "starts")
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho x >> " + mark + "\nexec cat >/dev/null\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h.FFmpeg = bin
	id := idOf(t, st, "4.1")
	if _, err := h.Watch(context.Background(), id, Rendition{Video: "copy", Audio: "copy"}, false); err != nil {
		t.Fatal(err)
	}
	starts := func() int {
		b, _ := os.ReadFile(mark)
		return strings.Count(string(b), "x")
	}
	waitUntil(t, 15*time.Second, "a second start", func() bool { return starts() >= 2 })
	waitUntil(t, 30*time.Second, "the tuner given back", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.channels[id] == nil
	})
	// The first tune's scan can rebuild the encode once; that is not a restart.
	if n := starts(); n < restartBudget+1 || n > restartBudget+2 {
		t.Fatalf("%d starts, want %d", n, restartBudget+1)
	}
	if why := h.StartFailure(id, "copy.copy"); why != "" {
		t.Logf("last words: %s", why)
	}
}
