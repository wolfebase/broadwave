package live

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestPictureFacts(t *testing.T) {
	// 1280×720, 16:9, frame_rate_code 7 (59.94).
	seq := []byte{0x00, 0x00, 0x01, 0xB3, 0x50, 0x02, 0xD0, 0x37}
	got, ok := pictureFacts(programTS(1, streamMPEG2, 0x100, seq), 1)
	if !ok || got.Width != 1280 || got.Height != 720 || got.FPS != "59.94" {
		t.Fatalf("mpeg2 picture: %+v ok=%v", got, ok)
	}
	if _, ok := pictureFacts(programTS(1, streamMPEG2, 0x100, seq), 2); ok {
		t.Fatal("wrong program")
	}
}

func TestPictureFactsFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	path := t.TempDir() + "/s.ts"
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-t", "0.4", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "mpegts", path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	raw, err := exec.Command("cat", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := pictureFacts(raw, 1)
	if !ok || got.Width != 320 || got.Height != 240 {
		t.Fatalf("h264 picture: %+v ok=%v", got, ok)
	}

	// 1080 lines are stored as 1088 and cropped. The panel must show 1080.
	hd := t.TempDir() + "/hd.ts"
	args = []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30",
		"-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "mpegts", hd,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	raw, err = os.ReadFile(hd)
	if err != nil {
		t.Fatal(err)
	}
	got, ok = pictureFacts(raw, 1)
	if !ok || got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("cropped h264 picture: %+v ok=%v", got, ok)
	}
}

func TestFilmDoesNotStick(t *testing.T) {
	if storedFieldOrder("film") != "tt" {
		t.Fatal("film must be stored as interlaced")
	}
	if storedFieldOrder("progressive") != "progressive" || storedFieldOrder("bb") != "bb" {
		t.Fatal("a stable scan type is stored as itself")
	}
	src := sourceOf(store.SourceChannel{FieldOrder: "film"})
	if src.Film || src.Progressive || src.Lace {
		t.Fatalf("a stored film flag must not lock the channel: %+v", src)
	}
	prog := sourceOf(store.SourceChannel{FieldOrder: "progressive"})
	if !prog.Progressive || prog.Film || prog.Lace {
		t.Fatalf("progressive stays progressive: %+v", prog)
	}
	plain := sourceOf(store.SourceChannel{Channel: store.Channel{VideoCodec: "H264"}})
	if plain.Lace || plain.Progressive {
		t.Fatalf("unscanned h264 must keep its rate: %+v", plain)
	}
	laced := sourceOf(store.SourceChannel{Channel: store.Channel{VideoCodec: "H264"}, FieldOrder: "tt"})
	if !laced.Lace || laced.Progressive {
		t.Fatalf("stored tt laces h264: %+v", laced)
	}
}

func TestScanTypeCapturedHeaders(t *testing.T) {
	prog := mpeg2TS(1, true)
	if order, ok := scanType(prog, 1); !ok || order != "progressive" {
		t.Fatalf("progressive sequence: got %q %v", order, ok)
	}
	lace := mpeg2TS(1, false)
	if order, ok := scanType(lace, 1); !ok || order != "tt" {
		t.Fatalf("interlaced sequence: got %q %v", order, ok)
	}
	// progressive_frame=1 alone is not enough: film sets it in an interlaced sequence.
	film := programTS(1, streamMPEG2, 0x100, []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80})
	if _, ok := scanType(film, 1); ok {
		t.Fatal("progressive picture header must not decide the sequence")
	}
	if order, ok := scanType(h264TS(1, true), 1); !ok || order != "progressive" {
		t.Fatalf("h264 frames: got %q %v", order, ok)
	}
	if order, ok := scanType(filmTS(1), 1); !ok || order != "film" {
		t.Fatalf("soft 3:2: got %q %v", order, ok)
	}
	if order, ok := scanType(h264TS(1, false), 1); !ok || order != "tt" {
		t.Fatalf("h264 fields: got %q %v", order, ok)
	}
	if _, ok := scanType(prog, 99); ok {
		t.Fatal("wrong program should not match")
	}
}

func TestScanTypeFFmpegHeaders(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"mpeg2 progressive", []string{"-c:v", "mpeg2video", "-b:v", "2M"}, "progressive"},
		{"mpeg2 interlaced", []string{"-c:v", "mpeg2video", "-b:v", "2M", "-flags", "+ilme+ildct", "-top", "1"}, "tt"},
		{"h264 frames", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p"}, "progressive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/s.ts"
			args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-t", "0.4", "-f", "mpegts"}
			args = append(args, tc.args...)
			args = append(args, path)
			if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v %s", err, out)
			}
			raw, err := exec.Command("cat", path).Output()
			if err != nil {
				t.Fatal(err)
			}
			order, ok := scanType(raw, 1)
			if !ok || order != tc.want {
				t.Fatalf("got %q ok=%v, want %s", order, ok, tc.want)
			}
		})
	}
}

func TestStoredScanStartsBeforeTheHeader(t *testing.T) {
	h, m := testHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		renditions: map[string]*rendition{},
	}
	m.feeds["4.1"] = f
	h.channels[1] = f
	// Keep writing. One payload can pass through before the scan subscribes,
	// and then the window ends with an empty buffer.
	go writeUntil(ctx, pw, audioTS(1, []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}))
	started := time.Now()
	h.learnScanLocked(m, f)
	if waited := time.Since(started); waited > 400*time.Millisecond {
		t.Fatalf("stored scan waited %s for a header that was not in the buffer", waited)
	}
	if !f.source.Progressive || f.source.Film {
		t.Fatalf("graph %+v", f.source)
	}
	if len(f.tracks) != 1 || f.tracks[0].PID != 0x101 || f.tracks[0].Role != "main" {
		t.Fatalf("tracks %+v", f.tracks)
	}
}

func TestQuietTunerStillScansForFilm(t *testing.T) {
	h, m := testHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		tracks:     []AudioTrack{{PID: 0x101, Role: "main", Codec: "ac3"}},
		renditions: map[string]*rendition{},
	}
	m.feeds["5.1"] = f
	h.channels[1] = f
	started := time.Now()
	h.learnScanLocked(m, f)
	if waited := time.Since(started); waited < 500*time.Millisecond {
		t.Fatalf("empty buffer returned in %s", waited)
	}
	if f.headerOrder != "" || f.source.Film {
		t.Fatalf("decided with no bytes: %+v %q", f.source, f.headerOrder)
	}
	go writeUntil(ctx, pw, filmTS(1))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		film := f.source.Film
		h.mu.Unlock()
		if film {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !f.source.Film || f.source.Progressive || f.headerOrder != "film" {
		t.Fatalf("quiet tuner dropped the film header: %+v %q", f.source, f.headerOrder)
	}
}

func TestStoredProgressiveStillScansForFilm(t *testing.T) {
	h, m := testHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		tracks:     []AudioTrack{{PID: 0x101, Role: "main", Codec: "ac3"}},
		renditions: map[string]*rendition{},
	}
	m.feeds["5.1"] = f
	h.channels[1] = f
	go writeUntil(ctx, pw, filmTS(1))
	h.learnScanLocked(m, f)
	if !f.source.Film || f.source.Progressive {
		t.Fatalf("stored progressive skipped film: %+v", f.source)
	}
	if f.channel.FieldOrder != "tt" {
		t.Fatalf("film stored as %q", f.channel.FieldOrder)
	}
	if f.headerOrder != "film" {
		t.Fatalf("header %q", f.headerOrder)
	}
}

func TestLateHeaderAppliesAfterTheWindow(t *testing.T) {
	h, m := testHub(t)
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq},
		source:     Source{VideoCodec: "MPEG2"},
		program:    1,
		renditions: map[string]*rendition{},
	}
	m.feeds["4.1"] = f
	h.channels[1] = f
	buf := &scanBuf{wake: make(chan struct{}, 1), b: mpeg2TS(1, true)}
	h.finishScan(m, f, buf, nil)
	if !f.source.Progressive || f.source.Film || f.headerOrder != "progressive" {
		t.Fatalf("late progressive: %+v %q", f.source, f.headerOrder)
	}
	if f.channel.FieldOrder != "progressive" {
		t.Fatalf("stored %q", f.channel.FieldOrder)
	}

	film := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		renditions: map[string]*rendition{},
	}
	h.channels[2] = film
	buf = &scanBuf{wake: make(chan struct{}, 1), b: filmTS(1)}
	h.finishScan(m, film, buf, nil)
	if !film.source.Film || film.source.Progressive || film.channel.FieldOrder != "tt" || film.headerOrder != "film" {
		t.Fatalf("late film: %+v stored %q header %q", film.source, film.channel.FieldOrder, film.headerOrder)
	}
}

func TestProbeDoesNotOverrideTheHeader(t *testing.T) {
	h, _ := testHub(t)
	f := &feed{
		channel:     store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FieldOrder: "tt"},
		source:      Source{VideoCodec: "MPEG2", Film: true},
		headerOrder: "film",
		renditions:  map[string]*rendition{},
	}
	h.applyProbeLocked(f, "progressive")
	if !f.source.Film || f.source.Progressive || f.channel.FieldOrder != "tt" {
		t.Fatalf("probe overrode film: %+v stored %q", f.source, f.channel.FieldOrder)
	}
}

func TestProbeRebuildsTheRunningGraph(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	h, m := testHub(t)
	h.FFmpeg = "ffmpeg"
	h.Encoder = "libx264"
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq},
		source:     Source{VideoCodec: "MPEG2"},
		program:    1,
		renditions: map[string]*rendition{},
		timeline:   NewTimeline(),
	}
	m.feeds["4.1"] = f
	h.channels[1] = f
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.stopFeedLocked(f)
	})
	spec, ok := ParseRenditionKey("1080.aac2.broadcast")
	if !ok {
		t.Fatal("rendition key")
	}
	r, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.args, " "), "bwdif") {
		t.Fatalf("started without field deinterlace: %s", strings.Join(r.args, " "))
	}
	seen := time.Now().Add(-time.Second)
	r.viewers = 2
	r.seen = seen
	h.applyProbeLocked(f, "progressive")
	nr := f.renditions[spec.Key()]
	if nr == nil || nr == r {
		t.Fatal("probe did not rebuild the rendition")
	}
	if nr.viewers != 2 || !nr.seen.Equal(seen) {
		t.Fatalf("viewers %d", nr.viewers)
	}
	line := strings.Join(nr.args, " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "60000/1001") {
		t.Fatalf("rebuilt graph still field-deinterlaces: %s", line)
	}
	if !f.source.Progressive || f.channel.FieldOrder != "progressive" {
		t.Fatalf("stored %+v %q", f.source, f.channel.FieldOrder)
	}

	// A stored progressive rendition is rebuilt when the late header is film.
	r2, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	if r2 != nr {
		t.Fatal("same rendition should still be running")
	}
	buf := &scanBuf{wake: make(chan struct{}, 1), b: filmTS(1)}
	h.finishScan(m, f, buf, nil)
	filmR := f.renditions[spec.Key()]
	if filmR == nil || filmR == nr {
		t.Fatal("film header did not rebuild the rendition")
	}
	if filmR.viewers != 2 {
		t.Fatalf("viewers %d", filmR.viewers)
	}
	filmLine := strings.Join(filmR.args, " ")
	if !strings.Contains(filmLine, "pullup") || strings.Contains(filmLine, "bwdif") {
		t.Fatalf("film graph: %s", filmLine)
	}
	if !f.source.Film || f.source.Progressive || f.channel.FieldOrder != "tt" {
		t.Fatalf("film stored %+v %q", f.source, f.channel.FieldOrder)
	}
}

func TestUnscannedH264ProbeRebuilds(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	h, m := testHub(t)
	h.FFmpeg = "ffmpeg"
	h.Encoder = "libx264"
	ch := store.SourceChannel{Channel: store.Channel{ID: 14, GuideNumber: "14.1", VideoCodec: "H264"}, FrequencyHz: m.freq}
	f := &feed{
		channel:    ch,
		source:     sourceOf(ch),
		program:    1,
		renditions: map[string]*rendition{},
		timeline:   NewTimeline(),
	}
	m.feeds["14.1"] = f
	h.channels[14] = f
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.stopFeedLocked(f)
	})
	spec, ok := ParseRenditionKey("1080.aac2.broadcast")
	if !ok {
		t.Fatal("rendition key")
	}
	r, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	started := strings.Join(r.args, " ")
	if strings.Contains(started, "bwdif") || strings.Contains(started, "60000/1001") || strings.Contains(started, "deinterlace_vaapi") {
		t.Fatalf("unscanned h264 started doubled: %s", started)
	}
	seen := time.Now().Add(-time.Second)
	r.viewers = 2
	r.seen = seen

	h.applyProbeLocked(f, "progressive")
	kept := f.renditions[spec.Key()]
	if kept == nil {
		t.Fatal("progressive probe dropped the rendition")
	}
	line := strings.Join(kept.args, " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "60000/1001") {
		t.Fatalf("progressive probe doubled the graph: %s", line)
	}
	if !f.source.Progressive || f.source.Lace || f.channel.FieldOrder != "progressive" {
		t.Fatalf("stored %+v %q", f.source, f.channel.FieldOrder)
	}
	if kept.viewers != 2 {
		t.Fatalf("viewers %d", kept.viewers)
	}

	h.applyProbeLocked(f, "tt")
	nr := f.renditions[spec.Key()]
	if nr == nil || nr == kept {
		t.Fatal("interlaced probe did not rebuild the rendition")
	}
	if nr.viewers != 2 || !nr.seen.Equal(seen) {
		t.Fatalf("viewers %d", nr.viewers)
	}
	laced := strings.Join(nr.args, " ")
	if !strings.Contains(laced, "bwdif=mode=send_field") || !strings.Contains(laced, "fps=60000/1001") {
		t.Fatalf("known interlaced h264: %s", laced)
	}
	if !f.source.Lace || f.source.Progressive || f.channel.FieldOrder != "tt" {
		t.Fatalf("lace stored %+v %q", f.source, f.channel.FieldOrder)
	}
}

func TestHLSSegmentProbeContextSurvivesPlaylistCancel(t *testing.T) {
	playlist, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	cancel()
	if playlist.Err() == nil {
		t.Fatal("playlist context was not cancelled")
	}
	child, stopChild := context.WithTimeout(playlist, 8*time.Second)
	defer stopChild()
	if child.Err() == nil {
		t.Fatal("a child of the cancelled playlist context should be done")
	}

	seg, stop := hlsSegmentProbeContext()
	defer stop()
	select {
	case <-seg.Done():
		t.Fatal("cancelled playlist context cancelled the segment probe")
	default:
	}
	deadline, ok := seg.Deadline()
	if !ok {
		t.Fatal("segment probe context has no deadline")
	}
	remain := time.Until(deadline)
	if remain < 7*time.Second || remain > 8*time.Second+time.Second {
		t.Fatalf("segment probe budget %s, want 8s", remain)
	}
}

func TestHLSProbeTargetFindsTheSegment(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "live.m3u8")
	master := filepath.Join(dir, "master.m3u8")
	if err := os.WriteFile(media, []byte("#EXTM3U\n#EXTINF:1.0,\nseg.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(master, []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\nlive.m3u8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := hlsProbeTarget(master, "", "")
	want := filepath.Join(dir, "seg.ts")
	if got != want {
		t.Fatalf("master: got %q want %q", got, want)
	}
	fmp4 := filepath.Join(dir, "fmp4.m3u8")
	body := "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1.0,\nseg.m4s\n"
	if err := os.WriteFile(fmp4, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got = hlsProbeTarget(fmp4, "", "")
	want = "concat:" + filepath.Join(dir, "init.mp4") + "|" + filepath.Join(dir, "seg.m4s")
	if got != want {
		t.Fatalf("fmp4: got %q want %q", got, want)
	}
	if resolveMedia("http://example/a/live.m3u8", "seg.ts") != "http://example/a/seg.ts" {
		t.Fatal("remote segment was not resolved")
	}
}

func TestInputProbeStoresProgressive(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH")
	}
	dir := t.TempDir()
	seg := filepath.Join(dir, "seg.ts")
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "nullsrc=s=320x180:r=60:d=1,format=yuv420p",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "30", "-f", "mpegts", seg)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	playlist := filepath.Join(dir, "live.m3u8")
	body := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:1.0,\nseg.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(playlist, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h, m := testHub(t)
	h.FFmpeg = "ffmpeg"
	m.input = playlist
	ch := store.SourceChannel{Channel: store.Channel{ID: 30, GuideNumber: "30.1", VideoCodec: "H264"}}
	f := &feed{
		channel:    ch,
		source:     sourceOf(ch),
		renditions: map[string]*rendition{},
	}
	m.feeds["30.1"] = f
	h.channels[30] = f
	h.probeInputLocked(m, f)
	deadline := time.Now().Add(8 * time.Second)
	var order string
	for time.Now().Before(deadline) {
		h.mu.Lock()
		order = f.channel.FieldOrder
		h.mu.Unlock()
		if order != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if order != "progressive" || !f.source.Progressive || f.source.Lace {
		t.Fatalf("playlist probe %+v stored %q", f.source, order)
	}
	line := strings.Join(RenditionArgs(0, f.source, Rendition{Video: "1080", Audio: "aac2"}, "libx264", "motion_adaptive"), " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "60000/1001") || strings.Contains(line, "fps=") {
		t.Fatalf("60p playlist doubled: %s", line)
	}
}

func writeUntil(ctx context.Context, w io.Writer, payload []byte) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := w.Write(payload); err != nil {
				return
			}
		}
	}
}

func mpeg2TS(program int, progressive bool) []byte {
	ext := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	if progressive {
		ext[5] = 0x08
	} else {
		// picture_coding_extension, progressive_frame clear (MSB of the 5th payload byte).
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x00}
		ext = append(ext, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, ext)
}

func filmTS(program int) []byte {
	// Interlaced sequence, then four progressive pictures with repeat_first_field on two of them.
	es := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	for i := 0; i < 4; i++ {
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80}
		if i%2 == 0 {
			pic[7] = 0x02
		}
		es = append(es, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, es)
}

func h264TS(program int, frames bool) []byte {
	// Main profile SPS: frame_mbs_only_flag is the last bit written below.
	sps := []byte{0x4d, 0x00, 0x28, 0xfb, 0x00}
	if frames {
		sps[4] = 0x80
	}
	nal := append([]byte{0x00, 0x00, 0x01, 0x67}, sps...)
	return programTS(program, streamH264, 0x100, nal)
}

func programTS(program, streamType, videoPID int, es []byte) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{
		0x00, 0x01, 0xc1, 0x00, 0x00,
	}, progPID(program, pmtPID)...))
	pmt := psiSection(0x02, pmtBody(program, streamType, videoPID))
	pes := pesPacket(es)
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	out = append(out, tsPacket(pmtPID, true, pmt)...)
	out = append(out, tsPacket(videoPID, true, pes)...)
	return out
}

func progPID(program, pid int) []byte {
	return []byte{
		byte(program >> 8), byte(program),
		0xe0 | byte(pid>>8), byte(pid),
	}
}

func pmtBody(program, streamType, videoPID int) []byte {
	body := []byte{
		byte(program >> 8), byte(program),
		0xc1, 0x00, 0x00,
		0xe0, 0x00, // PCR
		0xf0, 0x00, // program info length
		byte(streamType),
		0xe0 | byte(videoPID>>8), byte(videoPID),
		0xf0, 0x00,
	}
	return body
}

func psiSection(tableID byte, body []byte) []byte {
	// length covers body + CRC.
	n := len(body) + 4
	sec := []byte{tableID, 0xb0 | byte(n>>8), byte(n)}
	sec = append(sec, body...)
	sec = append(sec, 0, 0, 0, 0)
	pkt := []byte{0x00} // pointer
	return append(pkt, sec...)
}

func pesPacket(es []byte) []byte {
	pes := []byte{0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x00, 0x00}
	return append(pes, es...)
}

func tsPacket(pid int, start bool, payload []byte) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid >> 8)
	if start {
		pkt[1] |= 0x40
	}
	pkt[2] = byte(pid)
	pkt[3] = 0x10
	if len(payload) > 184 {
		payload = payload[:184]
	}
	copy(pkt[4:], payload)
	return pkt
}
