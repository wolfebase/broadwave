package live

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	apple := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}}
	web := Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	cases := []struct {
		name string
		src  Source
		caps Caps
		p    Prefs
		want string
	}{
		{"apple tv gets the original h264 with dolby", Source{"H264", "AC3", true, false, "", "", 0, false}, apple, Prefs{}, "copy.copy"},
		{"mpeg-2 is converted but keeps dolby on apple tv", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{}, "1080.copy.broadcast.hevc"},
		{"browser keeps h264 picture, converts sound", Source{"H264", "AC3", true, false, "", "", 0, false}, web, Prefs{}, "copy.aac2"},
		{"unprobed h264 is converted, not copied or doubled", Source{"H264", "AC3", false, false, "", "", 0, false}, web, Prefs{}, "1080.aac2.broadcast"},
		{"cellular drops to 720", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, Caps{Platform: "ios", Video: []string{"h264"}, Audio: []string{"ac3", "aac"}, Network: "cellular"}, Prefs{}, "720.copy.broadcast"},
		{"saver is small and stereo", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Quality: "saver"}, "540.aac2.broadcast.hevc"},
		{"a tile is 540 and silent", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Quality: "tile"}, "540.none.broadcast.hevc"},
		{"the smaller tile is 360", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Quality: "360", Audio: "none"}, "360.none.broadcast.hevc"},
		{"a tile can keep sound when asked", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Quality: "tile", Audio: "stereo"}, "540.aac2.broadcast.hevc"},
		{"surround without dolby decode is 5.1 aac", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, web, Prefs{Audio: "surround"}, "1080.aac6.broadcast"},
		{"film mode is part of the key", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, web, Prefs{Picture: "film"}, "1080.aac2.film"},
		{"small screens cap the height", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, Caps{Video: []string{"h264"}, Audio: []string{"aac"}, MaxHeight: 720}, Prefs{}, "720.aac2.broadcast"},
		{"second language is its own rendition", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Track: "language"}, "1080.copy.broadcast.lang.hevc"},
		{"even volume re-encodes passthrough", Source{"MPEG2", "AC3", false, false, "", "", 0, false}, apple, Prefs{Even: true}, "1080.aac6.broadcast.even.hevc"},
		{"described video on the web", Source{"H264", "AC3", true, false, "", "", 0, false}, web, Prefs{Track: "described"}, "copy.aac2.vi"},
	}
	for _, c := range cases {
		got := Decide(c.src, c.caps, c.p)
		if got.Rendition.Key() != c.want {
			t.Errorf("%s: got %s (%s), want %s", c.name, got.Rendition.Key(), got.Reason, c.want)
		}
		if got.Reason == "" {
			t.Errorf("%s: decision needs a reason", c.name)
		}
	}
}

func TestRenditionKeyRoundTrip(t *testing.T) {
	for _, key := range []string{"copy.copy", "copy.aac2", "1080.copy.broadcast", "1080.copy.broadcast.hevc", "540.aac6.film", "540.none.broadcast", "360.none.broadcast", "1080.aac2.broadcast.lang.even.hevc", "copy.aac2.vi", "540.aac2.broadcast.60", "360.none.broadcast.60.hevc"} {
		r, ok := ParseRenditionKey(key)
		if !ok || r.Key() != key {
			t.Errorf("%s did not round trip: %+v %v", key, r, ok)
		}
	}
	for _, bad := range []string{"", "copy", "../x.copy", "1080.aac2", "copy.copy.film", "999.copy.broadcast"} {
		if _, ok := ParseRenditionKey(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestFocusedTileIs60(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	apple := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}}
	gpu := DecideOn(src, apple, Prefs{Quality: "focus", Audio: "stereo"}, "h264_vaapi")
	if gpu.Rendition.Key() != "720.aac2.broadcast.hevc" || !strings.Contains(gpu.Reason, "720p60 tile") {
		t.Fatalf("gpu focus: %s %s", gpu.Rendition.Key(), gpu.Reason)
	}
	soft := DecideOn(src, apple, Prefs{Quality: "focus", Audio: "stereo"}, "libx264")
	if soft.Rendition.Key() != "540.aac2.broadcast.60.hevc" || !soft.Rendition.FullRate || !strings.Contains(soft.Reason, "540p60 tile") {
		t.Fatalf("software focus: %+v %s", soft.Rendition, soft.Reason)
	}
	back := DecideOn(src, apple, Prefs{Quality: "tile"}, "h264_vaapi")
	if back.Rendition.Key() != "540.none.broadcast.hevc" || back.Rendition.FullRate {
		t.Fatalf("background tile: %+v", back.Rendition)
	}
	heard := DecideOn(src, apple, Prefs{Quality: "focus"}, "h264_vaapi")
	if heard.Rendition.Audio != "copy" {
		t.Fatalf("a focused tile keeps sound: %s", heard.Rendition.Audio)
	}
	capped := DecideOn(src, Caps{Video: []string{"h264"}, Audio: []string{"aac"}, MaxHeight: 400}, Prefs{Quality: "focus", Audio: "none"}, "h264_vaapi")
	if capped.Rendition.Key() != "360.none.broadcast.60" {
		t.Fatalf("a tiny screen still gets 60: %s", capped.Rendition.Key())
	}
}

func TestHLSInputReconnects(t *testing.T) {
	line := strings.Join(renditionArgs(0, Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true, UserAgent: "Broadwave", Referrer: "http://example/"}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "", "http://example/live.m3u8"), " ")
	if !strings.Contains(line, "-reconnect 1") || !strings.Contains(line, "-i http://example/live.m3u8") || !strings.Contains(line, "aac_adtstoasc") || !strings.Contains(line, "User-Agent: Broadwave") {
		t.Fatal(line)
	}
}

func TestOpeningSegmentsAreShort(t *testing.T) {
	live := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "720", Audio: "aac2"}, "libx264", ""), " ")
	for _, want := range []string{"-probesize 8000000", "-analyzeduration 1000000", "-hls_time 2", "-muxdelay 0", openingKeyframes} {
		if !strings.Contains(live, want) {
			t.Errorf("missing %q in %s", want, live)
		}
	}
	if strings.Contains(live, "hls_init_time") || strings.Contains(live, "hls_time 0.5") {
		t.Fatalf("a live transcode stays on the two-second cut: %s", live)
	}
	remote := strings.Join(renditionArgs(0, Source{VideoCodec: "H264"}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "", "http://example/live.m3u8"), " ")
	if !strings.Contains(remote, "-analyzeduration 1500000") || !strings.Contains(remote, "-hls_time 2") || strings.Contains(remote, "force_key_frames") || strings.Contains(remote, "hls_init_time") {
		t.Fatalf("a remote copy keeps the longer probe and the two-second cut: %s", remote)
	}
}

func TestTimelineEarliestIsTheFirstAnchor(t *testing.T) {
	fixed := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	if _, ok := tl.Earliest(); ok {
		t.Fatal("an unset timeline has no first frame")
	}
	tl.Wall(90000 * 10)
	got, ok := tl.Earliest()
	if !ok || !got.Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("first frame: %v %v", got, ok)
	}
	tl.Wall(90000 * 12)
	got, _ = tl.Earliest()
	if !got.Equal(fixed.Add(-4 * time.Second)) {
		t.Fatalf("a later segment moved the first frame to %v", got)
	}
}

func TestEarliestMediaUsesTheNewestRendition(t *testing.T) {
	fixed := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	older := NewTimeline()
	older.now = func() time.Time { return fixed }
	older.Wall(1)
	newer := NewTimeline()
	newer.now = func() time.Time { return fixed.Add(time.Minute) }
	newer.Wall(1)
	h := &Hub{channels: map[int64]*feed{
		4: {renditions: map[string]*rendition{
			"a": {clock: older},
			"b": {clock: newer},
		}},
	}}
	ms, ok := h.EarliestMedia(4)
	if !ok {
		t.Fatal("expected a first frame")
	}
	want := float64(fixed.Add(time.Minute).Add(-4*time.Second).UnixNano()) / 1e6
	if math.Abs(ms-want) > 0.5 {
		t.Fatalf("newest first frame: got %v want %v", ms, want)
	}
	if _, ok := h.EarliestMedia(9); ok {
		t.Fatal("a quiet channel has no first frame")
	}
}

func TestCopyRenditionKeepsBroadcastTimestamps(t *testing.T) {
	line := strings.Join(RenditionArgs(3, Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}, Rendition{Video: "copy", Audio: "copy"}, "libx264", ""), " ")
	for _, want := range []string{"-copyts", "-map 0:p:3:v:0", "-c:v copy", "-c:a copy", "-hls_list_size 2700"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "first_pts=0") || strings.Contains(line, "n_forced*2") {
		t.Errorf("copyts graphs must not rebase audio or force keyframes from zero: %s", line)
	}
}

func TestTileRenditionIsSilentAndSmall(t *testing.T) {
	line := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "360", Audio: "none", Mode: "broadcast"}, "libx264", ""), " ")
	for _, want := range []string{"-copyts", "-an", "min(640,iw)", "min(360,ih)", "prev_forced_t+2", "-hls_time 2", "-hls_segment_type fmp4"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "-c:a") || strings.Contains(line, "0:a:0") {
		t.Errorf("a silent tile must not encode audio: %s", line)
	}
}

func TestRenditionMapsChosenPID(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3", AudioPID: 0x102}
	line := strings.Join(RenditionArgs(3, src, Rendition{Video: "1080", Audio: "copy"}, "libx264", ""), " ")
	if !strings.Contains(line, "-map 0:i:258") || strings.Contains(line, "a:0") {
		t.Fatalf("sap pid: %s", line)
	}
	even := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2", Even: true, Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(even, "loudnorm=I=-16:LRA=11:TP=-1.5") {
		t.Fatalf("even volume: %s", even)
	}
}

// Two renditions of one broadcast, stamped by one Timeline, must agree on wall time.
func TestRenditionsShareOneTimeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=30000/1001", "-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "9", "-c:v", "libx264", "-g", "15", "-c:a", "ac3", "-output_ts_offset", "95000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	source := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}
	run := func(r Rendition) string {
		out := filepath.Join(dir, r.Key())
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		cmd := exec.Command(ffmpeg, RenditionArgs(0, source, r, "libx264", "")...)
		cmd.Dir = out
		cmd.Stdin = in
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", r.Key(), err, b)
		}
		return out
	}
	copyDir := run(Rendition{Video: "copy", Audio: "copy"})
	smallDir := run(Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"})

	tl := NewTimeline()
	fixed := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	tl.now = func() time.Time { return fixed }
	times := func(d string) map[string]time.Time {
		raw, err := readPlaylist(filepath.Join(d, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		var p playlistStamper
		stamped := string(p.stamp(d, raw, tl))
		out := map[string]time.Time{}
		var last time.Time
		for _, line := range strings.Split(stamped, "\n") {
			if v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); ok {
				last, _ = time.Parse("2006-01-02T15:04:05.000Z", v)
			} else if strings.HasSuffix(line, ".m4s") {
				out[line] = last
			}
		}
		return out
	}
	a, b := times(copyDir), times(smallDir)
	if len(a) < 3 || len(b) < 3 {
		t.Fatalf("expected segments, got %d and %d", len(a), len(b))
	}
	for name, ta := range a {
		tb, ok := b[name]
		if !ok {
			continue
		}
		if d := ta.Sub(tb); d > 50*time.Millisecond || d < -50*time.Millisecond {
			t.Errorf("%s: renditions disagree by %v (%v vs %v)", name, d, ta, tb)
		}
	}
	if first := a["seg00001.m4s"]; !first.Equal(fixed.Add(-4 * time.Second)) {
		t.Errorf("the first served segment should anchor the timeline, got %v", first)
	}
	if _, served := a["seg00000.m4s"]; served {
		t.Error("segment 0 must not be served")
	}
}

func TestSeparateRenditionClocksAnchorToNow(t *testing.T) {
	fixed := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	longRunning := NewTimeline()
	justStarted := NewTimeline()
	longRunning.now = func() time.Time { return fixed }
	justStarted.now = func() time.Time { return fixed }
	// fMP4 timestamps start at zero for each encode, so the newer one is not
	// 170s behind the one that has been running.
	if got := longRunning.Wall(90000 * 200); !got.Equal(fixed.Add(-4 * time.Second)) {
		t.Fatal(got)
	}
	if got := justStarted.Wall(90000 * 30); !got.Equal(fixed.Add(-4 * time.Second)) {
		t.Fatal(got)
	}
	shared := NewTimeline()
	shared.now = func() time.Time { return fixed }
	shared.Wall(90000 * 200)
	if got := shared.Wall(90000 * 30); !got.Equal(fixed.Add(-4*time.Second - 170*time.Second)) {
		t.Fatal(got)
	}
}

func TestPTSDiffWraps(t *testing.T) {
	if d := ptsDiff(5, ptsWrap-5); d != 10 {
		t.Fatalf("wrap forward: %d", d)
	}
	if d := ptsDiff(ptsWrap-5, 5); d != -10 {
		t.Fatalf("wrap back: %d", d)
	}
}

func TestFirstSegmentIsWithheld(t *testing.T) {
	src := "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2.0,\nseg00000.ts\n#EXTINF:2.0,\nseg00001.ts\n#EXTINF:2.0,\nseg00002.ts\n"
	var p playlistStamper
	out := string(p.stamp(t.TempDir(), []byte(src), nil))
	if strings.Contains(out, "seg00000.ts") {
		t.Fatalf("first segment must not be served:\n%s", out)
	}
	if !strings.Contains(out, "#EXT-X-MEDIA-SEQUENCE:1\n") || strings.Count(out, "#EXTINF") != 2 {
		t.Fatalf("sequence and entries should follow the withheld segment:\n%s", out)
	}
	later := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:40\n#EXTINF:2.0,\nseg00040.ts\n"
	if got := string(p.stamp(t.TempDir(), []byte(later), nil)); !strings.Contains(got, "#EXT-X-MEDIA-SEQUENCE:40\n") {
		t.Fatalf("once segment 0 has rolled off, the sequence is untouched:\n%s", got)
	}
}

func TestCopyArgsReadsURL(t *testing.T) {
	line := strings.Join(copyArgs(0, "http://example/live.m3u8", "Broadwave", "http://example/", "out.ts"), " ")
	if strings.Contains(line, "pipe:0") || !strings.Contains(line, "-reconnect 1") || !strings.Contains(line, "-i http://example/live.m3u8") || !strings.Contains(line, "User-Agent: Broadwave") {
		t.Fatalf("hls recording args: %s", line)
	}
	if pipe := strings.Join(copyArgs(0, "", "", "", "out.ts"), " "); !strings.Contains(pipe, "-i pipe:0") || strings.Contains(pipe, "-headers") {
		t.Fatalf("tuner recording args: %s", pipe)
	}
}
