package live

import (
	"bytes"
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
	for _, want := range []string{"-probesize 8000000", "-analyzeduration 1000000", "frag_keyframe", "delay_moov", "pipe:1", "-muxdelay 0", "-force_key_frames source", "-g 600", "-sc_threshold 0"} {
		if !strings.Contains(live, want) {
			t.Errorf("missing %q in %s", want, live)
		}
	}
	if strings.Contains(live, "hls_init_time") || strings.Contains(live, "hls_time") || strings.Contains(live, "prev_forced_t") || strings.Contains(live, "frag_duration") {
		t.Fatalf("a live transcode cuts on the source keyframes: %s", live)
	}
	remote := strings.Join(renditionArgs(0, Source{VideoCodec: "H264"}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "", "http://example/live.m3u8"), " ")
	if !strings.Contains(remote, "-analyzeduration 1500000") || !strings.Contains(remote, "frag_keyframe") || !strings.Contains(remote, "pipe:1") || strings.Contains(remote, "force_key_frames") || strings.Contains(remote, "hls_init_time") || strings.Contains(remote, "hls_time") || strings.Contains(remote, "frag_duration") {
		t.Fatalf("a remote copy keeps the longer probe and the source cut: %s", remote)
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
	for _, want := range []string{"-copyts", "-map 0:p:3:v:0", "-c:v copy", "-c:a copy", "frag_keyframe", "pipe:1"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "first_pts=0") || strings.Contains(line, "n_forced*2") {
		t.Errorf("copyts graphs must not rebase audio or force keyframes from zero: %s", line)
	}
	aac := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "1080", Audio: "aac2"}, "libx264", ""), " ")
	if !strings.Contains(aac, "aresample=async=1000:min_hard_comp=1000000") || strings.Contains(aac, "first_pts=0") {
		t.Fatalf("audio filter: %s", aac)
	}
}

// A multi-thousand-second timestamp step must not make the resampler
// allocate the missing audio. Hard compensation injects one sample per
// missing tick; 9000s of that is about two gigabytes.
//
// The step is applied in the filter graph. Two MPEG-TS clips run together
// are rewritten by the demuxer into a 33-bit wrap, and that wrap does not
// reach the resampler on every ffmpeg build. The ceiling is the same graph
// with a one-second step: a host that maps every hardware library already
// sits in the hundreds of megabytes, and an absolute floor below that fails
// when the resampler did nothing.
func TestAudioJumpDoesNotFillTheGap(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	small, _ := audioStepRSS(t, ffmpeg, "1")
	big, stderr := audioStepRSS(t, ffmpeg, "9000")
	if strings.Contains(stderr, "Failed to compensate") {
		t.Fatalf("resampler tried to fill the jump: %s", stderr)
	}
	if big > 1<<30 || (small > 0 && big > small+(128<<20) && big > small*2) {
		t.Fatalf("audio jump used %d bytes, short step %d: %s", big, small, stderr)
	}
}

func audioStepRSS(t *testing.T, ffmpeg, step string) (uint64, string) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	graph := "[1:a]asetpts=PTS+" + step + "/TB[b];[0:a][b]concat=n=2:v=0:a=1," + audioFilter(Rendition{Audio: "aac2"}) + "[a]"
	out := filepath.Join(dir, "out.m4a")
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "warning",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=0.4",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=0.4",
		"-filter_complex", graph,
		"-map", "[a]", "-c:a", "aac", "-t", "3", out)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("audio step %s did not finish: %s", step, stderr.String())
	}
	if err != nil {
		if _, statErr := os.Stat(out); statErr != nil {
			t.Fatalf("encode step %s: %v %s", step, err, stderr.String())
		}
	}
	return maxRSS(cmd), stderr.String()
}

func maxRSS(cmd *exec.Cmd) uint64 {
	if cmd.ProcessState == nil {
		return 0
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	rss := uint64(ru.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss
}

func TestTileRenditionIsSilentAndSmall(t *testing.T) {
	line := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "360", Audio: "none", Mode: "broadcast"}, "libx264", ""), " ")
	for _, want := range []string{"-copyts", "-an", "min(640,iw)", "min(360,ih)", "-force_key_frames source", "frag_keyframe", "pipe:1"} {
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
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		packErr := Pack(out, stdout, nil)
		waitErr := cmd.Wait()
		if packErr != nil || waitErr != nil {
			t.Fatalf("%s: pack %v wait %v %s", r.Key(), packErr, waitErr, stderr.String())
		}
		return out
	}
	copyDir := run(Rendition{Video: "copy", Audio: "copy"})
	smallDir := run(Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"})

	tl := NewTimeline()
	fixed := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	tl.now = func() time.Time { return fixed }
	type timedSeg struct {
		name string
		at   time.Time
		dur  time.Duration
	}
	times := func(d string) []timedSeg {
		raw, err := readPlaylist(filepath.Join(d, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		var p playlistStamper
		stamped := string(p.stamp(d, raw, tl))
		var out []timedSeg
		var last time.Time
		var dur time.Duration
		var haveDur bool
		for _, line := range strings.Split(stamped, "\n") {
			if v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); ok {
				last, _ = time.Parse("2006-01-02T15:04:05.000Z", v)
			} else if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
				v = strings.TrimSuffix(v, ",")
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					t.Fatal(err)
				}
				dur = time.Duration(f * float64(time.Second))
				haveDur = true
			} else if haveDur && strings.HasSuffix(line, ".m4s") {
				out = append(out, timedSeg{name: line, at: last, dur: dur})
				haveDur = false
			}
		}
		return out
	}
	a, b := times(copyDir), times(smallDir)
	if len(a) < 3 || len(b) < 3 {
		t.Fatalf("expected segments, got %d and %d", len(a), len(b))
	}
	for _, d := range []string{copyDir, smallDir} {
		raw, err := os.ReadFile(filepath.Join(d, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		assertSegmentCuts(t, string(raw))
	}
	// The same segment name is the same frame. A trailing partial from EOF
	// may belong to only one rendition.
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if d := len(a) - len(b); d > 1 || d < -1 {
		t.Fatalf("cuts differ: %d copy segments and %d transcode segments", len(a), len(b))
	}
	if len(a) != len(b) {
		n--
	}
	for i := 0; i < n; i++ {
		if a[i].name != b[i].name {
			t.Errorf("segment %d is %s on copy and %s on the transcode", i, a[i].name, b[i].name)
		}
		if d := a[i].at.Sub(b[i].at); d > 50*time.Millisecond || d < -50*time.Millisecond {
			t.Errorf("%s: renditions disagree by %v (%v vs %v)", a[i].name, d, a[i].at, b[i].at)
		}
		if d := a[i].dur - b[i].dur; d > 80*time.Millisecond || d < -80*time.Millisecond {
			t.Errorf("%s: duration %s vs %s", a[i].name, a[i].dur, b[i].dur)
		}
	}
	if first := a[0].at; !first.Equal(fixed.Add(-4 * time.Second)) {
		t.Errorf("the first served segment should anchor the timeline, got %v", first)
	}
	for _, playlist := range [][]timedSeg{a, b} {
		if playlist[0].name != "seg00000.m4s" {
			t.Errorf("segment 0 must be served, got %s", playlist[0].name)
		}
		for _, s := range playlist[:len(playlist)-1] {
			if s.dur > 1200*time.Millisecond {
				t.Errorf("%s is %s; this fixture's groups of pictures are half a second", s.name, s.dur)
			}
		}
	}
}

// A source group longer than the old two-second interval must still close
// copy and transcode on the same frame. -g used to insert the extra cut.
func TestLongGroupOfPicturesCutsTogether(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=30000/1001", "-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "7", "-c:v", "libx264", "-preset", "ultrafast", "-g", "90", "-sc_threshold", "0",
		"-c:a", "ac3", "-output_ts_offset", "95000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	source := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}
	run := func(r Rendition) []float64 {
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
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		packErr := Pack(out, stdout, nil)
		waitErr := cmd.Wait()
		if packErr != nil || waitErr != nil {
			t.Fatalf("%s: pack %v wait %v %s", r.Key(), packErr, waitErr, stderr.String())
		}
		raw, err := os.ReadFile(filepath.Join(out, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		var durs []float64
		for _, line := range strings.Split(string(raw), "\n") {
			v, ok := strings.CutPrefix(line, "#EXTINF:")
			if !ok {
				continue
			}
			f, err := strconv.ParseFloat(strings.TrimSuffix(v, ","), 64)
			if err != nil {
				t.Fatal(err)
			}
			durs = append(durs, f)
		}
		return durs
	}
	copyDurs := run(Rendition{Video: "copy", Audio: "copy"})
	transDurs := run(Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"})
	if len(copyDurs) < 2 || len(transDurs) < 2 {
		t.Fatalf("cuts: copy %v transcode %v", copyDurs, transDurs)
	}
	n := len(copyDurs)
	if len(transDurs) < n {
		n = len(transDurs)
	}
	if d := len(copyDurs) - len(transDurs); d > 1 || d < -1 {
		t.Fatalf("cuts differ: copy %v transcode %v", copyDurs, transDurs)
	}
	for i := 0; i < n-1; i++ {
		if copyDurs[i] < 2.5 {
			t.Errorf("copy segment %d is %.3fs; the source group is 3s", i, copyDurs[i])
		}
		if d := copyDurs[i] - transDurs[i]; d > 0.08 || d < -0.08 {
			t.Errorf("segment %d: copy %.3f transcode %.3f", i, copyDurs[i], transDurs[i])
		}
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

func TestFirstSegmentIsServed(t *testing.T) {
	src := "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2.0,\nseg00000.m4s\n#EXTINF:2.0,\nseg00001.m4s\n#EXT-X-PART:DURATION=0.500,URI=\"part00004.m4s\"\n"
	var p playlistStamper
	out := string(p.stamp(t.TempDir(), []byte(src), nil))
	if !strings.Contains(out, "seg00000.m4s") || !strings.Contains(out, "#EXT-X-PART:") {
		t.Fatalf("the first segment and the open part stay in the playlist:\n%s", out)
	}
	if !strings.Contains(out, "#EXT-X-MEDIA-SEQUENCE:0\n") || strings.Count(out, "#EXTINF") != 2 {
		t.Fatalf("the sequence is not bumped for segment 0:\n%s", out)
	}
	later := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:40\n#EXTINF:2.0,\nseg00040.m4s\n"
	if got := string(p.stamp(t.TempDir(), []byte(later), nil)); !strings.Contains(got, "#EXT-X-MEDIA-SEQUENCE:40\n") {
		t.Fatalf("a later sequence is untouched:\n%s", got)
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
