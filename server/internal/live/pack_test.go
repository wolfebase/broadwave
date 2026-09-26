package live

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

func TestPackListsAPartBeforeTheSegmentCloses(t *testing.T) {
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
		if err == nil && strings.Contains(string(b), "#EXT-X-PART:") {
			playlist = string(b)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if playlist == "" {
		t.Fatal("no part was listed before the rest of the stream")
	}
	if strings.Contains(playlist, "#EXTINF") {
		t.Fatalf("the first fragment closed a segment:\n%s", playlist)
	}
	partDur := 0.0
	for _, line := range strings.Split(playlist, "\n") {
		v, ok := strings.CutPrefix(line, "#EXT-X-PART:DURATION=")
		if !ok {
			continue
		}
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		partDur, err = strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	// This source's group of pictures is two seconds. Advertising 0.500 would
	// make the player treat the whole fragment as half a second.
	hold := 0.0
	if i := strings.Index(playlist, "PART-HOLD-BACK="); i >= 0 {
		_, _ = fmt.Sscanf(playlist[i+len("PART-HOLD-BACK="):], "%f", &hold)
	}
	if partDur < 1.5 || hold+0.001 < partDur*3 || !strings.Contains(playlist, "CAN-BLOCK-RELOAD=YES") {
		t.Fatalf("part %.3f hold %.3f, playlist:\n%s", partDur, hold, playlist)
	}
	fixed := time.Date(2026, 9, 26, 4, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var stamper playlistStamper
	stamped := string(stamper.stamp(out, []byte(playlist), tl))
	if !strings.Contains(stamped, "#EXT-X-PART:") {
		t.Fatalf("stamping dropped the part:\n%s", stamped)
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
	if _, err := os.Stat(filepath.Join(longDir, "seg00000.m4s")); !os.IsNotExist(err) {
		t.Fatalf("segment past 90 minutes still on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(longDir, "seg00003.m4s")); err != nil {
		t.Fatal(err)
	}
}

func headTail(s string) string {
	if len(s) < 400 {
		return s
	}
	return s[:200] + "\n...\n" + s[len(s)-200:]
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
	short := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:0.500,\nseg00000.m4s\n"
	if got := string(DeltaPlaylist([]byte(short))); got != short {
		t.Fatalf("a short playlist is unchanged:\n%s", got)
	}
}
