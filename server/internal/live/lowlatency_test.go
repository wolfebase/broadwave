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

// A split encode sends a keyframe fragment and then fragments of a third of a
// second inside the group. Each group is one segment of three parts, the first
// independent, and the playlist advertises the short part target.
func TestSplitEncodePublishesPartsInsideAGroup(t *testing.T) {
	const key, inter = 0, 0x10000
	var frags [][]byte
	pts := int64(900000)
	for range 4 {
		for i := range 3 {
			flags := uint32(inter)
			if i == 0 {
				flags = key
			}
			frags = append(frags, fragmentAt(pts, 30030, flags, 0))
			pts += 30030
		}
	}
	text := packSplit(t, t.TempDir(), frags...)
	for _, tag := range []string{"PART-TARGET=0.375", "PART-HOLD-BACK=1.125", "HOLD-BACK=6.000", "#EXT-X-TARGETDURATION:2\n"} {
		if !strings.Contains(text, tag) {
			t.Fatalf("missing %s:\n%s", tag, text)
		}
	}
	segs := listedSegments(t, text)
	if len(segs) != 4 {
		t.Fatalf("want four one-group segments:\n%s", text)
	}
	for i, s := range segs {
		if s.dur != 1.001 || len(s.parts) != 3 {
			t.Fatalf("segment %d: %.3f s, %d parts:\n%s", i, s.dur, len(s.parts), text)
		}
		for j, p := range s.parts {
			if strings.Contains(p, "INDEPENDENT=YES") != (j == 0) {
				t.Fatalf("segment %d part %d: %s", i, j, p)
			}
		}
	}
}

// A keyframe fragment long enough for a segment must not close one on its
// own in a split encode: the fragment after it is cut inside the same group,
// and a segment that starts there cannot be decoded alone.
func TestSplitEncodeClosesSegmentsOnlyOnKeyframes(t *testing.T) {
	text := packSplit(t, t.TempDir(),
		fragmentAt(900000, 50000, 0, 0),
		fragmentAt(950000, 30000, 0x10000, 0),
		fragmentAt(980000, 30000, 0, 0),
		fragmentAt(1010000, 30000, 0x10000, 0),
	)
	segs := listedSegments(t, text)
	if len(segs) != 2 || len(segs[0].parts) != 2 || len(segs[1].parts) != 2 {
		t.Fatalf("segments must start on the keyframes:\n%s", text)
	}
	for _, s := range segs {
		if !strings.Contains(s.parts[0], "INDEPENDENT=YES") {
			t.Fatalf("a segment starts on a part that is not a keyframe:\n%s", text)
		}
	}
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
// Every segment starts on a keyframe part, holds several parts no longer than
// the target, and keeps the group's length, so a copy and a transcode still
// share each cut.
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
	dur := regexp.MustCompile(`DURATION=([0-9.]+)`)
	for i, s := range segs[:len(segs)-1] {
		if s.dur < 0.95 || s.dur > 1.05 {
			t.Fatalf("segment %d is %.3f s, not one group:\n%s", i, s.dur, text)
		}
		if len(s.parts) == 0 {
			continue // listed only near the live edge
		}
		if len(s.parts) < 3 || !strings.Contains(s.parts[0], "INDEPENDENT=YES") {
			t.Fatalf("segment %d parts %v", i, s.parts)
		}
		for _, p := range s.parts {
			d, _ := strconv.ParseFloat(dur.FindStringSubmatch(p)[1], 64)
			if d > 0.375 {
				t.Fatalf("part longer than the target: %s", p)
			}
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
