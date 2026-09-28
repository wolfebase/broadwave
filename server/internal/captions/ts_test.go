package captions

import (
	"bytes"
	"testing"
)

const videoPID = 0x31

func tsPackets(pid int, payload []byte, start bool) []byte {
	var out []byte
	first := true
	for len(payload) > 0 || first {
		pkt := make([]byte, 188)
		pkt[0] = 0x47
		pkt[1] = byte(pid >> 8 & 0x1f)
		if first && start {
			pkt[1] |= 0x40
		}
		pkt[2] = byte(pid)
		n := min(len(payload), 184)
		if n < 184 {
			// Stuff with an adaptation field so the payload ends the packet.
			pkt[3] = 0x30
			pkt[4] = byte(183 - n)
			if pkt[4] > 0 {
				pkt[5] = 0
				for i := 6; i < 5+int(pkt[4]); i++ {
					pkt[i] = 0xff
				}
			}
			copy(pkt[188-n:], payload[:n])
		} else {
			pkt[3] = 0x10
			copy(pkt[4:], payload[:n])
		}
		payload = payload[n:]
		out = append(out, pkt...)
		first = false
	}
	return out
}

func psi(tableID byte, body []byte) []byte {
	n := len(body) + 5 + 4
	sec := []byte{0, tableID, 0xb0 | byte(n>>8), byte(n), 0, 1, 0xc1, 0, 0}
	sec = append(sec, body...)
	return append(sec, 0, 0, 0, 0) // the reader does not check the CRC
}

func header(kind int) []byte {
	pat := psi(0x00, []byte{0, 3, 0xe0, 0x30})
	// PCR PID, no program info, then one video stream.
	pmt := psi(0x02, []byte{0xe0, videoPID, 0xf0, 0, byte(kind), 0xe0, videoPID, 0xf0, 0})
	return append(tsPackets(0, pat, true), tsPackets(0x30, pmt, true)...)
}

func pes(pts int64, es []byte) []byte {
	b := []byte{0, 0, 1, 0xe0, 0, 0, 0x80, 0x80, 5,
		byte(0x21 | pts>>29&0x0e), byte(pts >> 22), byte(pts>>14 | 1), byte(pts >> 7), byte(pts<<1 | 1)}
	return tsPackets(videoPID, append(b, es...), true)
}

func ccBytes(pair []byte) []byte {
	return []byte{0x40 | 1, 0xff, 0xfc, pair[0], pair[1]}
}

func mpeg2Picture(pair []byte) []byte {
	es := []byte{0, 0, 1, 0x00, 0x12, 0x34, 0x56, 0x78}
	es = append(es, 0, 0, 1, 0xb2)
	es = append(es, ga94...)
	es = append(es, 0x03)
	es = append(es, ccBytes(pair)...)
	return append(es, 0xff, 0, 0, 1, 0x01, 0xaa, 0xbb)
}

func h264Picture(pair []byte) []byte {
	msg := append([]byte{0xb5, 0x00, 0x31}, ga94...)
	msg = append(msg, 0x03)
	msg = append(msg, ccBytes(pair)...)
	msg = append(msg, 0x00, 0x00, 0x00) // escaped below
	sei := append([]byte{4, byte(len(msg))}, msg...)
	sei = append(sei, 0x80)
	var esc []byte
	zeros := 0
	for _, c := range sei {
		if zeros >= 2 && c <= 3 {
			esc = append(esc, 3)
			zeros = 0
		}
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
		esc = append(esc, c)
	}
	es := []byte{0, 0, 0, 1, 0x09, 0xf0, 0, 0, 0, 1, 0x06}
	es = append(es, esc...)
	return append(es, 0, 0, 0, 1, 0x65, 0x88, 0x84)
}

// A caption spelled one pair per picture, sent in decode order: each P
// picture comes before the two B pictures shown ahead of it.
func stream(kind int, base int64, picture func([]byte) []byte) []byte {
	pairs := [][]byte{rcl, rcl, enm, enm, pac15, pac15}
	pairs = append(pairs, text("B-FRAMES FIRST")...)
	pairs = append(pairs, eoc, eoc)
	for len(pairs)%3 != 1 {
		pairs = append(pairs, []byte{0, 0})
	}
	ts := header(kind)
	at := func(i int) int64 { return (base + int64(i)*3003) & (1<<33 - 1) }
	ts = append(ts, pes(at(0), picture(pairs[0]))...)
	for i := 1; i+2 < len(pairs); i += 3 {
		ts = append(ts, pes(at(i+2), picture(pairs[i+2]))...)
		ts = append(ts, pes(at(i), picture(pairs[i]))...)
		ts = append(ts, pes(at(i+1), picture(pairs[i+1]))...)
	}
	return ts
}

func TestReaderPutsPicturesInDisplayOrder(t *testing.T) {
	for _, c := range []struct {
		name    string
		kind    int
		picture func([]byte) []byte
	}{{"mpeg2", streamMPEG2, mpeg2Picture}, {"h264", streamH264, h264Picture}} {
		t.Run(c.name, func(t *testing.T) {
			ts := stream(c.kind, 1<<33-30000, c.picture) // wraps partway
			for _, chunk := range []int{188, 1000, 7} {
				cues := decodeAll(t, ts, chunk)
				if len(cues) != 1 || cues[0].Text != "B-FRAMES FIRST" {
					t.Fatalf("chunk %d: cues %+v", chunk, cues)
				}
			}
		})
	}
}

func TestReaderFindsTheProgram(t *testing.T) {
	ts := stream(streamMPEG2, 0, mpeg2Picture)
	if cues := decodeAll(t, ts, 188); len(cues) != 1 {
		t.Fatalf("program 0: %d cues", len(cues))
	}
	var got int
	rd := NewReader(4, func(int64, []byte) { got++ })
	rd.Write(ts)
	rd.Flush()
	if got != 0 {
		t.Fatalf("program 4 is not in the stream, got %d pictures", got)
	}
	if !bytes.Contains(ts, ga94) {
		t.Fatal("fixture has no caption data")
	}
}

// Broadcast bytes are untrusted. Nothing in the reader or decoder may panic.
func FuzzReader(f *testing.F) {
	f.Add(stream(streamMPEG2, 1000, mpeg2Picture))
	f.Add(stream(streamH264, 1<<33-9000, h264Picture))
	f.Fuzz(func(t *testing.T, ts []byte) {
		d := NewDecoder()
		r := NewReader(0, d.Feed)
		for len(ts) > 0 {
			n := min(len(ts), 1000)
			_, _ = r.Write(ts[:n])
			ts = ts[n:]
		}
		r.Flush()
		_ = Segment(0, 90000, d.Take())
	})
}
