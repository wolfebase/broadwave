package fake

import (
	"encoding/binary"
	"net/http"
	"time"

	"broadwave/internal/dvbsi"
)

// A DVB profile's streams carry an SDT and EIT (EN 300 468) for its lineup,
// with made-up listings around the current time: now and next for every
// service, and three hours of schedule for the services on the tuned
// multiplex. Services on another multiplex are sent as "other" tables.

const fakeNetworkID = 0x3FF0

// fakeListing is one made-up show. Title is the coded string, table byte
// included, so the default Latin table and UTF-8 are both on the air.
type fakeListing struct {
	title, text []byte
	genre       byte
}

var fakeSchedule = map[string][]fakeListing{
	"1": {
		{[]byte("Quiz Night"), []byte("Teams of four take on the board."), 0x31},
		{[]byte("Harbour Lights"), []byte("A ferry crew waits out the fog."), 0x10},
		{[]byte{'C', 'a', 'f', 0xC2, 'e', ' ', 'S', 't', 'o', 'r', 'i', 'e', 's'}, []byte("Regulars at a corner table."), 0x15},
	},
	"2": {
		{[]byte("Little Explorers"), []byte("A rock pool at low tide."), 0x50},
		{[]byte("Studio Kitchen"), []byte("Three soups from one stock."), 0xA5},
		{[]byte("Night Match"), []byte("Second leg, from the coast."), 0x43},
	},
	"21": {
		{[]byte("Evening Bulletin"), []byte("The day's news."), 0x20},
		{[]byte("\x15Weather Watch ☀"), []byte("The week ahead."), 0x21},
	},
}

const fakeSlot = 30 * time.Minute

// streamID is the fake transport_stream_id of a multiplex.
func streamID(freq int) uint16 {
	return uint16(freq / 1000000)
}

type siCarrier struct {
	cc map[int]int
}

func (c *siCarrier) packets(pid int, sections ...[]byte) []byte {
	var out []byte
	for _, sec := range sections {
		first := true
		for len(sec) > 0 {
			p := make([]byte, 188)
			p[0] = 0x47
			p[1] = byte(pid>>8) & 0x1F
			p[2] = byte(pid)
			p[3] = 0x10 | byte(c.cc[pid]&0x0F)
			c.cc[pid]++
			off := 4
			if first {
				p[1] |= 0x40
				off = 5
				first = false
			}
			n := copy(p[off:], sec)
			sec = sec[n:]
			for i := off + n; i < 188; i++ {
				p[i] = 0xFF
			}
			out = append(out, p...)
		}
	}
	return out
}

func siSection(table byte, ext uint16, rest []byte) []byte {
	length := 5 + len(rest) + 4
	b := []byte{table, 0xF0 | byte(length>>8), byte(length), byte(ext >> 8), byte(ext), 0xC1, 0x00, 0x00}
	b = append(b, rest...)
	return binary.BigEndian.AppendUint32(b, dvbsi.CRC(b))
}

// carousel is one pass of the SI a multiplex at freq sends.
func (s *Server) carousel(c *siCarrier, freq int, now time.Time) []byte {
	sdt := map[uint16][]byte{}
	var eit [][]byte
	for _, ch := range s.Channels {
		if ch.Program == 0 {
			continue
		}
		tsid := streamID(ch.Freq)
		actual := ch.Freq == freq
		name := []byte(ch.Name)
		desc := append([]byte{0x48, byte(3 + len(name)), 0x01, 0x00, byte(len(name))}, name...)
		svc := []byte{byte(ch.Program >> 8), byte(ch.Program), 0xFF, 0x80 | byte(len(desc)>>8), byte(len(desc))}
		sdt[tsid] = append(sdt[tsid], append(svc, desc...)...)
		shows := fakeSchedule[ch.Number]
		if len(shows) == 0 {
			continue
		}
		slot := now.UTC().Truncate(fakeSlot)
		pfTable, schedTable := byte(0x4F), byte(0x60)
		if actual {
			pfTable, schedTable = 0x4E, 0x50
		}
		eit = append(eit, s.eitSection(pfTable, ch, tsid, shows, slot, 2))
		if actual {
			eit = append(eit, s.eitSection(schedTable, ch, tsid, shows, slot.Add(-fakeSlot), 6))
		}
	}
	var out []byte
	for tsid, services := range sdt {
		table := byte(0x46)
		if tsid == streamID(freq) {
			table = 0x42
		}
		rest := append([]byte{byte(fakeNetworkID >> 8), byte(fakeNetworkID & 0xFF), 0xFF}, services...)
		out = append(out, c.packets(0x11, siSection(table, tsid, rest))...)
	}
	return append(out, c.packets(0x12, eit...)...)
}

func (s *Server) eitSection(table byte, ch Channel, tsid uint16, shows []fakeListing, from time.Time, n int) []byte {
	rest := []byte{byte(tsid >> 8), byte(tsid), byte(fakeNetworkID >> 8), byte(fakeNetworkID & 0xFF), 0x00, table}
	for i := range n {
		start := from.Add(time.Duration(i) * fakeSlot)
		index := int(start.Unix() / int64(fakeSlot/time.Second))
		show := shows[index%len(shows)]
		short := append([]byte("eng"), byte(len(show.title)))
		short = append(short, show.title...)
		short = append(short, byte(len(show.text)))
		short = append(short, show.text...)
		desc := append([]byte{0x4D, byte(len(short))}, short...)
		desc = append(desc, 0x54, 0x02, show.genre, 0x00)
		id := uint16(index)
		rest = append(rest, byte(id>>8), byte(id))
		rest = append(rest, mjdBCD(start)...)
		rest = append(rest, 0x00, 0x30, 0x00) // 30 minutes
		rest = append(rest, 0x80|byte(len(desc)>>8), byte(len(desc)))
		rest = append(rest, desc...)
	}
	return siSection(table, uint16(ch.Program), rest)
}

func mjdBCD(t time.Time) []byte {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	mjd := int(day.Sub(time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	clock := int(t.Sub(day) / time.Second)
	digit := func(v int) byte { return byte(v/10<<4 | v%10) }
	return []byte{byte(mjd >> 8), byte(mjd), digit(clock / 3600), digit(clock / 60 % 60), digit(clock % 60)}
}

// siWriter puts the SI carousel after every fourth write of a stream. The
// stream's own first packet still comes first.
type siWriter struct {
	http.ResponseWriter
	s      *Server
	freq   int
	writes int
	c      siCarrier
}

func (w *siWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return n, err
	}
	if w.writes%4 == 0 {
		if _, err := w.ResponseWriter.Write(w.s.carousel(&w.c, w.freq, time.Now())); err != nil {
			return n, err
		}
	}
	w.writes++
	return n, nil
}

func (w *siWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func (s *Server) withSI(w http.ResponseWriter, freq int) http.ResponseWriter {
	if !s.spec.DVB {
		return w
	}
	return &siWriter{ResponseWriter: w, s: s, freq: freq, c: siCarrier{cc: map[int]int{}}}
}
