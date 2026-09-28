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

// A roll-up broadcast, in BROADWAVE_ROLLUP_SAMPLE. Each line must start on
// screen within 200 ms of when the broadcast starts typing it, which ffmpeg's
// real-time mode reports character by character.
func TestRollUpLinesStartWithTheBroadcast(t *testing.T) {
	path := os.Getenv("BROADWAVE_ROLLUP_SAMPLE")
	if path == "" {
		t.Skip("BROADWAVE_ROLLUP_SAMPLE is not set")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "quiet", "-copyts", "-real_time", "1", "-real_time_latency_msec", "0",
		"-f", "lavfi", "-i", "movie="+filepath.Base(path)+"[out0+subcc]", "-map", "0:s", "-f", "srt", "-")
	cmd.Dir = filepath.Dir(path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffmpeg: %v", err)
	}
	lines := typedLines(string(out))
	if len(lines) < 10 {
		t.Fatalf("reference has %d roll-up lines", len(lines))
	}
	ts, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cues := decodeAll(t, ts, 1316)
	within := 0
	for i, l := range lines[1:] {
		d, ok := lineStart(cues, l, lines[i])
		if !ok {
			t.Errorf("line %q never shown", l.text)
			continue
		}
		if d >= -200 && d <= 200 {
			within++
		}
		t.Logf("starts %+5d ms, whole %+5d ms %q", d, lineDone(cues, l), l.text)
	}
	n := len(lines) - 1
	t.Logf("%d of %d lines within 200 ms, %d cues", within, n, len(cues))
	if within*10 < n*9 {
		t.Errorf("%d of %d lines start within 200 ms, want 90%%", within, n)
	}
}

// lineDone is how long after the broadcast finished typing l a cue first
// shows all of it, on the bottom row or rolled up above the next line.
func lineDone(cues []Cue, l typedLine) int64 {
	for _, c := range cues {
		start := c.Start / 90
		if start < l.done-2000 {
			continue
		}
		for _, row := range strings.Split(c.Text, "\n") {
			if normal(row) == l.text {
				return start - l.done
			}
		}
	}
	return -1 << 31
}

type typedLine struct {
	start, done int64 // ms
	text        string
}

var srtTime = regexp.MustCompile(`^(\d+):(\d+):(\d+),(\d+) -->`)

// typedLines finds where each bottom row starts being typed and its final text.
func typedLines(srt string) []typedLine {
	var out []typedLine
	prev := ""
	for _, block := range strings.Split(strings.TrimSpace(srt), "\n\n") {
		ls := strings.Split(block, "\n")
		if len(ls) < 3 {
			continue
		}
		m := srtTime.FindStringSubmatch(ls[1])
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		sec, _ := strconv.Atoi(m[3])
		ms, _ := strconv.Atoi(m[4])
		at := int64(((h*60+mi)*60+sec)*1000 + ms)
		raw := regexp.MustCompile(`<[^>]+>|\{\\an\d\}`).ReplaceAllString(strings.Join(ls[2:], "\n"), "")
		rows := strings.Split(raw, "\n")
		last := normal(rows[len(rows)-1])
		switch {
		case last == "" || last == prev:
		case prev != "" && strings.HasPrefix(last, prev) && len(out) > 0:
			out[len(out)-1].text, out[len(out)-1].done = last, at
		default:
			out = append(out, typedLine{start: at, done: at, text: last})
		}
		prev = last
	}
	return out
}

// lineStart is how long after the broadcast began typing l the first cue
// showing part of it starts. Part of the line before it is not a match.
func lineStart(cues []Cue, l, before typedLine) (int64, bool) {
	for _, c := range cues {
		start := c.Start / 90
		if start < l.start-2000 || start > l.start+3000 {
			continue
		}
		rows := strings.Split(c.Text, "\n")
		last := normal(rows[len(rows)-1])
		// While a line is typed, the one before has rolled up above it.
		above := len(rows) > 1 && normal(rows[len(rows)-2]) == before.text
		if last != "" && strings.HasPrefix(l.text, last) && (above || !strings.HasPrefix(before.text, last)) {
			return start - l.start, true
		}
	}
	return 0, false
}
