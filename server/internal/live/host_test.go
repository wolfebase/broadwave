package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestClassifyNamesEveryFamily(t *testing.T) {
	cases := []struct {
		encoder, vendor, want string
	}{
		{"h264_vaapi", "0x8086", "intel-vaapi"},
		{"hevc_vaapi", "0x8086", "intel-vaapi"},
		{"h264_qsv", "", "intel-qsv"},
		{"hevc_qsv", "0x8086", "intel-qsv"},
		{"h264_vaapi", "0x1002", "amd-vaapi"},
		{"h264_vaapi", "0x1022", "amd-vaapi"},
		{"h264_vaapi", "", "vaapi"},
		{"h264_nvenc", "", "nvidia"},
		{"hevc_nvenc", "", "nvidia"},
		{"h264_videotoolbox", "", "apple"},
		{"hevc_videotoolbox", "", "apple"},
		{"libx264", "", "software"},
		{"libx264", "0x8086", "software"},
	}
	for _, c := range cases {
		if got := Classify(c.encoder, c.vendor); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.encoder, c.vendor, got, c.want)
		}
	}
}

func TestBudgetCoversEveryHost(t *testing.T) {
	cases := []struct {
		name   string
		class  string
		speed  float64
		height int
		focus  string
		tiles  int
		full   bool
	}{
		{"intel vaapi at 5.4x", "intel-vaapi", 5.4, 1080, "720", 4, true},
		{"intel qsv at 3x", "intel-qsv", 3, 1080, "720", 2, true},
		{"amd vaapi at 2.5x", "amd-vaapi", 2.5, 1080, "720", 2, true},
		{"nvidia at 8x", "nvidia", 8, 1080, "720", 4, true},
		{"apple at 11x", "apple", 11, 1080, "720", 4, true},
		{"unknown vaapi at 4x", "vaapi", 4, 1080, "720", 4, true},
		{"fast software at 1.5x", "software", 1.5, 720, "540", 2, true},
		{"pi 5 class software at 0.7x", "software", 0.7, 540, "360", 1, true},
		{"j4125 class software at 0.4x", "software", 0.4, 540, "360", 1, false},
		{"unmeasured software", "software", 0, 540, "360", 1, false},
		{"unmeasured intel", "intel-vaapi", 0, 1080, "720", 2, true},
		{"boundary 2x", "nvidia", 2, 1080, "720", 2, true},
		{"boundary 1x", "apple", 1, 720, "540", 2, true},
		{"boundary 0.5x", "software", 0.5, 540, "360", 1, true},
	}
	for _, c := range cases {
		got := Budget(c.class, c.speed)
		if got.Height != c.height || got.Focus != c.focus || got.Tiles != c.tiles || got.FullRate != c.full {
			t.Errorf("%s: height %d focus %s tiles %d full %v", c.name, got.Height, got.Focus, got.Tiles, got.FullRate)
		}
	}
}

func TestHostLine(t *testing.T) {
	fast := Budget("intel-vaapi", 5.4)
	fast.Encoder = "h264_vaapi"
	if got := fast.Line(); got != "Intel GPU found: 1080p60 at 5.4x real time. 720p60 on the selected tile, 4 tiles." {
		t.Fatalf("fast: %q", got)
	}
	slow := Budget("software", 0.4)
	slow.Encoder = "libx264"
	if got := slow.Line(); got != "Software encoder: 1080p60 at 0.4x real time. Picture up to 540p. 360p on the selected tile, 1 tile." {
		t.Fatalf("slow: %q", got)
	}
	plain := Budget("software", 0)
	plain.Encoder = "libx264"
	if got := plain.Line(); got != "Software encoder. Picture up to 540p. 360p on the selected tile, 1 tile." {
		t.Fatalf("unmeasured: %q", got)
	}
}

func TestMeasureHostReadsAScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho 'frame=60 speed=0.7x' >&2\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got := MeasureHost(context.Background(), path, "libx264")
	if got.Class != "software" || got.Speed != 0.7 || got.Height != 540 || got.Focus != "360" || got.Tiles != 1 || !got.FullRate {
		t.Fatalf("measured %+v", got)
	}
	if !strings.Contains(got.Line(), "0.7x") || !strings.Contains(got.Line(), "360p60") {
		t.Fatalf("line %q", got.Line())
	}
}

func TestDecideFollowsTheMeasuredHost(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	caps := Caps{Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}}
	slow := Budget("software", 0.4)
	slow.Encoder = "libx264"
	main := DecideFor(src, caps, Prefs{}, "libx264", slow)
	if main.Rendition.Video != "540" || !strings.Contains(main.Reason, "540p on this server") {
		t.Fatalf("main: %s %s", main.Rendition.Key(), main.Reason)
	}
	focus := DecideFor(src, caps, Prefs{Quality: "focus", Audio: "stereo"}, "libx264", slow)
	if focus.Rendition.Video != "360" || focus.Rendition.FullRate || !strings.Contains(focus.Reason, "360p tile") || strings.Contains(focus.Reason, "360p60") {
		t.Fatalf("focus: %s full %v %s", focus.Rendition.Key(), focus.Rendition.FullRate, focus.Reason)
	}
	tile := DecideFor(src, caps, Prefs{Quality: "tile"}, "libx264", slow)
	if tile.Rendition.Video != "360" {
		t.Fatalf("background: %s", tile.Rendition.Key())
	}
	kept := DecideFor(Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true}, caps, Prefs{}, "libx264", slow)
	if kept.Rendition.Video != "copy" {
		t.Fatalf("a picture the device can play stays original: %s", kept.Rendition.Key())
	}
	gpu := Budget("intel-vaapi", 5.4)
	gpu.Encoder = "h264_vaapi"
	wide := DecideFor(src, caps, Prefs{Quality: "focus", Audio: "stereo"}, "h264_vaapi", gpu)
	if wide.Rendition.Key() != "720.aac2.broadcast.hevc" || !strings.Contains(wide.Reason, "720p60 tile") {
		t.Fatalf("gpu focus: %s %s", wide.Rendition.Key(), wide.Reason)
	}
}

func TestTileBudgetStopsAnExtraPicture(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	h.Host = Budget("software", 0.4)
	h.Host.Encoder = "libx264"
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range h.channels {
			for _, r := range f.renditions {
				if r.cmd != nil && r.cmd.Process != nil {
					_ = r.cmd.Process.Kill()
				}
			}
		}
	})
	defer lockHub(h)()
	m := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f1 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	f2 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "5.1"}, FieldOrder: "progressive"})
	spec := Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}
	held, err := h.ensureRenditionLocked(f1, spec)
	if err != nil {
		t.Fatal(err)
	}
	held.viewers = 1
	_, err = h.ensureRenditionLocked(f2, spec)
	var full *PictureError
	if !errors.As(err, &full) || full.Tiles != 1 || full.Error() != "This server can play 1 picture at once. Stop it to watch another." {
		t.Fatalf("second picture: %v", err)
	}
	joined, err := h.ensureRenditionLocked(f1, Rendition{Video: "360", Audio: "none", Mode: "broadcast"})
	if err != nil || joined == nil || joined.spec.Video != "540" {
		t.Fatalf("join: %v", err)
	}
	if _, err := h.ensureRenditionLocked(f2, Rendition{Video: "copy", Audio: "copy"}); err != nil {
		t.Fatalf("a copied broadcast does not take a tile: %v", err)
	}
}

func TestTileJoinPrefersTheClosestPicture(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "h264_videotoolbox")
	h.Host = Budget("apple", 3)
	h.Host.Encoder = "h264_videotoolbox"
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range h.channels {
			for _, r := range f.renditions {
				if r.cmd != nil && r.cmd.Process != nil {
					_ = r.cmd.Process.Kill()
				}
			}
		}
	})
	defer lockHub(h)()
	m := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	full, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "copy", Mode: "broadcast", Codec: "hevc"})
	if err != nil {
		t.Fatal(err)
	}
	full.viewers = 1
	small, err := h.ensureRenditionLocked(f, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"})
	if err != nil {
		t.Fatal(err)
	}
	small.viewers = 1
	joined, err := h.ensureRenditionLocked(f, Rendition{Video: "360", Audio: "none", Mode: "broadcast"})
	if err != nil || joined == nil || joined.spec.Video != "540" {
		t.Fatalf("a small tile joins the 540, not the full picture: %+v %v", joined, err)
	}
	again, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"})
	if err != nil || again == nil || again.spec.Video != "1080" {
		t.Fatalf("the same size joins itself: %+v %v", again, err)
	}
}

func TestIdlePictureDoesNotHoldTheSlot(t *testing.T) {
	h, m := tileHub(t, "software", 0.4)
	defer lockHub(h)()
	f1 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	f2 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "5.1"}, FieldOrder: "progressive"})
	spec := Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}
	if _, err := h.ensureRenditionLocked(f1, spec); err != nil {
		t.Fatal(err)
	}
	started, err := h.ensureRenditionLocked(f2, spec)
	if err != nil || started == nil || started.spec.Video != "540" {
		t.Fatalf("an idle picture gives up its slot: %v", err)
	}
	if len(f1.renditions) != 0 {
		t.Fatal("the idle picture is still running")
	}
}

func TestSharedFeedCountsOnePicture(t *testing.T) {
	h, m := tileHub(t, "apple", 3)
	defer lockHub(h)()
	first := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 7, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	same := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 8, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	if first != same {
		t.Fatal("one guide number is one feed")
	}
	held, err := h.ensureRenditionLocked(first, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"})
	if err != nil {
		t.Fatal(err)
	}
	held.viewers = 1
	other := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 9, GuideNumber: "5.1"}, FieldOrder: "progressive"})
	if _, err := h.ensureRenditionLocked(other, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}); err != nil {
		t.Fatalf("a shared feed is one picture, so the second channel fits: %v", err)
	}
}

func TestTileJoinWillNotReplaceSound(t *testing.T) {
	h, m := tileHub(t, "software", 0.4)
	defer lockHub(h)()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	silent, err := h.ensureRenditionLocked(f, Rendition{Video: "360", Audio: "none", Mode: "broadcast"})
	if err != nil {
		t.Fatal(err)
	}
	silent.viewers = 1
	_, err = h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"})
	var full *PictureError
	if !errors.As(err, &full) {
		t.Fatalf("a silent tile is not the main picture: %v", err)
	}
}

func TestTileJoinKeepsTheCodec(t *testing.T) {
	h, m := tileHub(t, "software", 0.4)
	h.HEVC = true
	defer lockHub(h)()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	hevc, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast", Codec: "hevc"})
	if err != nil {
		t.Fatal(err)
	}
	hevc.viewers = 1
	_, err = h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"})
	var full *PictureError
	if !errors.As(err, &full) {
		t.Fatalf("H.264 does not join HEVC: %v", err)
	}
}

// lockHub holds the hub lock until the test returns, and stops every rendition
// first so a killed encode is not started again in the temp directory.
func lockHub(h *Hub) func() {
	h.mu.Lock()
	return func() {
		for _, f := range h.channels {
			var keys []string
			for key, r := range f.renditions {
				r.restarted = true
				keys = append(keys, key)
			}
			for _, key := range keys {
				h.stopRenditionLocked(f, key)
			}
		}
		h.mu.Unlock()
	}
}

func tileHub(t *testing.T, class string, speed float64) (*Hub, *mux) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	h.Host = Budget(class, speed)
	h.Host.Encoder = "libx264"
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range h.channels {
			for _, r := range f.renditions {
				if r.cmd != nil && r.cmd.Process != nil {
					_ = r.cmd.Process.Kill()
				}
			}
		}
	})
	m := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	return h, m
}

func TestMeasureHostKeepsASpeedFromAFailedEncode(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'speed=2.5x' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := MeasureHost(context.Background(), script, "libx264")
	if got.Height != 1080 || got.Tiles != 2 || got.Speed != 2.5 {
		t.Fatalf("%+v", got)
	}
}

func TestMeasureHostTreatsATimedOutBenchAsSlow(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'speed=9.0x' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Already past the deadline, so the encode never starts.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	got := MeasureHost(ctx, script, "h264_vaapi")
	if got.Class != "vaapi" || got.Height != 540 || got.Tiles != 1 || got.Speed != 0 || got.FullRate {
		t.Fatalf("%+v", got)
	}
}
