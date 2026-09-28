package live

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"broadwave/internal/captions"
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
		"BANDWIDTH=8000000", `SUBTITLES="cc"`, "CLOSED-CAPTIONS=NONE", "\nindex.m3u8\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
}

func TestCaptionSpanMovesCuesOntoTheEncodeClock(t *testing.T) {
	c := newCaptionTrack(0)
	const off = int64(1<<33 - 45000) // the broadcast clock wraps half a second into the encode
	c.cues = []captions.Cue{
		{Start: wrapPTS(off + 10), End: wrapPTS(off + 90000), Text: "before"},
		{Start: wrapPTS(off + 180000), End: wrapPTS(off + 270000), Text: "inside"},
		{Start: wrapPTS(off + 400000), End: wrapPTS(off + 500000), Text: "after"},
	}
	got := c.span(wrapPTS(off+90000), 180000, off)
	if len(got) != 1 || got[0].Text != "inside" || got[0].Start != 180000 || got[0].End != 270000 {
		t.Fatalf("span = %+v", got)
	}
}

func TestCaptionsForgetAClockReset(t *testing.T) {
	c := newCaptionTrack(0)
	c.picture(5_000_000, nil)
	c.cues = []captions.Cue{{Start: 4_000_000, End: 4_900_000, Text: "old"}}
	c.picture(5_000_000+2*3600*90000, nil)
	if len(c.cues) != 0 {
		t.Fatalf("cues kept across a clock reset: %+v", c.cues)
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
	track.cues = []captions.Cue{{Start: off + segStart + 45000, End: off + segStart + 135000, Text: "Hello & bye"}}
	r := &rendition{spec: Rendition{Video: "720", Audio: "aac2"}, dir: dir, input: &packInput{offset: off, known: true}}
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
