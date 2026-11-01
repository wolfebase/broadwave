package dvbsi

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	tablePAT      = 0x00
	tableSDT      = 0x42
	tableSDTOther = 0x46
	tableNowNext  = 0x4E
	tableNowOther = 0x4F

	tagService       = 0x48
	tagShortEvent    = 0x4D
	tagExtendedEvent = 0x4E
	tagContent       = 0x54
	tagParental      = 0x55
)

// Service is one SDT row: a service_id and the name it goes by.
type Service struct {
	NetworkID int
	StreamID  int
	ID        int
	Name      string
	// Actual is a service of the tuned mux (SDT actual), not one it lists for another.
	Actual bool
}

// Event is one EIT listing. Start and End are UTC.
type Event struct {
	NetworkID int
	StreamID  int
	ServiceID int
	EventID   int
	// Actual is a listing for the tuned mux (EIT actual), where the service_id
	// is the program number.
	Actual bool
	// NowNext is from the present/following table, which a station keeps
	// more current than its schedule.
	NowNext     bool
	Start       time.Time
	End         time.Time
	Title       string
	Subtitle    string
	Description string
	Genres      []string
	Live        bool
	// Rating is a minimum age ("12+") when the station sends one.
	Rating string
}

// Guide is what a capture of one mux yielded.
type Guide struct {
	Services []Service
	Events   []Event
	// Programs are the program numbers the PAT lists. A tuner's single
	// program stream lists one.
	Programs []int
}

// Parse reads a transport stream and returns its SDT services and EIT
// listings. A listing sent more than once is kept once: the present/following
// copy wins over the schedule's, and a later copy over an earlier one.
func Parse(raw []byte) Guide {
	var g Guide
	svc := map[[3]int]int{}
	ev := map[[4]int]int{}
	for _, sec := range sections(raw) {
		t := sec.Body[0]
		switch {
		case sec.PID == pidPAT && t == tablePAT:
			g.Programs = parsePAT(sec.Body)
		case sec.PID == pidSDT && (t == tableSDT || t == tableSDTOther):
			for _, s := range parseSDT(sec.Body) {
				key := [3]int{s.NetworkID, s.StreamID, s.ID}
				if i, ok := svc[key]; ok {
					g.Services[i] = s
					continue
				}
				svc[key] = len(g.Services)
				g.Services = append(g.Services, s)
			}
		case sec.PID == pidEIT && t >= tableNowNext && t <= 0x6F:
			for _, e := range parseEIT(sec.Body) {
				key := [4]int{e.NetworkID, e.StreamID, e.ServiceID, e.EventID}
				if i, ok := ev[key]; ok {
					if g.Events[i].NowNext && !e.NowNext {
						continue
					}
					g.Events[i] = e
					continue
				}
				ev[key] = len(g.Events)
				g.Events = append(g.Events, e)
			}
		}
	}
	return g
}

// The PAT header is 8 bytes; each program is 4. Program 0 names the NIT.
func parsePAT(b []byte) []int {
	if len(b) < 12 || b[1]&0x80 == 0 || b[5]&0x01 == 0 {
		return nil
	}
	var out []int
	for pos := 8; pos+4 <= len(b)-4; pos += 4 {
		if n := int(binary.BigEndian.Uint16(b[pos : pos+2])); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

// The SDT header is 11 bytes; each service is 5 more and its descriptors.
func parseSDT(b []byte) []Service {
	if len(b) < 15 || b[1]&0x80 == 0 || b[5]&0x01 == 0 {
		// Not a long-form section, or the next version, not the current one.
		return nil
	}
	stream := int(binary.BigEndian.Uint16(b[3:5]))
	network := int(binary.BigEndian.Uint16(b[8:10]))
	end := len(b) - 4
	pos := 11
	var out []Service
	for pos+5 <= end {
		id := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		n := int(binary.BigEndian.Uint16(b[pos+3:pos+5])) & 0x0FFF
		pos += 5
		if pos+n > end {
			break
		}
		s := Service{NetworkID: network, StreamID: stream, ID: id, Actual: b[0] == tableSDT}
		forDescriptors(b[pos:pos+n], func(tag byte, body []byte) {
			if tag != tagService || len(body) < 2 {
				return
			}
			provider := int(body[1])
			if 2+provider >= len(body) {
				return
			}
			nameLen := int(body[2+provider])
			name := body[3+provider:]
			if nameLen > len(name) {
				return
			}
			s.Name = Text(name[:nameLen])
		})
		pos += n
		out = append(out, s)
	}
	return out
}

// The EIT header is 14 bytes; each event is 12 more and its descriptors.
func parseEIT(b []byte) []Event {
	if len(b) < 18 || b[1]&0x80 == 0 || b[5]&0x01 == 0 {
		return nil
	}
	t := b[0]
	service := int(binary.BigEndian.Uint16(b[3:5]))
	stream := int(binary.BigEndian.Uint16(b[8:10]))
	network := int(binary.BigEndian.Uint16(b[10:12]))
	actual := t == tableNowNext || (t >= 0x50 && t <= 0x5F)
	nowNext := t == tableNowNext || t == tableNowOther
	lang := language()
	end := len(b) - 4
	pos := 14
	var out []Event
	for pos+12 <= end {
		id := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		start, okStart := startTime(b[pos+2 : pos+7])
		length, okLength := bcdDuration(b[pos+7 : pos+10])
		n := int(binary.BigEndian.Uint16(b[pos+10:pos+12])) & 0x0FFF
		pos += 12
		if pos+n > end {
			break
		}
		desc := b[pos : pos+n]
		pos += n
		if !okStart || !okLength || length <= 0 {
			continue
		}
		e := Event{
			NetworkID: network, StreamID: stream, ServiceID: service, EventID: id,
			Actual: actual, NowNext: nowNext, Start: start, End: start.Add(length),
		}
		describe(&e, desc, lang)
		if e.Title == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

type shortEvent struct {
	lang, name, text string
}

type extendedPart struct {
	number int
	text   string
}

func describe(e *Event, desc []byte, want string) {
	var shorts []shortEvent
	extended := map[string][]extendedPart{}
	forDescriptors(desc, func(tag byte, body []byte) {
		switch tag {
		case tagShortEvent:
			if s, ok := parseShort(body); ok {
				shorts = append(shorts, s)
			}
		case tagExtendedEvent:
			if lang, part, ok := parseExtended(body); ok {
				extended[lang] = append(extended[lang], part)
			}
		case tagContent:
			names, live := genreNames(body)
			for _, n := range names {
				if !slices.Contains(e.Genres, n) {
					e.Genres = append(e.Genres, n)
				}
			}
			e.Live = e.Live || live
		case tagParental:
			if e.Rating == "" {
				e.Rating = parentalRating(body)
			}
		}
	})
	short, ok := pickShort(shorts, want)
	if !ok {
		return
	}
	e.Title = short.name
	var parts []extendedPart
	for _, lang := range []string{short.lang, want} {
		for _, key := range sortedKeys(extended) {
			if parts == nil && sameLanguage(key, lang) {
				parts = extended[key]
			}
		}
	}
	if keys := sortedKeys(extended); parts == nil && len(keys) > 0 {
		parts = extended[keys[0]]
	}
	slices.SortStableFunc(parts, func(a, b extendedPart) int { return a.number - b.number })
	var long strings.Builder
	for _, p := range parts {
		long.WriteString(p.text)
	}
	more := clean(long.String())
	switch {
	case more == "":
		e.Description = short.text
	case short.text == "" || strings.HasPrefix(more, short.text):
		e.Description = more
	case utf8.RuneCountInString(short.text) <= 80:
		// A short line ahead of a long text is the episode's name.
		e.Subtitle = short.text
		e.Description = more
	default:
		e.Description = short.text + " " + more
	}
}

func parseShort(body []byte) (shortEvent, bool) {
	if len(body) < 5 {
		return shortEvent{}, false
	}
	lang := langCode(body[0:3])
	n := int(body[3])
	if 4+n >= len(body) {
		return shortEvent{}, false
	}
	name := Text(body[4 : 4+n])
	rest := body[4+n:]
	m := int(rest[0])
	if 1+m > len(rest) {
		m = len(rest) - 1
	}
	return shortEvent{lang: lang, name: name, text: Text(rest[1 : 1+m])}, name != ""
}

// parseExtended keeps the text of one extended_event_descriptor. The items
// (cast and crew as label and value) are skipped.
func parseExtended(body []byte) (string, extendedPart, bool) {
	if len(body) < 6 {
		return "", extendedPart{}, false
	}
	number := int(body[0] >> 4)
	lang := langCode(body[1:4])
	items := int(body[4])
	pos := 5 + items
	if pos >= len(body) {
		return "", extendedPart{}, false
	}
	n := int(body[pos])
	pos++
	if pos+n > len(body) {
		return "", extendedPart{}, false
	}
	// Each part is decoded alone: every one starts with its own table byte.
	return lang, extendedPart{number: number, text: textKeepEnds(body[pos : pos+n])}, true
}

// textKeepEnds decodes a part of a longer text, keeping a space at its
// edges, so two parts split at a word join as two words.
func textKeepEnds(b []byte) string {
	s := Text(b)
	if s == "" {
		return ""
	}
	raw := b
	if len(raw) > 0 && raw[0] < 0x20 {
		raw = raw[1:]
	}
	if len(raw) > 0 && (raw[0] == ' ' || raw[0] == 0x8A) {
		s = " " + s
	}
	if len(raw) > 0 && (raw[len(raw)-1] == ' ' || raw[len(raw)-1] == 0x8A) {
		s += " "
	}
	return s
}

func pickShort(items []shortEvent, want string) (shortEvent, bool) {
	var first, eng *shortEvent
	for i := range items {
		it := &items[i]
		if it.name == "" {
			continue
		}
		if first == nil {
			first = it
		}
		if sameLanguage(it.lang, want) {
			return *it, true
		}
		if it.lang == "eng" && eng == nil {
			eng = it
		}
	}
	if eng != nil {
		return *eng, true
	}
	if first != nil {
		return *first, true
	}
	return shortEvent{}, false
}

func parentalRating(body []byte) string {
	for i := 0; i+3 < len(body); i += 4 {
		r := body[i+3]
		if r >= 0x01 && r <= 0x0F {
			return fmt.Sprintf("%d+", int(r)+3)
		}
	}
	return ""
}

func forDescriptors(desc []byte, fn func(tag byte, body []byte)) {
	for len(desc) >= 2 {
		tag, n := desc[0], int(desc[1])
		desc = desc[2:]
		if n > len(desc) {
			return
		}
		fn(tag, desc[:n])
		desc = desc[n:]
	}
}

// mjdEpoch is day 0 of the Modified Julian Date.
var mjdEpoch = time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)

// startTime reads a 16-bit MJD and a BCD hhmmss, UTC. All ones is undefined.
func startTime(b []byte) (time.Time, bool) {
	if b[0] == 0xFF && b[1] == 0xFF && b[2] == 0xFF && b[3] == 0xFF && b[4] == 0xFF {
		return time.Time{}, false
	}
	mjd := int(binary.BigEndian.Uint16(b[0:2]))
	clock, ok := bcdDuration(b[2:5])
	if !ok || clock >= 24*time.Hour {
		return time.Time{}, false
	}
	return mjdEpoch.AddDate(0, 0, mjd).Add(clock), true
}

// bcdDuration reads six BCD digits as hours, minutes, and seconds.
func bcdDuration(b []byte) (time.Duration, bool) {
	var v [3]int
	for i := range 3 {
		hi, lo := int(b[i]>>4), int(b[i]&0x0F)
		if hi > 9 || lo > 9 {
			return 0, false
		}
		v[i] = hi*10 + lo
	}
	if v[1] > 59 || v[2] > 59 {
		return 0, false
	}
	return time.Duration(v[0])*time.Hour + time.Duration(v[1])*time.Minute + time.Duration(v[2])*time.Second, true
}

func sortedKeys(m map[string][]extendedPart) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
