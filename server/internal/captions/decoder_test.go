package captions

import (
	"strings"
	"testing"
)

// The 608 byte pairs a caption encoder sends, parity bit left off.
var (
	rcl = []byte{0x14, 0x20}
	eoc = []byte{0x14, 0x2f}
	edm = []byte{0x14, 0x2c}
	enm = []byte{0x14, 0x2e}
	cr  = []byte{0x14, 0x2d}
	ru2 = []byte{0x14, 0x25}
	rdc = []byte{0x14, 0x29}
	// row 15, column 0 and row 1, column 0
	pac15 = []byte{0x14, 0x60}
	pac1  = []byte{0x11, 0x40}
)

func text(s string) [][]byte {
	var out [][]byte
	for i := 0; i < len(s); i += 2 {
		if i+1 < len(s) {
			out = append(out, []byte{s[i], s[i+1]})
		} else {
			out = append(out, []byte{s[i], 0})
		}
	}
	return out
}

// feed sends one pair per frame at 30 fps from pts, control codes twice.
func feed(d *Decoder, pts int64, pairs ...[]byte) int64 {
	for _, p := range pairs {
		n := 1
		if p[0] >= 0x10 && p[0] <= 0x1f {
			n = 2
		}
		for range n {
			d.Feed(pts, p)
			pts += 3003
		}
	}
	return pts
}

func popOnCaption(pts int64, d *Decoder, pac []byte, s string) int64 {
	pts = feed(d, pts, rcl, enm, pac)
	pts = feed(d, pts, text(s)...)
	return feed(d, pts, eoc)
}

func TestPopOnShowsAtEndOfCaption(t *testing.T) {
	d := NewDecoder()
	pts := popOnCaption(1000, d, pac15, "HELLO THERE")
	if _, on := d.Showing(); !on {
		t.Fatal("no caption on screen after EOC")
	}
	shownAt := pts - 3003 // the second EOC is ignored; the first one shows it
	pts = popOnCaption(pts+90000, d, pac15, "SECOND")
	cues := d.Take()
	if len(cues) != 1 || cues[0].Text != "HELLO THERE" {
		t.Fatalf("cues %+v", cues)
	}
	if cues[0].Start != shownAt-3003 {
		t.Errorf("start %d, want %d", cues[0].Start, shownAt-3003)
	}
	if cues[0].Top {
		t.Error("a bottom-row caption is marked top")
	}
	feed(d, pts+30000, edm)
	cues = d.Take()
	if len(cues) != 1 || cues[0].Text != "SECOND" || cues[0].End != pts+30000 {
		t.Fatalf("cues after EDM %+v", cues)
	}
}

func TestTopRowCaptionIsMarkedTop(t *testing.T) {
	d := NewDecoder()
	pts := popOnCaption(0, d, pac1, "SCORE UPDATE")
	feed(d, pts+9000, edm)
	cues := d.Take()
	if len(cues) != 1 || !cues[0].Top {
		t.Fatalf("cues %+v", cues)
	}
}

func TestRollUpShowsTypingAndScrollsAtCarriageReturn(t *testing.T) {
	d := NewDecoder()
	pts := feed(d, 0, ru2, pac15)
	typing := pts
	pts = feed(d, pts, text("FIRST LINE")...)
	pts = feed(d, pts, cr)
	pts = feed(d, pts, text("SECOND")...)
	scroll := pts
	pts = feed(d, pts, cr)
	feed(d, pts, edm)
	cues := d.Take()
	var got []string
	for _, c := range cues {
		got = append(got, c.Text)
	}
	// Two rows: the second carriage return rolls the first line off.
	want := []string{"FI", "FIRST LINE", "FIRST LINE\nSE", "SECOND"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("cues %q, want %q", got, want)
	}
	if cues[0].Start != typing {
		t.Errorf("typing shows at %d, want %d", cues[0].Start, typing)
	}
	if cues[3].Start != scroll {
		t.Errorf("the scroll shows at %d, want %d", cues[3].Start, scroll)
	}
}

func TestPaintOnShowsAtMostFiveTimesASecond(t *testing.T) {
	d := NewDecoder()
	pts := feed(d, 0, rdc, pac15)
	first := pts
	// 30 characters typed over 2.5 s, as a live captioner sends them.
	for _, p := range text(strings.Repeat("WORD ", 6)) {
		pts = feed(d, pts, p, []byte{0, 0}, []byte{0, 0}, []byte{0, 0}, []byte{0, 0})
	}
	for range 40 {
		pts = feed(d, pts, []byte{0, 0})
	}
	feed(d, pts, edm)
	cues := d.Take()
	if len(cues) < 10 || len(cues) > 14 {
		t.Fatalf("%d cues for 2.5 s of typing: %+v", len(cues), cues)
	}
	if cues[0].Start != first {
		t.Errorf("first characters show at %d, want %d", cues[0].Start, first)
	}
	for i := 1; i < len(cues); i++ {
		if gap := cues[i].Start - cues[i-1].Start; gap < shownEvery {
			t.Errorf("cue %d shows %d after the one before", i, gap)
		}
	}
	if last := cues[len(cues)-1].Text; last != strings.TrimSpace(strings.Repeat("WORD ", 6)) {
		t.Errorf("last cue %q", last)
	}
}

func TestSecondChannelAndTextServiceAreIgnored(t *testing.T) {
	d := NewDecoder()
	pts := feed(d, 0, []byte{0x1c, 0x20}, []byte{0x1c, 0x2e}, []byte{0x1c, 0x60})
	pts = feed(d, pts, text("CHANNEL TWO")...)
	pts = feed(d, pts, []byte{0x1c, 0x2f})
	pts = feed(d, pts, []byte{0x14, 0x2a})
	pts = feed(d, pts, text("TEXT MODE")...)
	pts = popOnCaption(pts, d, pac15, "ONE")
	feed(d, pts, edm)
	cues := d.Take()
	if len(cues) != 1 || cues[0].Text != "ONE" {
		t.Fatalf("cues %+v", cues)
	}
}

func TestSpecialAndExtendedCharacters(t *testing.T) {
	d := NewDecoder()
	pts := feed(d, 0, rcl, enm, pac15)
	pts = feed(d, pts, []byte{0x11, 0x37}) // ♪
	pts = feed(d, pts, text(" NINO")...)
	pts = feed(d, pts, []byte{0x13, 0x33}) // ö replaces the O
	pts = feed(d, pts, text(" 'A")...)
	pts = feed(d, pts, eoc)
	feed(d, pts, edm)
	cues := d.Take()
	if len(cues) != 1 || cues[0].Text != "♪ NINö ’A" {
		t.Fatalf("cues %+v", cues)
	}
}

func TestSegmentClipsCuesAndMapsTime(t *testing.T) {
	start := int64(1<<33 - 45000) // the clock wraps inside this segment
	cues := []Cue{
		{Start: start - 90000, End: start + 45000, Text: "A & <B>"},
		{Start: (start + 90000) & (1<<33 - 1), End: (start + 900000) & (1<<33 - 1), Text: "LATER", Top: true},
		{Start: start + 400000, End: start + 500000, Text: "OUTSIDE"},
	}
	got := string(Segment(start, 180000, cues))
	want := "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:8589889592\n" +
		"\n00:00:00.000 --> 00:00:00.500\nA &amp; &lt;B&gt;\n" +
		"\n00:00:01.000 --> 00:00:02.000 line:10%\nLATER\n"
	if got != want {
		t.Fatalf("segment\n%s\nwant\n%s", got, want)
	}
	if empty := string(Segment(0, 90000, nil)); empty != "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:0\n" {
		t.Fatalf("empty segment %q", empty)
	}
}

func TestTypingAfterTheClockWentBackShowsAtOnce(t *testing.T) {
	d := NewDecoder()
	pts := feed(d, 900000, ru2, pac15)
	feed(d, pts, text("AB")...)
	back := int64(3003)
	feed(d, back, text("CD")...)
	d.Close(back + 9000)
	cues := d.Take()
	// "AB" would end before it started, so only the new showing is kept.
	if len(cues) != 1 || cues[0].Text != "ABCD" || cues[0].Start != back {
		t.Fatalf("cues %+v", cues)
	}
}
