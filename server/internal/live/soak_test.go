package live

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// soakEncode is one synthetic encode of a broadcast: its fragment clock
// starts at offset on the broadcast clock, and shift moves its pictures off
// the broadcast's grid, as fps= does for a transcode.
type soakEncode struct {
	offset  int64
	shift   int64
	from    int // first segment it wrote
	stamper playlistStamper
	clock   *Timeline
	input   *packInput
}

// playlist is the encode's live playlist over segments [lo, hi) of the
// broadcast's keyframes: the packer's start lines and the segments.
func (e *soakEncode) playlist(keys []int64, lo, hi int) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:3\n")
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-MAP:URI=\"init.mp4\"\n", lo)
	for i := lo; i < hi; i++ {
		start := e.own(keys[i])
		dur := ptsDiff(e.own(keys[i+1]), start)
		name := fmt.Sprintf("seg%05d.m4s", i)
		fmt.Fprintf(&b, "#EXT-X-PTS:%s:%d\n#EXTINF:%s,\n%s\n", name, start, fmtDur(dur), name)
	}
	return []byte(b.String())
}

// own is a broadcast time on this encode's fragment clock.
func (e *soakEncode) own(broadcast int64) int64 {
	return ((broadcast+e.shift-e.offset)%ptsWrap + ptsWrap) % ptsWrap
}

// Hours of live playlists from two encodes of one broadcast, a copy and a
// transcode that joined later on its own fragment clock, each with its own
// Timeline seeded as seedClockLocked seeds it. Irregular groups, anchor
// slides every ten minutes, and a 33-bit wrap must not part them: the same
// broadcast frame keeps the same date on both, to the millisecond.
func TestRenditionClocksAgreeForHours(t *testing.T) {
	const hours = 3
	rng := rand.New(rand.NewSource(7))
	// The broadcast clock wraps an hour in.
	start := ptsWrap - 3600*90000
	keys := []int64{start}
	var total int64
	for total < hours*3600*90000 {
		// Groups of 0.5 to 2 s, mostly one second, on the 29.97 grid.
		frames := int64(30)
		switch rng.Intn(10) {
		case 0:
			frames = 15 + rng.Int63n(45)
		case 1:
			frames = 60
		}
		step := frames * 3003
		total += step
		keys = append(keys, (keys[len(keys)-1]+step)%ptsWrap)
	}
	n := len(keys) - 1
	fixed := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	newEncode := func(offset, shift int64, from int) *soakEncode {
		e := &soakEncode{offset: offset, shift: shift, from: from, clock: NewTimeline(), input: &packInput{}}
		e.input.noteEncodeStart(float64(offset)/90000, true)
		return e
	}
	copied := newEncode(start-90000, 0, 0)
	// The transcode starts ten and a half minutes in, half a frame off the grid.
	late := 630
	trans := newEncode(keys[late], 1501, late)
	at := func(i int) time.Time {
		return fixed.Add(time.Duration(ptsDiff(keys[i], start)) * time.Second / 90000)
	}
	copied.clock.now = func() time.Time { return fixed }
	f := &feed{renditions: map[string]*rendition{}}
	rc := &rendition{input: copied.input, clock: copied.clock}
	rt := &rendition{input: trans.input, clock: trans.clock}
	f.renditions["copy"] = rc
	const window = 60
	var worst time.Duration
	checked := 0
	for hi := 1; hi <= n; hi++ {
		lo := max(0, hi-window)
		copied.stamper.stamp("", copied.playlist(keys, lo, hi), copied.clock)
		if hi <= late {
			continue
		}
		if hi == late+1 {
			// Its own guess is seconds off the copy's; the seed decides.
			trans.clock.now = func() time.Time { return at(late).Add(3 * time.Second) }
			f.renditions["540"] = rt
			seedClockLocked(f, rt)
		}
		trans.stamper.stamp("", trans.playlist(keys, max(lo, late), hi), trans.clock)
		i := hi - 1
		name := fmt.Sprintf("seg%05d.m4s", i)
		a, okA := copied.stamper.walls[name]
		b, okB := trans.stamper.walls[name]
		if !okA || !okB {
			t.Fatalf("%s undated: copy %v transcode %v", name, okA, okB)
		}
		// The transcode's picture is shift later on the broadcast clock.
		d := b.Sub(a) - time.Duration(trans.shift)*time.Second/90000
		if d < 0 {
			d = -d
		}
		worst = max(worst, d)
		if d > time.Millisecond {
			t.Fatalf("%s, %.2f h in: the same broadcast frame is %v apart", name, float64(ptsDiff(keys[i], start))/90000/3600, d)
		}
		// The dates players read, rounded to the millisecond.
		fa := a.UTC().Format("2006-01-02T15:04:05.000Z")
		fb := b.Add(-time.Duration(trans.shift) * time.Second / 90000).UTC().Format("2006-01-02T15:04:05.000Z")
		pa, _ := time.Parse("2006-01-02T15:04:05.000Z", fa)
		pb, _ := time.Parse("2006-01-02T15:04:05.000Z", fb)
		if g := pa.Sub(pb); g > time.Millisecond || g < -time.Millisecond {
			t.Fatalf("%s: written dates %s and %s", name, fa, fb)
		}
		// Both stay on the broadcast clock from where the copy anchored.
		if want := at(i).Add(-4 * time.Second); !a.Equal(want) {
			t.Fatalf("%s: the copy drifted %v from the broadcast clock", name, a.Sub(want))
		}
		checked++
	}
	if checked < hours*3000 {
		t.Fatalf("checked %d segments", checked)
	}
	t.Logf("%d segments over %d h, widest %v", checked, hours, worst)
}
