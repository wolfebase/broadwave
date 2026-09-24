package live

import (
	"bytes"
	"context"
	"log"
	"sync"
	"time"
)

// scanWait is how long the first tune may read the mux before ffmpeg starts.
// A sequence header or SPS in that window decides progressive vs interlaced.
// A sequence header rides the GOP. On a 720p station that landed about 460 ms
// into the tune, so 200 ms was not enough to see it. The read returns as soon
// as the header arrives and gives up after one GOP.
const scanWait = 800 * time.Millisecond

// scanType reports the video field order from MPEG-TS: "progressive" or "tt".
// ok is false until a sequence extension or an H.264 SPS has been seen.
func scanType(data []byte, program int) (order string, ok bool) {
	if i := bytes.IndexByte(data, 0x47); i > 0 && i < 188 {
		data = data[i:]
	}
	pmtPID, videoPID, kind := programVideo(data, program)
	if videoPID == 0 || pmtPID == 0 && program != 0 {
		return "", false
	}
	es := elementary(data, videoPID)
	switch kind {
	case streamMPEG2:
		return mpeg2Scan(es)
	case streamH264:
		return h264Scan(es)
	default:
		return "", false
	}
}

const (
	streamMPEG2 = 0x02
	streamH264  = 0x1b
)

func programVideo(data []byte, program int) (pmtPID, videoPID, kind int) {
	for _, sec := range sections(data, 0) {
		if len(sec) < 12 || sec[0] != 0x00 {
			continue
		}
		pmtPID = patPMT(sec, program)
		if pmtPID != 0 {
			break
		}
	}
	if pmtPID == 0 {
		return 0, 0, 0
	}
	for _, sec := range sections(data, pmtPID) {
		if len(sec) < 12 || sec[0] != 0x02 {
			continue
		}
		videoPID, kind = pmtVideo(sec)
		if videoPID != 0 {
			return pmtPID, videoPID, kind
		}
	}
	return pmtPID, 0, 0
}

func patPMT(sec []byte, program int) int {
	end := sectionEnd(sec)
	for off := 8; off+4 <= end; off += 4 {
		prog := int(sec[off])<<8 | int(sec[off+1])
		pid := int(sec[off+2]&0x1f)<<8 | int(sec[off+3])
		if prog == 0 {
			continue
		}
		if program == 0 || prog == program {
			return pid
		}
	}
	return 0
}

func pmtVideo(sec []byte) (pid, kind int) {
	if len(sec) < 12 {
		return 0, 0
	}
	info := int(sec[10]&0x0f)<<8 | int(sec[11])
	off := 12 + info
	end := sectionEnd(sec)
	for off+5 <= end {
		streamType := int(sec[off])
		epid := int(sec[off+1]&0x1f)<<8 | int(sec[off+2])
		esInfo := int(sec[off+3]&0x0f)<<8 | int(sec[off+4])
		off += 5 + esInfo
		if streamType == streamMPEG2 || streamType == streamH264 {
			return epid, streamType
		}
	}
	return 0, 0
}

func sectionEnd(sec []byte) int {
	if len(sec) < 3 {
		return 0
	}
	n := int(sec[1]&0x0f)<<8 | int(sec[2])
	end := 3 + n - 4 // drop CRC
	if end > len(sec) {
		end = len(sec)
	}
	if end < 0 {
		return 0
	}
	return end
}

func sections(data []byte, pid int) [][]byte {
	var out [][]byte
	var cur []byte
	eachPacket(data, pid, func(start bool, payload []byte) {
		if start {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			if len(payload) == 0 {
				cur = nil
				return
			}
			pointer := int(payload[0])
			if 1+pointer > len(payload) {
				cur = nil
				return
			}
			cur = append([]byte(nil), payload[1+pointer:]...)
			return
		}
		cur = append(cur, payload...)
	})
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func elementary(data []byte, pid int) []byte {
	var es []byte
	eachPacket(data, pid, func(start bool, payload []byte) {
		if start {
			payload = pesPayload(payload)
		}
		es = append(es, payload...)
	})
	return es
}

func pesPayload(b []byte) []byte {
	if len(b) < 9 || b[0] != 0 || b[1] != 0 || b[2] != 1 {
		return nil
	}
	header := int(b[8])
	off := 9 + header
	if off > len(b) {
		return nil
	}
	return b[off:]
}

func eachPacket(data []byte, pid int, fn func(start bool, payload []byte)) {
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		if pkt[0] != 0x47 {
			continue
		}
		got := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if got != pid {
			continue
		}
		afc := (pkt[3] >> 4) & 0x3
		start := 4
		if afc == 2 || afc == 0 {
			continue
		}
		if afc == 3 {
			if 5+int(pkt[4]) > 188 {
				continue
			}
			start += 1 + int(pkt[4])
		}
		fn(pkt[1]&0x40 != 0, pkt[start:])
	}
}

// mpeg2Scan reads sequence_extension.progressive_sequence. A progressive
// sequence is the answer. An interlaced sequence is field video unless the
// pictures are soft 3:2 pulldown (progressive_frame plus repeat_first_field),
// which is film and should play at 24p.
func mpeg2Scan(es []byte) (string, bool) {
	sawSeq := false
	progressiveSeq := false
	var pics, progPics, rff int
	sawInterlacedPic := false
	for i := 0; i+8 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 || es[i+3] != 0xB5 {
			continue
		}
		id := es[i+4] >> 4
		switch id {
		case 1: // sequence_extension
			// identifier 4 + profile_and_level 8; progressive_sequence is the next bit.
			sawSeq = true
			progressiveSeq = es[i+5]&0x08 != 0
		case 8: // picture_coding_extension
			pics++
			if es[i+8]&0x80 != 0 {
				progPics++
				if es[i+7]&0x02 != 0 { // repeat_first_field
					rff++
				}
			} else {
				sawInterlacedPic = true
			}
		}
	}
	if progressiveSeq {
		return "progressive", true
	}
	if sawInterlacedPic {
		return "tt", true
	}
	if filmCadence(pics, progPics, rff) {
		return "film", true
	}
	if sawSeq && pics >= 8 {
		return "tt", true
	}
	return "", false
}

// filmCadence is soft NTSC 3:2: every picture is progressive, and
// repeat_first_field is set on about half of them (two of every four).
func filmCadence(pics, prog, rff int) bool {
	if pics < 4 || prog != pics || rff == 0 || rff == pics {
		return false
	}
	ratio := float64(rff) / float64(pics)
	return ratio >= 0.25 && ratio <= 0.75
}

// h264Scan reads SPS frame_mbs_only_flag. Set means every picture is a frame.
func h264Scan(es []byte) (string, bool) {
	for i := 0; i+5 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 {
			continue
		}
		nal := es[i+3]
		if nal&0x1f != 7 {
			continue
		}
		rbsp := unescape(es[i+4:])
		if flag, ok := frameMBsOnly(rbsp); ok {
			if flag {
				return "progressive", true
			}
			return "tt", true
		}
	}
	return "", false
}

func unescape(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if i+2 < len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 3 {
			out = append(out, 0, 0)
			i += 2
			continue
		}
		out = append(out, b[i])
	}
	return out
}

type bitReader struct {
	b   []byte
	off int
}

func (r *bitReader) u(n int) (uint, bool) {
	if n < 0 || r.off+n > len(r.b)*8 {
		return 0, false
	}
	var v uint
	for i := 0; i < n; i++ {
		v <<= 1
		if r.b[r.off/8]&(0x80>>uint(r.off%8)) != 0 {
			v |= 1
		}
		r.off++
	}
	return v, true
}

func (r *bitReader) ue() (uint, bool) {
	zeros := 0
	for {
		bit, ok := r.u(1)
		if !ok {
			return 0, false
		}
		if bit == 1 {
			break
		}
		zeros++
		if zeros > 31 {
			return 0, false
		}
	}
	if zeros == 0 {
		return 0, true
	}
	rest, ok := r.u(zeros)
	if !ok {
		return 0, false
	}
	return (1 << uint(zeros)) - 1 + rest, true
}

func (r *bitReader) se() (int, bool) {
	v, ok := r.ue()
	if !ok {
		return 0, false
	}
	if v%2 == 0 {
		return -int(v / 2), true
	}
	return int(v/2) + 1, true
}

func frameMBsOnly(rbsp []byte) (bool, bool) {
	r := &bitReader{b: rbsp}
	profile, ok := r.u(8)
	if !ok {
		return false, false
	}
	if _, ok = r.u(8); !ok { // constraint flags
		return false, false
	}
	if _, ok = r.u(8); !ok { // level
		return false, false
	}
	if _, ok = r.ue(); !ok { // seq_parameter_set_id
		return false, false
	}
	if highProfile(profile) {
		chroma, ok := r.ue()
		if !ok {
			return false, false
		}
		if chroma == 3 {
			if _, ok = r.u(1); !ok {
				return false, false
			}
		}
		if _, ok = r.ue(); !ok { // bit_depth_luma_minus8
			return false, false
		}
		if _, ok = r.ue(); !ok { // bit_depth_chroma_minus8
			return false, false
		}
		if _, ok = r.u(1); !ok { // qpprime
			return false, false
		}
		present, ok := r.u(1)
		if !ok {
			return false, false
		}
		if present == 1 && !skipScaling(r, chroma) {
			return false, false
		}
	}
	if _, ok = r.ue(); !ok { // log2_max_frame_num_minus4
		return false, false
	}
	poc, ok := r.ue()
	if !ok {
		return false, false
	}
	switch poc {
	case 0:
		if _, ok = r.ue(); !ok {
			return false, false
		}
	case 1:
		if _, ok = r.u(1); !ok {
			return false, false
		}
		if _, ok = r.se(); !ok {
			return false, false
		}
		if _, ok = r.se(); !ok {
			return false, false
		}
		n, ok := r.ue()
		if !ok {
			return false, false
		}
		for i := uint(0); i < n; i++ {
			if _, ok = r.se(); !ok {
				return false, false
			}
		}
	}
	if _, ok = r.ue(); !ok { // max_num_ref_frames
		return false, false
	}
	if _, ok = r.u(1); !ok { // gaps
		return false, false
	}
	if _, ok = r.ue(); !ok { // width
		return false, false
	}
	if _, ok = r.ue(); !ok { // height
		return false, false
	}
	flag, ok := r.u(1)
	if !ok {
		return false, false
	}
	return flag == 1, true
}

func highProfile(profile uint) bool {
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

func skipScaling(r *bitReader, chroma uint) bool {
	n := 8
	if chroma == 3 {
		n = 12
	}
	for i := 0; i < n; i++ {
		present, ok := r.u(1)
		if !ok {
			return false
		}
		if present == 0 {
			continue
		}
		size := 16
		if i >= 6 {
			size = 64
		}
		last, next := 8, 8
		for j := 0; j < size; j++ {
			if next != 0 {
				delta, ok := r.se()
				if !ok {
					return false
				}
				next = (last + delta + 256) % 256
			}
			if next != 0 {
				last = next
			}
		}
	}
	return true
}

// learnScanLocked reads the mux until the scan type is known or scanWait
// elapses, stores it, and sets the feed source before a rendition starts.
// A miss falls through to the background probe, which only helps the next tune.
func (h *Hub) learnScanLocked(m *mux, f *feed) {
	if f.channel.FieldOrder != "" || m == nil || m.input != "" {
		return
	}
	buf := &scanBuf{wake: make(chan struct{}, 1)}
	sub := h.attachPipeLocked(m, buf)
	started := time.Now()
	deadline := started.Add(scanWait)
	var order string
	for time.Now().Before(deadline) {
		buf.mu.Lock()
		data := append([]byte(nil), buf.b...)
		buf.mu.Unlock()
		if got, ok := scanType(data, f.program); ok {
			order = got
			break
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			break
		}
		timer := time.NewTimer(wait)
		select {
		case <-buf.wake:
		case <-timer.C:
		}
		timer.Stop()
	}
	m.detach(sub)
	if order == "" {
		buf.mu.Lock()
		n := len(buf.b)
		buf.mu.Unlock()
		log.Printf("scan type for %s program %d not in %s (%d bytes)", f.channel.GuideNumber, f.program, time.Since(started).Round(time.Millisecond), n)
		h.probeFieldOrderLocked(m, f)
		return
	}
	log.Printf("scan type %s for %s in %s", order, f.channel.GuideNumber, time.Since(started).Round(time.Millisecond))
	f.channel.FieldOrder = order
	f.source.Progressive = order == "progressive"
	f.source.Film = order == "film"
	if h.Store != nil {
		id := f.channel.ID
		go func() { _ = h.Store.SetFieldOrder(context.Background(), id, order) }()
	}
}

// scanBuf collects mux bytes for the scan-type read. Write must not block:
// the mux drops a subscriber that falls behind.
type scanBuf struct {
	mu   sync.Mutex
	b    []byte
	wake chan struct{}
}

func (s *scanBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.b = append(s.b, p...)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (s *scanBuf) Close() error { return nil }
