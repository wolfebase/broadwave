package live

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// bufferHub is a hub on a live fake tuner with the buffer on.
func bufferHub(t *testing.T) (*Hub, *store.Store) {
	t.Helper()
	h, st, _ := liveHub(t)
	h.Buffer = time.Hour
	return h, st
}

// liveHub is a hub on a live fake tuner with one channel, 4.1.
func liveHub(t *testing.T) (*Hub, *store.Store, *fake.Server) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{{Number: "4.1", Name: "KBWV", Freq: 593000000}}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	// Runs before the temp dirs are removed, so no frame grab writes into
	// a directory being deleted.
	t.Cleanup(h.Shutdown)
	return h, st, srv
}

func probeDuration(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(strings.Split(string(out), "\n")[0]), 64)
	if err != nil {
		t.Fatalf("duration %q: %v", out, err)
	}
	return d
}

// seamless fails when the picture's timestamps step back or skip, which is
// what a byte written twice or lost at the hand-over to live would do.
func seamless(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=dts_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	prev := -1.0
	n := 0
	for _, line := range strings.Fields(string(out)) {
		v, err := strconv.ParseFloat(strings.TrimSuffix(line, ","), 64)
		if err != nil {
			continue
		}
		if prev >= 0 && (v <= prev || v-prev > 0.1) {
			t.Fatalf("frame %d: %.3f after %.3f", n, v, prev)
		}
		prev = v
		n++
	}
	if n < 100 {
		t.Fatalf("%d frames", n)
	}
}

func TestRecordingStartsFromTheBufferedShowStart(t *testing.T) {
	h, st := bufferHub(t)
	id := idOf(t, st, "4.1")
	// Someone is on the channel: the export keeps the tune and fills the ring.
	ctx, cancel := context.WithCancel(context.Background())
	exported := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, io.Discard)
		close(exported)
	}()
	defer func() {
		cancel()
		<-exported
	}()
	time.Sleep(6 * time.Second)
	status := h.Status()
	if len(status) != 1 || status[0].Buffer == nil || status[0].Buffer.State != "on" || status[0].Buffer.Minutes < 0.05 || status[0].Buffer.Bytes == 0 {
		t.Fatalf("diagnostics: %+v", status)
	}
	showStart := time.Now().Add(-4 * time.Second)
	rec, err := h.RecordMeta(context.Background(), 1, store.Recording{ChannelID: id, Title: "News", StartedAt: showStart})
	if err != nil {
		t.Fatal(err)
	}
	// The catalog keeps whole seconds.
	if d := rec.StartedAt.Sub(showStart); d < -time.Second || d > 2*time.Second {
		t.Fatalf("started %s after the show", d)
	}
	time.Sleep(3 * time.Second)
	h.StopRecord(rec.ID)
	got := probeDuration(t, rec.Path)
	// Four seconds from the buffer and three live. Without the buffer it is three.
	if got < 6 || got > 8.5 {
		t.Fatalf("recording is %.2f s", got)
	}
	seamless(t, rec.Path)
	if entries, _ := os.ReadDir(filepath.Join(h.Dir, "ring")); len(entries) != 1 {
		t.Fatalf("%d rings while tuned", len(entries))
	}
}

func TestRecordingWithoutAStartTimeStartsNow(t *testing.T) {
	h, st := bufferHub(t)
	id := idOf(t, st, "4.1")
	ctx, cancel := context.WithCancel(context.Background())
	exported := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, io.Discard)
		close(exported)
	}()
	time.Sleep(5 * time.Second)
	h.RecordingsDir = filepath.Join(t.TempDir(), "elsewhere")
	rec, err := h.RecordMeta(context.Background(), 1, store.Recording{ChannelID: id, Title: "News", PassID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(rec.Path) != h.RecordingsDir {
		t.Fatalf("recorded to %s, not the recordings folder %s", rec.Path, h.RecordingsDir)
	}
	if saved, err := st.Recording(context.Background(), rec.ID); err != nil || saved.PassID != 5 {
		t.Fatalf("the recording forgot its pass: %+v %v", saved, err)
	}
	time.Sleep(3 * time.Second)
	h.StopRecord(rec.ID)
	if got := probeDuration(t, rec.Path); got < 2 || got > 4.5 {
		t.Fatalf("recording is %.2f s", got)
	}
	cancel()
	<-exported
	// The tune is gone, and its ring with it.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(filepath.Join(h.Dir, "ring"))
		if len(entries) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the ring outlived its tune")
}

// TestBufferSoak is opt-in: BROADWAVE_BUFFER_SOAK=61m holds a tune that long
// with a BROADWAVE_BUFFER_WINDOW ring (default 15m), samples the heap, and in
// the last minute records from ten minutes back.
func TestBufferSoak(t *testing.T) {
	soak, err := time.ParseDuration(os.Getenv("BROADWAVE_BUFFER_SOAK"))
	if err != nil || soak < 12*time.Minute {
		t.Skip("set BROADWAVE_BUFFER_SOAK to 12m or more")
	}
	h, st := bufferHub(t)
	h.Buffer = 15 * time.Minute
	if w, err := time.ParseDuration(os.Getenv("BROADWAVE_BUFFER_WINDOW")); err == nil {
		h.Buffer = w
	}
	id := idOf(t, st, "4.1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = h.Export(ctx, id, io.Discard) }()
	start := time.Now()
	var heaps []uint64
	sample := func() {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		heaps = append(heaps, ms.HeapInuse)
		t.Logf("t=%s heap=%d MB ring=%d MB", time.Since(start).Round(time.Second), ms.HeapInuse>>20, dirSize(filepath.Join(h.Dir, "ring"))>>20)
	}
	for time.Since(start) < soak-time.Minute {
		time.Sleep(30 * time.Second)
		sample()
	}
	rec, err := h.RecordMeta(context.Background(), 2, store.Recording{ChannelID: id, Title: "Soak", StartedAt: time.Now().Add(-10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
	h.StopRecord(rec.ID)
	got := probeDuration(t, rec.Path)
	t.Logf("recording %.1f s from %s", got, rec.StartedAt.Format(time.TimeOnly))
	if got < 10.8*60 {
		t.Fatalf("recording is %.1f s; want 10 minutes back plus one live", got)
	}
	seamless(t, rec.Path)
	// Past the window's first fill, the heap stays within 16 MB of where it was.
	settled := int(h.Buffer/(30*time.Second)) + 1
	if settled >= len(heaps) {
		settled = len(heaps) / 2
	}
	base := heaps[settled]
	for i, v := range heaps[settled:] {
		if v > base+16<<20 {
			t.Fatalf("heap %d MB at sample %d, %d MB when the window filled", v>>20, settled+i, base>>20)
		}
	}
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func TestStoppingARecordingMidBackfillLetsGo(t *testing.T) {
	h, st := bufferHub(t)
	id := idOf(t, st, "4.1")
	ctx, cancel := context.WithCancel(context.Background())
	exported := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, io.Discard)
		close(exported)
	}()
	time.Sleep(5 * time.Second)
	before := runtime.NumGoroutine()
	for range 3 {
		rec, err := h.RecordMeta(context.Background(), 1, store.Recording{ChannelID: id, Title: "News", StartedAt: time.Now().Add(-4 * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		h.StopRecord(rec.ID)
		if d := time.Since(start); d > 4*time.Second {
			t.Fatalf("stop took %s", d)
		}
		got, err := st.Recording(context.Background(), rec.ID)
		if err != nil || got.Status == "recording" {
			t.Fatalf("status %q err %v", got.Status, err)
		}
	}
	cancel()
	<-exported
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > before {
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("%d goroutines, %d before", n, before)
	}
}

func TestBufferWindowFollowsTheSetting(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := &Hub{Store: st, Buffer: time.Hour}
	if got := h.bufferWindow(); got != time.Hour {
		t.Fatalf("unset: %s", got)
	}
	for raw, want := range map[string]time.Duration{"0": 0, "30": 30 * time.Minute, "240": 4 * time.Hour} {
		if err := st.PutSettings(context.Background(), map[string]string{"bufferMinutes": raw}); err != nil {
			t.Fatal(err)
		}
		if got := h.bufferWindow(); got != want {
			t.Fatalf("%s: %s", raw, got)
		}
	}
}

func TestBufferedSinceFollowsTheTunedFrequency(t *testing.T) {
	h, st := bufferHub(t)
	id := idOf(t, st, "4.1")
	if !h.BufferedSince(context.Background(), id).IsZero() {
		t.Fatal("nothing is tuned yet")
	}
	ctx, cancel := context.WithCancel(context.Background())
	exported := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, io.Discard)
		close(exported)
	}()
	before := time.Now()
	time.Sleep(3 * time.Second)
	since := h.BufferedSince(context.Background(), id)
	if since.IsZero() || since.Before(before.Add(-time.Second)) || since.After(before.Add(2*time.Second)) {
		t.Fatalf("since %v, tuned at %v", since, before)
	}
	cancel()
	<-exported
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

func TestASecondExportHasItsPictureAtOnce(t *testing.T) {
	h, st := bufferHub(t)
	defer h.Shutdown()
	id := idOf(t, st, "4.1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, io.Discard)
		close(first)
	}()
	time.Sleep(exportHead + 2*time.Second)
	out := &syncBuffer{}
	second := make(chan struct{})
	go func() {
		_ = h.Export(ctx, id, out)
		close(second)
	}()
	// Without the buffer the copy waits a second to probe and then for a
	// keyframe; with it, three seconds are there at once.
	time.Sleep(700 * time.Millisecond)
	got := out.bytes()
	path := filepath.Join(t.TempDir(), "head.ts")
	if err := os.WriteFile(path, got, 0o644); err != nil {
		t.Fatal(err)
	}
	if d := probeDuration(t, path); d < 1.5 {
		t.Fatalf("%d bytes, %.2f s of picture 700 ms into the export", len(got), d)
	}
	cancel()
	<-first
	<-second
}

// playlistSpan is the program time of a stamped playlist's first segment and
// where its last one ends.
func playlistSpan(body []byte) (first, end time.Time, ok bool) {
	var at time.Time
	for _, line := range strings.Split(string(body), "\n") {
		if v, found := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); found {
			t, err := time.Parse("2006-01-02T15:04:05.000Z", v)
			if err != nil {
				return time.Time{}, time.Time{}, false
			}
			if first.IsZero() {
				first = t
			}
			at, end = t, t
		} else if v, found := strings.CutPrefix(line, "#EXTINF:"); found && !at.IsZero() {
			sec, err := strconv.ParseFloat(strings.TrimSuffix(v, ","), 64)
			if err == nil {
				end = end.Add(time.Duration(sec * float64(time.Second)))
			}
		}
	}
	return first, end, !first.IsZero()
}

// A browser joining a room that plays well behind live used to get a new
// encode at the live edge and hold its first picture until the room caught
// up, 11-15 s. The encode now starts in the buffer at the room's frame.
func TestANewEncodeForARoomBehindLiveHoldsItsFrame(t *testing.T) {
	h, st := bufferHub(t)
	id := idOf(t, st, "4.1")
	ctx := context.Background()
	first, err := h.Watch(ctx, id, Rendition{Video: "copy", Audio: "copy"}, false)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if body, err := h.Playlist(id, first.Rendition); err == nil {
			if _, _, ok := playlistSpan(body); ok {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the first encode wrote no segment")
		}
		time.Sleep(200 * time.Millisecond)
	}
	// The room plays 7 s behind; the buffer holds it once the tune is 11 s old.
	time.Sleep(12 * time.Second)
	// A paused group room minutes back would make an encode chew through
	// the backlog; it starts at the edge.
	paused, err := h.WatchAt(ctx, id, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}, false, time.Now().Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if h.FromBuffer(id, paused.Rendition) {
		t.Fatal("an encode for a room two minutes back started in the buffer")
	}
	frame := time.Now().Add(-7 * time.Second)
	asked := time.Now()
	second, err := h.WatchAt(ctx, id, Rendition{Video: "360", Audio: "aac2", Mode: "broadcast"}, false, frame)
	if err != nil {
		t.Fatal(err)
	}
	if second.Rendition == first.Rendition {
		t.Fatalf("joined %s instead of starting an encode", second.Rendition)
	}
	for {
		body, err := h.Playlist(id, second.Rendition)
		if err == nil {
			if start, end, ok := playlistSpan(body); ok {
				if start.After(frame) {
					t.Fatalf("the new encode starts %v after the room's frame", start.Sub(frame))
				}
				// The room moves on while the encode catches up.
				if !end.Before(frame.Add(time.Since(asked) + time.Second)) {
					t.Logf("frame held %v after the watch, first picture %v before it", time.Since(asked), frame.Sub(start))
					h.mu.Lock()
					dated := h.channels[id].renditions[second.Rendition].dated
					h.mu.Unlock()
					if dated {
						t.Fatal("the new encode was dated by arrival, not by the running one")
					}
					if !h.FromBuffer(id, second.Rendition) || h.FromBuffer(id, first.Rendition) {
						t.Fatal("FromBuffer does not tell the encode started in the buffer from the one at the edge")
					}
					return
				}
			}
		}
		if time.Since(asked) > 8*time.Second {
			t.Fatalf("the new encode did not reach the room's frame in 8 s: %s", body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
