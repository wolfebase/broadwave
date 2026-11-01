package dvbsi

import (
	"encoding/binary"
	"time"
)

// These helpers lay out sections and packets field by field, as EN 300 468
// and ISO/IEC 13818-1 draw them. No broadcast capture is used.

// longSection is a long-form section: table_id, length, the 16-bit table id
// extension, version 0 current, section 0 of 0, the rest, and the CRC.
func longSection(table byte, ext uint16, rest []byte) []byte {
	length := 5 + len(rest) + 4
	b := []byte{table, 0xF0 | byte(length>>8), byte(length), byte(ext >> 8), byte(ext), 0xC1, 0x00, 0x00}
	b = append(b, rest...)
	return binary.BigEndian.AppendUint32(b, CRC(b))
}

func eitSection(table byte, service, stream, network uint16, events ...[]byte) []byte {
	rest := []byte{byte(stream >> 8), byte(stream), byte(network >> 8), byte(network), 0x00, table}
	for _, e := range events {
		rest = append(rest, e...)
	}
	return longSection(table, service, rest)
}

func sdtSection(table byte, stream, network uint16, services ...[]byte) []byte {
	rest := []byte{byte(network >> 8), byte(network), 0xFF}
	for _, s := range services {
		rest = append(rest, s...)
	}
	return longSection(table, stream, rest)
}

func sdtService(id uint16, name []byte) []byte {
	desc := []byte{tagService, byte(3 + len(name)), 0x01, 0x00, byte(len(name))}
	desc = append(desc, name...)
	b := []byte{byte(id >> 8), byte(id), 0xFF, 0x80 | byte(len(desc)>>8), byte(len(desc))}
	return append(b, desc...)
}

// event is one EIT event: event_id, start (MJD and BCD), BCD duration,
// running (4), and its descriptors.
func event(id uint16, start time.Time, length time.Duration, descs ...[]byte) []byte {
	var desc []byte
	for _, d := range descs {
		desc = append(desc, d...)
	}
	b := []byte{byte(id >> 8), byte(id)}
	b = append(b, mjdBCD(start)...)
	b = append(b, bcd(length)...)
	b = append(b, 0x80|byte(len(desc)>>8), byte(len(desc)))
	return append(b, desc...)
}

func mjdBCD(t time.Time) []byte {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	mjd := int(day.Sub(mjdEpoch).Hours() / 24)
	return append([]byte{byte(mjd >> 8), byte(mjd)}, bcd(t.Sub(day))...)
}

func bcd(d time.Duration) []byte {
	s := int(d / time.Second)
	h, m, sec := s/3600, s/60%60, s%60
	digit := func(v int) byte { return byte(v/10<<4 | v%10) }
	return []byte{digit(h), digit(m), digit(sec)}
}

func shortDesc(lang string, name, text []byte) []byte {
	if 5+len(name)+len(text) > 255 {
		panic("a descriptor holds 255 bytes")
	}
	body := append([]byte(lang), byte(len(name)))
	body = append(body, name...)
	body = append(body, byte(len(text)))
	body = append(body, text...)
	return append([]byte{tagShortEvent, byte(len(body))}, body...)
}

func extendedDesc(number, last int, lang string, text []byte) []byte {
	body := append([]byte{byte(number<<4 | last)}, lang...)
	body = append(body, 0x00, byte(len(text)))
	body = append(body, text...)
	return append([]byte{tagExtendedEvent, byte(len(body))}, body...)
}

func contentDesc(pairs ...byte) []byte {
	var body []byte
	for _, p := range pairs {
		body = append(body, p, 0x00)
	}
	return append([]byte{tagContent, byte(len(body))}, body...)
}

func parentalDesc(country string, rating byte) []byte {
	body := append([]byte(country), rating)
	return append([]byte{tagParental, byte(len(body))}, body...)
}

// carrier packs sections onto one PID. Each call continues the
// continuity counter.
type carrier struct {
	pid int
	cc  int
}

func (c *carrier) packet(pusi bool, payload []byte) []byte {
	p := make([]byte, 188)
	p[0] = 0x47
	p[1] = byte(c.pid >> 8 & 0x1F)
	if pusi {
		p[1] |= 0x40
	}
	p[2] = byte(c.pid)
	p[3] = 0x10 | byte(c.cc&0x0F)
	c.cc++
	n := copy(p[4:], payload)
	for i := 4 + n; i < 188; i++ {
		p[i] = 0xFF
	}
	return p
}

// alone starts each section in a packet of its own, pointer 0, stuffed.
func (c *carrier) alone(secs ...[]byte) []byte {
	var out []byte
	for _, s := range secs {
		first := true
		for len(s) > 0 || first {
			var payload []byte
			if first {
				payload = append(payload, 0x00)
			}
			room := 184 - len(payload)
			n := min(room, len(s))
			payload = append(payload, s[:n]...)
			s = s[n:]
			out = append(out, c.packet(first, payload)...)
			first = false
		}
	}
	return out
}

// packed sends the sections back to back: a packet that ends one section
// and starts the next sets the pointer to where the new one begins.
func (c *carrier) packed(secs ...[]byte) []byte {
	var stream []byte
	var starts []int
	for _, s := range secs {
		starts = append(starts, len(stream))
		stream = append(stream, s...)
	}
	var out []byte
	pos := 0
	for pos < len(stream) {
		next := -1
		for _, st := range starts {
			if st >= pos && st < pos+183 {
				next = st
				break
			}
		}
		if next < 0 {
			n := min(184, len(stream)-pos)
			out = append(out, c.packet(false, stream[pos:pos+n])...)
			pos += n
			continue
		}
		payload := []byte{byte(next - pos)}
		n := min(183, len(stream)-pos)
		payload = append(payload, stream[pos:pos+n]...)
		out = append(out, c.packet(true, payload)...)
		pos += n
	}
	return out
}

func nullPackets(n int) []byte {
	var out []byte
	for range n {
		p := make([]byte, 188)
		p[0], p[1], p[2], p[3] = 0x47, 0x1F, 0xFF, 0x10
		out = append(out, p...)
	}
	return out
}
