package dvbsi

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

var evening = time.Date(2026, 3, 14, 19, 30, 0, 0, time.UTC)

func TestCRCIsTheMPEGOne(t *testing.T) {
	// The check value of CRC-32/MPEG-2.
	if got := CRC([]byte("123456789")); got != 0x0376E6E7 {
		t.Fatalf("crc %08x", got)
	}
}

// EN 300 468 annex C: 93/10/13 12:45:00 is coded 0xC079124500.
func TestStartTimeIsMJDAndBCD(t *testing.T) {
	got, ok := startTime([]byte{0xC0, 0x79, 0x12, 0x45, 0x00})
	if !ok || !got.Equal(time.Date(1993, 10, 13, 12, 45, 0, 0, time.UTC)) {
		t.Fatalf("%s %v", got, ok)
	}
	if d, ok := bcdDuration([]byte{0x01, 0x45, 0x30}); !ok || d != time.Hour+45*time.Minute+30*time.Second {
		t.Fatalf("duration %s %v", d, ok)
	}
	for _, bad := range [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // undefined
		{0xC0, 0x79, 0x1A, 0x00, 0x00}, // not BCD
		{0xC0, 0x79, 0x25, 0x00, 0x00}, // hour 25
		{0xC0, 0x79, 0x12, 0x60, 0x00}, // minute 60
	} {
		if got, ok := startTime(bad); ok {
			t.Errorf("% x read as %s", bad, got)
		}
	}
	if got := mjdBCD(evening); !bytes.Equal(got[2:], []byte{0x19, 0x30, 0x00}) {
		t.Fatalf("helper % x", got)
	}
}

// One section, three events, spread over several packets by a long text.
func TestAnEITSectionAcrossPackets(t *testing.T) {
	long := strings.Repeat("A long evening of quiz rounds. ", 7)
	sec := eitSection(0x50, 0x1001, 0x0101, 0x2001,
		event(1, evening, 30*time.Minute, shortDesc("eng", []byte("Quiz Night"), []byte("Round one")), contentDesc(0x30)),
		event(2, evening.Add(30*time.Minute), time.Hour,
			shortDesc("eng", []byte("Harbour Lights"), nil),
			extendedDesc(0, 1, "eng", []byte(long[:110])),
			extendedDesc(1, 1, "eng", []byte(long[110:])),
			contentDesc(0x10)),
		event(3, evening.Add(90*time.Minute), 2*time.Hour+15*time.Minute,
			shortDesc("eng", []byte("Cup Final"), []byte("Live from the coast")), contentDesc(0x43, 0xB3), parentalDesc("GBR", 0x09)),
	)
	if len(sec) <= 2*184 {
		t.Fatalf("section is %d bytes; want one that spans packets", len(sec))
	}
	c := &carrier{pid: pidEIT}
	g := Parse(c.alone(sec))
	if len(g.Events) != 3 {
		t.Fatalf("events %+v", g.Events)
	}
	quiz, harbour, final := g.Events[0], g.Events[1], g.Events[2]
	if quiz.Title != "Quiz Night" || quiz.Description != "Round one" || !quiz.Start.Equal(evening) || quiz.End.Sub(quiz.Start) != 30*time.Minute {
		t.Fatalf("quiz %+v", quiz)
	}
	if quiz.ServiceID != 0x1001 || quiz.StreamID != 0x0101 || quiz.NetworkID != 0x2001 || !quiz.Actual || quiz.NowNext {
		t.Fatalf("quiz ids %+v", quiz)
	}
	if !slices.Equal(quiz.Genres, []string{"Entertainment"}) {
		t.Fatalf("quiz genres %v", quiz.Genres)
	}
	if harbour.Description != strings.TrimSpace(long) || !slices.Equal(harbour.Genres, []string{"Movie"}) {
		t.Fatalf("harbour %q %v", harbour.Description, harbour.Genres)
	}
	if final.End.Sub(final.Start) != 2*time.Hour+15*time.Minute || !final.Live || final.Rating != "12+" {
		t.Fatalf("final %+v", final)
	}
	if !slices.Equal(final.Genres, []string{"Sports", "Soccer"}) {
		t.Fatalf("final genres %v", final.Genres)
	}
}

// A packet that ends one section and starts the next carries both.
func TestPackedSectionsShareAPacket(t *testing.T) {
	var secs [][]byte
	for i := range 6 {
		secs = append(secs, eitSection(0x50+byte(i%2), 0x1001+uint16(i), 0x0101, 0x2001,
			event(uint16(10+i), evening.Add(time.Duration(i)*time.Hour), time.Hour,
				shortDesc("eng", []byte("Show "+string(rune('A'+i))), []byte(strings.Repeat("word ", 10+i*6))))))
	}
	c := &carrier{pid: pidEIT}
	raw := c.packed(secs...)
	if len(raw)/188 >= len(secs)*2 {
		t.Fatalf("%d packets for %d sections; the helper did not pack them", len(raw)/188, len(secs))
	}
	g := Parse(raw)
	if len(g.Events) != len(secs) {
		t.Fatalf("got %d events, want %d", len(g.Events), len(secs))
	}
	for i, e := range g.Events {
		if e.Title != "Show "+string(rune('A'+i)) || e.ServiceID != 0x1001+i {
			t.Fatalf("event %d %+v", i, e)
		}
	}
}

func TestBrokenSectionsAreDropped(t *testing.T) {
	good := eitSection(0x4E, 0x1001, 0x0101, 0x2001, event(1, evening, time.Hour, shortDesc("eng", []byte("Kept"), nil)))
	bad := eitSection(0x4E, 0x1002, 0x0101, 0x2001, event(2, evening, time.Hour, shortDesc("eng", []byte("Corrupt"), nil)))
	bad[20] ^= 0x40
	long := eitSection(0x50, 0x1003, 0x0101, 0x2001, event(3, evening, time.Hour, shortDesc("eng", []byte("Lost"), bytes.Repeat([]byte("x"), 240))))
	c := &carrier{pid: pidEIT}
	lost := c.alone(long)
	if len(lost) < 2*188 {
		t.Fatal("the lost section fits one packet")
	}
	lost = lost[188:] // its first packet never arrived
	var raw []byte
	raw = append(raw, c.alone(bad)...)
	raw = append(raw, bytes.Repeat([]byte{0x47, 0x12, 0x34}, 300)...) // not aligned packets
	raw = raw[:len(raw)/188*188]
	raw = append(raw, c.alone(good)...)
	// The rest of a section whose start was lost is not read as a start.
	raw = append(raw, lost...)
	// A packet repeated with the same counter is not read twice.
	again := c.alone(good)
	raw = append(raw, again...)
	raw = append(raw, again...)
	// A section length that cannot be a section.
	junk := c.packet(true, []byte{0x00, 0x50, 0xFF, 0xFF, 0x01})
	raw = append(raw, junk...)
	raw = append(raw, nullPackets(3)...)
	g := Parse(raw)
	if len(g.Events) != 1 || g.Events[0].Title != "Kept" {
		t.Fatalf("events %+v", g.Events)
	}
	if got := Parse(bytes.Repeat([]byte{0x47, 0x40, 0x12, 0x10}, 470)); len(got.Services)+len(got.Events) != 0 {
		t.Fatalf("garbage gave %+v", got)
	}
}

func TestNowNextOutranksTheSchedule(t *testing.T) {
	sched := eitSection(0x50, 0x1001, 0x0101, 0x2001, event(7, evening, time.Hour, shortDesc("eng", []byte("Running Late"), nil)))
	nowNext := eitSection(0x4E, 0x1001, 0x0101, 0x2001, event(7, evening.Add(5*time.Minute), time.Hour, shortDesc("eng", []byte("Running Late"), nil)))
	for _, order := range [][][]byte{{sched, nowNext}, {nowNext, sched}} {
		c := &carrier{pid: pidEIT}
		g := Parse(c.alone(order...))
		if len(g.Events) != 1 || !g.Events[0].NowNext || !g.Events[0].Start.Equal(evening.Add(5*time.Minute)) {
			t.Fatalf("events %+v", g.Events)
		}
	}
}

func TestOtherStreamListingsAreMarked(t *testing.T) {
	c := &carrier{pid: pidEIT}
	g := Parse(c.alone(
		eitSection(0x4F, 0x2001, 0x0202, 0x2001, event(1, evening, time.Hour, shortDesc("eng", []byte("Elsewhere Now"), nil))),
		eitSection(0x60, 0x2001, 0x0202, 0x2001, event(2, evening.Add(time.Hour), time.Hour, shortDesc("eng", []byte("Elsewhere Later"), nil))),
	))
	if len(g.Events) != 2 || g.Events[0].Actual || g.Events[1].Actual || !g.Events[0].NowNext || g.Events[1].NowNext {
		t.Fatalf("events %+v", g.Events)
	}
}

func TestSDTNamesServices(t *testing.T) {
	c := &carrier{pid: pidSDT}
	g := Parse(c.alone(
		sdtSection(0x42, 0x0101, 0x2001, sdtService(0x1001, []byte("Channel One HD")), sdtService(0x1002, []byte{0x15, 'C', 'a', 'f', 0xC3, 0xA9})),
		sdtSection(0x46, 0x0202, 0x2001, sdtService(0x2001, []byte("News 21"))),
	))
	want := []Service{
		{NetworkID: 0x2001, StreamID: 0x0101, ID: 0x1001, Name: "Channel One HD", Actual: true},
		{NetworkID: 0x2001, StreamID: 0x0101, ID: 0x1002, Name: "Café", Actual: true},
		{NetworkID: 0x2001, StreamID: 0x0202, ID: 0x2001, Name: "News 21"},
	}
	if !slices.Equal(g.Services, want) {
		t.Fatalf("services %+v", g.Services)
	}
}

func TestTheServerLanguageIsPicked(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "de_DE.UTF-8")
	sec := eitSection(0x50, 0x1001, 0x0101, 0x2001, event(1, evening, time.Hour,
		shortDesc("eng", []byte("The Lighthouse"), []byte("A keeper's year")),
		shortDesc("deu", []byte("Der Leuchtturm"), []byte("Ein Jahr")),
		extendedDesc(0, 0, "eng", []byte("English text")),
		extendedDesc(0, 0, "ger", []byte("Deutscher Text")),
	))
	c := &carrier{pid: pidEIT}
	g := Parse(c.alone(sec))
	if len(g.Events) != 1 {
		t.Fatalf("events %+v", g.Events)
	}
	e := g.Events[0]
	if e.Title != "Der Leuchtturm" || e.Subtitle != "Ein Jahr" || e.Description != "Deutscher Text" {
		t.Fatalf("%+v", e)
	}
	t.Setenv("LANG", "fr_FR.UTF-8")
	g = Parse((&carrier{pid: pidEIT}).alone(sec))
	if g.Events[0].Title != "The Lighthouse" {
		t.Fatalf("no French: want English, got %q", g.Events[0].Title)
	}
}

func TestShortAndExtendedText(t *testing.T) {
	long := strings.Repeat("This is a long synopsis that goes on. ", 4)
	cases := []struct {
		short, ext, subtitle, desc string
	}{
		{"Just a synopsis.", "", "", "Just a synopsis."},
		{"", "Only the long one.", "", "Only the long one."},
		{"Episode Four", "The crew sets out.", "Episode Four", "The crew sets out."},
		{"The crew sets", "The crew sets out.", "", "The crew sets out."},
		{long, "More.", "", strings.TrimSpace(long) + " More."},
	}
	for _, tc := range cases {
		descs := [][]byte{shortDesc("eng", []byte("Title"), []byte(tc.short))}
		if tc.ext != "" {
			descs = append(descs, extendedDesc(0, 0, "eng", []byte(tc.ext)))
		}
		sec := eitSection(0x50, 1, 1, 1, event(1, evening, time.Hour, descs...))
		g := Parse((&carrier{pid: pidEIT}).alone(sec))
		if len(g.Events) != 1 || g.Events[0].Subtitle != tc.subtitle || g.Events[0].Description != tc.desc {
			t.Errorf("%q + %q: %+v", tc.short, tc.ext, g.Events)
		}
	}
}

func TestEventsWithoutATitleOrTimeAreSkipped(t *testing.T) {
	undefined := event(2, evening, time.Hour, shortDesc("eng", []byte("No Time"), nil))
	copy(undefined[2:7], []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	sec := eitSection(0x50, 1, 1, 1,
		event(1, evening, time.Hour, contentDesc(0x20)),
		undefined,
		event(3, evening, 0, shortDesc("eng", []byte("No Length"), nil)),
		event(4, evening, time.Hour, shortDesc("eng", []byte("Fine"), nil)),
	)
	g := Parse((&carrier{pid: pidEIT}).alone(sec))
	if len(g.Events) != 1 || g.Events[0].Title != "Fine" {
		t.Fatalf("%+v", g.Events)
	}
}

func TestGenres(t *testing.T) {
	cases := []struct {
		pairs []byte
		want  []string
		live  bool
	}{
		{[]byte{0x10}, []string{"Movie"}, false},
		{[]byte{0x13}, []string{"Movie", "Science fiction"}, false},
		{[]byte{0x21}, []string{"News", "Weather"}, false},
		{[]byte{0x2F}, []string{"News"}, false}, // reserved: the family's name
		{[]byte{0x43, 0x40}, []string{"Sports", "Soccer"}, false},
		{[]byte{0x55}, []string{"Children", "Animation"}, false},
		{[]byte{0x54}, []string{"Children", "Educational"}, false},
		{[]byte{0xB3, 0x47}, []string{"Sports", "Motorsports"}, true},
		{[]byte{0xF1, 0x00, 0xC0}, nil, false}, // user defined, undefined, reserved
	}
	for _, tc := range cases {
		var body []byte
		for _, p := range tc.pairs {
			body = append(body, p, 0x00)
		}
		got, live := genreNames(body)
		if !slices.Equal(got, tc.want) || live != tc.live {
			t.Errorf("% x: %v %v", tc.pairs, got, live)
		}
	}
}
