package live

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestASilenceMovesTheClockOn(t *testing.T) {
	start := time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return start }
	at := tl.Wall(0)
	// The tuner was silent for 70 s, then carried on with the next timestamp.
	tl.Shift(70 * time.Second)
	if got := tl.Wall(90000).Sub(at); got != 71*time.Second {
		t.Fatalf("one second of picture after 70 s of silence is %v after the last, want 71s", got)
	}
	tl.Settle()
	// A station clock break later is pinned to the last segment, as before.
	lastEnd := at.Add(80 * time.Second)
	tl.Reanchor(5_000_000, lastEnd)
	if got := tl.Wall(5_000_000); !got.Equal(lastEnd) {
		t.Fatalf("a break without silence moved %v", got.Sub(lastEnd))
	}
}

func TestABreakAfterASilenceLandsOnTheWallClock(t *testing.T) {
	tl := NewTimeline()
	tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
	at := tl.Wall(0)
	lastEnd := at.Add(2 * time.Second)
	// The silence, then a frame with a stray timestamp: the break keeps the silence.
	tl.Shift(70 * time.Second)
	tl.Reanchor(7_000_000, lastEnd)
	if got := tl.Wall(7_000_000).Sub(lastEnd); got != 70*time.Second {
		t.Fatalf("the break after the silence is %v after the last segment, want 70s", got)
	}
	// The silence is counted once: the next break is pinned again.
	tl.Reanchor(180_000, lastEnd.Add(73*time.Second))
	if got := tl.Wall(180_000).Sub(lastEnd); got != 73*time.Second {
		t.Fatalf("second break %v, want 73s", got)
	}
	// Before the first anchor there is no clock to move.
	fresh := NewTimeline()
	fresh.Shift(time.Minute)
	if _, _, set := fresh.Anchor(); set {
		t.Fatal("a shift anchored a clock")
	}
}

func TestEveryEncodeOnAMuxMovesTogether(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	m := &mux{freq: 539000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	other := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.mu.Lock()
	h.muxes[m.freq] = m
	h.muxes[other.freq] = other
	f1 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "104.1"}})
	f2 := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "138.1"}})
	f3 := h.addFeedLocked(other, store.SourceChannel{Channel: store.Channel{ID: 3, GuideNumber: "4.1"}})
	clocks := map[string]*Timeline{}
	for name, f := range map[string]*feed{"copy": f1, "tile": f2, "other": f3} {
		tl := NewTimeline()
		tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
		tl.Wall(0)
		clocks[name] = tl
		f.renditions[name] = &rendition{clock: tl}
	}
	f1.renditions["none"] = &rendition{}
	h.mu.Unlock()
	h.shiftClocks(m, 70*time.Second)
	base := time.Date(2026, 10, 2, 15, 50, 56, 0, time.UTC)
	for name, want := range map[string]time.Duration{"copy": 70 * time.Second, "tile": 70 * time.Second, "other": 0} {
		if got := clocks[name].Wall(0).Sub(base); got != want {
			t.Errorf("%s moved %v, want %v", name, got, want)
		}
	}
}

// The first part after a break stays listed while its segment is open and
// for three targets after. Anchoring it on every stamp counted the silence
// once and then dropped it, so later frames went back by the silence.
func TestAListedPartAfterABreakKeepsTheSilence(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"init.mp4":      videoInit(),
		"seg00000.m4s":  keyframeFragment(0, 90000),
		"part00001.m4s": keyframeFragment(9_000_000, 90000),
		"seg00001.m4s":  keyframeFragment(9_000_000, 90000),
		"part00002.m4s": keyframeFragment(9_090_000, 90000),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tl := NewTimeline()
	tl.now = func() time.Time { return time.Date(2026, 10, 2, 15, 51, 0, 0, time.UTC) }
	var st playlistStamper
	head := "#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1.000,\nseg00000.m4s\n"
	st.stamp(dir, []byte(head), tl)
	lastEnd := tl.Wall(90000)
	tl.Shift(70 * time.Second)
	after := head + "#EXT-X-DISCONTINUITY\n#EXT-X-PART:DURATION=1.000,INDEPENDENT=YES,URI=\"part00001.m4s\"\n#EXTINF:1.000,\nseg00001.m4s\n#EXT-X-PART:DURATION=1.000,INDEPENDENT=YES,URI=\"part00002.m4s\"\n"
	for i := range 3 {
		st.stamp(dir, []byte(after), tl)
		if got := tl.Wall(9_090_000).Sub(lastEnd); got != 71*time.Second {
			t.Fatalf("stamp %d: the part after the break is %v after the last segment, want 71s", i+1, got)
		}
	}
}

// An encode fed from the buffer dates its first picture when that byte
// arrived, not seconds later when it was encoded. A sibling's seed wins.
func TestAStartDatesTheFirstPictureUnlessSeeded(t *testing.T) {
	now := time.Date(2026, 10, 3, 20, 0, 0, 0, time.UTC)
	arrived := now.Add(-20 * time.Second)
	tl := NewTimeline()
	tl.now = func() time.Time { return now }
	tl.Start(arrived)
	if got := tl.Wall(90000); !got.Equal(arrived) {
		t.Fatalf("first picture at %v, want %v", got, arrived)
	}
	seeded := NewTimeline()
	seeded.now = func() time.Time { return now }
	seeded.Seed(90000, arrived.Add(time.Second))
	seeded.Start(arrived)
	if got := seeded.Wall(90000); !got.Equal(arrived.Add(time.Second)) {
		t.Fatalf("seeded first picture at %v, want the seed's %v", got, arrived.Add(time.Second))
	}
}

// A segment that is not on disk yet stays undated, and the next reload of
// the same playlist dates it once the file is there.
func TestStampRetriesASegmentThatWasNotOnDisk(t *testing.T) {
	dir := t.TempDir()
	src := []byte("#EXTM3U\n#EXTINF:1.0,\nseg00000.ts\n")
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	first := string(p.stamp(dir, src, tl))
	if strings.Contains(first, "PROGRAM-DATE-TIME") {
		t.Fatalf("dated a segment that is not on disk:\n%s", first)
	}
	pes := []byte{0x00, 0x00, 0x01, 0xE0, 0x00, 0x00, 0x80, 0x80, 0x05, 0x21, 0x00, 0x01, 0x00, 0x01}
	if err := os.WriteFile(filepath.Join(dir, "seg00000.ts"), tsPacket(0x100, true, pes), 0o644); err != nil {
		t.Fatal(err)
	}
	second := string(p.stamp(dir, src, tl))
	if !strings.Contains(second, "PROGRAM-DATE-TIME") {
		t.Fatalf("a segment on disk was left undated:\n%s", second)
	}
}

// An unchanged playlist keeps its dates. A different playlist, and a stamp
// after the encode restarts, do not reuse that answer.
func TestStampKeepsAnUnchangedPlaylist(t *testing.T) {
	src := []byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2.0,\nseg00000.m4s\n")
	when := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var p playlistStamper
	p.cache = map[string]int64{"seg00000.m4s": 0}
	p.walls = map[string]time.Time{"seg00000.m4s": when}
	tl := NewTimeline()
	tl.now = func() time.Time { return when }
	a := string(p.stamp(t.TempDir(), src, tl))
	b := string(p.stamp(t.TempDir(), src, tl))
	if a != b || !strings.Contains(a, "PROGRAM-DATE-TIME") {
		t.Fatalf("second stamp changed:\n%s\n%s", a, b)
	}
	later := []byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:1\n#EXTINF:2.0,\nseg00001.m4s\n")
	if got := string(p.stamp(t.TempDir(), later, tl)); !strings.Contains(got, "MEDIA-SEQUENCE:1") {
		t.Fatalf("a new playlist reused the old one:\n%s", got)
	}
	p.reset()
	if got := string(p.stamp(t.TempDir(), src, tl)); strings.Contains(got, "PROGRAM-DATE-TIME") {
		t.Fatalf("reset kept a date:\n%s", got)
	}
}

// The packer writes each fragment's start into the playlist. Stamp uses that
// and does not need the media file, and the tag is gone from the playlist a
// player receives. A start that was never measured is not invented as zero.
func TestStampUsesThePackedStart(t *testing.T) {
	dir := t.TempDir()
	if err := writePacked(dir, []byte("init"), []packedSeg{{
		name: "seg00000.m4s", pts: 900000, dur: 90000, timed: true,
	}}, nil, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "#EXT-X-PTS:seg00000.m4s:900000\n") {
		t.Fatalf("playlist has no start:\n%s", raw)
	}
	untimed := t.TempDir()
	if err := writePacked(untimed, []byte("init"), []packedSeg{{
		name: "seg00000.m4s", dur: 90000,
	}}, nil, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	plain, err := os.ReadFile(filepath.Join(untimed, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "EXT-X-PTS") {
		t.Fatalf("an unmeasured start was written:\n%s", plain)
	}
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	stamped := string(p.stamp(dir, raw, tl))
	if strings.Contains(stamped, "EXT-X-PTS") {
		t.Fatalf("the start tag was served:\n%s", stamped)
	}
	walls := dateWalls(t, stamped)
	if len(walls) != 1 || !walls[0].Equal(fixed.Add(-4*time.Second)) {
		t.Fatalf("segment dated %v", walls)
	}
	if got := tl.Wall(990000).Sub(walls[0]); got != time.Second {
		t.Fatalf("anchor moved the clock by %v", got)
	}

	partDir := t.TempDir()
	if err := writePacked(partDir, []byte("init"), nil, []packedPart{{
		name: "part00000.m4s", pts: 990000, dur: 45000, sync: true, timed: true,
	}}, 0, true, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	partRaw, err := os.ReadFile(filepath.Join(partDir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(partRaw), "#EXT-X-PTS:part00000.m4s:990000\n") {
		t.Fatalf("open part has no start:\n%s", partRaw)
	}
	tl = NewTimeline()
	tl.now = func() time.Time { return fixed }
	p = playlistStamper{}
	// A part anchors the clock. Its date is written on the next segment, not on the part.
	if out := string(p.stamp(partDir, partRaw, tl)); strings.Contains(out, "EXT-X-PTS") {
		t.Fatalf("the part start was served:\n%s", out)
	}
	pts, _, ok := tl.Anchor()
	if !ok || pts != 990000 {
		t.Fatalf("open part anchored at %d %v", pts, ok)
	}
}

// windowPlaylist is a live playlist of the last segments, as the packer
// writes it: each with its measured start and a duration rounded to a
// millisecond.
func windowPlaylist(first int, starts, durs []int64) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n")
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:" + strconv.Itoa(first) + "\n")
	for i := range starts {
		name := "seg" + strconv.Itoa(first+i) + ".m4s"
		b.WriteString("#EXT-X-PTS:" + name + ":" + strconv.FormatInt(starts[i], 10) + "\n")
		b.WriteString("#EXTINF:" + fmtDur(durs[i]) + ",\n" + name + "\n")
	}
	return []byte(b.String())
}

// A long game keeps its dates on the broadcast clock. Durations are written
// to the millisecond, and a rounded-up one once pushed every later date
// forward a fraction of a millisecond, by a different amount on each
// encode: two rooms on different pictures ended a game an echo apart.
func TestDatesFollowTheBroadcastClockForHours(t *testing.T) {
	fixed := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	patterns := [][]int64{
		{91592, 76581, 70570, 93093, 51051},
		{90090, 180180, 45045, 135135},
	}
	for _, pattern := range patterns {
		tl := NewTimeline()
		tl.now = func() time.Time { return fixed }
		var p playlistStamper
		dir := t.TempDir()
		const window = 30
		var starts, durs []int64
		// Near the 33-bit wrap, so the run crosses it.
		base := ptsWrap - 90000*1800
		pts, first := base, 0
		for elapsed := int64(0); elapsed < 90000*3600*3; {
			d := pattern[(first+len(starts))%len(pattern)]
			starts, durs = append(starts, pts%ptsWrap), append(durs, d)
			if len(starts) > window {
				starts, durs = starts[1:], durs[1:]
				first++
			}
			walls := dateWalls(t, string(p.stamp(dir, windowPlaylist(first, starts, durs), tl)))
			if len(walls) != len(starts) {
				t.Fatalf("%d dates for %d segments", len(walls), len(starts))
			}
			last := walls[len(walls)-1]
			want := fixed.Add(-4*time.Second + time.Duration(elapsed)*time.Second/90000)
			if off := last.Sub(want); off > time.Millisecond || off < -time.Millisecond {
				t.Fatalf("after %v the date is %v off the broadcast clock", time.Duration(elapsed)*time.Second/90000, off)
			}
			pts += d
			elapsed += d
		}
	}
}

// The clock holds for as long as a channel stays tuned. Measured from an
// anchor that never moved, a picture an hour in looked like a station
// clock reset, and the dates stepped onto the wall clock every hour.
func TestTheClockDoesNotStepAfterAnHour(t *testing.T) {
	fixed := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	now := fixed
	tl := NewTimeline()
	tl.now = func() time.Time { return now }
	base := ptsWrap - 90000*600
	first := tl.Wall(base)
	for s := int64(1); s <= 3600*30; s++ {
		// The wall clock runs a little off the broadcast clock.
		now = fixed.Add(time.Duration(s) * 1001 * time.Millisecond)
		got := tl.Wall((base + s*90000) % ptsWrap)
		if want := first.Add(time.Duration(s) * time.Second); !got.Equal(want) {
			t.Fatalf("second %d: %v, want %v", s, got, want)
		}
	}
	// A picture of a few seconds ago still dates from the same clock.
	back := tl.Wall((base + (3600*30-5)*90000) % ptsWrap)
	if want := first.Add((3600*30 - 5) * time.Second); !back.Equal(want) {
		t.Fatalf("an earlier picture: %v, want %v", back, want)
	}
	// A station that resets its clock still re-anchors.
	jump := tl.Wall((base + (3600*30+2*3600)*90000) % ptsWrap)
	if !jump.Equal(now.Add(-4 * time.Second)) {
		t.Fatalf("a two-hour jump: %v, want the wall clock %v", jump, now.Add(-4*time.Second))
	}
}

// A real backward step still pins to the end of the segment before it.
func TestABackwardStepStillPinsToTheLastEnd(t *testing.T) {
	fixed := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	src := windowPlaylist(0, []int64{900000, 990000, 1035000}, []int64{90000, 90000, 90000})
	walls := dateWalls(t, string(p.stamp(t.TempDir(), src, tl)))
	if len(walls) != 3 {
		t.Fatalf("%d dates", len(walls))
	}
	if want := walls[1].Add(time.Second); !walls[2].Equal(want) {
		t.Fatalf("the segment after a half-second step back is at %v, want %v", walls[2], want)
	}
}

// A carried seed is the encode before a restart. When it would date the
// first picture far from the wall clock, the station's clock changed while
// the encode was down, and the picture is dated as a fresh tune's.
func TestACarriedClockGivesWayToAStationReset(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	fresh := now.Add(-4 * time.Second)
	for _, tc := range []struct {
		name string
		off  time.Duration
		want time.Time
	}{
		{"same clock", -2 * time.Second, fresh.Add(-2 * time.Second)},
		{"edge", carryWithin, fresh.Add(carryWithin)},
		{"reset forward", 40 * time.Second, fresh},
		{"reset back", -11 * time.Second, fresh},
	} {
		tl := NewTimeline()
		tl.now = func() time.Time { return now }
		tl.Carry(90000, fresh.Add(tc.off))
		if got := tl.Wall(90000); !got.Equal(tc.want) {
			t.Errorf("%s: first picture at %v, want %v", tc.name, got, tc.want)
		}
	}
	// A sibling's seed is the room's own clock and is always kept.
	tl := NewTimeline()
	tl.now = func() time.Time { return now }
	tl.Carry(90000, fresh)
	tl.Seed(90000, fresh.Add(40*time.Second))
	if got := tl.Wall(90000); !got.Equal(fresh.Add(40 * time.Second)) {
		t.Fatalf("a sibling's seed gave way: %v", got)
	}
}

// A segment listed first by its part and then closed keeps the broadcast
// clock too. The open segment's date may sit a fraction of a millisecond
// past its own (the rounded end before it); carried into the closed
// segment, those fractions added up until the clock was pinned forward.
func TestDatesFollowTheBroadcastClockThroughOpenSegments(t *testing.T) {
	fixed := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	// Every duration rounds up.
	pattern := []int64{91592, 76590, 91592, 60030}
	tl := NewTimeline()
	tl.now = func() time.Time { return fixed }
	var p playlistStamper
	dir := t.TempDir()
	const window = 12
	var starts, durs []int64
	base := int64(900000)
	pts, first := base, 0
	for n := 0; pts-base < 90000*3600*3; n++ {
		d := pattern[n%len(pattern)]
		// The segment is open: one part is listed after the closed ones.
		part := "part" + strconv.Itoa(n) + ".m4s"
		open := string(windowPlaylist(first, starts, durs)) +
			"#EXT-X-PTS:" + part + ":" + strconv.FormatInt(pts, 10) + "\n" +
			"#EXT-X-PART:DURATION=0.334,URI=\"" + part + "\",INDEPENDENT=YES\n"
		p.stamp(dir, []byte(open), tl)
		// Then it closes.
		starts, durs = append(starts, pts), append(durs, d)
		if len(starts) > window {
			starts, durs = starts[1:], durs[1:]
			first++
		}
		closed := string(windowPlaylist(first, starts, durs))
		closed = strings.Replace(closed, "#EXT-X-PTS:seg"+strconv.Itoa(n)+".m4s", "#EXT-X-PTS:"+part+":"+strconv.FormatInt(pts, 10)+"\n#EXT-X-PART:DURATION=0.334,URI=\""+part+"\",INDEPENDENT=YES\n#EXT-X-PTS:seg"+strconv.Itoa(n)+".m4s", 1)
		walls := dateWalls(t, string(p.stamp(dir, []byte(closed), tl)))
		last := walls[len(walls)-1]
		want := fixed.Add(-4*time.Second + time.Duration(pts-base)*time.Second/90000)
		if off := last.Sub(want); off > 2*time.Millisecond || off < -2*time.Millisecond {
			t.Fatalf("after %v the date is %v off the broadcast clock", time.Duration(pts-base)*time.Second/90000, off)
		}
		pts += d
	}
}
