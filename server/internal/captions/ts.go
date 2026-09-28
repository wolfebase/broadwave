// Package captions reads CEA-608 captions out of a broadcast transport
// stream and turns them into timed cues for WebVTT.
package captions

const (
	streamMPEG2 = 0x02
	streamH264  = 0x1b

	// reorderDepth pictures are held so captions come out in display order.
	// MPEG-2 sends B-frames after the picture they follow on screen, and the
	// 608 bytes in each picture only make sense in display order.
	reorderDepth = 8
)

type picture struct {
	pts int64
	cc  []byte
}

// Reader takes a transport stream in chunks of any size and hands each
// picture's field-1 caption bytes to fn in display order, stamped with the
// picture's presentation time (90 kHz, 33-bit).
type Reader struct {
	program int
	fn      func(pts int64, pairs []byte)

	carry  []byte
	pmtPID int
	video  int
	kind   int
	pes    []byte
	held   []picture
}

// NewReader follows the given program (0 is the first one in the PAT).
func NewReader(program int, fn func(pts int64, pairs []byte)) *Reader {
	return &Reader{program: program, fn: fn, pmtPID: -1, video: -1}
}

func (r *Reader) Write(p []byte) (int, error) {
	data := p
	if len(r.carry) > 0 {
		data = append(r.carry, p...)
		r.carry = nil
	}
	off := 0
	for off+188 <= len(data) {
		if data[off] != 0x47 {
			off++
			continue
		}
		r.packet(data[off : off+188])
		off += 188
	}
	if off < len(data) {
		r.carry = append([]byte(nil), data[off:]...)
	}
	return len(p), nil
}

// Flush hands over every picture still held for reordering.
func (r *Reader) Flush() {
	r.endPES()
	for len(r.held) > 0 {
		r.emit()
	}
}

func (r *Reader) packet(pkt []byte) {
	pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
	start := pkt[1]&0x40 != 0
	afc := pkt[3] >> 4 & 0x3
	if afc == 0 || afc == 2 {
		return
	}
	body := pkt[4:]
	if afc == 3 {
		n := int(pkt[4])
		if 1+n > len(body) {
			return
		}
		body = body[1+n:]
	}
	switch {
	case pid == 0 && start:
		r.pat(body)
	case pid == r.pmtPID && start:
		r.pmt(body)
	case pid == r.video:
		if start {
			r.endPES()
			r.pes = append(r.pes[:0], body...)
		} else if len(r.pes) > 0 {
			r.pes = append(r.pes, body...)
		}
	}
}

func section(body []byte) []byte {
	if len(body) == 0 || int(body[0])+1 > len(body) {
		return nil
	}
	sec := body[1+int(body[0]):]
	if len(sec) < 12 {
		return nil
	}
	n := 3 + (int(sec[1]&0x0f)<<8 | int(sec[2])) - 4
	if n > len(sec) || n < 12 {
		return nil
	}
	return sec[:n]
}

func (r *Reader) pat(body []byte) {
	sec := section(body)
	if sec == nil || sec[0] != 0x00 {
		return
	}
	for off := 8; off+4 <= len(sec); off += 4 {
		prog := int(sec[off])<<8 | int(sec[off+1])
		if prog == 0 || (r.program != 0 && prog != r.program) {
			continue
		}
		r.pmtPID = int(sec[off+2]&0x1f)<<8 | int(sec[off+3])
		return
	}
}

func (r *Reader) pmt(body []byte) {
	sec := section(body)
	if sec == nil || sec[0] != 0x02 {
		return
	}
	off := 12 + (int(sec[10]&0x0f)<<8 | int(sec[11]))
	for off+5 <= len(sec) {
		kind := int(sec[off])
		pid := int(sec[off+1]&0x1f)<<8 | int(sec[off+2])
		off += 5 + (int(sec[off+3]&0x0f)<<8 | int(sec[off+4]))
		if kind != streamMPEG2 && kind != streamH264 {
			continue
		}
		if pid != r.video {
			r.pes = r.pes[:0]
		}
		r.video, r.kind = pid, kind
		return
	}
}

func (r *Reader) endPES() {
	b := r.pes
	r.pes = r.pes[:0]
	if len(b) < 14 || b[0] != 0 || b[1] != 0 || b[2] != 1 || b[7]&0x80 == 0 {
		return
	}
	payload := 9 + int(b[8])
	if payload > len(b) {
		return
	}
	pts := int64(b[9]>>1&0x07)<<30 | int64(b[10])<<22 | int64(b[11]>>1)<<15 | int64(b[12])<<7 | int64(b[13]>>1)
	var cc []byte
	if r.kind == streamH264 {
		cc = fromH264(b[payload:])
	} else {
		cc = fromMPEG2(b[payload:])
	}
	r.held = append(r.held, picture{pts: pts, cc: cc})
	if len(r.held) > reorderDepth {
		r.emit()
	}
}

// emit hands over the held picture that shows first.
func (r *Reader) emit() {
	first := 0
	for i := 1; i < len(r.held); i++ {
		if ptsDelta(r.held[i].pts, r.held[first].pts) < 0 {
			first = i
		}
	}
	pic := r.held[first]
	r.held = append(r.held[:first], r.held[first+1:]...)
	if len(pic.cc) > 0 {
		r.fn(pic.pts, pic.cc)
	}
}

// ptsDelta is a - b on the 33-bit clock, across a wrap.
func ptsDelta(a, b int64) int64 {
	d := (a - b) & (1<<33 - 1)
	if d >= 1<<32 {
		d -= 1 << 33
	}
	return d
}
