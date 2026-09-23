package live

import (
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
		{"apple tv gets the original h264 with dolby", Source{"H264", "AC3", true}, apple, Prefs{}, "copy.copy"},
		{"mpeg-2 is converted but keeps dolby on apple tv", Source{"MPEG2", "AC3", false}, apple, Prefs{}, "1080.copy.broadcast"},
		{"browser keeps h264 picture, converts sound", Source{"H264", "AC3", true}, web, Prefs{}, "copy.aac2"},
		{"unprobed h264 is deinterlaced, not copied", Source{"H264", "AC3", false}, web, Prefs{}, "1080.aac2.broadcast"},
		{"cellular drops to 720", Source{"MPEG2", "AC3", false}, Caps{Platform: "ios", Video: []string{"h264"}, Audio: []string{"ac3", "aac"}, Network: "cellular"}, Prefs{}, "720.copy.broadcast"},
		{"saver is small and stereo", Source{"MPEG2", "AC3", false}, apple, Prefs{Quality: "saver"}, "540.aac2.broadcast"},
		{"a tile is 540 and silent", Source{"MPEG2", "AC3", false}, apple, Prefs{Quality: "tile"}, "540.none.broadcast"},
		{"the smaller tile is 360", Source{"MPEG2", "AC3", false}, apple, Prefs{Quality: "360", Audio: "none"}, "360.none.broadcast"},
		{"a tile can keep sound when asked", Source{"MPEG2", "AC3", false}, apple, Prefs{Quality: "tile", Audio: "stereo"}, "540.aac2.broadcast"},
		{"surround without dolby decode is 5.1 aac", Source{"MPEG2", "AC3", false}, web, Prefs{Audio: "surround"}, "1080.aac6.broadcast"},
		{"film mode is part of the key", Source{"MPEG2", "AC3", false}, web, Prefs{Picture: "film"}, "1080.aac2.film"},
		{"small screens cap the height", Source{"MPEG2", "AC3", false}, Caps{Video: []string{"h264"}, Audio: []string{"aac"}, MaxHeight: 720}, Prefs{}, "720.aac2.broadcast"},
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
	for _, key := range []string{"copy.copy", "copy.aac2", "1080.copy.broadcast", "540.aac6.film", "540.none.broadcast", "360.none.broadcast"} {
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

func TestCopyRenditionKeepsBroadcastTimestamps(t *testing.T) {
	line := strings.Join(RenditionArgs(3, Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "", false), " ")
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
	line := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "360", Audio: "none", Mode: "broadcast"}, "libx264", "", false), " ")
	for _, want := range []string{"-copyts", "-an", "min(640,iw)", "min(360,ih)", "prev_forced_t+2", "-hls_segment_type fmp4"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "-c:a") || strings.Contains(line, "0:a:0") {
		t.Errorf("a silent tile must not encode audio: %s", line)
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
		cmd := exec.Command(ffmpeg, RenditionArgs(0, source, r, "libx264", "", false)...)
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
