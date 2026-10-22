package live

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// packSplit runs the packager over fragments as a split encode writes them.
func packSplit(t *testing.T, dir string, frags ...[]byte) string {
	t.Helper()
	raw := videoInit()
	for _, f := range frags {
		raw = append(raw, f...)
	}
	gate := newPlaylistGate()
	gate.split = true
	if err := Pack(dir, bytes.NewReader(raw), gate); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type listedSeg struct {
	dur   float64
	parts []string // part lines, in order
}

func listedSegments(t *testing.T, playlist string) []listedSeg {
	t.Helper()
	var out []listedSeg
	var parts []string
	for _, line := range strings.Split(playlist, "\n") {
		if strings.HasPrefix(line, "#EXT-X-PART:") {
			parts = append(parts, line)
		}
		if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			d, err := strconv.ParseFloat(strings.TrimSuffix(v, ","), 64)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, listedSeg{dur: d, parts: parts})
			parts = nil
		}
	}
	return out
}

// pictures is a split encode's output: a fragment per picture, each group of
// pictures starting on a keyframe. groups are the pictures in each group.
func pictures(pts int64, frame uint32, groups ...int) [][]byte {
	var out [][]byte
	for _, n := range groups {
		for i := range n {
			flags := uint32(0x10000)
			if i == 0 {
				flags = 0
			}
			out = append(out, fragmentAt(pts, frame, flags, 0))
			pts += int64(frame)
		}
	}
	return out
}

// assertParts checks what AVPlayer checks: no part over PART-TARGET, no part
// but a segment's last under 85% of it, and every segment starting on an
// independent part.
func assertParts(t *testing.T, text string) []listedSeg {
	t.Helper()
	dur := regexp.MustCompile(`DURATION=([0-9.]+)`)
	segs := listedSegments(t, text)
	for i, s := range segs {
		for j, p := range s.parts {
			d, _ := strconv.ParseFloat(dur.FindStringSubmatch(p)[1], 64)
			if d > 0.390 || (j < len(s.parts)-1 && d < 0.3315) {
				t.Fatalf("segment %d part %d is %.3f s:\n%s", i, j, d, text)
			}
			if j == 0 && !strings.Contains(p, "INDEPENDENT=YES") {
				t.Fatalf("segment %d starts on a part that is not independent:\n%s", i, text)
			}
		}
	}
	return segs
}

// One-second groups at 29.97 make three parts of ten pictures each once the
// packager has seen a group; the playlist advertises the short part target.
func TestSplitEncodePublishesPartsInsideAGroup(t *testing.T) {
	text := packSplit(t, t.TempDir(), pictures(900000, 3003, 30, 30, 30, 30, 1)...)
	for _, tag := range []string{"PART-TARGET=0.390", "PART-HOLD-BACK=1.170", "HOLD-BACK=6.000", "#EXT-X-TARGETDURATION:2\n"} {
		if !strings.Contains(text, tag) {
			t.Fatalf("missing %s:\n%s", tag, text)
		}
	}
	segs := assertParts(t, text)
	if len(segs) != 5 {
		t.Fatalf("want one segment per group:\n%s", text)
	}
	for i, s := range segs[:4] {
		if s.dur != 1.001 || len(s.parts) != 3 {
			t.Fatalf("segment %d: %.3f s, %d parts:\n%s", i, s.dur, len(s.parts), text)
		}
		if i > 0 && strings.Count(strings.Join(s.parts, "\n"), "DURATION=0.334") != 3 {
			t.Fatalf("segment %d is not three even parts:\n%s", i, text)
		}
	}
}

// Broadcast groups are not even: a splice sends a 0.3 s group, a 3.0 station
// keys at 0.17 s now and then. A group under half a second shares a segment
// with the next, and no short part may be left inside it.
func TestSplitEncodeKeepsPartsLegalOnUnevenGroups(t *testing.T) {
	cases := map[string][]int{
		"short group":        {60, 18, 60, 60, 1},
		"very short group":   {60, 10, 60, 1},
		"group of 0.8 s":     {48, 48, 48, 1},
		"0.45 s group":       {60, 27, 60, 1},
		"field-doubled keys": {1, 60, 1, 60, 1},
		"long group":         {150, 60, 1},
	}
	for name, groups := range cases {
		t.Run(name, func(t *testing.T) {
			text := packSplit(t, t.TempDir(), pictures(900000, 1501, groups...)...)
			assertParts(t, text)
		})
	}
}

// reorderedAt is one picture of a split encode with B-frames: decoded at dec,
// shown cto later.
func reorderedAt(dec int64, dur, flags, cto uint32) []byte {
	tfhd := make([]byte, 8)
	binary.BigEndian.PutUint32(tfhd[4:8], 1)
	tfdt := make([]byte, 12)
	tfdt[0] = 1
	binary.BigEndian.PutUint64(tfdt[4:12], uint64(dec))
	const trFlags = 0x904 // first-sample flags, sample duration, composition offset
	trun := make([]byte, 20)
	binary.BigEndian.PutUint32(trun[0:4], trFlags)
	binary.BigEndian.PutUint32(trun[4:8], 1)
	binary.BigEndian.PutUint32(trun[8:12], flags)
	binary.BigEndian.PutUint32(trun[12:16], dur)
	binary.BigEndian.PutUint32(trun[16:20], cto)
	traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
	moof := mp4Box("moof", append(mp4Box("mfhd", make([]byte, 8)), mp4Box("traf", traf)...))
	return append(moof, mp4Box("mdat", nil)...)
}

// A 3.0 station (HEVC, 60 fps, two-second groups) sends a second keyframe
// half a second into a group now and then, with a picture after it that is
// shown before it. The keyframe is half a second in, the picture before it is
// not: the short part before the keyframe must still end its segment, or
// AVPlayer drops the playlist.
func TestSplitEncodeKeyframeAfterItsLeadingPicture(t *testing.T) {
	const frame = 1500
	var frags [][]byte
	dec := int64(900000)
	for g := range 3 {
		for i := range 120 {
			flags, cto := uint32(0x10000), uint32(frame)
			switch {
			case i == 0:
				flags = 0
			case g == 1 && i == 29:
				// Shown after the next picture.
				flags, cto = 0, 2*frame
			case g == 1 && i == 30:
				cto = 0
			}
			frags = append(frags, reorderedAt(dec, frame, flags, cto))
			dec += frame
		}
	}
	frags = append(frags, reorderedAt(dec, frame, 0, frame))
	assertParts(t, packSplit(t, t.TempDir(), frags...))
}

func TestLiveEncodesSplitTheirFragments(t *testing.T) {
	src := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}
	if !splitFragments(RenditionArgs(0, src, Rendition{Video: "copy", Audio: "copy"}, "libx264", "")) {
		t.Fatal("a copy rendition must cut parts inside a group")
	}
	if !splitFragments(RenditionArgs(0, src, Rendition{Video: "720", Audio: "aac"}, "libx264", "")) {
		t.Fatal("a transcode must cut parts inside a group")
	}
	if !splitFragments(LadderArgs(0, src, []string{"1080", "480"}, "broadcast", "libx264", "")) {
		t.Fatal("a ladder must cut parts inside a group")
	}
	if splitFragments([]string{"-movflags", "frag_keyframe", "pipe:1"}) {
		t.Fatal("keyframe fragments only are not split")
	}
}

// The real thing: ffmpeg's fragments of a copied one-second group, packed.
// Every segment starts on a keyframe part, holds three legal parts, and keeps
// the group's length, so a copy and a transcode still share each cut.
func TestSplitCopyRenditionKeepsKeyframeSegments(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=30000/1001", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "6", "-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-keyint_min", "30", "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-c:a", "ac3",
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
	args := RenditionArgs(0, Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "")
	cmd := exec.Command(ffmpeg, args...)
	cmd.Stdin = in
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(stdout)
	if werr := cmd.Wait(); err != nil || werr != nil {
		t.Fatalf("encode: %v %v", err, werr)
	}
	gate := newPlaylistGate()
	gate.split = splitFragments(args)
	if err := Pack(out, bytes.NewReader(raw), gate); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	segs := listedSegments(t, text)
	if len(segs) < 4 {
		t.Fatalf("too few segments:\n%s", text)
	}
	assertParts(t, text)
	for i, s := range segs[:len(segs)-1] {
		if s.dur < 0.95 || s.dur > 1.05 {
			t.Fatalf("segment %d is %.3f s, not one group:\n%s", i, s.dur, text)
		}
		if len(s.parts) > 0 && len(s.parts) != 3 {
			t.Fatalf("segment %d parts %v", i, s.parts)
		}
	}
	// Every segment file starts on a keyframe.
	names, _ := filepath.Glob(filepath.Join(out, "seg*.m4s"))
	slices.Sort(names)
	init, _ := os.ReadFile(filepath.Join(out, "init.mp4"))
	track, _, _ := videoTrack(init)
	for _, n := range names {
		body, _ := os.ReadFile(n)
		if !fragmentIndependent(body, track) {
			t.Fatalf("%s does not start on a keyframe", filepath.Base(n))
		}
	}
}

// soundFragment is a fragment with sound and no picture, as a split encode
// writes while the picture stalls.
func soundFragment(pts int64) []byte {
	tfhd := binary.BigEndian.AppendUint32(make([]byte, 4), 2)
	tfdt := binary.BigEndian.AppendUint64([]byte{1, 0, 0, 0}, uint64(pts))
	trun := []byte{0, 0, 0x01, 0}
	trun = binary.BigEndian.AppendUint32(trun, 1)
	trun = binary.BigEndian.AppendUint32(trun, 1536)
	traf := mp4Box("traf", append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...))
	return append(mp4Box("moof", append(mp4Box("mfhd", make([]byte, 8)), traf...)), mp4Box("mdat", []byte{0})...)
}

// Sound that arrives in its own fragment, while the picture stalls, goes out
// with the next part inside the group, never in front of a keyframe.
func TestSplitEncodeKeepsSoundWithoutAPicture(t *testing.T) {
	dir := t.TempDir()
	raw := initWith(trakBox(1, 90000, "vide", nil), trakBox(2, 48000, "soun", nil))
	for _, f := range [][]byte{
		fragmentAt(900000, 30000, 0, 0),
		fragmentAt(930000, 30000, 0x10000, 0),
		soundFragment(960000 * 48000 / 90000),
		fragmentAt(960000, 30000, 0x10000, 0),
		soundFragment(990000 * 48000 / 90000),
		fragmentAt(990000, 30000, 0, 0),
		fragmentAt(1020000, 30000, 0x10000, 0),
	} {
		raw = append(raw, f...)
	}
	gate := newPlaylistGate()
	gate.split = true
	if err := Pack(dir, bytes.NewReader(raw), gate); err != nil {
		t.Fatal(err)
	}
	moofs := func(name string) int {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, top := range boxes(b) {
			if top.kind == "moof" {
				n++
			}
		}
		return n
	}
	if n := moofs("part00002.m4s"); n != 2 {
		t.Fatalf("the part after the sound has %d fragments, want the sound and its own", n)
	}
	if n := moofs("part00003.m4s"); n != 1 {
		t.Fatalf("a keyframe part has %d fragments; sound in front would hide its keyframe", n)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "seg00001.m4s"))
	if init, _ := os.ReadFile(filepath.Join(dir, "init.mp4")); len(init) == 0 || !fragmentIndependent(b, 1) {
		t.Fatal("the second segment must start on its keyframe")
	}
}

// An encode's first fragment can be a whole group or more (sound that starts
// late holds it). A part longer than PART-TARGET makes AVPlayer reject the
// playlist, so that segment lists no parts, and the open segment no hint.
func TestSplitEncodeListsNoPartLongerThanTheTarget(t *testing.T) {
	dir := t.TempDir()
	long := packSplit(t, dir,
		fragmentAt(900000, 90090, 0, 0),
	)
	if strings.Contains(long, "#EXT-X-PART:") {
		t.Fatalf("a part over the target was listed:\n%s", long)
	}
	text := packSplit(t, t.TempDir(),
		fragmentAt(900000, 90090, 0, 0),
		fragmentAt(990090, 30030, 0, 0),
		fragmentAt(1020120, 30030, 0x10000, 0),
		fragmentAt(1050150, 30030, 0x10000, 0),
		fragmentAt(1080180, 30030, 0, 0),
	)
	segs := listedSegments(t, text)
	if len(segs) < 2 || len(segs[0].parts) != 0 || len(segs[1].parts) != 3 {
		t.Fatalf("the long segment lists no parts, the next lists its own:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#EXT-X-PART:") && !strings.Contains(line, "DURATION=0.334") {
			t.Fatalf("part over the target: %s", line)
		}
	}
}

// AVPlayer will not play low latency from a playlist without a date
// (CoreMedia -15412 "Playlist does not have EXT-X-PROGRAM-DATE-TIME"), and
// then holds back three target durations for the whole session. The open
// segment's parts carry its date, the same one the segment gets on closing.
func TestOpenPartsCarryTheirSegmentsDate(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	open := []packedPart{
		{name: "part00000.m4s", pts: 900000, dur: 30030, sync: true, timed: true},
		{name: "part00001.m4s", pts: 930030, dur: 30030, timed: true},
	}
	if err := writePacked(dir, []byte("init"), nil, open, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	first := string(p.stamp(dir, raw, tl))
	lines := strings.Split(first, "\n")
	at := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "#EXT-X-PART:") })
	if at < 1 || !strings.HasPrefix(lines[at-1], "#EXT-X-PROGRAM-DATE-TIME:") || strings.Count(first, "PROGRAM-DATE-TIME") != 1 {
		t.Fatalf("a parts-only playlist needs its date before the first part:\n%s", first)
	}
	date := lines[at-1]
	closed := []packedSeg{{name: "seg00000.m4s", pts: 900000, dur: 90090, timed: true, parts: []partRef{
		{name: "part00000.m4s", dur: 30030, sync: true, pts: 900000, timed: true},
		{name: "part00001.m4s", dur: 30030, pts: 930030, timed: true},
		{name: "part00002.m4s", dur: 30030, pts: 960060, timed: true},
	}}}
	next := []packedPart{{name: "part00003.m4s", pts: 990090, dur: 30030, sync: true, timed: true}}
	if err := writePacked(dir, []byte("init"), closed, next, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "index.m3u8"))
	second := string(p.stamp(dir, raw, tl))
	if !strings.Contains(second, date+"\n#EXTINF:1.001,\nseg00000.m4s") {
		t.Fatalf("the closed segment keeps the date its parts had (%s):\n%s", date, second)
	}
	if !strings.Contains(second, "#EXT-X-PROGRAM-DATE-TIME:2026-10-16T11:59:57.001Z\n#EXT-X-PART:DURATION=0.334,INDEPENDENT=YES,URI=\"part00003.m4s\"") {
		t.Fatalf("the next open segment is dated where the last one ended:\n%s", second)
	}
}
