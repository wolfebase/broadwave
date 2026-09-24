// Package psip reads the ATSC A/65 guide tables carried in a broadcast mux.
package psip

import (
	"encoding/binary"
	"io"
	"time"
)

const (
	pidBase   = 0x1FFB
	tableMGT  = 0xC7
	tableVCT  = 0xC8
	tableCVCT = 0xC9
	tableEIT  = 0xCB
	tableETT  = 0xCC
	tableSTT  = 0xCD
	tagGenre  = 0xAB
)

// GPS epoch is 1980-01-06 UTC.
var gpsEpoch = time.Date(1980, 1, 6, 0, 0, 0, 0, time.UTC)

// Channel is one row of the virtual channel table.
type Channel struct {
	Major     int
	Minor     int
	ShortName string
	SourceID  int
	Program   int
}

// Event is one EIT listing. Start is UTC.
type Event struct {
	SourceID int
	EventID  int
	Title    string
	Start    time.Time
	End      time.Time
	Genres   []string
}

// Text is one extended description, keyed by source and event.
type Text struct {
	SourceID int
	EventID  int
	Body     string
}

// Guide is everything a capture of one mux yielded.
type Guide struct {
	Channels []Channel
	Events   []Event
	Texts    []Text
	// EITPIDs maps an EIT table number (0 = the current 3 hours) to its PID.
	EITPIDs map[int]int
	Offset  time.Duration
}

// Parse reads a transport stream and returns the PSIP it contains.
func Parse(r io.Reader) (Guide, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Guide{}, err
	}
	sections := assemble(raw)
	g := Guide{EITPIDs: map[int]int{}}
	var offset uint8
	var haveSTT bool
	for _, sec := range sections {
		if len(sec.Body) < 1 {
			continue
		}
		switch sec.Body[0] {
		case tableMGT:
			for n, pid := range parseMGT(sec.Body) {
				g.EITPIDs[n] = pid
			}
		case tableVCT, tableCVCT:
			g.Channels = append(g.Channels, parseVCT(sec.Body)...)
		case tableSTT:
			off, ok := parseSTTOffset(sec.Body)
			if ok {
				offset = off
				haveSTT = true
			}
		}
	}
	if haveSTT {
		g.Offset = time.Duration(offset) * time.Second
	}
	for _, sec := range sections {
		if len(sec.Body) < 1 {
			continue
		}
		switch sec.Body[0] {
		case tableEIT:
			g.Events = append(g.Events, parseEIT(sec.Body, offset)...)
		case tableETT:
			if t, ok := parseETT(sec.Body); ok {
				g.Texts = append(g.Texts, t)
			}
		}
	}
	return g, nil
}

type section struct {
	PID  int
	Body []byte
}

func assemble(raw []byte) []section {
	type acc struct {
		buf []byte
	}
	open := map[int]*acc{}
	var out []section
	for i := 0; i+188 <= len(raw); i += 188 {
		if raw[i] != 0x47 {
			continue
		}
		pid := int(raw[i+1]&0x1F)<<8 | int(raw[i+2])
		if pid != pidBase && (pid < 0x1300 || pid > 0x1FFF) {
			continue
		}
		pusi := raw[i+1]&0x40 != 0
		afc := (raw[i+3] >> 4) & 3
		off := 4
		if afc&0x2 != 0 {
			off = 5 + int(raw[i+4])
			if off > 188 {
				continue
			}
		}
		payload := raw[i+off : i+188]
		if len(payload) == 0 {
			continue
		}
		if pusi {
			pointer := int(payload[0])
			if pointer+1 > len(payload) {
				continue
			}
			payload = payload[1+pointer:]
			open[pid] = &acc{}
		}
		a := open[pid]
		if a == nil {
			continue
		}
		a.buf = append(a.buf, payload...)
		for len(a.buf) >= 3 {
			seclen := int(a.buf[1]&0x0F)<<8 | int(a.buf[2])
			total := 3 + seclen
			if seclen < 4 || total > 4096 || len(a.buf) < total {
				break
			}
			body := append([]byte(nil), a.buf[:total]...)
			if crcOK(body) {
				out = append(out, section{PID: pid, Body: body})
			}
			a.buf = a.buf[total:]
		}
	}
	return out
}

func crcOK(section []byte) bool {
	if len(section) < 4 {
		return false
	}
	return mpegCRC(section[:len(section)-4]) == binary.BigEndian.Uint32(section[len(section)-4:])
}

func parseMGT(b []byte) map[int]int {
	out := map[int]int{}
	// header 8 bytes, then protocol_version.
	if len(b) < 11 {
		return out
	}
	pos := 9
	n := int(binary.BigEndian.Uint16(b[pos : pos+2]))
	pos += 2
	for i := 0; i < n && pos+11 <= len(b)-4; i++ {
		tableType := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		pid := int(binary.BigEndian.Uint16(b[pos+2:pos+4])) & 0x1FFF
		descLen := int(binary.BigEndian.Uint16(b[pos+9:pos+11])) & 0x0FFF
		pos += 11
		if pos+descLen > len(b)-4 {
			break
		}
		pos += descLen
		if tableType >= 0x0100 && tableType <= 0x017F {
			out[tableType-0x0100] = pid
		}
	}
	return out
}

func parseVCT(b []byte) []Channel {
	if len(b) < 11 {
		return nil
	}
	pos := 9
	n := int(b[pos])
	pos++
	var out []Channel
	for i := 0; i < n && pos+32 <= len(b)-4; i++ {
		name := utf16Name(b[pos : pos+14])
		pos += 14
		word := uint32(b[pos])<<16 | uint32(b[pos+1])<<8 | uint32(b[pos+2])
		major := int((word >> 10) & 0x3FF)
		minor := int(word & 0x3FF)
		pos += 3 + 1 + 4 + 2
		program := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		pos += 2 + 2
		source := int(binary.BigEndian.Uint16(b[pos : pos+2]))
		pos += 2
		descLen := int(binary.BigEndian.Uint16(b[pos:pos+2])) & 0x03FF
		pos += 2
		if pos+descLen > len(b)-4 {
			break
		}
		pos += descLen
		out = append(out, Channel{Major: major, Minor: minor, ShortName: name, SourceID: source, Program: program})
	}
	return out
}

func parseSTTOffset(b []byte) (uint8, bool) {
	// 8 byte header + protocol_version + system_time(4) + offset.
	if len(b) < 14 {
		return 0, false
	}
	return b[13], true
}

func parseEIT(b []byte, offset uint8) []Event {
	if len(b) < 11 {
		return nil
	}
	source := int(binary.BigEndian.Uint16(b[3:5]))
	pos := 9
	n := int(b[pos])
	pos++
	var out []Event
	for i := 0; i < n && pos+8 <= len(b)-4; i++ {
		eventID := int(binary.BigEndian.Uint16(b[pos:pos+2])) & 0x3FFF
		startGPS := binary.BigEndian.Uint32(b[pos+2 : pos+6])
		if pos+9 >= len(b)-4 {
			break
		}
		packed := uint32(b[pos+6])<<16 | uint32(b[pos+7])<<8 | uint32(b[pos+8])
		length := packed & 0x000FFFFF
		pos += 9
		if pos >= len(b)-4 {
			break
		}
		titleLen := int(b[pos])
		pos++
		if pos+titleLen > len(b)-4 {
			break
		}
		title := decodeText(b[pos : pos+titleLen])
		pos += titleLen
		if pos+2 > len(b)-4 {
			break
		}
		descLen := int(binary.BigEndian.Uint16(b[pos:pos+2])) & 0x0FFF
		pos += 2
		if pos+descLen > len(b)-4 {
			break
		}
		genres := genresIn(b[pos : pos+descLen])
		pos += descLen
		start := gpsEpoch.Add(time.Duration(startGPS) * time.Second).Add(-time.Duration(offset) * time.Second)
		out = append(out, Event{
			SourceID: source,
			EventID:  eventID,
			Title:    title,
			Start:    start,
			End:      start.Add(time.Duration(length) * time.Second),
			Genres:   genres,
		})
	}
	return out
}

func parseETT(b []byte) (Text, bool) {
	// header 8 + protocol_version 1 + ETM_id 4 + text.
	if len(b) < 14 {
		return Text{}, false
	}
	etm := binary.BigEndian.Uint32(b[9:13])
	source := int((etm >> 16) & 0xFFFF)
	event := int((etm >> 2) & 0x3FFF)
	body := decodeText(b[13 : len(b)-4])
	if body == "" {
		return Text{}, false
	}
	return Text{SourceID: source, EventID: event, Body: body}, true
}

func genresIn(desc []byte) []string {
	var out []string
	for len(desc) >= 2 {
		tag := desc[0]
		n := int(desc[1])
		desc = desc[2:]
		if n > len(desc) {
			break
		}
		body := desc[:n]
		desc = desc[n:]
		if tag != tagGenre || len(body) < 1 {
			continue
		}
		// reserved 4 bits is inside attribute_count? attribute_count is the remaining bytes.
		for _, attr := range body {
			if name := genreName(attr); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func genreName(attr byte) string {
	// A/65 genre attributes. The high nibble is the category.
	switch attr {
	case 0x20:
		return "Education"
	case 0x21:
		return "Entertainment"
	case 0x22:
		return "Movie"
	case 0x23:
		return "News"
	case 0x24:
		return "Religious"
	case 0x25:
		return "Sports"
	case 0x26:
		return "Series"
	case 0x27:
		return "Special"
	case 0x34:
		return "Football"
	case 0x35:
		return "Basketball"
	case 0x37:
		return "Baseball"
	case 0x39:
		return "Hockey"
	default:
		return ""
	}
}

func utf16Name(b []byte) string {
	if len(b) < 14 {
		return ""
	}
	var runes []rune
	for i := 0; i+1 < 14; i += 2 {
		r := rune(binary.BigEndian.Uint16(b[i : i+2]))
		if r == 0 {
			break
		}
		runes = append(runes, r)
	}
	return string(runes)
}

// mpegCRC is the MPEG-2 CRC-32 (polynomial 0x04C11DB7, init 0xFFFFFFFF).
func mpegCRC(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, by := range data {
		crc ^= uint32(by) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
