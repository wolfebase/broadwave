package live

import (
	"io"
)

// programFilterCap is how much of a multiplex to hold while looking for one
// program's map. A PAT is repeated within 100 ms and a PMT within 400 ms,
// which is over a megabyte on a full multiplex. Past this, the bytes go
// through unchanged so a stream we cannot parse still plays.
const programFilterCap = 2 << 20

// programPipe writes one program to a rendition. ffmpeg reads every video
// stream in its input before it emits a frame, and it waits out the probe
// ceiling while any of them still has no picture size. A sibling subchannel
// on the same multiplex does that. Recordings and the scan still see the
// whole multiplex; only the rendition's pipe is narrowed.
type programPipe struct {
	w       io.WriteCloser
	program int
	ready   bool
	pass    bool
	synced  bool
	keep    map[int]bool
	pmtPID  int
	pmtVer  int
	pmtPkts []byte
	video   int
	kind    int
	started bool
	skipped int
	pat     [188]byte
	cc      byte
	hold    []byte
	rest    []byte
}

func newProgramPipe(w io.WriteCloser, program int) io.WriteCloser {
	if w == nil || program <= 0 {
		return w
	}
	return &programPipe{w: w, program: program, pmtVer: -1}
}

func (p *programPipe) Write(chunk []byte) (int, error) {
	n := len(chunk)
	if p.pass {
		_, err := p.w.Write(chunk)
		return n, err
	}
	p.rest = append(p.rest, chunk...)
	if !p.synced {
		// An encode that attaches mid-stream starts inside a packet, and a
		// payload holds 0x47 bytes too. Lock only where three sync bytes line up.
		i := syncOffset(p.rest, 3)
		if i < 0 {
			if len(p.rest) < 188*16 {
				return n, nil
			}
			return n, p.giveUp()
		}
		p.synced = true
		p.rest = append([]byte(nil), p.rest[i:]...)
	}
	pkts := p.packets()
	if !p.ready {
		p.hold = append(p.hold, pkts...)
		if !p.learn(p.hold) {
			if !p.pass && len(p.hold) < programFilterCap {
				return n, nil
			}
			return n, p.giveUp()
		}
		pkts = p.hold
		p.hold = nil
	}
	out := p.filter(pkts)
	if !p.started {
		out = p.start(out)
	}
	if len(out) == 0 {
		return n, nil
	}
	_, err := p.w.Write(out)
	return n, err
}

// giveUp sends everything held, and the rest of the stream, unchanged.
func (p *programPipe) giveUp() error {
	p.pass = true
	buf := append(p.hold, p.rest...)
	p.hold, p.rest = nil, nil
	if len(buf) == 0 {
		return nil
	}
	_, err := p.w.Write(buf)
	return err
}

func (p *programPipe) Close() error {
	if !p.pass && !p.ready {
		_ = p.giveUp()
	}
	return p.w.Close()
}

// syncOffset is the first offset where n sync bytes sit one packet apart.
func syncOffset(data []byte, n int) int {
	for i := 0; i+188*(n-1) < len(data); i++ {
		ok := true
		for k := range n {
			if data[i+188*k] != 0x47 {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// packets takes the whole packets out of p.rest. A packet counts only when
// the next two start where it ends: a slow encode loses whole reads, which
// are not packet sized, and the packet across that seam is half one packet
// and half another. The last two packets wait for the next write.
func (p *programPipe) packets() []byte {
	data := p.rest
	var out []byte
	off := 0
	for off+376 < len(data) {
		if data[off] != 0x47 || data[off+188] != 0x47 || data[off+376] != 0x47 {
			off++
			continue
		}
		out = append(out, data[off:off+188]...)
		off += 188
	}
	// The first write can be the whole lead; keep only what is left.
	if cap(data) > 64<<10 {
		p.rest = append([]byte(nil), data[off:]...)
	} else {
		p.rest = append(data[:0], data[off:]...)
	}
	return out
}

// start drops the program until its first sequence header and puts the
// tables in front of it. ffmpeg probes one second of input; an encode that
// joins mid-picture group can reach that ceiling before a sequence header,
// and a VAAPI decoder opened without a picture size never recovers.
func (p *programPipe) start(out []byte) []byte {
	if p.video == 0 {
		p.started = true
		return out
	}
	for off := 0; off+188 <= len(out); off += 188 {
		pkt := out[off : off+188]
		if int(pkt[1]&0x1f)<<8|int(pkt[2]) != p.video || pkt[1]&0x40 == 0 {
			continue
		}
		if !sequenceStart(tsPayload(pkt), p.kind) {
			continue
		}
		p.started = true
		pat := p.pat
		pat[3] = 0x10 | (p.cc & 0x0f)
		p.cc = (p.cc + 1) & 0x0f
		head := append(pat[:], p.pmtPkts...)
		return append(head, out[off:]...)
	}
	// A picture group is at most a few seconds; past that, send what comes.
	p.skipped += len(out)
	if p.skipped > programFilterCap {
		p.started = true
		return out
	}
	return nil
}

func tsPayload(pkt []byte) []byte {
	switch (pkt[3] >> 4) & 0x3 {
	case 1:
		return pkt[4:]
	case 3:
		if 5+int(pkt[4]) <= len(pkt) {
			return pkt[5+int(pkt[4]):]
		}
	}
	return nil
}

// sequenceStart reports whether a PES start carries a sequence header
// (MPEG-2) or a sequence parameter set (H.264).
func sequenceStart(payload []byte, kind int) bool {
	es := pesPayload(payload)
	for i := 0; i+3 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 {
			continue
		}
		code := es[i+3]
		if kind == streamMPEG2 && code == 0xb3 {
			return true
		}
		if kind == streamH264 && code&0x1f == 7 {
			return true
		}
	}
	return false
}

func (p *programPipe) learn(buf []byte) bool {
	var tsID, pmtPID int
	absent := false
	for _, sec := range sections(buf, 0) {
		if len(sec) < 8 || sec[0] != 0x00 {
			continue
		}
		// A section cut off by the end of this buffer is not the whole table.
		if sectionEnd(sec)+4 > len(sec) {
			continue
		}
		tsID = int(sec[3])<<8 | int(sec[4])
		if pid := patPMT(sec, p.program); pid != 0 {
			pmtPID = pid
			absent = false
			break
		}
		absent = true
	}
	if pmtPID == 0 {
		// This program is not in a complete table, so holding cannot help.
		if absent {
			p.pass = true
		}
		return false
	}
	p.pmtPID = pmtPID
	for _, sec := range sections(buf, pmtPID) {
		if p.useMap(sec) {
			p.pat = singleProgramPAT(p.program, pmtPID, tsID)
			p.ready = true
			return true
		}
	}
	return false
}

// useMap keeps the streams a complete program map lists for this program.
func (p *programPipe) useMap(sec []byte) bool {
	if len(sec) < 12 || sec[0] != 0x02 {
		return false
	}
	// A table split across packets is not finished in the first one.
	// Parsing that prefix freezes the stream list without the rest.
	if sectionEnd(sec)+4 > len(sec) {
		return false
	}
	if int(sec[3])<<8|int(sec[4]) != p.program {
		return false
	}
	pids := pmtElementary(sec)
	if len(pids) == 0 {
		return false
	}
	keep := map[int]bool{0: true, p.pmtPID: true}
	if pcr := int(sec[8]&0x1f)<<8 | int(sec[9]); pcr != 0x1FFF {
		keep[pcr] = true
	}
	for _, pid := range pids {
		keep[pid] = true
	}
	p.keep = keep
	p.pmtVer = int(sec[5]>>1) & 0x1f
	p.video, p.kind = pmtVideo(sec)
	return true
}

func (p *programPipe) filter(data []byte) []byte {
	var out []byte
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if pid == 0 {
			pat := p.pat
			pat[3] = 0x10 | (p.cc & 0x0f)
			p.cc = (p.cc + 1) & 0x0f
			out = append(out, pat[:]...)
			continue
		}
		if pid == p.pmtPID {
			if pkt[1]&0x40 != 0 {
				p.pmtPkts = p.pmtPkts[:0]
			}
			if len(p.pmtPkts) < 188*4 {
				p.pmtPkts = append(p.pmtPkts, pkt...)
			}
		}
		if pid == p.pmtPID && pkt[1]&0x40 != 0 {
			// A station can move a stream to a new PID; follow its new map.
			for _, sec := range sections(pkt, pid) {
				if len(sec) > 5 && sec[0] == 0x02 && int(sec[5]>>1)&0x1f != p.pmtVer {
					p.useMap(sec)
				}
			}
		}
		if p.keep[pid] {
			out = append(out, pkt...)
		}
	}
	return out
}

func pmtElementary(sec []byte) []int {
	info := int(sec[10]&0x0f)<<8 | int(sec[11])
	off := 12 + info
	end := sectionEnd(sec)
	var pids []int
	for off+5 <= end {
		pid := int(sec[off+1]&0x1f)<<8 | int(sec[off+2])
		esInfo := int(sec[off+3]&0x0f)<<8 | int(sec[off+4])
		off += 5 + esInfo
		if pid != 0 && pid != 0x1FFF {
			pids = append(pids, pid)
		}
	}
	return pids
}

func singleProgramPAT(program, pmtPID, tsID int) [188]byte {
	body := []byte{
		byte(tsID >> 8), byte(tsID),
		0xc1, 0x00, 0x00,
		byte(program >> 8), byte(program),
		0xe0 | byte(pmtPID>>8), byte(pmtPID),
	}
	n := len(body) + 4
	sec := []byte{0x00, 0xb0 | byte(n>>8), byte(n)}
	sec = append(sec, body...)
	crc := mpegCRC(sec)
	sec = append(sec, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	var pkt [188]byte
	pkt[0] = 0x47
	pkt[1] = 0x40
	pkt[3] = 0x10
	pkt[4] = 0x00
	copy(pkt[5:], sec)
	for i := 5 + len(sec); i < len(pkt); i++ {
		pkt[i] = 0xff
	}
	return pkt
}

// mpegCRC is the MPEG-2 PSI checksum (CRC-32/MPEG-2), not IEEE.
func mpegCRC(data []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc = (crc << 8) ^ mpegCRCTable[byte(crc>>24)^b]
	}
	return crc
}

var mpegCRCTable = func() [256]uint32 {
	var t [256]uint32
	for i := range t {
		crc := uint32(i) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
		t[i] = crc
	}
	return t
}()
