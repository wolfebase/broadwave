package live

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gapPlaylist checks a packed live playlist the way players read it: parts
// AVPlayer accepts, every closed segment as long as the parts listed for
// it, and no segment that holds a hole in the picture.
func gapPlaylist(t *testing.T, text string, longest float64) []listedSeg {
	t.Helper()
	segs := assertParts(t, text)
	dur := regexp.MustCompile(`DURATION=([0-9.]+)`)
	for i, s := range segs {
		if s.dur > longest {
			t.Fatalf("segment %d lasts %.3f s, a hole in the picture is inside it:\n%s", i, s.dur, text)
		}
		if len(s.parts) == 0 {
			continue
		}
		var sum float64
		for _, p := range s.parts {
			d, _ := strconv.ParseFloat(dur.FindStringSubmatch(p)[1], 64)
			sum += d
		}
		if d := sum - s.dur; d > 0.002*float64(len(s.parts)) || d < -0.002*float64(len(s.parts)) {
			t.Fatalf("segment %d lasts %.3f s and its parts %.3f s:\n%s", i, s.dur, sum, text)
		}
	}
	return segs
}

// stampedAt stamps a packed playlist and returns each segment's date and its
// first picture's time, in order.
func stampedAt(t *testing.T, dir, text string) ([]time.Time, []int64) {
	t.Helper()
	tl := NewTimeline()
	fixed := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	tl.now = func() time.Time { return fixed }
	var st playlistStamper
	stamped := string(st.stamp(dir, []byte(text), tl))
	var walls []time.Time
	var pts []int64
	var at time.Time
	for _, line := range strings.Split(stamped, "\n") {
		if v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); ok {
			at, _ = time.Parse("2006-01-02T15:04:05.000Z", v)
			continue
		}
		if strings.HasPrefix(line, "seg") && strings.HasSuffix(line, ".m4s") {
			p, ok := st.cache[line]
			if !ok || at.IsZero() {
				t.Fatalf("%s has no date:\n%s", line, stamped)
			}
			walls = append(walls, at)
			pts = append(pts, p)
			at = time.Time{}
		}
	}
	again := string(st.stamp(dir, []byte(text), tl))
	if strings.Join(programDates(again), "\n") != strings.Join(programDates(stamped), "\n") {
		t.Fatalf("a second stamp moved the dates:\n%s\n---\n%s", stamped, again)
	}
	return walls, pts
}

// onBroadcastClock checks that every segment is dated by its own picture
// time: the broadcast clock ran on through a gap in the tuner's stream, so a
// frame after it sits where the wall clock puts it. Each date is rounded to
// the millisecond.
func onBroadcastClock(t *testing.T, walls []time.Time, pts []int64) {
	t.Helper()
	for i := 1; i < len(walls); i++ {
		want := time.Duration(ptsDiff(pts[i], pts[0])) * time.Second / 90000
		if d := walls[i].Sub(walls[0]) - want; d > time.Millisecond || d < -time.Millisecond {
			t.Fatalf("segment %d is dated %v from the first, its picture %v", i, walls[i].Sub(walls[0]), want)
		}
	}
}

// Three seconds of the broadcast lost in the middle of a group of pictures:
// the packager closes the segment where its pictures end and starts the next
// after a discontinuity, so no segment or part spans the hole, and the dates
// after it follow the broadcast clock.
func TestAForwardGapIsADiscontinuityOnTheBroadcastClock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		split bool
	}{{"split", true}, {"whole groups", false}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			const frame = 3003
			before := pictures(900000, frame, 30, 30, 30, 17)
			after := int64(900000) + 107*frame + 3*90000
			frags := append(before, pictures(after, frame, 13, 30, 30, 30, 1)...)
			var text string
			if tc.split {
				text = packSplit(t, dir, frags...)
			} else {
				text = packGroups(t, dir, frags)
			}
			if n := strings.Count(text, "#EXT-X-DISCONTINUITY"); n != 1 {
				t.Fatalf("%d discontinuities for one gap:\n%s", n, text)
			}
			// The group after the gap is under half a second, so it shares a
			// segment with the next: 1.435 s.
			if tc.split {
				gapPlaylist(t, text, 1.5)
			} else {
				for i, s := range listedSegments(t, text) {
					if s.dur > 1.5 {
						t.Fatalf("segment %d lasts %.3f s, the hole is inside it:\n%s", i, s.dur, text)
					}
				}
			}
			walls, pts := stampedAt(t, dir, text)
			onBroadcastClock(t, walls, pts)
		})
	}
}

// packGroups packs fragments the way an encode without split fragments
// writes them: a fragment per group of pictures. frags is a fragment per
// picture, joined at each keyframe.
func packGroups(t *testing.T, dir string, frags [][]byte) string {
	t.Helper()
	var raw []byte
	raw = append(raw, videoInit()...)
	var pts int64
	var dur int64
	have := false
	flush := func() {
		if have {
			raw = append(raw, fragmentAt(pts, uint32(dur), 0, 0)...)
		}
		have = false
	}
	for _, f := range frags {
		p, _ := fragmentStart(f, 1, 90000)
		d, _ := fragmentDuration(f, 1, 90000)
		if fragmentIndependent(f, 1) || (have && p != pts+dur) {
			flush()
			pts, dur, have = p, d, true
			continue
		}
		dur += d
	}
	flush()
	if err := Pack(dir, bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A few pictures lost (a reception error) or a short sound dropout are not a
// break: the playlist stays one timeline, every part legal.
func TestAShortLossIsNotADiscontinuity(t *testing.T) {
	t.Run("pictures", func(t *testing.T) {
		dir := t.TempDir()
		const frame = 3003
		frags := pictures(900000, frame, 30, 30, 20)
		// Ten pictures, a third of a second, never arrive.
		frags = append(frags, pictures(900000+90*frame, frame, 30, 30, 1)...)
		text := packSplit(t, dir, frags...)
		if strings.Contains(text, "DISCONTINUITY") {
			t.Fatalf("a third of a second is not a break:\n%s", text)
		}
		assertParts(t, text)
		walls, pts := stampedAt(t, dir, text)
		onBroadcastClock(t, walls, pts)
	})
	t.Run("sound", func(t *testing.T) {
		dir := t.TempDir()
		raw := initWith(trakBox(1, 90000, "vide", nil), trakBox(2, 48000, "soun", nil))
		const frame = 3003
		pts := int64(900000)
		for g := range 5 {
			for i := range 30 {
				flags := uint32(0x10000)
				if i == 0 {
					flags = 0
				}
				// The sound drops out for half a second in the third group.
				if g != 2 || i >= 15 {
					raw = append(raw, soundFragment(pts*48000/90000)...)
				}
				raw = append(raw, fragmentAt(pts, frame, flags, 0)...)
				pts += frame
			}
		}
		raw = append(raw, fragmentAt(pts, frame, 0, 0)...)
		gate := newPlaylistGate()
		gate.split = true
		if err := Pack(dir, bytes.NewReader(raw), gate); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		if strings.Contains(text, "DISCONTINUITY") {
			t.Fatalf("a sound dropout is not a break:\n%s", text)
		}
		gapPlaylist(t, text, 1.2)
		walls, at := stampedAt(t, dir, text)
		onBroadcastClock(t, walls, at)
	})
}

// The tuner sends nothing for six seconds and comes back six seconds of
// broadcast later. The silence moves the clock on (Shift), the packager's
// discontinuity follows the broadcast clock: the six seconds count once.
func TestASilenceBeforeAShortGapCountsOnce(t *testing.T) {
	dir := t.TempDir()
	const frame = 3003
	frags := pictures(900000, frame, 30, 30, 30)
	head := packSplit(t, dir, frags...)
	tl := NewTimeline()
	fixed := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	tl.now = func() time.Time { return fixed }
	var st playlistStamper
	st.stamp(dir, []byte(head), tl)
	tl.Shift(6 * time.Second)
	after := int64(900000) + 90*frame + 6*90000
	frags = append(frags, pictures(after, frame, 30, 30, 1)...)
	whole := packSplit(t, t.TempDir(), frags...)
	// Pack wrote whole into its own directory; the stamper reads the starts
	// from the playlist itself.
	stamped := string(st.stamp(dir, []byte(whole), tl))
	walls := dateWalls(t, stamped)
	if len(walls) < 4 {
		t.Fatalf("dates %d:\n%s", len(walls), stamped)
	}
	first := walls[0]
	for i, w := range walls {
		if i == 0 {
			continue
		}
		want := time.Duration(i) * 1001 * time.Millisecond
		if i >= 3 {
			want += 6 * time.Second
		}
		if d := w.Sub(first) - want; d > time.Millisecond || d < -time.Millisecond {
			t.Fatalf("segment %d is %v after the first, want %v:\n%s", i, w.Sub(first), want, stamped)
		}
	}
}
