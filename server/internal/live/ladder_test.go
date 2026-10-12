package live

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestParseLadderKey(t *testing.T) {
	r, ok := ParseRenditionKey("1080.aac2.broadcast.abr")
	if !ok || !r.ABR || r.Key() != "1080.aac2.broadcast.abr" {
		t.Fatalf("1080 ladder: ok=%v %+v", ok, r)
	}
	short, ok := ParseRenditionKey("480.aac2.film.abr")
	if !ok || short.Video != "480" || short.Mode != "film" || short.Key() != "480.aac2.film.abr" {
		t.Fatalf("480 ladder: ok=%v %+v", ok, short)
	}
	old, ok := ParseRenditionKey("720.aac2.broadcast")
	if !ok || old.ABR || old.Key() != "720.aac2.broadcast" {
		t.Fatalf("old key: ok=%v %+v", ok, old)
	}
	if _, ok := ParseRenditionKey("abr"); ok {
		t.Fatal("abr alone parsed")
	}
	if _, ok := ParseRenditionKey("1080.none.broadcast.abr"); ok {
		t.Fatal("a silent ladder key parsed")
	}
	if _, ok := ParseRenditionKey("480.aac2.broadcast"); ok {
		t.Fatal("480p outside a ladder parsed")
	}
}

func TestApplyLadder(t *testing.T) {
	base := Decision{Rendition: Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, Reason: "original"}
	off := ApplyLadder(false, Caps{Platform: "web"}, Prefs{Quality: "auto"}, Host{}, base)
	if off.Rendition.ABR || off.Rendition.Key() != base.Rendition.normalized().Key() || off.Reason != "original" {
		t.Fatalf("off: %+v", off)
	}
	on := ApplyLadder(true, Caps{Platform: "web"}, Prefs{Quality: "auto"}, Host{}, base)
	if on.Rendition.Key() != "1080.aac2.broadcast.abr" || on.Reason != "Adaptive: 1080p, 720p, and 480p, and audio only" {
		t.Fatalf("web: %+v", on)
	}
	if tile := ApplyLadder(true, Caps{Platform: "web"}, Prefs{Quality: "tile"}, Host{}, base); tile.Rendition.ABR {
		t.Fatal("a tile joined the ladder")
	}
	if ios := ApplyLadder(true, Caps{Platform: "ios"}, Prefs{}, Host{}, base); !ios.Rendition.ABR {
		t.Fatal("iOS stayed on one picture")
	}
	if unknown := ApplyLadder(true, Caps{Platform: "roku"}, Prefs{}, Host{}, base); unknown.Rendition.ABR {
		t.Fatal("an unknown platform joined the ladder")
	}
	if empty := ApplyLadder(true, Caps{}, Prefs{}, Host{}, base); empty.Rendition.ABR {
		t.Fatal("an empty platform joined the ladder")
	}
	silent := base
	silent.Rendition.Audio = "none"
	if got := ApplyLadder(true, Caps{Platform: "web"}, Prefs{}, Host{}, silent); got.Rendition.ABR {
		t.Fatal("silent audio joined the ladder")
	}
	capped := ApplyLadder(true, Caps{Platform: "web"}, Prefs{}, Host{Height: 720}, base)
	if capped.Rendition.Key() != "720.aac2.broadcast.abr" || capped.Reason != "Adaptive: 720p and 480p, and audio only" {
		t.Fatalf("capped: %+v", capped)
	}
	if videos := LadderVideos(Host{Height: 480}); len(videos) != 1 || videos[0] != "480" {
		t.Fatalf("short host: %v", videos)
	}
}

func TestLadderMaster(t *testing.T) {
	rungs := []Rendition{
		{Video: "1080", Audio: "aac2", Mode: "broadcast", ABR: true},
		{Video: "720", Audio: "aac2", Mode: "broadcast", ABR: true},
		{Video: "480", Audio: "aac2", Mode: "broadcast", ABR: true},
	}
	variants := []ladderVariant{
		{spec: rungs[0], uri: "index.m3u8"},
		{spec: rungs[1], uri: "../" + rungs[1].Key() + "/index.m3u8"},
		{spec: rungs[2], uri: "../" + rungs[2].Key() + "/index.m3u8"},
		{audioOnly: true},
	}
	body := string(ladderMaster(variants, "audio-2.m3u8", "", "", false))
	if !strings.Contains(body, "#EXT-X-VERSION:6") || !strings.Contains(body, "#EXT-X-INDEPENDENT-SEGMENTS") {
		t.Fatalf("header:\n%s", body)
	}
	streams, uris := streamLines(body)
	if len(streams) != 4 {
		t.Fatalf("streams %d\n%s", len(streams), body)
	}
	for i, video := range []string{"1080", "720", "480"} {
		line := streams[i]
		for _, want := range []string{"BANDWIDTH=", "AVERAGE-BANDWIDTH=", "CODECS=", "RESOLUTION="} {
			if !strings.Contains(line, want) {
				t.Fatalf("%s missing %s: %s", video, want, line)
			}
		}
		rate := renditionBandwidth(rungs[i]) + ladderAudioBits
		avg := renditionRate(rungs[i]) + ladderAudioBits
		if !strings.Contains(line, "BANDWIDTH="+strconv.Itoa(rate)) || !strings.Contains(line, "AVERAGE-BANDWIDTH="+strconv.Itoa(avg)) {
			t.Fatalf("%s rates: %s", video, line)
		}
		if strings.Contains(line, "SUBTITLES=") {
			t.Fatalf("captions were not asked for: %s", line)
		}
	}
	if uris[0] != "index.m3u8" || uris[3] != "audio-2.m3u8" {
		t.Fatalf("uris %v", uris)
	}
	audio := streams[3]
	if strings.Contains(audio, "RESOLUTION=") || strings.Contains(audio, "CLOSED-CAPTIONS=") || !strings.Contains(audio, `CODECS="mp4a.40.2"`) {
		t.Fatalf("audio only: %s", audio)
	}
	if !strings.Contains(audio, "BANDWIDTH=160000") || !strings.Contains(audio, "AVERAGE-BANDWIDTH=160000") {
		t.Fatalf("audio rate: %s", audio)
	}
	with := string(ladderMaster(variants, "audio-2.m3u8", "mp4a.40.2", "59.94", true))
	if !strings.Contains(with, captionMedia) {
		t.Fatal("captions group missing")
	}
	capStreams, _ := streamLines(with)
	if !strings.Contains(capStreams[0], `SUBTITLES="cc"`) || strings.Contains(capStreams[3], "SUBTITLES=") {
		t.Fatalf("captions on the wrong lines:\n%s", with)
	}
}

// A player on the short rung asks for ladder.m3u8 in that rung's directory.
// The other variants have to point up at the primary, with the field-rate
// peaks, and the audio-only line is the primary's sound view.
func TestLadderPlaylistFromTheShortRung(t *testing.T) {
	specs := []Rendition{
		ladderRung("broadcast", "1080"),
		ladderRung("broadcast", "720"),
		ladderRung("broadcast", "480"),
	}
	root := t.TempDir()
	dirs := make([]string, len(specs))
	for i, spec := range specs {
		dirs[i] = filepath.Join(root, spec.Key())
		if err := os.MkdirAll(dirs[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirs[0], "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := ladderPlaylistFromDisk(specs[2], specs, dirs, Source{VideoCodec: "MPEG2", Progressive: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	const want = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-STREAM-INF:BANDWIDTH=28160000,AVERAGE-BANDWIDTH=14160000,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1920x1080,CLOSED-CAPTIONS=NONE
../1080.aac2.broadcast.abr/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=16160000,AVERAGE-BANDWIDTH=8160000,CODECS="avc1.64001F,mp4a.40.2",RESOLUTION=1280x720,CLOSED-CAPTIONS=NONE
../720.aac2.broadcast.abr/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3760000,AVERAGE-BANDWIDTH=1960000,CODECS="avc1.64001F,mp4a.40.2",RESOLUTION=848x480,CLOSED-CAPTIONS=NONE
index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=160000,AVERAGE-BANDWIDTH=160000,CODECS="mp4a.40.2"
../1080.aac2.broadcast.abr/audio-2.m3u8
`
	if string(body) != want {
		t.Fatalf("short rung master:\n%s", body)
	}
	lace, err := ladderPlaylistFromDisk(specs[2], specs, dirs, Source{VideoCodec: "MPEG2"}, false)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(lace), `#EXT-X-STREAM-INF:BANDWIDTH=160000,AVERAGE-BANDWIDTH=160000,CODECS="mp4a.40.2"`)
	if len(parts) != 2 || strings.Count(parts[0], "FRAME-RATE=59.94") != 3 || strings.Contains(parts[1], "FRAME-RATE=") {
		t.Fatalf("interlaced frame rate:\n%s", lace)
	}
}

func TestLadderSessionNamesTheMaster(t *testing.T) {
	spec := ladderRung("broadcast", "1080")
	r := &rendition{spec: spec, dir: t.TempDir()}
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 7}}, renditions: map[string]*rendition{spec.Key(): r}}
	h := &Hub{channels: map[int64]*feed{7: f}}
	s := h.sessionLocked(f, r)
	if want := "/media/live/7/" + spec.Key() + "/ladder.m3u8"; s.MainPlaylist != want {
		t.Fatalf("mainPlaylist = %q", s.MainPlaylist)
	}
	if want := "/media/live/7/" + spec.Key() + "/index.m3u8"; s.Playlist != want {
		t.Fatalf("playlist = %q", s.Playlist)
	}
}

func TestLadderArgs(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", Progressive: true, HD: true}
	videos := []string{"1080", "720", "480"}
	args := LadderArgs(0, src, videos, "broadcast", "libx264", "pipe:0")
	if countArg(args, "-i") != 1 {
		t.Fatalf("decodes: %d", countArg(args, "-i"))
	}
	graph := argAfter(args, "-filter_complex")
	for _, want := range []string{"split=3", "asplit=3", "1920", "1280", "848", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "aresample=async=1000:min_hard_comp=1000000"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("graph missing %s: %s", want, graph)
		}
	}
	if strings.Count(graph, "aresample=") != 1 {
		t.Fatalf("audio resampled %d times", strings.Count(graph, "aresample="))
	}
	if countArg(args, "-force_key_frames") != 3 || countArg(args, "-c:v") != 3 {
		t.Fatalf("encoders: %v", args)
	}
	for _, pipe := range []string{"pipe:3", "pipe:4", "pipe:5"} {
		if !containsArg(args, pipe) {
			t.Fatalf("missing %s", pipe)
		}
	}
	if containsArg(args, "-hwaccel") || containsArg(args, "pipe:1") {
		t.Fatalf("ladder used a hardware decode or stdout: %v", args)
	}
	joined := strings.Join(args, " ")
	if strings.Count(joined, "-force_key_frames source") != 3 || strings.Count(joined, "-g 600") != 3 || strings.Count(joined, "-sc_threshold 0") != 3 {
		t.Fatalf("keyframes: %s", joined)
	}

	lace := LadderArgs(0, Source{VideoCodec: "MPEG2"}, videos, "broadcast", "libx264", "pipe:0")
	laceGraph := argAfter(lace, "-filter_complex")
	if strings.Count(laceGraph, "bwdif=") != 1 || strings.Count(laceGraph, "fps=60000/1001") != 1 {
		t.Fatalf("deinterlace: %s", laceGraph)
	}
	film := LadderArgs(0, src, videos, "film", "libx264", "pipe:0")
	if strings.Count(argAfter(film, "-filter_complex"), "pullup") != 1 || countArg(film, ladderFilmKeys) != 3 {
		t.Fatalf("film: %v", film)
	}
	if strings.Contains(argAfter(film, "-filter_complex"), "bwdif") {
		t.Fatal("film also deinterlaced")
	}
	program := argAfter(LadderArgs(1, src, videos, "broadcast", "libx264", "pipe:0"), "-filter_complex")
	if !strings.Contains(program, "[0:p:1:v:0]") || !strings.Contains(program, "[0:p:1:a:0]") {
		t.Fatalf("program: %s", program)
	}
	pid := argAfter(LadderArgs(0, Source{AudioPID: 100, Progressive: true}, videos, "broadcast", "libx264", "pipe:0"), "-filter_complex")
	if !strings.Contains(pid, "[0:i:100]") {
		t.Fatalf("pid: %s", pid)
	}
	nv := strings.Join(LadderArgs(0, src, videos, "broadcast", "h264_nvenc", "pipe:0"), " ")
	if strings.Count(nv, "-forced-idr 1") != 3 {
		t.Fatalf("nvenc keyframes were not IDRs: %s", nv)
	}
	qsv := strings.Join(LadderArgs(0, src, videos, "broadcast", "h264_qsv", "pipe:0"), " ")
	if strings.Count(qsv, "-forced_idr 1") != 3 || strings.Count(qsv, "-adaptive_i 0") != 3 {
		t.Fatalf("qsv keyframes were not closed: %s", qsv)
	}
	for _, enc := range []string{"h264_nvenc", "h264_qsv"} {
		one := strings.Join(RenditionArgs(0, src, Rendition{Video: "720", Audio: "aac2"}, enc, ""), " ")
		if strings.Contains(one, "forced") || strings.Contains(one, "adaptive_i") {
			t.Fatalf("%s outside a ladder changed: %s", enc, one)
		}
	}
	va := LadderArgs(0, Source{VideoCodec: "MPEG2", HD: true}, videos, "smooth", "h264_vaapi", "pipe:0")
	if !containsArg(va, "-init_hw_device") || containsArg(va, "-hwaccel") || strings.Contains(argAfter(va, "-filter_complex"), "deinterlace_vaapi") {
		t.Fatalf("vaapi ladder decoded in hardware or used its deinterlacer: %v", va)
	}
	if !strings.Contains(argAfter(va, "-filter_complex"), "bwdif=") {
		t.Fatal("smooth ladder skipped the software deinterlace")
	}
	one := LadderArgs(0, src, []string{"480"}, "broadcast", "libx264", "pipe:0")
	if strings.Contains(argAfter(one, "-filter_complex"), "split=") || !containsArg(one, "pipe:3") || containsArg(one, "pipe:4") {
		t.Fatalf("one rung: %v", one)
	}
}

func TestLadderChildIdleDoesNotStopTheGroup(t *testing.T) {
	h, m := testHub(t)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h.Store = st
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 2}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "River 4", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true},
	}); err != nil {
		t.Fatal(err)
	}
	id := idOf(t, st, "4.1")
	f := addTestFeed(h, m, id, "4.1")
	group := &ladderGroup{}
	primarySpec, _ := ParseRenditionKey("1080.aac2.broadcast.abr")
	childSpec, _ := ParseRenditionKey("480.aac2.broadcast.abr")
	primary := &rendition{spec: primarySpec, dir: filepath.Join(h.Dir, primarySpec.Key()), viewers: 1, seen: time.Now(), group: group}
	child := &rendition{spec: childSpec, dir: filepath.Join(h.Dir, childSpec.Key()), seen: time.Now(), group: group, ladderChild: true}
	group.rungs = []*rendition{primary, child}
	h.mu.Lock()
	f.renditions[primary.spec.Key()] = primary
	f.renditions[child.spec.Key()] = child
	h.mu.Unlock()
	t.Cleanup(func() { stopTimer(&primary.idle) })

	h.idleStop(id, child.spec.Key())
	if f.renditions[primary.spec.Key()] != primary || primary.viewers != 1 {
		t.Fatal("an idle shorter rung stopped the ladder someone is watching")
	}
	if f.renditions[child.spec.Key()] != child {
		t.Fatal("the shorter rung was removed on its own")
	}

	if ok, err := h.Warm(ctx, id, childSpec, false); err != nil || !ok {
		t.Fatalf("warm shorter rung: ok=%v err=%v", ok, err)
	}
	if child.idle != nil || primary.idle != nil {
		t.Fatal("warm armed idle while someone is watching, or armed it on the shorter rung")
	}
	primary.viewers = 0
	if ok, err := h.Warm(ctx, id, childSpec, false); err != nil || !ok {
		t.Fatalf("warm idle ladder: ok=%v err=%v", ok, err)
	}
	if primary.idle == nil || child.idle != nil {
		t.Fatal("warm of a shorter rung should arm idle on the primary only")
	}
	primary.guess = true
	stopTimer(&primary.idle)
	if ok, err := h.Warm(ctx, id, childSpec, false); err != nil || !ok || f.renditions[primary.spec.Key()] != primary {
		t.Fatalf("warming a shorter rung stopped the guessed ladder: ok=%v err=%v", ok, err)
	}
}

func TestStuckLadderRungRestartsTheGroup(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "4.1")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	group := &ladderGroup{}
	primarySpec, _ := ParseRenditionKey("1080.aac2.broadcast.abr")
	childSpec, _ := ParseRenditionKey("720.aac2.broadcast.abr")
	primary := &rendition{spec: primarySpec, cmd: cmd, group: group, gate: newPlaylistGate()}
	child := &rendition{spec: childSpec, group: group, ladderChild: true, gate: newPlaylistGate()}
	group.rungs = []*rendition{primary, child}
	ago := time.Now().Add(-time.Minute)
	child.gate.moved.Store(ago.UnixNano())
	h.mu.Lock()
	f.renditions[primary.spec.Key()] = primary
	f.renditions[child.spec.Key()] = child
	h.mu.Unlock()

	h.unfreezeLocked(m, time.Now(), ago)
	wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-wait.Done():
		t.Fatal("a rung that stopped writing left the ladder process running")
	}
	if primary.unfroze.IsZero() {
		t.Fatal("the ladder was not marked restarted")
	}
}

func TestLadderRungsShareAClock(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 4, "4.1")
	group := &ladderGroup{}
	primarySpec, _ := ParseRenditionKey("1080.aac2.broadcast.abr")
	childSpec, _ := ParseRenditionKey("720.aac2.broadcast.abr")
	fixed := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	primary := &rendition{
		spec: primarySpec, dir: filepath.Join(h.Dir, "primary"), clock: NewTimeline(),
		group: group, input: &packInput{},
	}
	child := &rendition{
		spec: childSpec, dir: filepath.Join(h.Dir, "child"), clock: NewTimeline(),
		group: group, ladderChild: true, input: &packInput{},
	}
	primary.clock.now = func() time.Time { return fixed }
	// A shorter rung that dates itself is five seconds off the primary.
	child.clock.now = func() time.Time { return fixed.Add(5 * time.Second) }
	primary.input.noteEncodeStart(10, true)
	child.input.noteEncodeStart(10, true)
	group.rungs = []*rendition{primary, child}
	const playlist = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PTS:seg00000.m4s:900000\n#EXTINF:2.000,\nseg00000.m4s\n"
	for _, dir := range []string{primary.dir, child.dir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(playlist), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.mu.Lock()
	f.renditions[primary.spec.Key()] = primary
	f.renditions[child.spec.Key()] = child
	h.mu.Unlock()

	// Both rungs pass the sibling search before either anchors. That is the
	// window a player opens by fetching every playlist at once.
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var open sync.Once
	unhold := func() { open.Do(func() { close(release) }) }
	defer unhold()
	ladderAnchorHook = func() {
		entered <- struct{}{}
		<-release
	}
	t.Cleanup(func() { ladderAnchorHook = nil })

	keys := []string{primary.spec.Key(), child.spec.Key()}
	bodies := make([][]byte, len(keys))
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			bodies[i], errs[i] = h.stamped(4, key)
		}(i, key)
	}
	for range keys {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("a rung stamped before the other had looked for a clock")
		}
	}
	unhold()
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("rung %d: %v", i, err)
		}
	}
	want := fixed.Add(-4 * time.Second)
	for i, body := range bodies {
		if got := programDate(t, body); !got.Equal(want) {
			t.Fatalf("rung %d dated %s, primary clock %s\n%s", i, got, want, body)
		}
	}
}

func programDate(t *testing.T, body []byte) time.Time {
	t.Helper()
	for _, line := range strings.Split(string(body), "\n") {
		v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")
		if !ok {
			continue
		}
		at, err := time.Parse("2006-01-02T15:04:05.000Z", v)
		if err != nil {
			t.Fatalf("date %q: %v", v, err)
		}
		return at
	}
	t.Fatalf("no program date:\n%s", body)
	return time.Time{}
}

func TestLadderSegmentsAlign(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	writeLadderSample(t, ctx, ffmpeg, sample)
	src := Source{VideoCodec: "MPEG2", Progressive: true}
	videos := []string{"1080", "720", "480"}
	specs := make([]Rendition, len(videos))
	dirs := make([]string, len(videos))
	for i, video := range videos {
		specs[i] = ladderRung("broadcast", video)
		dirs[i] = filepath.Join(dir, video)
		if err := os.MkdirAll(dirs[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(ctx, ffmpeg, LadderArgs(0, src, videos, "broadcast", "libx264", sample)...)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipes, err := ladderOutputs(cmd, len(videos))
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errc := make(chan error, len(videos))
	for i := range videos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pipes[i].started()
			if err := Pack(dirs[i], pipes[i], newPlaylistGate()); err != nil {
				errc <- err
			}
		}(i)
	}
	waitErr := cmd.Wait()
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Errorf("pack: %v", err)
	}
	if waitErr != nil {
		t.Fatalf("ladder encode: %v\n%s", waitErr, stderr.String())
	}
	var lists [][]float64
	for i, video := range videos {
		got := extinfDurations(t, filepath.Join(dirs[i], "index.m3u8"))
		if len(got) < 2 {
			t.Fatalf("%s segments: %v", video, got)
		}
		lists = append(lists, got)
	}
	for i := 1; i < len(lists); i++ {
		if len(lists[i]) != len(lists[0]) {
			t.Fatalf("segment counts %v vs %v", lists[0], lists[i])
		}
		for n := range lists[0] {
			if delta := lists[i][n] - lists[0][n]; delta > 0.05 || delta < -0.05 {
				t.Fatalf("segment %d differs by %v: %v vs %v", n, delta, lists[0], lists[i])
			}
		}
	}
	body, err := ladderPlaylistFromDisk(specs[0], specs, dirs, src, false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "avc1.") || !strings.Contains(text, "mp4a.") || !strings.Contains(text, "audio-2.m3u8") {
		t.Fatalf("master from inits:\n%s", text)
	}
	streams, _ := streamLines(text)
	if len(streams) != 4 || strings.Contains(streams[3], "RESOLUTION=") {
		t.Fatalf("served master:\n%s", text)
	}
	heights := make([]int, 3)
	bands := make([]int, 3)
	for i, line := range streams[:3] {
		heights[i] = streamHeight(line)
		bands[i] = streamBandwidth(line)
		if !strings.Contains(line, "CODECS=") || !strings.Contains(line, "RESOLUTION=") {
			t.Fatalf("variant: %s", line)
		}
	}
	if heights[2] == 0 || heights[2] > 480 {
		t.Fatalf("480 height %d", heights[2])
	}
	if !(bands[0] > bands[1] && bands[1] > bands[2]) {
		t.Fatalf("bandwidth order %v", bands)
	}
}

func TestLadderEncodeCost(t *testing.T) {
	if os.Getenv("LADDER_COST") != "1" {
		t.Skip("set LADDER_COST=1 to measure the ladder")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	writeLadderSample(t, ctx, ffmpeg, sample)
	src := Source{VideoCodec: "MPEG2", Progressive: true}
	videos := []string{"1080", "720", "480"}
	ladder := exec.CommandContext(ctx, ffmpeg, LadderArgs(0, src, videos, "broadcast", "libx264", sample)...)
	ladder.Stdout = io.Discard
	ladder.Stderr = io.Discard
	pipes, err := ladderOutputs(ladder, len(videos))
	if err != nil {
		t.Fatal(err)
	}
	if err := ladder.Start(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, pipe := range pipes {
		wg.Add(1)
		go func(p *packPipe) {
			defer wg.Done()
			p.started()
			_, _ = io.Copy(io.Discard, p)
		}(pipe)
	}
	if err := ladder.Wait(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	ladderCPU := processCPU(ladder)
	var separate time.Duration
	parts := make([]time.Duration, len(videos))
	for i, video := range videos {
		cmd := exec.CommandContext(ctx, ffmpeg, renditionArgs(0, src, Rendition{Video: video, Audio: "aac2", Mode: "broadcast"}, "libx264", "", sample)...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		parts[i] = processCPU(cmd)
		separate += parts[i]
	}
	t.Logf("ladder %s; separate 1080 %s, 720 %s, 480 %s; sum %s", ladderCPU, parts[0], parts[1], parts[2], separate)
}

func writeLadderSample(t *testing.T, ctx context.Context, ffmpeg, path string) {
	t.Helper()
	gen := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "4", "-c:v", "mpeg2video", "-g", "15", "-bf", "0", "-b:v", "2000k", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-b:a", "192k", "-f", "mpegts", path)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
}

func processCPU(cmd *exec.Cmd) time.Duration {
	if cmd.ProcessState == nil {
		return 0
	}
	return cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
}

func extinfDurations(t *testing.T, path string) []float64 {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []float64
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		rest := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		out = append(out, v)
	}
	return out
}

func streamLines(body string) (streams, uris []string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			continue
		}
		streams = append(streams, line)
		if i+1 < len(lines) {
			uris = append(uris, lines[i+1])
		}
	}
	return streams, uris
}

func streamBandwidth(line string) int {
	const key = "BANDWIDTH="
	i := strings.Index(line, key)
	if i < 0 {
		return 0
	}
	rest := line[i+len(key):]
	n := 0
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func streamHeight(line string) int {
	const key = "RESOLUTION="
	i := strings.Index(line, key)
	if i < 0 {
		return 0
	}
	rest := line[i+len(key):]
	_, h, _ := strings.Cut(rest, "x")
	n := 0
	for _, c := range h {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func countArg(args []string, name string) int {
	n := 0
	for _, arg := range args {
		if arg == name {
			n++
		}
	}
	return n
}

func containsArg(args []string, name string) bool {
	return countArg(args, name) > 0
}

func argAfter(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
