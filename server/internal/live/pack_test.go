package live

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlaylistGateWakesForTheNextPart(t *testing.T) {
	g := newPlaylistGate()
	g.publish(0, 0, 0)
	if g.ready(0, 0) {
		t.Fatal("an empty open segment is not ready")
	}
	done := make(chan struct{})
	go func() {
		g.wait(0, 0, time.Second)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	g.publish(0, 0, 1)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the gate did not wake for part 0")
	}
	if !g.ready(0, 0) || g.ready(0, 1) {
		t.Fatal("only the published part is ready")
	}
	start := time.Now()
	g.wait(3, 0, 80*time.Millisecond)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("a missing part waited past the timeout")
	}
	start = time.Now()
	g.wait(-1, 0, time.Second)
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("a negative sequence should not wait")
	}
}

func TestPlaylistGateWholeSegmentIgnoresOpenParts(t *testing.T) {
	g := newPlaylistGate()
	g.publish(0, 0, 3)
	if g.ready(0, -1) {
		t.Fatal("an open segment is not a finished segment")
	}
	g.publish(0, 1, 1)
	if !g.ready(0, -1) {
		t.Fatal("segment 0 is finished once a later segment is open")
	}
}

func TestPackClosesAFinishedGroupBeforeTheNext(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=30", "-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "5", "-c:v", "libx264", "-preset", "ultrafast", "-g", "60", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-output_ts_offset", "95000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	out := filepath.Join(dir, "rendition")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	cmd := exec.Command(ffmpeg, RenditionArgs(0, Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "")...)
	cmd.Dir = out
	cmd.Stdin = in
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Capture the fragment stream, then feed it one fragment at a time.
	raw, err := io.ReadAll(stdout)
	waitErr := cmd.Wait()
	if err != nil || waitErr != nil {
		t.Fatalf("encode: %v %v", err, waitErr)
	}
	boxes := topBoxes(raw)
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(out, pr, nil) }()

	wroteFragment := false
	for _, box := range boxes {
		if _, err := pw.Write(box); err != nil {
			t.Fatal(err)
		}
		if string(box[4:8]) == "mdat" {
			wroteFragment = true
			break
		}
	}
	if !wroteFragment {
		t.Fatal("the encode produced no fragment")
	}
	var playlist string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(out, "index.m3u8"))
		if err == nil && strings.Contains(string(b), "#EXTINF:") && strings.Contains(string(b), "seg00000.m4s") {
			playlist = string(b)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if playlist == "" {
		b, _ := os.ReadFile(filepath.Join(out, "index.m3u8"))
		t.Fatalf("the first group was not a segment:\n%s", b)
	}
	segDur := 0.0
	for _, line := range strings.Split(playlist, "\n") {
		v, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		v = strings.TrimSuffix(v, ",")
		segDur, err = strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	// This source's group of pictures is two seconds. Closing it as 0.500
	// would make the player expect the next group half a second later.
	// A closed segment lists its own parts; nothing is left open after it.
	openParts := strings.Contains(playlist[strings.LastIndex(playlist, "#EXTINF:"):], "#EXT-X-PART:")
	if segDur < 1.5 || openParts || !strings.Contains(playlist, "CAN-BLOCK-RELOAD=YES") {
		t.Fatalf("segment %.3f, playlist:\n%s", segDur, playlist)
	}
	fixed := time.Date(2026, 9, 26, 4, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var stamper playlistStamper
	stamped := string(stamper.stamp(out, []byte(playlist), tl))
	if !strings.Contains(stamped, "seg00000.m4s") || !strings.Contains(stamped, "#EXT-X-PROGRAM-DATE-TIME:") {
		t.Fatalf("stamping dropped the first segment:\n%s", stamped)
	}
	earliest, ok := tl.Earliest()
	if !ok || !earliest.Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("the first part should anchor the clock, got %v %v", earliest, ok)
	}

	if _, err := pw.Write(restAfter(boxes)); err != nil {
		t.Fatal(err)
	}
	_ = pw.Close()
	if err := <-packErr; err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(filepath.Join(out, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), "seg00000.m4s") {
		t.Fatalf("segment 0 missing:\n%s", final)
	}
	assertSegmentCuts(t, string(final))
}

// A finished keyframe group is a segment before the next group arrives.
// A shorter group stays a part. hls.js does not play a part at startup.
func TestFinishedGroupClosesBeforeTheNextFragment(t *testing.T) {
	dir := t.TempDir()
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(dir, pr, nil) }()
	if _, err := pw.Write(videoInit()); err != nil {
		t.Fatal(err)
	}
	if _, err := pw.Write(keyframeFragment(0, 90000)); err != nil {
		t.Fatal(err)
	}
	playlist := waitPlaylist(t, dir, "#EXTINF:1.000,\nseg00000.m4s\n")
	if openParts(playlist) > 0 {
		t.Fatalf("a one-second group stayed a part:\n%s", playlist)
	}
	if _, err := pw.Write(keyframeFragment(90000, 20000)); err != nil {
		t.Fatal(err)
	}
	short := waitPlaylist(t, dir, "#EXT-X-PART:")
	if strings.Count(short, "#EXTINF:") != 1 {
		t.Fatalf("the short group closed a second segment:\n%s", short)
	}
	_ = pw.Close()
	if err := <-packErr; err != nil {
		t.Fatal(err)
	}
}

func waitPlaylist(t *testing.T, dir, needle string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if err == nil && strings.Contains(string(b), needle) {
			return string(b)
		}
		time.Sleep(5 * time.Millisecond)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	t.Fatalf("playlist never contained %q:\n%s", needle, b)
	return ""
}

func restAfter(boxes [][]byte) []byte {
	seen := false
	var b []byte
	for _, box := range boxes {
		if !seen {
			if string(box[4:8]) == "mdat" {
				seen = true
			}
			continue
		}
		b = append(b, box...)
	}
	return b
}

func topBoxes(b []byte) [][]byte {
	var out [][]byte
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b[:4]))
		head := 8
		if size == 1 {
			if len(b) < 16 {
				break
			}
			size = int(binary.BigEndian.Uint64(b[8:16]))
			head = 16
		}
		if size < head || size > len(b) {
			break
		}
		out = append(out, append([]byte(nil), b[:size]...))
		b = b[size:]
	}
	return out
}

func assertSegmentCuts(t *testing.T, playlist string) {
	t.Helper()
	var durs []float64
	for _, line := range strings.Split(playlist, "\n") {
		v, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		v = strings.TrimSuffix(v, ",")
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatal(err)
		}
		durs = append(durs, f)
	}
	if len(durs) < 3 {
		t.Fatalf("expected at least three segments, got %v", durs)
	}
	// A segment is one fragment: a half-second transcode or one source GOP.
	// The tail can be short because the encode ended mid-fragment.
	body := durs
	if body[len(body)-1] < 0.3 {
		body = body[:len(body)-1]
	}
	for _, d := range body {
		if d < 0.3 || d > 2.6 {
			t.Errorf("segment duration %.3f is not one fragment (%v)", d, durs)
		}
	}
	if strings.Contains(playlist, "DISCONTINUITY") {
		t.Errorf("a steady encode was marked as a jump:\n%s", playlist)
	}
}

// mp4Box is a short ISO-BMFF box. The body does not include the size or type.
func mp4Box(kind string, body []byte) []byte {
	b := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(b[:4], uint32(len(b)))
	copy(b[4:8], kind)
	copy(b[8:], body)
	return b
}

// keyframeFragment is one video fragment at pts, lasting dur ticks at 90 kHz.
func keyframeFragment(pts int64, dur uint32) []byte {
	tfhd := make([]byte, 8)
	binary.BigEndian.PutUint32(tfhd[4:8], 1)
	tfdt := make([]byte, 12)
	tfdt[0] = 1
	binary.BigEndian.PutUint64(tfdt[4:12], uint64(pts))
	const trFlags = 0x104 // sample duration and first-sample flags
	trun := make([]byte, 16)
	trun[1] = byte(trFlags >> 16)
	trun[2] = byte(trFlags >> 8)
	trun[3] = byte(trFlags & 0xff)
	binary.BigEndian.PutUint32(trun[4:8], 1)
	binary.BigEndian.PutUint32(trun[12:16], dur)
	traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
	moof := mp4Box("moof", append(mp4Box("mfhd", make([]byte, 8)), mp4Box("traf", traf)...))
	return append(moof, mp4Box("mdat", []byte{0})...)
}

func videoInit() []byte {
	tkhd := make([]byte, 24)
	binary.BigEndian.PutUint32(tkhd[12:16], 1)
	mdhd := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhd[12:16], 90000)
	hdlr := make([]byte, 12)
	copy(hdlr[8:12], "vide")
	mdia := append(mp4Box("mdhd", mdhd), mp4Box("hdlr", hdlr)...)
	trak := append(mp4Box("tkhd", tkhd), mp4Box("mdia", mdia)...)
	return append(mp4Box("ftyp", []byte("isom")), mp4Box("moov", mp4Box("trak", trak))...)
}

func packKeyframes(t *testing.T, dir string, pts []int64, dur uint32) string {
	t.Helper()
	var raw []byte
	raw = append(raw, videoInit()...)
	for _, p := range pts {
		raw = append(raw, keyframeFragment(p, dur)...)
	}
	if err := Pack(dir, bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func extinfSum(playlist string) (n int, seconds float64) {
	for _, line := range strings.Split(playlist, "\n") {
		v, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		v = strings.TrimSuffix(v, ",")
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return n, seconds
		}
		n++
		seconds += f
	}
	return n, seconds
}

// A segment name cut from the playlist used to keep that whole playlist
// alive in the clock map. Each new segment pinned one more copy, so a long
// window grew with the square of its length.
func TestStampDoesNotKeepPlaylistText(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	const n = 160
	const pad = 4096
	for i := 0; i < n; i++ {
		seg := keyframeFragment(int64(i)*90000, 90000)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("seg%05d.m4s", i)), seg, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var st playlistStamper
	tl := NewTimeline()
	marker := strings.Repeat("Q", pad)
	heap := func() uint64 {
		runtime.GC()
		debug.FreeOSMemory()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	before := heap()
	for i := 1; i <= n; i++ {
		var b strings.Builder
		b.WriteString("#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:1\n")
		for j := 0; j < i; j++ {
			fmt.Fprintf(&b, "#%s\n#EXTINF:1.000,\nseg%05d.m4s\n", marker, j)
		}
		st.stamp(dir, []byte(b.String()), tl)
	}
	if len(st.cache) != n {
		t.Fatalf("cached %d of %d segments", len(st.cache), n)
	}
	after := heap()
	var grew uint64
	if after > before {
		grew = after - before
	}
	// The retained history is about n²/2 markers. The names themselves are not.
	history := uint64(n*(n+1)/2) * pad
	if grew > history/10 {
		t.Fatalf("stamp kept %d bytes after %d playlists; a retained history is about %d", grew, n, history)
	}
}

// Half-second segments must not bring the rewind window down to a count of
// 2700 (that was 90 minutes only while every segment was 2 seconds).
func TestLiveWindowKeepsNinetyMinutes(t *testing.T) {
	dir := t.TempDir()
	const half = 45000
	pts := make([]int64, 2702)
	for i := range pts {
		pts[i] = int64(i) * half
	}
	short := packKeyframes(t, dir, pts, half)
	n, seconds := extinfSum(short)
	if n != 2702 || seconds < 1350 || seconds > 1352 {
		t.Fatalf("short playlist kept %d segments, %.3fs:\n%s", n, seconds, headTail(short))
	}
	if _, err := os.Stat(filepath.Join(dir, "seg00000.m4s")); err != nil {
		t.Fatal("the oldest short segment was dropped before 90 minutes")
	}
	if _, err := os.Stat(filepath.Join(dir, "seg02701.m4s")); err != nil {
		t.Fatal(err)
	}

	longDir := t.TempDir()
	const halfHour = 30 * 60 * 90000
	longPTS := make([]int64, 6)
	for i := range longPTS {
		longPTS[i] = int64(i) * halfHour
	}
	long := packKeyframes(t, longDir, longPTS, halfHour)
	n, seconds = extinfSum(long)
	if n != 3 || seconds < 5399 || seconds > 5401 {
		t.Fatalf("90 minute window kept %d segments, %.3fs:\n%s", n, seconds, long)
	}
	if !strings.Contains(long, "#EXT-X-MEDIA-SEQUENCE:3\n") {
		t.Fatalf("sequence:\n%s", long)
	}
	if strings.Contains(long, "DISCONTINUITY") {
		t.Fatalf("a 30 minute segment is one group, not a jump:\n%s", long)
	}
	if _, err := os.Stat(filepath.Join(longDir, "seg00000.m4s")); !os.IsNotExist(err) {
		t.Fatalf("segment past 90 minutes still on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(longDir, "seg00003.m4s")); err != nil {
		t.Fatal(err)
	}
}

// fragmentAt is one video fragment. flags is the first sample's trun flags
// (0x10000 means it is not a keyframe). pad is extra mdat bytes.
func fragmentAt(pts int64, dur uint32, flags uint32, pad int) []byte {
	tfhd := make([]byte, 8)
	binary.BigEndian.PutUint32(tfhd[4:8], 1)
	tfdt := make([]byte, 12)
	tfdt[0] = 1
	binary.BigEndian.PutUint64(tfdt[4:12], uint64(pts))
	const trFlags = 0x104 // sample duration and first-sample flags
	trun := make([]byte, 16)
	trun[1] = byte(trFlags >> 16)
	trun[2] = byte(trFlags >> 8)
	trun[3] = byte(trFlags & 0xff)
	binary.BigEndian.PutUint32(trun[4:8], 1)
	binary.BigEndian.PutUint32(trun[8:12], flags)
	binary.BigEndian.PutUint32(trun[12:16], dur)
	traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
	moof := mp4Box("moof", append(mp4Box("mfhd", make([]byte, 8)), mp4Box("traf", traf)...))
	return append(moof, mp4Box("mdat", make([]byte, pad))...)
}

// A backwards presentation time must close the open segment and mark a
// discontinuity. A second stamp must keep those date-times and the earliest
// anchor: the discontinuity moved the clock the first stamp uses.
func TestTimestampJumpClosesTheSegment(t *testing.T) {
	dir := t.TempDir()
	const step = int64(45000)
	base := int64(2868510171)
	pts := []int64{base, base + step}
	jumped := base + step - 9011*90000
	for i := 0; i < 24; i++ {
		pts = append(pts, jumped+int64(i)*step)
	}
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(dir, pr, nil) }()
	if _, err := pw.Write(videoInit()); err != nil {
		t.Fatal(err)
	}
	for _, p := range pts {
		if _, err := pw.Write(fragmentAt(p, uint32(step), 0, 32<<10)); err != nil {
			t.Fatal(err)
		}
	}
	var playlist string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if err == nil && strings.Count(string(b), "#EXTINF") >= 20 && strings.Contains(string(b), "#EXT-X-DISCONTINUITY\n") {
			playlist = string(b)
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if playlist == "" {
		b, _ := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		t.Fatalf("the jump did not close segments:\n%s", b)
	}
	if n := strings.Count(playlist, "#EXT-X-DISCONTINUITY"); n != 1 {
		t.Fatalf("discontinuity count %d:\n%s", n, playlist)
	}
	for _, line := range strings.Split(playlist, "\n") {
		v, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, ","), 64)
		if err != nil || f > 2 || f < 0.4 {
			t.Fatalf("segment %.3f would stall:\n%s", f, playlist)
		}
	}
	if n := openParts(playlist); n > 4 {
		t.Fatalf("open segment kept %d parts", n)
	}
	_ = pw.Close()
	if err := <-packErr; err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 9, 26, 14, 16, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var stamper playlistStamper
	stamped := string(stamper.stamp(dir, final, tl))
	var walls []time.Time
	var afterGap bool
	var gapAt int
	for _, line := range strings.Split(stamped, "\n") {
		if strings.HasPrefix(line, "#EXT-X-DISCONTINUITY") {
			afterGap = true
			continue
		}
		v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")
		if !ok {
			continue
		}
		wall, err := time.Parse("2006-01-02T15:04:05.000Z", v)
		if err != nil {
			t.Fatal(err)
		}
		if afterGap && gapAt == 0 {
			gapAt = len(walls)
		}
		afterGap = false
		walls = append(walls, wall)
	}
	if gapAt < 1 || gapAt >= len(walls) {
		t.Fatalf("program times %d, gap at %d:\n%s", len(walls), gapAt, stamped)
	}
	stepWall := walls[gapAt].Sub(walls[gapAt-1])
	if stepWall < 400*time.Millisecond || stepWall > 600*time.Millisecond {
		t.Fatalf("date-time jumped by %s across the discontinuity, want about 0.5s", stepWall)
	}
	earliest, ok := tl.Earliest()
	if !ok || !earliest.Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("earliest moved to %v", earliest)
	}
	tl.now = func() time.Time { return fixed.Add(30 * time.Second) }
	again := string(stamper.stamp(dir, final, tl))
	if strings.Join(programDates(again), "\n") != strings.Join(programDates(stamped), "\n") {
		t.Fatalf("a later playlist moved date-times:\n%s", again)
	}
	earliest, ok = tl.Earliest()
	if !ok || !earliest.Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("earliest moved on the second stamp to %v", earliest)
	}
}

func programDates(playlist string) []string {
	var out []string
	for _, line := range strings.Split(playlist, "\n") {
		if strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:") {
			out = append(out, line)
		}
	}
	return out
}

func TestForwardJumpAndWrap(t *testing.T) {
	forward := t.TempDir()
	const step = int64(45000)
	pts := []int64{0, step, step + step + 11*90000, step + step + 11*90000 + step}
	pl := packFragments(t, forward, pts, 0, 1)
	if strings.Count(pl, "#EXT-X-DISCONTINUITY") != 1 {
		t.Fatalf("an 11s step is a discontinuity:\n%s", pl)
	}
	fixed := time.Date(2026, 9, 26, 14, 16, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var stamper playlistStamper
	first := string(stamper.stamp(forward, []byte(pl), tl))
	second := string(stamper.stamp(forward, []byte(pl), tl))
	if strings.Join(programDates(first), "\n") != strings.Join(programDates(second), "\n") {
		t.Fatalf("an 11s jump moved date-times on the next playlist:\n%s\n---\n%s", first, second)
	}
	wrapped := t.TempDir()
	start := ptsWrap - 2*step
	wrapPTS := []int64{start, start + step, 0}
	if pl := packFragments(t, wrapped, wrapPTS, 0, 1); strings.Contains(pl, "DISCONTINUITY") {
		t.Fatalf("a 33-bit wrap of one group is not a jump:\n%s", pl)
	}
}

func TestStampOverlapStaysMonotonic(t *testing.T) {
	fixed := time.Date(2026, 9, 26, 21, 32, 48, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var stamper playlistStamper
	const tick = 90000.0
	stamper.cache = map[string]int64{
		"seg00000.m4s": 0,
		"seg00001.m4s": int64(0.096 * tick),
		"seg00002.m4s": int64((0.096 + 1.001) * tick),
	}
	raw := []byte("#EXTM3U\n#EXTINF:1.001,\nseg00000.m4s\n#EXTINF:1.001,\nseg00001.m4s\n#EXTINF:1.001,\nseg00002.m4s\n")
	stamped := string(stamper.stamp(t.TempDir(), raw, tl))
	walls := dateWalls(t, stamped)
	if len(walls) != 3 {
		t.Fatalf("dates %d:\n%s", len(walls), stamped)
	}
	for i := 1; i < len(walls); i++ {
		step := walls[i].Sub(walls[i-1])
		if step < 900*time.Millisecond || step > 1200*time.Millisecond {
			t.Fatalf("step %d is %s, want about 1s:\n%s", i, step, stamped)
		}
	}
	earliest, ok := tl.Earliest()
	if !ok || !earliest.Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("earliest moved to %v", earliest)
	}
	again := string(stamper.stamp(t.TempDir(), raw, tl))
	if strings.Join(programDates(again), "\n") != strings.Join(programDates(stamped), "\n") {
		t.Fatalf("a later playlist moved date-times:\n%s", again)
	}

	straight := NewTimeline()
	straight.now = tl.now
	var plain playlistStamper
	plain.cache = map[string]int64{
		"seg00000.m4s": 0,
		"seg00001.m4s": int64(1.2 * tick),
	}
	plainRaw := []byte("#EXTM3U\n#EXTINF:1.001,\nseg00000.m4s\n#EXTINF:1.001,\nseg00001.m4s\n")
	got := dateWalls(t, string(plain.stamp(t.TempDir(), plainRaw, straight)))
	if len(got) != 2 {
		t.Fatal(got)
	}
	step := got[1].Sub(got[0])
	if step < 1150*time.Millisecond || step > 1250*time.Millisecond {
		t.Fatalf("a later program time was pulled back to %s", step)
	}
}

func dateWalls(t *testing.T, playlist string) []time.Time {
	t.Helper()
	var walls []time.Time
	for _, line := range programDates(playlist) {
		v, _ := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")
		wall, err := time.Parse("2006-01-02T15:04:05.000Z", v)
		if err != nil {
			t.Fatal(err)
		}
		walls = append(walls, wall)
	}
	return walls
}

func TestOpenRunStaysBounded(t *testing.T) {
	dir := t.TempDir()
	const step = int64(45000)
	// No keyframe, so the half-second cut never fires. The byte cap must.
	pts := make([]int64, 9)
	for i := range pts {
		pts[i] = int64(i) * step
	}
	pl := packFragments(t, dir, pts, 0x10000, 8<<20)
	n, _ := extinfSum(pl)
	if n < 2 {
		t.Fatalf("a run with no keyframe stayed one segment:\n%s", pl)
	}
	if n := openParts(pl); n > 6 {
		t.Fatalf("kept %d parts", n)
	}
}

// openParts counts the parts listed after the last closed segment.
func openParts(playlist string) int {
	if i := strings.LastIndex(playlist, "#EXTINF:"); i >= 0 {
		playlist = playlist[i:]
	}
	return strings.Count(playlist, "#EXT-X-PART:")
}

func TestPackRejectsAHugeBox(t *testing.T) {
	dir := t.TempDir()
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() {
		err := Pack(dir, pr, nil)
		_ = pr.Close()
		packErr <- err
	}()
	var read atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer pw.Close()
		if _, err := pw.Write(videoInit()); err != nil {
			return
		}
		hdr := make([]byte, 8)
		binary.BigEndian.PutUint32(hdr[:4], 64<<20)
		copy(hdr[4:], "mdat")
		if _, err := pw.Write(hdr); err != nil {
			return
		}
		buf := make([]byte, 1<<20)
		for read.Load() < 48<<20 {
			n, err := pw.Write(buf)
			read.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()
	select {
	case err := <-packErr:
		if err == nil || !strings.Contains(err.Error(), "fragment larger") {
			t.Fatalf("got %v after reading %d", err, read.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a huge box was still being buffered")
	}
	<-done
	if read.Load() > 40<<20 {
		t.Fatalf("buffered %d bytes", read.Load())
	}
}

func packFragments(t *testing.T, dir string, pts []int64, flags uint32, pad int) string {
	t.Helper()
	var raw []byte
	raw = append(raw, videoInit()...)
	for _, p := range pts {
		raw = append(raw, fragmentAt(p, 45000, flags, pad)...)
	}
	if err := Pack(dir, bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func headTail(s string) string {
	if len(s) < 400 {
		return s
	}
	return s[:200] + "\n...\n" + s[len(s)-200:]
}

func TestPartLongerThanASegmentRaisesTargetDuration(t *testing.T) {
	dir := t.TempDir()
	// 0.633s segment and a 1.001s open part. Ceil of the segment alone is 1,
	// and a part target of 1.001 against that is a playlist parse error.
	err := writePacked(dir, []byte("init"),
		[]packedSeg{{name: "seg00000.m4s", dur: 56970}},
		[]packedPart{{name: "part00001.m4s", dur: 90090, sync: true}},
		0, true, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "#EXT-X-VERSION:9\n") {
		t.Fatalf("parts require version 9:\n%s", text)
	}
	if !strings.Contains(text, "#EXT-X-TARGETDURATION:2\n") || !strings.Contains(text, "PART-TARGET=1.999") {
		t.Fatalf("target duration must cover the part:\n%s", text)
	}
	if !strings.Contains(text, "HOLD-BACK=6.000") || !strings.Contains(text, "PART-HOLD-BACK=5.997") {
		t.Fatalf("hold-back must clear one target duration:\n%s", text)
	}
	if !strings.Contains(text, "DURATION=1.001") {
		t.Fatalf("the part keeps its own duration:\n%s", text)
	}
}

func TestTargetDurationDoesNotShrink(t *testing.T) {
	dir := t.TempDir()
	var hold playlistCeiling
	write := func(seg, part int64) string {
		t.Helper()
		if err := writePacked(dir, []byte("init"),
			[]packedSeg{{name: "seg00000.m4s", dur: seg}},
			[]packedPart{{name: "part00001.m4s", dur: part, sync: true}},
			0, true, false, &hold, nil); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	short := write(45000, 45000)
	if !strings.Contains(short, "#EXT-X-TARGETDURATION:2\n") || !strings.Contains(short, "PART-TARGET=1.999") || !strings.Contains(short, "PART-HOLD-BACK=5.997") || !strings.Contains(short, "HOLD-BACK=6.000") {
		t.Fatalf("short segments still advertise 2s:\n%s", short)
	}
	grown := write(45000, 90090)
	for _, tag := range []string{"#EXT-X-TARGETDURATION:2\n", "PART-TARGET=1.999", "PART-HOLD-BACK=5.997", "HOLD-BACK=6.000", "DURATION=1.001"} {
		if !strings.Contains(grown, tag) {
			t.Fatalf("a longer part under the pin changed %s:\n%s", tag, grown)
		}
	}
	if text := write(45000, 200000); !strings.Contains(text, "#EXT-X-TARGETDURATION:3\n") || !strings.Contains(text, "PART-TARGET=2.222") {
		t.Fatalf("a longer part raises it:\n%s", text)
	}
	if text := write(45000, 45000); !strings.Contains(text, "#EXT-X-TARGETDURATION:3\n") || !strings.Contains(text, "PART-TARGET=2.222") {
		t.Fatalf("it must not shrink:\n%s", text)
	}
}

func TestDeltaPlaylistSkipsTheHead(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:1\n")
	b.WriteString("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,CAN-SKIP-UNTIL=6.000\n")
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:4\n#EXT-X-MAP:URI=\"init.mp4\"\n")
	for i := 0; i < 20; i++ {
		b.WriteString("#EXT-X-PROGRAM-DATE-TIME:2026-09-26T04:00:0" + strconv.Itoa(i%10) + ".000Z\n")
		b.WriteString("#EXTINF:0.500,\nseg" + strconv.Itoa(i) + ".m4s\n")
	}
	b.WriteString("#EXT-X-PART:DURATION=0.500,URI=\"part00020.m4s\"\n")
	out := string(DeltaPlaylist([]byte(b.String())))
	if !strings.Contains(out, "#EXT-X-SKIP:SKIPPED-SEGMENTS=8\n") {
		t.Fatalf("skip count:\n%s", out)
	}
	if !strings.Contains(out, "#EXT-X-VERSION:9\n") || strings.Contains(out, "seg0.m4s") || !strings.Contains(out, "seg8.m4s") {
		t.Fatalf("delta shape:\n%s", out)
	}
	if !strings.Contains(out, "#EXT-X-MEDIA-SEQUENCE:4\n") || !strings.Contains(out, "part00020.m4s") {
		t.Fatalf("sequence and the open part stay:\n%s", out)
	}
	if strings.Count(out, "#EXTINF") != 12 {
		t.Fatalf("kept %d segments:\n%s", strings.Count(out, "#EXTINF"), out)
	}
	// A jump rides with its segment. Skipping that segment drops the tag.
	// A jump on a segment that is still in the delta stays.
	withGap := strings.Replace(b.String(), "#EXTINF:0.500,\nseg8.m4s\n", "#EXT-X-DISCONTINUITY\n#EXTINF:0.500,\nseg8.m4s\n", 1)
	withGap = strings.Replace(withGap, "#EXTINF:0.500,\nseg1.m4s\n", "#EXT-X-DISCONTINUITY\n#EXTINF:0.500,\nseg1.m4s\n", 1)
	delta := string(DeltaPlaylist([]byte(withGap)))
	if strings.Contains(delta, "seg1.m4s") || strings.Count(delta, "DISCONTINUITY") != 1 || !strings.Contains(delta, "seg8.m4s") {
		t.Fatalf("delta discontinuity:\n%s", delta)
	}
	short := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:0.500,\nseg00000.m4s\n"
	if got := string(DeltaPlaylist([]byte(short))); got != short {
		t.Fatalf("a short playlist is unchanged:\n%s", got)
	}
}

// A broadcast join often has sound before the first picture. ffmpeg keeps that
// lead only in edit lists, which hls.js and Chrome ignore (sound played that
// much late) and Safari honors (its buffer sat hours from the playhead). The
// packager moves the lead into the fragments and drops the edits.
func TestPackLinesUpTracksWithoutEditLists(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-itsoffset", "1.3", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=30000/1001",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "6", "-c:v", "libx264", "-g", "15", "-c:a", "ac3", "-output_ts_offset", "72000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	probe, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_type,start_time", "-of", "csv=p=0", src).Output()
	if err != nil {
		t.Fatal(err)
	}
	began := map[string]float64{}
	for _, line := range strings.Fields(string(probe)) {
		kind, at, _ := strings.Cut(line, ",")
		began[kind], _ = strconv.ParseFloat(at, 64)
	}
	want := began["video"] - began["audio"]
	if want < 1 {
		t.Fatalf("source should lead with sound, got %v", began)
	}
	source := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}
	for _, r := range []Rendition{{Video: "copy", Audio: "copy"}, {Video: "540", Audio: "aac2", Mode: "broadcast"}} {
		t.Run(r.Key(), func(t *testing.T) {
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
			if err := cmd.Wait(); packErr != nil || err != nil {
				t.Fatalf("pack %v wait %v %s", packErr, err, stderr.String())
			}
			init, err := os.ReadFile(filepath.Join(out, "init.mp4"))
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[uint32]string{}
			scales := map[uint32]uint32{}
			for _, trak := range boxes(child(init, "moov")) {
				if trak.kind != "trak" {
					continue
				}
				if child(trak.body, "edts") != nil {
					t.Errorf("init.mp4 still has an edit list")
				}
				hdlr := child(child(trak.body, "mdia"), "hdlr")
				id, scale, ok := trackScale(trak.body)
				if !ok || len(hdlr) < 12 {
					t.Fatal("unreadable track")
				}
				kinds[id], scales[id] = string(hdlr[8:12]), scale
			}
			// Where each track's first sample in a segment is shown, in seconds.
			starts := func(name string) map[string]float64 {
				seg, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatal(err)
				}
				got := map[string]float64{}
				for _, traf := range boxes(child(seg, "moof")) {
					tfhd, tfdt := child(traf.body, "tfhd"), child(traf.body, "tfdt")
					if traf.kind != "traf" || len(tfhd) < 8 || len(tfdt) < 12 || tfdt[0] != 1 {
						continue
					}
					id := binary.BigEndian.Uint32(tfhd[4:8])
					base := int64(binary.BigEndian.Uint64(tfdt[4:12])) + firstCompositionOffset(child(traf.body, "trun"))
					got[kinds[id]] = float64(base) / float64(scales[id])
				}
				return got
			}
			first := starts("seg00000.m4s")
			if len(first) != 2 {
				t.Fatalf("first segment tracks: %v", first)
			}
			// Sound starts at 0 and the picture at its lead. The AAC encoder's
			// first 1024 samples are priming before the sound. Half a picture
			// of rounding is allowed; missing the priming is not.
			frame := 1536.0 / 48000
			priming := 0.0
			if r.Audio == "aac2" {
				frame, priming = 1024.0/48000, 1024.0/48000
			}
			if math.Abs(first["vide"]-priming-want) > 0.008 {
				t.Fatalf("picture starts at %.4fs in the fragments, want %.4fs after the sound (%v)", first["vide"], want, first)
			}
			// Sound from before the first picture is dropped whole frames at a
			// time, so what is left keeps its place.
			if gap := first["soun"] - first["vide"]; gap < 0 || gap >= frame {
				t.Fatalf("first sound %.4fs after the first picture, want within one audio frame (%v)", gap, first)
			}
			if k := first["soun"] / frame; math.Abs(k-math.Round(k)) > 1e-6 {
				t.Fatalf("first sound at %.4fs is off its frame grid (%v)", first["soun"], first)
			}
			playlist, err := os.ReadFile(filepath.Join(out, "index.m3u8"))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, line := range strings.Split(string(playlist), "\n") {
				if strings.HasPrefix(line, "seg") {
					names = append(names, line)
				}
			}
			// The rewritten first fragment must still decode, sound and picture.
			whole := append([]byte{}, init...)
			for _, n := range names {
				b, err := os.ReadFile(filepath.Join(out, n))
				if err != nil {
					t.Fatal(err)
				}
				whole = append(whole, b...)
			}
			dec := exec.Command(ffmpeg, "-hide_banner", "-v", "error", "-i", "pipe:0", "-f", "null", "-")
			dec.Stdin = bytes.NewReader(whole)
			if msg, err := dec.CombinedOutput(); err != nil || len(bytes.TrimSpace(msg)) > 0 {
				t.Fatalf("segments do not decode cleanly: %v %s", err, msg)
			}
			last := starts(names[len(names)-2])
			// A later segment's tracks start within one audio frame and one
			// picture of each other, not a second apart.
			if gap := last["vide"] - last["soun"]; math.Abs(gap) > 0.1 {
				t.Fatalf("%s: tracks %.3fs apart (%v)", names[len(names)-2], gap, last)
			}
		})
	}
}

func elstBox(version byte, entries [][2]int64) []byte {
	b := []byte{version, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[4:8], uint32(len(entries)))
	for _, e := range entries {
		if version == 1 {
			b = binary.BigEndian.AppendUint64(b, uint64(e[0]))
			b = binary.BigEndian.AppendUint64(b, uint64(e[1]))
		} else {
			b = binary.BigEndian.AppendUint32(b, uint32(e[0]))
			b = binary.BigEndian.AppendUint32(b, uint32(int32(e[1])))
		}
		b = append(b, 0, 1, 0, 0)
	}
	return mp4Box("elst", b)
}

// trakBox is a track with a v1 tkhd and mdhd, and an edit list when elst is set.
func trakBox(id, scale uint32, handler string, elst []byte) []byte {
	tkhd := make([]byte, 36)
	tkhd[0] = 1
	binary.BigEndian.PutUint32(tkhd[20:24], id)
	mdhd := make([]byte, 36)
	mdhd[0] = 1
	binary.BigEndian.PutUint32(mdhd[20:24], scale)
	hdlr := make([]byte, 12)
	copy(hdlr[8:12], handler)
	body := mp4Box("tkhd", tkhd)
	if elst != nil {
		body = append(body, mp4Box("edts", elst)...)
	}
	return mp4Box("trak", append(body, mp4Box("mdia", append(mp4Box("mdhd", mdhd), mp4Box("hdlr", hdlr)...))...))
}

func initWith(traks ...[]byte) []byte {
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	moov := mp4Box("mvhd", mvhd)
	for _, t := range traks {
		moov = append(moov, t...)
	}
	return append(mp4Box("ftyp", []byte("isom")), mp4Box("moov", append(moov, mp4Box("mvex", make([]byte, 8))...))...)
}

func TestFlattenEditsMovesEachTrackToItsStart(t *testing.T) {
	video := trakBox(1, 90000, "vide", elstBox(1, [][2]int64{{72448376, -1}, {0, 1501}}))
	// Two empty edits in a v0 list add up.
	audio := trakBox(2, 48000, "soun", elstBox(0, [][2]int64{{72447000, -1}, {910, -1}, {0, 1024}}))
	out, shifts, first := flattenEdits(initWith(video, audio))
	if bytes.Contains(out, []byte("edts")) || bytes.Contains(out, []byte("elst")) {
		t.Fatal("edit lists survived")
	}
	for _, kind := range []string{"ftyp", "mvhd", "mvex", "mdhd", "hdlr"} {
		if !bytes.Contains(out, []byte(kind)) {
			t.Fatalf("%s was dropped", kind)
		}
	}
	if id, scale, ok := videoTrack(out); !ok || id != 1 || scale != 90000 {
		t.Fatalf("video track after flattening: %d %d %v", id, scale, ok)
	}
	// Video is shown at 72448.376 - 1501/90000; audio at 72447.910 - 1024/48000.
	videoAt := 72448.376 - 1501.0/90000
	audioAt := 72447.910 - 1024.0/48000
	want := map[uint32]int64{1: int64(math.Round((videoAt - audioAt) * 90000)), 2: 0}
	if len(shifts) != 2 || shifts[1] != want[1] || shifts[2] != want[2] {
		t.Fatalf("shifts %v, want %v", shifts, want)
	}
	if math.Abs(first-audioAt) > 1e-9 {
		t.Fatalf("fragment time zero is broadcast %f, want %f", first, audioAt)
	}

	frag := keyframeFragment(1000, 1501)
	shiftTracks(frag, shifts)
	if pts, ok := fragmentPTS(frag, 1, 90000); !ok || pts != 1000+want[1] {
		t.Fatalf("shifted pts %d, want %d", pts, 1000+want[1])
	}
}

func TestFlattenEditsLeavesOddInitsAlone(t *testing.T) {
	cases := map[string][]byte{
		"a track without an edit list": initWith(
			trakBox(1, 90000, "vide", elstBox(1, [][2]int64{{72448376, -1}, {0, 0}})),
			trakBox(2, 48000, "soun", nil)),
		"tracks hours apart": initWith(
			trakBox(1, 90000, "vide", elstBox(1, [][2]int64{{72448376, -1}, {0, 0}})),
			trakBox(2, 48000, "soun", elstBox(1, [][2]int64{{1000, -1}, {0, 0}}))),
	}
	for name, init := range cases {
		out, shifts, _ := flattenEdits(init)
		if !bytes.Equal(out, init) || shifts != nil {
			t.Errorf("%s: changed the init (shifts %v)", name, shifts)
		}
	}
	one := initWith(trakBox(1, 90000, "vide", elstBox(1, [][2]int64{{72448376, -1}, {0, 1501}})))
	out, shifts, _ := flattenEdits(one)
	if bytes.Contains(out, []byte("edts")) || shifts[1] != 0 {
		t.Errorf("a single track: shifts %v", shifts)
	}
}

// testRun is one traf of a synthetic fragment: its samples' durations and
// payloads, and the flags ffmpeg would write.
type testRun struct {
	id     uint32
	tfdt   uint64
	tfdtV0 bool
	cts    uint32
	durs   []uint32
	data   [][]byte
	// flags, when set, are written per sample.
	flags      []uint32
	tfhdFlags  uint32
	noOffset   bool
	extraChild bool
}

// testFragment lays the runs' data out in mdat in the order given by order.
func testFragment(runs []testRun, order []int) []byte {
	build := func(offsets []int) []byte {
		body := mp4Box("mfhd", make([]byte, 8))
		for i, r := range runs {
			tfhd := []byte{0, byte(r.tfhdFlags >> 16), byte(r.tfhdFlags >> 8), byte(r.tfhdFlags)}
			tfhd = binary.BigEndian.AppendUint32(tfhd, r.id)
			var tfdt []byte
			if r.tfdtV0 {
				tfdt = binary.BigEndian.AppendUint32([]byte{0, 0, 0, 0}, uint32(r.tfdt))
			} else {
				tfdt = binary.BigEndian.AppendUint64([]byte{1, 0, 0, 0}, r.tfdt)
			}
			flags := uint32(0x100 | 0x200 | 0x800 | 0x1)
			if r.noOffset {
				flags &^= 0x1
			}
			if r.flags != nil {
				flags |= 0x400
			}
			trun := []byte{0, byte(flags >> 16), byte(flags >> 8), byte(flags)}
			trun = binary.BigEndian.AppendUint32(trun, uint32(len(r.durs)))
			if !r.noOffset {
				trun = binary.BigEndian.AppendUint32(trun, uint32(offsets[i]))
			}
			for j := range r.durs {
				cts := uint32(0)
				if j == 0 {
					cts = r.cts
				}
				trun = binary.BigEndian.AppendUint32(trun, r.durs[j])
				trun = binary.BigEndian.AppendUint32(trun, uint32(len(r.data[j])))
				if r.flags != nil {
					trun = binary.BigEndian.AppendUint32(trun, r.flags[j])
				}
				trun = binary.BigEndian.AppendUint32(trun, cts)
			}
			traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
			if r.extraChild {
				traf = append(traf, mp4Box("sbgp", make([]byte, 8))...)
			}
			body = append(body, mp4Box("traf", traf)...)
		}
		return mp4Box("moof", body)
	}
	moofLen := len(build(make([]int, len(runs))))
	offsets := make([]int, len(runs))
	var mdat []byte
	for _, i := range order {
		offsets[i] = moofLen + 8 + len(mdat)
		for _, d := range runs[i].data {
			mdat = append(mdat, d...)
		}
	}
	return append(build(offsets), mp4Box("mdat", mdat)...)
}

// runData reads back each traf's decode time and the bytes its trun points at.
func runData(t *testing.T, frag []byte) map[uint32][2]any {
	t.Helper()
	out := map[uint32][2]any{}
	for _, b := range boxes(child(frag, "moof")) {
		if b.kind != "traf" {
			continue
		}
		r, ok := parseRun(b.body)
		if !ok {
			t.Fatal("unreadable traf")
		}
		var size int64
		for _, s := range r.samples {
			size += r.sampleSize(s)
		}
		if r.dataOff < 0 || r.dataOff+size > int64(len(frag)) {
			t.Fatalf("track %d points outside the fragment", r.id)
		}
		out[r.id] = [2]any{r.decodeTime(), string(frag[r.dataOff : r.dataOff+size])}
	}
	return out
}

func TestTrimLeadInDropsSoundBeforeThePicture(t *testing.T) {
	const moofBase = 0x20000 | 0x38
	scales := map[uint32]uint32{1: 90000, 2: 48000}
	// The picture is shown at 1 s (sample 48000 at 48 kHz). Sound frames
	// start at 46464, 48000 and 49536; only the first is before the picture.
	for name, order := range map[string][]int{"video data first": {0, 1}, "sound data first": {1, 0}} {
		for _, v0 := range []bool{false, true} {
			runs := []testRun{
				{id: 1, tfdt: 87000, cts: 3000, durs: []uint32{1501, 1501}, data: [][]byte{[]byte("VVVV"), []byte("WW")}, tfhdFlags: moofBase},
				{id: 2, tfdt: 48000 - 1536, tfdtV0: v0, durs: []uint32{1536, 1536, 1536}, data: [][]byte{[]byte("aaa"), []byte("bbbbb"), []byte("c")}, tfhdFlags: moofBase},
			}
			in := testFragment(runs, order)
			out := trimLeadIn(in, 1, scales)
			got := runData(t, out)
			if got[1] != [2]any{int64(87000), "VVVVWW"} {
				t.Errorf("%s v0=%v: picture changed: %v", name, v0, got[1])
			}
			if got[2] != [2]any{int64(48000), "bbbbbc"} {
				t.Errorf("%s v0=%v: sound = %v, want the frames from the picture on", name, v0, got[2])
			}
			if len(out) != len(in)-3-12 {
				t.Errorf("%s v0=%v: fragment is %d bytes, want %d", name, v0, len(out), len(in)-15)
			}
		}
	}
}

// lateSound is a first fragment as delay_moov writes it when the sound
// starts late: groups of 30 frames from 0 s and sound from soundAt (48 kHz).
func lateSound(groups int, soundAt uint64) []testRun {
	v := testRun{id: 1, tfhdFlags: 0x20000 | 0x38}
	for g := 0; g < groups; g++ {
		for f := 0; f < 30; f++ {
			flags := uint32(0x10000)
			if f == 0 {
				flags = 0
			}
			v.durs = append(v.durs, 3003)
			v.flags = append(v.flags, flags)
			v.data = append(v.data, []byte{byte('A' + g)})
		}
	}
	return []testRun{v, {id: 2, tfdt: soundAt, durs: []uint32{1536, 1536}, data: [][]byte{[]byte("s"), []byte("t")}, tfhdFlags: 0x20000 | 0x38}}
}

func TestTrimLeadInDropsPictureGroupsBeforeTheSound(t *testing.T) {
	scales := map[uint32]uint32{1: 90000, 2: 48000}
	for _, c := range []struct {
		name    string
		soundAt uint64
		keep    int64 // first kept group
	}{
		{"sound in the last group", 172800, 3},  // 3.6 s
		{"sound in the third group", 120000, 2}, // 2.5 s
		{"sound on a group's first frame", 48048 * 2, 2},
		{"sound in the first group", 24000, 0},
	} {
		for name, order := range map[string][]int{"video data first": {0, 1}, "sound data first": {1, 0}} {
			in := testFragment(lateSound(4, c.soundAt), order)
			got := runData(t, trimLeadIn(in, 1, scales))
			want := ""
			for g := c.keep; g < 4; g++ {
				want += strings.Repeat(string(rune('A'+g)), 30)
			}
			if got[1] != [2]any{c.keep * 90090, want} {
				t.Errorf("%s, %s: picture %v, want from group %d", c.name, name, got[1], c.keep)
			}
			if got[2] != [2]any{int64(c.soundAt), "st"} {
				t.Errorf("%s, %s: sound changed: %v", c.name, name, got[2])
			}
		}
	}
	// Without per-sample flags the later groups are unknown.
	runs := lateSound(4, 172800)
	runs[0].flags = nil
	in := testFragment(runs, []int{0, 1})
	if out := trimLeadIn(in, 1, scales); !bytes.Equal(out, in) {
		t.Error("a fragment without sample flags changed")
	}
}

// One long first segment set the playlist's target duration, and AVPlayer's
// hold-back with it, for the whole 90 min it stayed listed.
func TestLateSoundLeavesTheFirstSegmentOneGroupLong(t *testing.T) {
	dir := t.TempDir()
	raw := initWith(trakBox(1, 90000, "vide", nil), trakBox(2, 48000, "soun", nil))
	raw = append(raw, testFragment(lateSound(4, 172800), []int{0, 1})...)
	for g := int64(4); g < 8; g++ {
		runs := lateSound(1, uint64(g*48048))
		runs[0].tfdt = uint64(g * 90090)
		raw = append(raw, testFragment(runs, []int{0, 1})...)
	}
	if err := Pack(dir, bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("#EXT-X-TARGETDURATION:2\n")) || !bytes.Contains(b, []byte("HOLD-BACK=6.000")) {
		t.Errorf("want a 2 s target and 6 s hold-back:\n%s", b)
	}
	if first := strings.SplitN(strings.SplitN(string(b), "#EXTINF:", 2)[1], ",", 2)[0]; first != "1.001" {
		t.Errorf("first segment %s s, want 1.001", first)
	}
}

func TestTrimLeadInLeavesOtherLayoutsAlone(t *testing.T) {
	const moofBase = 0x20000 | 0x38
	scales := map[uint32]uint32{1: 90000, 2: 48000}
	base := func() []testRun {
		return []testRun{
			{id: 1, tfdt: 90000, durs: []uint32{1501}, data: [][]byte{[]byte("VV")}, tfhdFlags: moofBase},
			{id: 2, tfdt: 0, durs: []uint32{48000, 1536}, data: [][]byte{[]byte("a"), []byte("b")}, tfhdFlags: moofBase},
		}
	}
	cases := map[string]func(r []testRun){
		"sound already after the picture": func(r []testRun) { r[1].tfdt = 48000 },
		"no data offset":                  func(r []testRun) { r[1].noOffset = true },
		"offsets not from the moof":       func(r []testRun) { r[1].tfhdFlags = 0x38 },
		"a sample group":                  func(r []testRun) { r[1].extraChild = true },
	}
	for name, change := range cases {
		runs := base()
		change(runs)
		in := testFragment(runs, []int{0, 1})
		if out := trimLeadIn(in, 1, scales); !bytes.Equal(out, in) {
			t.Errorf("%s: the fragment changed", name)
		}
	}
}

func TestPackInputMovesToTheNextEncode(t *testing.T) {
	pipe := func(body string) *packPipe {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			_, _ = w.WriteString(body)
			_ = w.Close()
		}()
		// started closes this second write end, as it does for ffmpeg's.
		_, extra, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		return &packPipe{File: r, w: extra}
	}
	in := &packInput{cur: pipe("first")}
	if !in.follow(pipe("second")) {
		t.Fatal("follow refused before close")
	}
	var got []string
	buf := make([]byte, 64)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			got = append(got, string(buf[:n]))
		}
		if errors.Is(err, errNextEncode) {
			got = append(got, "|")
			continue
		}
		if err != nil {
			break
		}
	}
	if strings.Join(got, "") != "first|second" {
		t.Fatalf("read %q", got)
	}
	in.close()
	if in.follow(pipe("late")) {
		t.Fatal("follow accepted after the packager stopped")
	}
}

// gopFragment is one group of pictures as a broadcast copy has it: n frames
// decoded from dts. An open group shows the two B-frames decoded after the
// keyframe before it.
func gopFragment(dts int64, n int, open bool) []byte {
	const d = 3003
	tfhd := make([]byte, 8)
	tfhd[1] = 0x02 // default-base-is-moof
	binary.BigEndian.PutUint32(tfhd[4:8], 1)
	tfdt := make([]byte, 12)
	tfdt[0] = 1
	binary.BigEndian.PutUint64(tfdt[4:12], uint64(dts))
	trun := make([]byte, 16, 16+n*8)
	trun[0] = 1                   // signed composition offsets
	trun[2], trun[3] = 0x09, 0x05 // data offset, first-sample flags, duration, composition offset
	binary.BigEndian.PutUint32(trun[4:8], uint32(n))
	binary.BigEndian.PutUint32(trun[12:16], 0x02000000) // a keyframe
	for i := 0; i < n; i++ {
		var cts int32
		if open && i == 0 {
			cts = 2 * d
		} else if open && i < 3 {
			cts = -d
		}
		trun = binary.BigEndian.AppendUint32(trun, d)
		trun = binary.BigEndian.AppendUint32(trun, uint32(cts))
	}
	traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
	moof := mp4Box("moof", append(mp4Box("mfhd", make([]byte, 8)), mp4Box("traf", traf)...))
	return append(moof, mp4Box("mdat", []byte{0})...)
}

// A copy of a broadcast with open groups and a scene cut now and then (5.1,
// measured): the dates stay on the timestamps instead of gaining two frames at
// every closed group.
func TestOpenGroupsKeepTheirDates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	groups := []struct {
		frames int
		open   bool
	}{{30, true}, {16, true}, {28, false}, {30, true}, {30, true}, {18, true}, {28, false}, {30, true}}
	var list strings.Builder
	list.WriteString("#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n")
	dts := int64(900000)
	for i, g := range groups {
		name := fmt.Sprintf("seg%05d.m4s", i)
		if err := os.WriteFile(filepath.Join(dir, name), gopFragment(dts, g.frames, g.open), 0o644); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&list, "#EXTINF:%.3f,\n%s\n", float64(g.frames*3003)/90000, name)
		dts += int64(g.frames * 3003)
	}
	tl := NewTimeline()
	fixed := time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	stamped := string(p.stamp(dir, []byte(list.String()), tl))
	var dates []time.Time
	for _, line := range strings.Split(stamped, "\n") {
		if v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); ok {
			at, _ := time.Parse("2006-01-02T15:04:05.000Z", v)
			dates = append(dates, at)
		}
	}
	if len(dates) != len(groups) {
		t.Fatalf("%d dates for %d segments:\n%s", len(dates), len(groups), stamped)
	}
	frames := 0
	for i, g := range groups {
		want := dates[0].Add(time.Duration(frames*3003) * time.Second / 90000)
		if d := dates[i].Sub(want); d > 2*time.Millisecond || d < -2*time.Millisecond {
			t.Errorf("segment %d is dated %v, %v off its timestamps", i, dates[i], d)
		}
		frames += g.frames
	}
}

// A short group held until the next keyframe and then closed with it is one
// segment of both groups' frames. Its length used to run from keyframe to
// keyframe, two frames long when the next group was open, and every such
// segment pushed the channel's dates two frames ahead.
func TestAHeldGroupSegmentIsAsLongAsItsFrames(t *testing.T) {
	dir := t.TempDir()
	raw := videoInit()
	dts := int64(900000)
	for _, g := range []struct {
		frames int
		open   bool
	}{{6, false}, {28, true}, {30, true}, {30, true}} {
		raw = append(raw, gopFragment(dts, g.frames, g.open)...)
		dts += int64(g.frames * 3003)
	}
	if err := Pack(dir, bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	var lens []string
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			lens = append(lens, strings.TrimSuffix(v, ","))
		}
	}
	if len(lens) < 2 || lens[0] != "1.134" || lens[1] != "1.001" {
		t.Fatalf("segment lengths %v, want 1.134 (6 + 28 frames) then 1.001:\n%s", lens, b)
	}
}

// encodeFeed hands Pack each encode's output in turn, as packInput does.
type encodeFeed struct {
	parts  [][]byte
	firsts []int
}

func (e *encodeFeed) Read(b []byte) (int, error) {
	if len(e.parts) == 0 {
		return 0, io.EOF
	}
	p := e.parts[0]
	e.parts = e.parts[1:]
	if p == nil {
		return 0, errNextEncode
	}
	return copy(b, p), nil
}

func (e *encodeFeed) noteEncodeStart(float64, bool) {}

func (e *encodeFeed) noteFirstSegment(seq int) { e.firsts = append(e.firsts, seq) }

func TestPackTellsWhereEachEncodeStarts(t *testing.T) {
	first := videoInit()
	for _, pts := range []int64{0, 90000, 180000} {
		first = append(first, keyframeFragment(pts, 90000)...)
	}
	second := videoInit()
	for _, pts := range []int64{0, 90000} {
		second = append(second, keyframeFragment(pts, 90000)...)
	}
	feed := &encodeFeed{parts: [][]byte{first, nil, second}}
	if err := Pack(t.TempDir(), feed, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(feed.firsts) != "[0 3]" {
		t.Fatalf("first segments %v, want [0 3]", feed.firsts)
	}
}

// avFragment is a keyframe fragment of track 1 lasting dur ticks, with one
// sound sample on track 2 when sound is set.
func avFragment(pts int64, dur uint32, sound bool) []byte {
	traf := func(id uint32, trFlags uint32, run []byte) []byte {
		tfhd := binary.BigEndian.AppendUint32(make([]byte, 4), id)
		tfdt := binary.BigEndian.AppendUint64([]byte{1, 0, 0, 0}, uint64(pts))
		trun := []byte{0, byte(trFlags >> 16), byte(trFlags >> 8), byte(trFlags)}
		trun = binary.BigEndian.AppendUint32(trun, 1)
		trun = append(trun, run...)
		return mp4Box("traf", append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...))
	}
	video := binary.BigEndian.AppendUint32(make([]byte, 4), dur) // first-sample flags 0 (sync), duration
	body := append(mp4Box("mfhd", make([]byte, 8)), traf(1, 0x104, video)...)
	if sound {
		body = append(body, traf(2, 0x100, binary.BigEndian.AppendUint32(nil, 2880))...)
	}
	return append(mp4Box("moof", body), mp4Box("mdat", []byte{0})...)
}

// An interlaced broadcast can start each group with a one-field keyframe
// fragment (17 ms) that carries no sound. Published alone, its sound view is
// an empty part and AVPlayer fails the whole master. Live, it is held until
// the next fragment arrives and goes out in the same part.
func TestNoPartIsOneField(t *testing.T) {
	dir := t.TempDir()
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(dir, pr, nil) }()
	write := func(b []byte) {
		t.Helper()
		if _, err := pw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	write(videoInit())
	const field, group = 1500, 90000
	for g := range int64(3) {
		write(avFragment(g*group, field, false))
		write(avFragment(g*group+field, group-field, true))
		name := fmt.Sprintf("seg%05d.m4s", g)
		playlist := waitPlaylist(t, dir, name)
		if strings.Contains(playlist, "DURATION=0.017") || strings.Contains(playlist, "#EXTINF:0.017") {
			t.Fatalf("group %d: the field was listed alone:\n%s", g, playlist)
		}
		seg, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if n := soundSamples(seg); n != 1 {
			t.Fatalf("group %d: segment has %d sound samples", g, n)
		}
	}
	write(avFragment(3*group, field, false))
	write(avFragment(3*group+field, 20000, true))
	playlist := waitPlaylist(t, dir, `URI="part00003.m4s"`)
	if !strings.Contains(playlist, "#EXT-X-PART:DURATION=0.239,INDEPENDENT=YES,URI=\"part00003.m4s\"") {
		t.Fatalf("the field and the frames after it should be one 0.239 s part:\n%s", playlist)
	}
	part, err := os.ReadFile(filepath.Join(dir, "part00003.m4s"))
	if err != nil {
		t.Fatal(err)
	}
	if got, n := len(topBoxes(part)), soundSamples(part); got != 4 || n != 1 {
		t.Fatalf("part has %d boxes and %d sound samples, want two moof/mdat pairs and one", got, n)
	}
	_ = pw.Close()
	if err := <-packErr; err != nil {
		t.Fatal(err)
	}
}

// soundSamples counts track 2's samples in every fragment of b.
func soundSamples(b []byte) int {
	n := 0
	for _, box := range topBoxes(b) {
		if string(box[4:8]) != "moof" {
			continue
		}
		for _, tr := range boxes(box[8:]) {
			tfhd, trun := child(tr.body, "tfhd"), child(tr.body, "trun")
			if tr.kind == "traf" && len(tfhd) >= 8 && binary.BigEndian.Uint32(tfhd[4:8]) == 2 && len(trun) >= 8 {
				n += int(binary.BigEndian.Uint32(trun[4:8]))
			}
		}
	}
	return n
}

// A blocking reload is answered with the segment it asked for. A copied
// broadcast is cut at its own keyframes, so the next segment can take longer
// than 1.5 s; answering without it made AVPlayer drop the stream's dates and
// pause itself.
func TestABlockingReloadWaitsForItsSegment(t *testing.T) {
	gate := newPlaylistGate()
	closed := []packedSeg{{name: "seg00000.m4s", dur: 225000}} // 2.5 s
	if err := writePacked(t.TempDir(), []byte("init"), closed, nil, 0, true, false, nil, gate); err != nil {
		t.Fatal(err)
	}
	if got := gate.holdFor(); got != 9*time.Second {
		t.Fatalf("hold %v for a 3 s target, want 9 s", got)
	}
	go func() {
		time.Sleep(1800 * time.Millisecond)
		gate.publish(0, 2, 0)
	}()
	began := time.Now()
	gate.wait(1, -1, gate.holdFor())
	waited := time.Since(began)
	gate.mu.Lock()
	ready := gate.ready(1, -1)
	gate.mu.Unlock()
	if !ready || waited < 1700*time.Millisecond || waited > 3*time.Second {
		t.Fatalf("answered after %v with segment 1 listed %v", waited, ready)
	}
}

// A blocking request that comes before the first playlist still gets three
// targets once the target is known, not the floor.
func TestABlockingReloadBeforeTheFirstPlaylistWaitsTheFullHold(t *testing.T) {
	gate := newPlaylistGate()
	go func() {
		time.Sleep(100 * time.Millisecond)
		gate.target.Store(int64(2 * time.Second))
		gate.publish(0, 1, 0)
		time.Sleep(2 * time.Second)
		gate.publish(0, 2, 0)
	}()
	began := time.Now()
	gate.block(1, -1)
	waited := time.Since(began)
	gate.mu.Lock()
	ready := gate.ready(1, -1)
	gate.mu.Unlock()
	if !ready || waited < 2*time.Second {
		t.Fatalf("answered after %v with segment 1 listed %v", waited, ready)
	}
}

// A closed segment keeps its parts in the list while it is within three
// target durations of the end, and its part files a while longer, so a
// player that was playing them finds them when the segment closes.
func TestARecentSegmentKeepsItsParts(t *testing.T) {
	dir := t.TempDir()
	parts := func(names ...string) []partRef {
		out := make([]partRef, len(names))
		for i, n := range names {
			out[i] = partRef{name: n, dur: 45045, sync: true}
		}
		return out
	}
	// Ten one-second segments under a 2 s target: seg00000 ends 9.5 s from
	// the end, past three targets (6 s).
	var closed []packedSeg
	for i := range 9 {
		closed = append(closed, packedSeg{name: fmt.Sprintf("seg%05d.m4s", i), dur: 90090, parts: parts(fmt.Sprintf("part%05d.m4s", i))})
	}
	closed = append(closed, packedSeg{name: "seg00009.m4s", dur: 90090, parts: parts("part00010.m4s", "part00011.m4s")})
	open := []packedPart{{name: "part00012.m4s", dur: 45045, sync: true}}
	if err := writePacked(dir, []byte("init"), closed, open, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "part00000.m4s") || !strings.Contains(text, "part00005.m4s") {
		t.Fatalf("parts listed for the wrong segments:\n%s", text)
	}
	want := "#EXT-X-PART:DURATION=0.500,INDEPENDENT=YES,URI=\"part00010.m4s\"\n#EXT-X-PART:DURATION=0.500,INDEPENDENT=YES,URI=\"part00011.m4s\"\n#EXTINF:1.001,\nseg00009.m4s\n"
	if !strings.Contains(text, want) {
		t.Fatalf("the last segment does not list its parts before it:\n%s", text)
	}
	if listed, kept := partsKept(int64(7*90000), 2*90000); listed || !kept {
		t.Fatalf("7 s from the end of a 2 s target: listed %v kept %v, want false true", listed, kept)
	}
}

// A part past the last one of a closed segment is the next segment's first
// part, so a blocking reload for it waits for that part.
func TestAPartPastAClosedSegmentWaitsForTheNext(t *testing.T) {
	g := newPlaylistGate()
	// Segments 0-2 closed with 1, 3, and 1 parts; segment 3 open, empty.
	g.publish(0, 3, 0, 1, 3, 1)
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, c := range []struct {
		msn, part int
		want      bool
	}{
		{1, 2, true},  // the third of segment 1's three parts
		{1, 3, true},  // past segment 1: segment 2's first part
		{2, 0, true},  // listed
		{2, 3, false}, // past segment 2: segment 3's first part, not out yet
		{2, -1, true}, // the whole segment 2
		{3, -1, false},
	} {
		if got := g.ready(c.msn, c.part); got != c.want {
			t.Errorf("ready(%d, %d) = %v, want %v", c.msn, c.part, got, c.want)
		}
	}
	g.openParts = 1
	if !g.ready(2, 3) {
		t.Error("segment 3's first part is out")
	}
}

// A source can key twice within a few frames, so a group starts with a short
// keyframe fragment (a tile: 5 frames, 0.167 s) and the rest of the group
// follows as a second one. The segment ends with that second fragment; it
// used to wait for the next group's keyframe, and a tile near the live edge
// ran out of picture while it did.
func TestAGroupAfterAShortKeyframeClosesWhenItArrives(t *testing.T) {
	dir := t.TempDir()
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(dir, pr, nil) }()
	write := func(b []byte) {
		t.Helper()
		if _, err := pw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	write(videoInit())
	write(avFragment(0, 90000, true))
	waitPlaylist(t, dir, "seg00000.m4s")
	write(avFragment(90000, 15015, true))
	write(avFragment(105015, 249000, true))
	playlist := waitPlaylist(t, dir, "seg00001.m4s")
	if !strings.Contains(playlist, "#EXTINF:2.933,\nseg00001.m4s") {
		t.Fatalf("segment 1 should be both fragments, 2.933 s:\n%s", playlist)
	}
	write(avFragment(354015, 90000, true))
	playlist = waitPlaylist(t, dir, "seg00002.m4s")
	if !strings.Contains(playlist, "#EXTINF:1.000,\nseg00002.m4s") {
		t.Fatalf("segment 2 should start on the next group:\n%s", playlist)
	}
	_ = pw.Close()
	if err := <-packErr; err != nil {
		t.Fatal(err)
	}
}
