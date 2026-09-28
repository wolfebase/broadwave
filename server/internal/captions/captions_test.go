package captions

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func decodeAll(t *testing.T, ts []byte, chunk int) []Cue {
	t.Helper()
	dec := NewDecoder()
	last := int64(0)
	rd := NewReader(0, func(pts int64, pairs []byte) {
		dec.Feed(pts, pairs)
		last = pts
	})
	for off := 0; off < len(ts); off += chunk {
		rd.Write(ts[off:min(off+chunk, len(ts))])
	}
	rd.Flush()
	dec.Close(last + 1)
	return dec.Take()
}

func normal(s string) string {
	s = strings.NewReplacer("\\h", " ", "‘", "'", "’", "'", "\n", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// samples are broadcast captures with captions, listed in
// BROADWAVE_CAPTION_SAMPLES (absolute paths, separated like PATH). Real
// broadcasts are not in the repo, so these tests skip without it.
func samples(t *testing.T) (string, []string) {
	t.Helper()
	list := filepath.SplitList(os.Getenv("BROADWAVE_CAPTION_SAMPLES"))
	if len(list) == 0 {
		t.Skip("BROADWAVE_CAPTION_SAMPLES is not set")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	return ffmpeg, list
}

// ffmpeg's decoder (lavfi movie subcc) is the reference for text and timing.
func TestMatchesFFmpegOnBroadcasts(t *testing.T) {
	ffmpeg, list := samples(t)
	for _, path := range list {
		t.Run(filepath.Base(path), func(t *testing.T) {
			ts, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			compare(t, ffmpeg, path, ts)
		})
	}
}

// libx264 carries A/53 captions into H.264 SEI, as the software renditions do.
func TestReadsCaptionsFromH264(t *testing.T) {
	ffmpeg, list := samples(t)
	out := filepath.Join(t.TempDir(), "h264.ts")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", list[0], "-map", "0:v", "-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-copyts", "-f", "mpegts", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("no libx264: %v %s", err, b)
	}
	ts, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	compare(t, ffmpeg, list[0], ts)
}

func compare(t *testing.T, ffmpeg, ref string, ts []byte) {
	t.Helper()
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "quiet", "-f", "lavfi", "-i", "movie="+filepath.Base(ref)+"[out0+subcc]", "-map", "0:s", "-f", "webvtt", "-")
	cmd.Dir = filepath.Dir(ref)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffmpeg: %v", err)
	}
	want := parseVTT(string(out))
	// A capture can start in the middle of a caption, which each decoder
	// shows differently, so the comparison starts at the second one.
	want = want[1:]
	if len(want) < 3 {
		t.Fatalf("reference has %d cues", len(want))
	}
	got := decodeAll(t, ts, 1316)[1:]
	for _, c := range got {
		t.Logf("%6d ms %q", ptsDelta(c.Start, got[0].Start)/90, c.Text)
	}
	// ffmpeg writes a cue when it ends, so a caption still up at the end of
	// the sample is missing from its file. Its clock starts at the earliest
	// stream, so compare times from the first cue.
	if len(got) < len(want) {
		t.Fatalf("got %d cues, ffmpeg %d", len(got), len(want))
	}
	for i := range want {
		if normal(got[i].Text) != normal(want[i].text) {
			t.Errorf("cue %d text %q, ffmpeg %q", i, got[i].Text, want[i].text)
		}
		start := ptsDelta(got[i].Start, got[0].Start) / 90
		if d := start - (want[i].start - want[0].start); d < -20 || d > 20 {
			t.Errorf("cue %d starts %d ms after the first, ffmpeg %d ms", i, start, want[i].start-want[0].start)
		}
	}
}

type refCue struct {
	start int64
	text  string
}

var vttTime = regexp.MustCompile(`^(?:(\d+):)?(\d+):(\d+)\.(\d+) -->`)

func parseVTT(s string) []refCue {
	var out []refCue
	for _, block := range strings.Split(s, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 {
			continue
		}
		m := vttTime.FindStringSubmatch(lines[0])
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		sec, _ := strconv.Atoi(m[3])
		ms, _ := strconv.Atoi(m[4])
		out = append(out, refCue{start: int64(((h*60+mi)*60+sec)*1000 + ms), text: strings.Join(lines[1:], "\n")})
	}
	return out
}
