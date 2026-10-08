package live

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

func TestMosaicKey(t *testing.T) {
	key, err := MosaicKey([]int64{7, 3, 12})
	if err != nil || key != "7-3-12" {
		t.Fatalf("key %q %v", key, err)
	}
	ids, err := ParseMosaicKey(key)
	if err != nil || !slices.Equal(ids, []int64{7, 3, 12}) {
		t.Fatalf("parse %v %v", ids, err)
	}
	for _, bad := range [][]int64{{1}, {1, 2, 3, 4, 5}, {1, 1}, {0, 2}, {-1, 2}} {
		if _, err := MosaicKey(bad); err == nil {
			t.Errorf("%v made a key", bad)
		}
	}
	// A key is also a directory name, so only the form MosaicKey writes passes.
	for _, bad := range []string{"1", "01-2", "1-2-", "-1-2", "1--2", "1-2/..", "1-+2", "1-2-3-4-5", "2-2"} {
		if _, err := ParseMosaicKey(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestMosaicArgs(t *testing.T) {
	two := strings.Join(mosaicArgs([]mosaicInput{{program: 3, input: "pipe:3"}, {input: "pipe:4"}}, "libx264"), " ")
	for _, want := range []string{
		"-i pipe:3", "-i pipe:4",
		"[0:p:3:v:0]yadif=mode=send_field:deint=interlaced,fps=60000/1001",
		"[1:v:0]yadif",
		"xstack=inputs=2:layout=0_0|w0_0:fill=black,pad=1920:1080:0:270,format=yuv420p[v]",
		"-map [v] -map 0:p:3:a:0?",
		"-c:v libx264", "-c:a aac",
		"frag_keyframe+empty_moov+default_base_moof+delay_moov pipe:1",
	} {
		if !strings.Contains(two, want) {
			t.Errorf("2-up args lack %q:\n%s", want, two)
		}
	}
	// Each input starts near zero, so pictures that arrive together show together.
	if strings.Contains(two, "-copyts") {
		t.Errorf("a mosaic must not keep broadcast timestamps: %s", two)
	}
	if n := strings.Count(two, "-fflags +genpts+discardcorrupt"); n != 2 {
		t.Errorf("input options apply to one -i each; fflags given %d times for 2 inputs", n)
	}
	three := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3"}, {input: "pipe:4"}, {input: "http://example.test/s.ts", userAgent: "TestAgent/1"}}, "h264_vaapi"), " ")
	for _, want := range []string{
		"-init_hw_device vaapi=va:/dev/dri/renderD128",
		"-reconnect_delay_max 5 -headers User-Agent: TestAgent/1\r\n -probesize 2000000 -analyzeduration 1500000 -i http://example.test/s.ts",
		"xstack=inputs=3:layout=0_0|w0_0|0_h0:fill=black,format=nv12,hwupload[v]",
		"-map [v] -map 0:a:0?",
		"-c:v h264_vaapi",
	} {
		if !strings.Contains(three, want) {
			t.Errorf("3-up args lack %q:\n%s", want, three)
		}
	}
	four := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3"}, {input: "pipe:4"}, {input: "pipe:5"}, {input: "pipe:6"}}, "libx264"), " ")
	if !strings.Contains(four, "layout=0_0|w0_0|0_h0|w0_h0:fill=black,format=yuv420p") {
		t.Errorf("quad layout: %s", four)
	}
}

// mosaicHub is a hub on a live fake tuner with two frequencies: 4.1 and 4.2
// share one, 5.1 has its own.
func mosaicHub(t *testing.T) (*Hub, *store.Store) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "4.2", Name: "KBWV2", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
	}}
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
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.RenditionIdle = 200 * time.Millisecond
	t.Cleanup(h.Shutdown)
	return h, st
}

func waitMosaicMedia(t *testing.T, h *Hub, key string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if body, err := h.MosaicPlaylist(key); err == nil && strings.Contains(string(body), "#EXTINF") {
			return string(body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("mosaic %s wrote no segment", key)
	return ""
}

func TestMosaicRidesTheTunesAndLetsThemGo(t *testing.T) {
	h, st := mosaicHub(t)
	ctx := context.Background()
	a, c := idOf(t, st, "4.1"), idOf(t, st, "5.1")

	// 4.1 already plays, so the mosaic of 4.1 and 5.1 adds one tune, not two.
	if _, err := h.Watch(ctx, a, Rendition{Video: "copy", Audio: "copy"}, false); err != nil {
		t.Fatal(err)
	}
	s, err := h.WatchMosaic(ctx, []int64{a, c})
	if err != nil {
		t.Fatal(err)
	}
	if s.Playlist != "/media/mosaic/"+s.Key+"/index.m3u8" || s.Viewers != 1 {
		t.Fatalf("session %+v", s)
	}
	body := waitMosaicMedia(t, h, s.Key)
	if !strings.Contains(body, "#EXT-X-PROGRAM-DATE-TIME") || !strings.Contains(body, "seg00000") {
		t.Fatalf("playlist:\n%s", body)
	}
	h.mu.Lock()
	tunes := len(h.muxes)
	h.mu.Unlock()
	if tunes != 2 {
		t.Fatalf("%d tunes, want 2", tunes)
	}
	if again, err := h.WatchMosaic(ctx, []int64{a, c}); err != nil || again.Key != s.Key || again.Viewers != 2 {
		t.Fatalf("second viewer %+v %v", again, err)
	}
	if path, ok := h.MosaicFile(s.Key, "init.mp4"); !ok {
		t.Fatal("no init.mp4")
	} else if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.MosaicFile(s.Key, "../index.m3u8"); ok {
		t.Fatal("a name outside the mosaic's folder was served")
	}

	// Leaving 4.1 keeps its tune: the mosaic still reads it.
	h.Release(a, "")
	h.ReleaseMosaic(s.Key)
	time.Sleep(500 * time.Millisecond)
	h.mu.Lock()
	_, held := h.channels[a]
	h.mu.Unlock()
	if !held {
		t.Fatal("the mosaic lost 4.1 when its other viewer left")
	}

	h.ReleaseMosaic(s.Key)
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.Lock()
		left := len(h.muxes) + len(h.mosaics)
		h.mu.Unlock()
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d tunes or mosaics left after the last viewer", left)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(h.mosaicDir(s.Key)); !os.IsNotExist(err) {
		t.Fatalf("mosaic folder left behind: %v", err)
	}
}

func TestMosaicTakesTwoPictures(t *testing.T) {
	h, st := mosaicHub(t)
	ctx := context.Background()
	a, b, c := idOf(t, st, "4.1"), idOf(t, st, "4.2"), idOf(t, st, "5.1")
	h.Host.Tiles = 2
	s, err := h.WatchMosaic(ctx, []int64{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if used, limit, _ := h.Pictures(); used != 2 || limit != 2 {
		t.Fatalf("pictures %d of %d", used, limit)
	}
	var full *PictureError
	if _, err := h.Watch(ctx, c, Rendition{Video: "540", Audio: "none"}, false); !errors.As(err, &full) {
		t.Fatalf("a tile past the budget: %v", err)
	}
	if _, err := h.WatchMosaic(ctx, []int64{c, a}); !errors.As(err, &full) {
		t.Fatalf("a second mosaic past the budget: %v", err)
	}
	// The refused mosaic let go of 5.1.
	h.mu.Lock()
	_, tuned := h.channels[c]
	h.mu.Unlock()
	if tuned {
		t.Fatal("5.1 stayed tuned after its mosaic was refused")
	}
	h.ReleaseMosaic(s.Key)
}

func TestALostTuneEndsItsMosaic(t *testing.T) {
	h, st := mosaicHub(t)
	ctx := context.Background()
	a, c := idOf(t, st, "4.1"), idOf(t, st, "5.1")
	s, err := h.WatchMosaic(ctx, []int64{a, c})
	if err != nil {
		t.Fatal(err)
	}
	waitMosaicMedia(t, h, s.Key)
	// A recording that needs 5.1's tuner, or a device that went away, stops
	// that feed. The mosaic would show its picture frozen; it ends instead
	// and lets 4.1 go too.
	h.mu.Lock()
	h.stopFeedLocked(h.channels[c])
	mosaics, tunes := len(h.mosaics), len(h.muxes)
	h.mu.Unlock()
	if mosaics != 0 || tunes != 0 {
		t.Fatalf("%d mosaics and %d tunes left after 5.1 stopped", mosaics, tunes)
	}
	if _, err := h.MosaicPlaylist(s.Key); err == nil {
		t.Fatal("the ended mosaic still answers")
	}
	h.ReleaseMosaic(s.Key)
}
