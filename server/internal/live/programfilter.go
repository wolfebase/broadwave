package live

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
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
	// pktBuf and filtBuf are refilled on every write. The writer copies
	// before Write returns (a pipe or a file), so the next write may reuse them.
	pktBuf  []byte
	filtBuf []byte
	clocks  map[int]*pesClock
	warned  time.Time
	// onBreak, when set, is told about a real backwards break in the picture
	// instead of the filter following it: ffmpeg clamps every later packet of
	// a copied track to the old timeline's high point, so the encode has to
	// start again. From the break on, the stream is kept in tail until the
	// next encode's input arrives on sw; it then starts from the first packet
	// of the new timeline.
	onBreak  func()
	sw       *pipeSwitch
	broken   bool
	catching bool
	pending  []byte
	tail     []byte
	breaks   []time.Time
}

// pipeSwitch passes the next encode's input to the goroutine writing the pipe.
type pipeSwitch struct {
	mu   sync.Mutex
	next io.WriteCloser
}

func (s *pipeSwitch) give(w io.WriteCloser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next != nil {
		_ = s.next.Close()
	}
	s.next = w
}

func (s *pipeSwitch) take() io.WriteCloser {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.next
	s.next = nil
	return w
}

// tailCap bounds what a broken pipe keeps while the next encode starts.
const tailCap = 16 << 20

// breakLimit is how many breaks a minute start the encode again. A stream
// that keeps jumping back is followed as before, with the clamp.
const breakLimit = 5

// pesJump is how far one stream's timestamp may move from one PES header to
// the next. A bit error the tuner did not flag can put a frame hours away.
// ffmpeg keeps broadcast timestamps (-copyts), so on a copied track every
// later packet is clamped to that one plus a tick and the Apple rendition
// never plays again; the transcode's resampler rides through.
const pesJump = 10 * 90000

// pesAgree is how many headers in a row must agree on a new timeline before
// it counts as a real break (a splice) rather than a bad header.
const pesAgree = 3

// pesBack is how far a timestamp may step back before it counts as a break.
// Decode times only move forward; video without them reorders by a few frames.
const pesBack = 90000

// pesClock is one stream's timeline. shift renumbers continuity counters
// after dropped packets, so ffmpeg does not also discard the next good frame.
type pesClock struct {
	last     int64
	cand     int64
	agree    int
	dropping bool
	shift    byte
}

// anyProgram asks a program pipe for the first program its PAT lists. An
// ATSC 3.0 tune is a stream of one program whose number is not stored, and
// an encode that joins it mid-group still needs to start on its parameter sets.
const anyProgram = -1

func newProgramPipe(w io.WriteCloser, program int) io.WriteCloser {
	if w == nil || program == 0 {
		return w
	}
	return &programPipe{w: w, program: program, pmtVer: -1}
}

func (p *programPipe) Write(chunk []byte) (int, error) {
	n := len(chunk)
	if p.broken {
		if len(p.tail)+len(chunk) <= tailCap {
			p.tail = append(p.tail, chunk...)
		}
		next := p.sw.take()
		if next == nil {
			return n, nil
		}
		old, tail := p.w, p.tail
		*p = programPipe{w: next, program: p.program, pmtVer: -1, onBreak: p.onBreak, sw: p.sw, breaks: p.breaks}
		_ = old.Close()
		_, err := p.Write(tail)
		return n, err
	}
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
	if p.sw != nil {
		if next := p.sw.take(); next != nil {
			_ = next.Close()
		}
	}
	if !p.pass && !p.ready && !p.broken {
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
	out := p.pktBuf[:0]
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
	p.pktBuf = out
	return out
}

// start drops the program until its first sequence header and puts the
// tables in front of it. ffmpeg probes half a second of input; an encode that
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
// (MPEG-2) or a parameter set (H.264, HEVC).
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
		// An HEVC random access point carries its VPS or SPS.
		if kind == streamHEVC && (code>>1)&0x3f >= 32 && (code>>1)&0x3f <= 33 {
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
		if p.program == anyProgram {
			p.program = firstProgram(sec)
		}
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
	out := p.filtBuf[:0]
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if p.catching {
			p.pending = append(p.pending, pkt...)
		}
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
			// The version sits in the section header, so an unchanged map
			// does not build a section copy on every repeat.
			if ver, ok := pmtVersion(pkt); !ok || ver != p.pmtVer {
				for _, sec := range sections(pkt, pid) {
					if len(sec) > 5 && sec[0] == 0x02 && int(sec[5]>>1)&0x1f != p.pmtVer {
						p.useMap(sec)
					}
				}
			}
		}
		if !p.keep[pid] || !p.keepPES(pid, pkt) {
			if p.broken {
				p.tail = append(append(p.pending, data[off+188:]...), p.rest...)
				p.rest, p.pending, p.catching = nil, nil, false
				p.filtBuf = out
				return out
			}
			continue
		}
		at := len(out)
		out = append(out, pkt...)
		if c := p.clocks[pid]; c != nil && c.shift != 0 && pkt[3]&0x10 != 0 {
			out[at+3] = out[at+3]&0xf0 | (pkt[3]-c.shift)&0x0f
		}
	}
	p.filtBuf = out
	return out
}

// pmtVersion is the version of a program map that starts in this packet.
// False means the header is not in this packet, and the caller parses it.
func pmtVersion(pkt []byte) (int, bool) {
	payload := tsPayload(pkt)
	if len(payload) < 7 {
		return 0, false
	}
	pointer := int(payload[0])
	off := 1 + pointer
	if off+6 > len(payload) || payload[off] != 0x02 {
		return 0, false
	}
	return int(payload[off+5]>>1) & 0x1f, true
}

// keepPES drops a PES whose timestamp is off its stream's timeline, and the
// rest of that PES, until pesAgree headers in a row agree on a new one.
func (p *programPipe) keepPES(pid int, pkt []byte) bool {
	if pid == p.pmtPID || pkt[3]&0x10 == 0 {
		return true
	}
	c := p.clocks[pid]
	if pkt[1]&0x40 == 0 {
		if c != nil && c.dropping {
			c.shift++
			return false
		}
		return true
	}
	ts, ok := pesTime(tsPayload(pkt))
	if !ok {
		if c != nil {
			c.dropping = false
		}
		return true
	}
	if c == nil {
		if p.clocks == nil {
			p.clocks = map[int]*pesClock{}
		}
		p.clocks[pid] = &pesClock{last: ts}
		return true
	}
	if d := ptsDelta(ts, c.last); d <= pesJump && d >= -pesBack {
		if pid == p.video {
			p.catching, p.pending = false, nil
		}
		c.last, c.agree, c.dropping = ts, 0, false
		return true
	}
	if c.agree > 0 && ptsGap(ts, c.cand) <= pesJump {
		c.agree++
	} else {
		c.agree = 1
		// The first packet of what may be a new timeline, and everything
		// after it, is what the next encode would start from.
		if pid == p.video && p.canBreak() && ptsDelta(ts, c.last) < 0 {
			p.catching = true
			p.pending = append(p.pending[:0], pkt...)
		}
		if time.Since(p.warned) > 10*time.Second {
			p.warned = time.Now()
			slog.Warn(fmt.Sprintf("program %d: dropped a frame on PID %d whose timestamp is %.0f s off", p.program, pid, float64(ptsDelta(ts, c.last))/90000))
		}
	}
	if p.catching && len(p.pending) > tailCap {
		p.catching, p.pending = false, nil
	}
	c.cand = ts
	if c.agree >= pesAgree {
		if pid == p.video && p.catching && ptsDelta(ts, c.last) < 0 {
			slog.Info(fmt.Sprintf("program %d: timestamps went back %.1f s; starting the encode again", p.program, float64(-ptsDelta(ts, c.last))/90000))
			p.breaks = append(p.breaks, time.Now())
			p.broken = true
			p.onBreak()
			return false
		}
		if pid == p.video {
			p.catching, p.pending = false, nil
		}
		c.last, c.agree, c.dropping = ts, 0, false
		return true
	}
	c.dropping = true
	c.shift++
	return false
}

// canBreak is whether a backwards break starts the encode again. Past
// breakLimit in a minute, the filter follows breaks the way it used to.
func (p *programPipe) canBreak() bool {
	if p.onBreak == nil || p.sw == nil {
		return false
	}
	cut := time.Now().Add(-time.Minute)
	for len(p.breaks) > 0 && p.breaks[0].Before(cut) {
		p.breaks = p.breaks[1:]
	}
	if len(p.breaks) < breakLimit {
		return true
	}
	if time.Since(p.warned) > 10*time.Second {
		p.warned = time.Now()
		slog.Warn(fmt.Sprintf("program %d: timestamps keep going back; following them in the same encode", p.program))
	}
	return false
}

// pesTime is the decode time of a PES that starts in this payload (the
// presentation time when there is no separate decode time), in 90 kHz ticks.
func pesTime(b []byte) (int64, bool) {
	if len(b) < 14 || b[0] != 0 || b[1] != 0 || b[2] != 1 {
		return 0, false
	}
	switch id := b[3]; {
	case id == 0xbd, id >= 0xc0 && id <= 0xef:
	default:
		return 0, false
	}
	switch b[7] >> 6 {
	case 2:
		return ptsField(b[9:14]), true
	case 3:
		if len(b) < 19 {
			return 0, false
		}
		return ptsField(b[14:19]), true
	}
	return 0, false
}

func ptsField(b []byte) int64 {
	return int64(b[0]>>1&0x07)<<30 | int64(b[1])<<22 | int64(b[2]>>1)<<15 | int64(b[3])<<7 | int64(b[4]>>1)
}

// ptsDelta is a - b on the 33-bit clock, across a wrap.
func ptsDelta(a, b int64) int64 {
	d := (a - b) & (1<<33 - 1)
	if d >= 1<<32 {
		d -= 1 << 33
	}
	return d
}

func ptsGap(a, b int64) int64 {
	d := ptsDelta(a, b)
	if d < 0 {
		return -d
	}
	return d
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
