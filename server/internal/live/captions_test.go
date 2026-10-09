package live

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"broadwave/internal/captions"
	"broadwave/internal/store"
)

func TestCaptionPlaylistMirrorsTheVideo(t *testing.T) {
	video := `#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:2
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,HOLD-BACK=6.000,PART-HOLD-BACK=3.000,CAN-SKIP-UNTIL=12.000
#EXT-X-PART-INF:PART-TARGET=1.000
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MEDIA-SEQUENCE:40
#EXT-X-MAP:URI="init.mp4"
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T18:00:00.000Z
#EXTINF:1.001,
seg00040.m4s
#EXT-X-DISCONTINUITY
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T18:00:01.001Z
#EXTINF:1.001,
seg00041.m4s
#EXT-X-DISCONTINUITY
#EXT-X-PART:DURATION=0.500,INDEPENDENT=YES,URI="part00042.m4s"
`
	want := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:2
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES
#EXT-X-MEDIA-SEQUENCE:40
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T18:00:00.000Z
#EXTINF:1.001,
seg00040.vtt
#EXT-X-DISCONTINUITY
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T18:00:01.001Z
#EXTINF:1.001,
seg00041.vtt
`
	if got := string(captionPlaylist([]byte(video))); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestMainPlaylistOffersCaptions(t *testing.T) {
	body := string(mainPlaylist(Rendition{Video: "720", Audio: "aac2"}))
	for _, want := range []string{
		`TYPE=SUBTITLES,GROUP-ID="cc"`, `LANGUAGE="en"`, `AUTOSELECT=YES`, `URI="captions.m3u8"`,
		"BANDWIDTH=16000000,AVERAGE-BANDWIDTH=8000000,", `SUBTITLES="cc"`, "CLOSED-CAPTIONS=NONE", "\nindex.m3u8\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
}

func TestCaptionSpanMovesCuesOntoTheEncodeClock(t *testing.T) {
	c := newCaptionTrack(0)
	const off = int64(1<<33 - 45000) // the broadcast clock wraps half a second into the encode
	c.cues = []keptCue{
		{Cue: captions.Cue{Start: wrapPTS(off + 10), End: wrapPTS(off + 90000), Text: "before"}},
		{Cue: captions.Cue{Start: wrapPTS(off + 180000), End: wrapPTS(off + 270000), Text: "inside"}},
		{Cue: captions.Cue{Start: wrapPTS(off + 400000), End: wrapPTS(off + 500000), Text: "after"}},
	}
	got := c.span(0, wrapPTS(off+90000), 180000, off)
	if len(got) != 1 || got[0].Text != "inside" || got[0].Start != 180000 || got[0].End != 270000 {
		t.Fatalf("span = %+v", got)
	}
}

// popOn sends one pop-on caption, one pair a frame at 30 fps from pts, holds
// it a second, and clears it. Control codes go twice, as encoders send them.
func popOn(c *captionTrack, pts int64, s string) int64 {
	pairs := [][]byte{{0x14, 0x20}, {0x14, 0x20}, {0x14, 0x2e}, {0x14, 0x2e}, {0x14, 0x60}, {0x14, 0x60}}
	for i := 0; i < len(s); i += 2 {
		pair := []byte{s[i], 0}
		if i+1 < len(s) {
			pair[1] = s[i+1]
		}
		pairs = append(pairs, pair)
	}
	pairs = append(pairs, []byte{0x14, 0x2f}, []byte{0x14, 0x2f})
	for range 30 {
		pairs = append(pairs, nil)
	}
	pairs = append(pairs, []byte{0x14, 0x2c}, []byte{0x14, 0x2c})
	for _, pair := range pairs {
		c.picture(pts, pair)
		pts += 3003
	}
	return pts
}

func texts(cues []captions.Cue) string {
	var out []string
	for _, cue := range cues {
		out = append(out, cue.Text)
	}
	return strings.Join(out, "|")
}

// A looping source or a station splice repeats timestamps. Each loop is its
// own timeline, so a segment from the first loop never shows the second's.
func TestCaptionsStartANewTimelineWhenTheClockGoesBack(t *testing.T) {
	c := newCaptionTrack(0)
	const start = int64(1_000_000)
	end := popOn(c, start, "FIRST")
	popOn(c, start, "SECOND")
	if c.line != 1 {
		t.Fatalf("line %d after the clock went back", c.line)
	}
	if got := texts(c.span(0, start, end-start, 0)); got != "FIRST" {
		t.Fatalf("first timeline: %q", got)
	}
	if got := texts(c.span(1, start, end-start, 0)); got != "SECOND" {
		t.Fatalf("second timeline: %q", got)
	}
}

// A splice onto an ad with no captions still starts the new timeline, so
// the encode that starts there does not take the old one's cues.
func TestABreakWithoutCaptionsStillStartsATimeline(t *testing.T) {
	c := newCaptionTrack(0)
	popOn(c, 90_000_000, "BEFORE")
	for i := range pesAgree {
		c.picture(36_000_000+int64(i)*3003, nil)
	}
	if c.timeline() != 1 {
		t.Fatalf("line %d after a break with no caption bytes", c.timeline())
	}
}

// One bad header is dropped, as the program filter drops it, and does not
// start a timeline the encode never follows.
func TestOneBadTimestampKeepsTheTimeline(t *testing.T) {
	c := newCaptionTrack(0)
	const start = int64(1_000_000)
	pts := popOn(c, start, "FIRST")
	c.picture(pts-30*90000, []byte{0x14, 0x20})
	c.picture(pts+5*3600*90000, []byte{0x14, 0x20})
	end := popOn(c, pts, "SECOND")
	if c.line != 0 {
		t.Fatalf("line %d after one bad header", c.line)
	}
	if got := texts(c.span(0, start, end-start, 0)); got != "FIRST|SECOND" {
		t.Fatalf("cues %q", got)
	}
}

// A clock that jumps ahead stays on the timeline; the caption on screen ends
// where the old clock stopped rather than hours later.
func TestAForwardJumpEndsTheCaptionOnScreen(t *testing.T) {
	c := newCaptionTrack(0)
	pts := int64(1_000_000)
	for _, pair := range [][]byte{{0x14, 0x20}, {0x14, 0x60}, {'H', 'I'}, {0x14, 0x2f}, nil, nil, nil} {
		c.picture(pts, pair)
		pts += 3003
	}
	last := pts - 3003
	for i := range pesAgree {
		c.picture(pts+2*3600*90000+int64(i)*3003, nil)
	}
	if c.line != 0 || len(c.cues) != 1 || c.cues[0].Text != "HI" || c.cues[0].End != last {
		t.Fatalf("line %d cues %+v, want HI ending at %d", c.line, c.cues, last)
	}
}

func TestCaptionsKeepTwoHoursOfMedia(t *testing.T) {
	c := newCaptionTrack(0)
	c.picture(1000, nil)
	c.cues = []keptCue{{Cue: captions.Cue{Start: 1, End: 2, Text: "old"}, at: 0}, {Cue: captions.Cue{Start: 3, End: 4, Text: "new"}, at: 90000}}
	c.age = captionKeep
	c.picture(4003, nil)
	if len(c.cues) != 1 || c.cues[0].Text != "new" {
		t.Fatalf("cues %+v", c.cues)
	}
}

func TestCaptionSegmentUsesTheSegmentsOwnStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	// A segment 10 s into the encode, 2 s long. The encode's fragment time
	// zero is broadcast time 1,000,000.
	const segStart, off = int64(900000), int64(1_000_000)
	if err := os.WriteFile(filepath.Join(dir, "seg00010.m4s"), keyframeFragment(segStart, 180000), 0o644); err != nil {
		t.Fatal(err)
	}
	track := newCaptionTrack(0)
	track.cues = []keptCue{{Cue: captions.Cue{Start: off + segStart + 45000, End: off + segStart + 135000, Text: "Hello & bye"}}}
	r := &rendition{spec: Rendition{Video: "720", Audio: "aac2"}, dir: dir, input: &packInput{spans: []encodeSpan{{seq: 0, offset: off}}}}
	h := &Hub{channels: map[int64]*feed{7: {renditions: map[string]*rendition{"720.aac2": r}, captions: track}}}

	body, err := h.CaptionSegment(7, "720.aac2", "seg00010.vtt")
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:900000\n\n00:00:00.500 --> 00:00:01.500\nHello &amp; bye\n"
	if string(body) != want {
		t.Fatalf("got\n%q\nwant\n%q", body, want)
	}
	if _, err := h.CaptionSegment(7, "720.aac2", "seg00011.vtt"); err == nil {
		t.Fatal("a segment that is not on disk has captions")
	}

	// With the encode's offset unknown the segment is still valid, just empty.
	r.input = &packInput{}
	body, err = h.CaptionSegment(7, "720.aac2", "seg00010.vtt")
	if err != nil || string(body) != "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:900000\n" {
		t.Fatalf("got %q, %v", body, err)
	}
}

// A second backward break inside respawnGap commits another caption line
// before the encode queued for the first break writes a segment.
// That encode's pictures are the first break's timeline.
func TestSecondBreakDuringRespawnGapKeepsTheQueuedEncodeLine(t *testing.T) {
	c := newCaptionTrack(0)
	f := &feed{captions: c}
	in := &packInput{line: captionLine(f)}

	popOn(c, 5_000_000, "OPENING")
	in.noteEncodeStart(float64(5_000_000)/90000, true)
	in.noteFirstSegment(0)
	if got, ok := in.encodeAt(0); !ok || got.line != 0 {
		t.Fatalf("opening encode line %d, ok %v", got.line, ok)
	}

	harborEnd := popOn(c, 1_000_000, "HARBOR")
	if c.timeline() != 1 {
		t.Fatalf("line %d after the queued encode's break", c.timeline())
	}
	lanternEnd := popOn(c, 90_000, "LANTERN")
	if c.timeline() != 2 {
		t.Fatalf("line %d after the break inside respawnGap", c.timeline())
	}

	in.noteEncodeStart(float64(1_000_000)/90000, true)
	in.noteFirstSegment(7)
	got, ok := in.encodeAt(7)
	if !ok {
		t.Fatal("queued encode has no caption line")
	}
	if got.line != 1 || texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)) != "HARBOR" {
		t.Fatalf("queued encode line %d text %q, want line 1 HARBOR", got.line, texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)))
	}

	in.noteEncodeStart(float64(90_000)/90000, true)
	in.noteFirstSegment(11)
	later, ok := in.encodeAt(11)
	if !ok || later.line != 2 || texts(c.span(later.line, 90_000, lanternEnd-90_000, 0)) != "LANTERN" {
		t.Fatalf("later encode line %d text %q, want line 2 LANTERN", later.line, texts(c.span(later.line, 90_000, lanternEnd-90_000, 0)))
	}
}

// A restart after a break keeps the line that break reserved. Binding a new
// registration would date the new encode with the line from before the break.
func TestRestartKeepsTheCaptionLineReservedForTheBreak(t *testing.T) {
	c := newCaptionTrack(0)
	f := &feed{captions: c}
	next, expect, drop := bindCaption(f)
	in := &packInput{line: next, expect: expect, drop: drop}
	popOn(c, 5_000_000, "OPENING")
	in.noteEncodeStart(float64(5_000_000)/90000, true)
	in.noteFirstSegment(0)

	in.expectBreak()
	line, expect2, drop2 := in.releaseHooks()
	defer drop2()
	restarted := &packInput{line: line, expect: expect2, drop: drop2}
	// The reader commits the line after the replacement encode is bound.
	harborEnd := popOn(c, 1_000_000, "HARBOR")
	restarted.noteEncodeStart(float64(1_000_000)/90000, true)
	restarted.noteFirstSegment(4)
	got, ok := restarted.encodeAt(4)
	if !ok || got.line != 1 || texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)) != "HARBOR" {
		t.Fatalf("restarted encode line %d text %q, want line 1 HARBOR", got.line, texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)))
	}
}

// The filter reserves a line when it accepts a break. A break it follows in
// the same encode is not reserved, so the next encode keeps the line from the
// break that started it.
func TestFilterBreakKeepsTheLineOfTheEncodeItStarted(t *testing.T) {
	c := newCaptionTrack(0)
	f := &feed{program: 1, captions: c}
	next, expect, drop := bindCaption(f)
	defer drop()
	in := &packInput{line: next, expect: expect}
	r := &rendition{input: in}
	w := (&Hub{}).renditionPipe(f, r, &nopWriter{})
	p, ok := w.(*programPipe)
	if !ok || p.onBreak == nil {
		t.Fatal("rendition pipe has no break handler")
	}

	popOn(c, 5_000_000, "OPENING")
	in.noteEncodeStart(float64(5_000_000)/90000, true)
	in.noteFirstSegment(0)

	p.onBreak()
	harborEnd := popOn(c, 1_000_000, "HARBOR")
	popOn(c, 90_000, "LANTERN")
	p.onBreak()
	pierEnd := popOn(c, 40_000, "PIER")

	in.noteEncodeStart(float64(1_000_000)/90000, true)
	in.noteFirstSegment(7)
	got, ok := in.encodeAt(7)
	if !ok || got.line != 1 || texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)) != "HARBOR" {
		t.Fatalf("queued encode line %d text %q, want line 1 HARBOR", got.line, texts(c.span(got.line, 1_000_000, harborEnd-1_000_000, 0)))
	}
	in.noteEncodeStart(float64(40_000)/90000, true)
	in.noteFirstSegment(11)
	later, ok := in.encodeAt(11)
	if !ok || later.line != 3 || texts(c.span(later.line, 40_000, pierEnd-40_000, 0)) != "PIER" {
		t.Fatalf("later encode line %d text %q, want line 3 PIER", later.line, texts(c.span(later.line, 40_000, pierEnd-40_000, 0)))
	}
}

// After a timestamp break the next encode writes on with its own offset and
// the captions' new timeline. A rewind to the first encode keeps its cues.
func TestCaptionSegmentsAfterABreakUseTheirOwnEncode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	const offA, offB = int64(1_000_000), int64(5_000_000)
	if err := os.WriteFile(filepath.Join(dir, "seg00010.m4s"), keyframeFragment(900000, 180000), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seg00012.m4s"), keyframeFragment(90000, 180000), 0o644); err != nil {
		t.Fatal(err)
	}
	track := newCaptionTrack(0)
	track.line = 1
	track.cues = []keptCue{
		{Cue: captions.Cue{Start: offA + 900000 + 45000, End: offA + 900000 + 90000, Text: "first loop"}, line: 0},
		// The loop repeats the first one's timestamps.
		{Cue: captions.Cue{Start: offA + 900000 + 45000, End: offA + 900000 + 90000, Text: "second loop, same time"}, line: 1},
		{Cue: captions.Cue{Start: offB + 90000 + 45000, End: offB + 90000 + 90000, Text: "after the break"}, line: 1},
	}
	in := &packInput{spans: []encodeSpan{{seq: 0, offset: offA, line: 0}, {seq: 12, offset: offB, line: 1}}}
	r := &rendition{spec: Rendition{Video: "720", Audio: "aac2"}, dir: dir, input: in}
	h := &Hub{channels: map[int64]*feed{7: {renditions: map[string]*rendition{"720.aac2": r}, captions: track}}}
	for name, want := range map[string]string{"seg00010.vtt": "first loop", "seg00012.vtt": "after the break"} {
		body, err := h.CaptionSegment(7, "720.aac2", name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) || strings.Count(string(body), "-->") != 1 {
			t.Errorf("%s: want only %q, got\n%s", name, want, body)
		}
	}
	// An encode whose header gave no start cannot place its cues.
	in.spans = append(in.spans, encodeSpan{seq: 12, line: -1})
	if body, _ := h.CaptionSegment(7, "720.aac2", "seg00012.vtt"); strings.Contains(string(body), "-->") {
		t.Fatalf("cues without an offset:\n%s", body)
	}
}

// A B-frame run: the first picture decodes first but shows last. The span is
// the pictures' presentation times, so a cue held across segments has no gap.
func TestSegmentSpanUsesPresentationTime(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	tfhd := make([]byte, 8)
	binary.BigEndian.PutUint32(tfhd[4:8], 1)
	tfdt := make([]byte, 12)
	tfdt[0] = 1
	binary.BigEndian.PutUint64(tfdt[4:12], 90000)
	trun := make([]byte, 24)
	trun[2] = 0x09 // sample duration and composition offset
	binary.BigEndian.PutUint32(trun[4:8], 2)
	binary.BigEndian.PutUint32(trun[8:12], 3003)
	binary.BigEndian.PutUint32(trun[12:16], 6006)
	binary.BigEndian.PutUint32(trun[16:20], 3003)
	traf := append(append(mp4Box("tfhd", tfhd), mp4Box("tfdt", tfdt)...), mp4Box("trun", trun)...)
	seg := append(mp4Box("moof", mp4Box("traf", traf)), mp4Box("mdat", []byte{0})...)
	if err := os.WriteFile(filepath.Join(dir, "seg00001.m4s"), seg, 0o644); err != nil {
		t.Fatal(err)
	}
	start, dur, ok := segmentSpan(dir, "seg00001.m4s")
	if !ok || start != 93003 || dur != 6006 {
		t.Fatalf("span = %d, %d, %v; want 93003, 6006", start, dur, ok)
	}
}

// A segment can hold the picture before the 33-bit wrap and the picture
// after it. The span starts at the earlier picture and covers both.
func TestSegmentSpanAcrossPTSWrap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), videoInit(), 0o644); err != nil {
		t.Fatal(err)
	}
	const step = int64(30000)
	want := ptsWrap - step
	write := func(name string, parts ...[]byte) {
		t.Helper()
		var body []byte
		for _, p := range parts {
			body = append(body, p...)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("seg00000.m4s", keyframeFragment(want, uint32(step)), keyframeFragment(0, uint32(step)))
	got, dur, ok := segmentSpan(dir, "seg00000.m4s")
	if !ok || got != want || dur != 2*step {
		t.Fatalf("span %d + %d, ok=%v; want %d + %d", got, dur, ok, want, 2*step)
	}
	// A B-frame can put the wrapped picture first in the file.
	write("seg00001.m4s", keyframeFragment(0, uint32(step)), keyframeFragment(want, uint32(step)))
	got, dur, ok = segmentSpan(dir, "seg00001.m4s")
	if !ok || got != want || dur != 2*step {
		t.Fatalf("reordered span %d + %d, ok=%v; want %d + %d", got, dur, ok, want, 2*step)
	}
}

func TestSessionNamesTheCaptionedPlaylistOnlyWithCaptions(t *testing.T) {
	r := &rendition{spec: Rendition{Video: "720", Audio: "aac2"}, dir: t.TempDir()}
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 7}}, renditions: map[string]*rendition{r.spec.Key(): r}}
	h := &Hub{channels: map[int64]*feed{7: f}}
	if s := h.sessionLocked(f, r); s.MainPlaylist != "" {
		t.Fatalf("mainPlaylist without captions: %q", s.MainPlaylist)
	}
	if _, err := h.MainPlaylist(7, r.spec.Key()); err == nil {
		t.Fatal("main.m3u8 served without captions")
	}
	f.captions = newCaptionTrack(0)
	s := h.sessionLocked(f, r)
	if want := "/media/live/7/" + r.spec.Key() + "/main.m3u8"; s.MainPlaylist != want {
		t.Fatalf("mainPlaylist = %q, want %q", s.MainPlaylist, want)
	}
	if _, err := h.MainPlaylist(7, r.spec.Key()); err != nil {
		t.Fatal(err)
	}
}
