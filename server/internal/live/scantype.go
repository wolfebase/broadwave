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
	flush := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, cur)
		cur = nil
	}
	eachPacket(data, pid, func(start bool, payload []byte) {
		if !start {
			cur = append(cur, payload...)
			return
		}
		// Pointer-field bytes finish the section already open. A packer
		// puts that tail in the same packet that starts the next section
		// (ISO/IEC 13818-1). Dropping it loses the last audio stream.
		if len(payload) == 0 {
			flush()
			return
		}
		pointer := int(payload[0])
		if 1+pointer > len(payload) {
			flush()
			return
		}
		if pointer > 0 && len(cur) > 0 {
			cur = append(cur, payload[1:1+pointer]...)
		}
		flush()
		cur = append([]byte(nil), payload[1+pointer:]...)
	})
	flush()
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

// storedFieldOrder is what the station is, not what this program is.
// Soft 3:2 is film for the tune in hand. Saving that would play the next
// game on the same 1080i channel at 24p.
func storedFieldOrder(order string) string {
	if order == "film" {
		return "tt"
	}
	return order
}

// interlacedOrder is a scan that showed real fields. Empty is unscanned.
func interlacedOrder(order string) bool {
	switch order {
	case "tt", "bb", "tb", "bt":
		return true
	default:
		return false
	}
}

// learnScanLocked reads the mux until the scan type is known or scanWait
// elapses, stores it, and sets the feed source before a rendition starts.
// A stored "progressive" is still read. ffprobe reports soft 3:2 as
// progressive, and skipping that scan plays the movie at 60. An interlaced
// channel is read again each tune, because a movie and a game share it.
// A header that misses the window keeps being read, and the stored probe
// rebuilds a rendition that already started on the wrong graph.
func (h *Hub) learnScanLocked(m *mux, f *feed) {
	if m == nil || m.input != "" {
		return
	}
	needAudio := len(f.tracks) == 0
	buf := &scanBuf{wake: make(chan struct{}, 1)}
	sub := h.attachPipeLocked(m, buf)
	started := time.Now()
	deadline := started.Add(scanWait)
	var order string
	for time.Now().Before(deadline) {
		buf.mu.Lock()
		data := append([]byte(nil), buf.b...)
		buf.mu.Unlock()
		if order == "" {
			if got, ok := scanType(data, f.program); ok {
				order = got
			}
		}
		if needAudio {
			if tracks := AudioTracks(data, f.program); len(tracks) > 0 {
				f.tracks = tracks
				needAudio = false
			}
		}
		if order != "" && !needAudio {
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
	if len(f.tracks) > 0 {
		log.Printf("audio tracks for %s: %s", f.channel.GuideNumber, trackLog(f.tracks))
	}
	if order != "" {
		m.detach(sub)
		log.Printf("scan type %s for %s in %s", order, f.channel.GuideNumber, time.Since(started).Round(time.Millisecond))
		f.headerOrder = order
		h.applyScanLocked(f, order)
		return
	}
	buf.mu.Lock()
	n := len(buf.b)
	buf.mu.Unlock()
	log.Printf("scan type for %s program %d not in %s (%d bytes)", f.channel.GuideNumber, f.program, time.Since(started).Round(time.Millisecond), n)
	// No bytes means the tuner has not delivered a GOP yet. Bytes without a
	// header are a late sequence start: keep reading them. ffprobe runs only
	// when nothing is stored yet. It cannot see soft 3:2, and leaving it
	// running holds the tuner.
	if n > 0 {
		go h.finishScan(m, f, buf, sub)
	} else {
		m.detach(sub)
	}
	if f.channel.FieldOrder == "" {
		h.probeFieldOrderLocked(m, f)
	}
}

// finishScan keeps reading after scanWait. The rendition may already be
// field-deinterlacing; a header that shows up rebuilds it.
func (h *Hub) finishScan(m *mux, f *feed, buf *scanBuf, sub *pipeSub) {
	deadline := time.Now().Add(8 * time.Second)
	program := f.program
	id := f.channel.ID
	guide := f.channel.GuideNumber
	var order string
	for time.Now().Before(deadline) {
		buf.mu.Lock()
		data := append([]byte(nil), buf.b...)
		buf.mu.Unlock()
		if got, ok := scanType(data, program); ok {
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
	h.mu.Lock()
	defer h.mu.Unlock()
	m.detach(sub)
	if order == "" || f.headerOrder != "" || h.channels[id] != f {
		return
	}
	log.Printf("scan type %s for %s after the window", order, guide)
	f.headerOrder = order
	h.applyScanLocked(f, order)
}

// applyScanLocked stores the station scan and sets this tune's picture.
// Film stays on the tune and is stored as interlaced, so the next program
// is scanned again. A running rendition whose graph changed is rebuilt.
func (h *Hub) applyScanLocked(f *feed, order string) {
	if order == "" {
		return
	}
	stored := storedFieldOrder(order)
	progressive := stored == "progressive"
	film := order == "film"
	// Film plays at 24p for this tune. The stored order stays interlaced so
	// the next program is scanned again, and that tune laces from "tt".
	lace := interlacedOrder(stored) && !film
	changed := f.source.Progressive != progressive || f.source.Film != film || f.source.Lace != lace
	f.source.Progressive = progressive
	f.source.Film = film
	f.source.Lace = lace
	if stored != "" && stored != f.channel.FieldOrder {
		f.channel.FieldOrder = stored
		if h.Store != nil {
			id := f.channel.ID
			go func() { _ = h.Store.SetFieldOrder(context.Background(), id, stored) }()
		}
	}
	if changed {
		h.rebuildRenditionsLocked(f)
	}
}

// applyProbeLocked is the ffprobe result. It rebuilds a rendition that
// started before the header arrived. A packet scan already on the feed wins:
// ffprobe cannot see soft 3:2.
func (h *Hub) applyProbeLocked(f *feed, order string) {
	if f.headerOrder != "" {
		return
	}
	h.applyScanLocked(f, order)
}

// scanBuf collects mux bytes for the scan-type read. Write must not block:
// the mux drops a subscriber that falls behind.
type scanBuf struct {
	mu   sync.Mutex
	b    []byte
	wake chan struct{}
}

// pictureFacts reads the broadcast width, height, and frame rate from a
// sequence header or an H.264 SPS. ok is false until that header is present.
func pictureFacts(data []byte, program int) (notedPicture, bool) {
	if i := bytes.IndexByte(data, 0x47); i > 0 && i < 188 {
		data = data[i:]
	}
	_, videoPID, kind := programVideo(data, program)
	if videoPID == 0 {
		return notedPicture{}, false
	}
	es := elementary(data, videoPID)
	switch kind {
	case streamMPEG2:
		return mpeg2Picture(es)
	case streamH264:
		return h264Picture(es)
	default:
		return notedPicture{}, false
	}
}

func mpeg2Picture(es []byte) (notedPicture, bool) {
	for i := 0; i+7 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 || es[i+3] != 0xB3 {
			continue
		}
		w := int(es[i+4])<<4 | int(es[i+5]>>4)
		h := int(es[i+5]&0x0f)<<8 | int(es[i+6])
		// aspect_ratio is the high nibble; frame_rate_code is the low nibble.
		fps := mpeg2Rate(es[i+7] & 0x0f)
		if w >= 16 && h >= 16 && w <= 7680 && h <= 4320 {
			return notedPicture{Width: w, Height: h, FPS: fps}, true
		}
	}
	return notedPicture{}, false
}

func mpeg2Rate(code byte) string {
	switch code {
	case 1:
		return "23.976"
	case 2:
		return "24"
	case 3:
		return "25"
	case 4:
		return "29.97"
	case 5:
		return "30"
	case 6:
		return "50"
	case 7:
		return "59.94"
	case 8:
		return "60"
	default:
		return ""
	}
}

func h264Picture(es []byte) (notedPicture, bool) {
	for i := 0; i+5 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 {
			continue
		}
		if es[i+3]&0x1f != 7 {
			continue
		}
		w, h, ok := spsSize(unescape(es[i+4:]))
		if ok && w >= 16 && h >= 16 && w <= 7680 && h <= 4320 {
			return notedPicture{Width: w, Height: h}, true
		}
	}
	return notedPicture{}, false
}

// spsSize walks an H.264 SPS to the cropped display size. A crop that does not
// parse returns nothing: 1080-line video is stored as 1088 and then cropped.
func spsSize(rbsp []byte) (int, int, bool) {
	r := &bitReader{b: rbsp}
	profile, ok := r.u(8)
	if !ok {
		return 0, 0, false
	}
	if _, ok = r.u(8); !ok {
		return 0, 0, false
	}
	if _, ok = r.u(8); !ok {
		return 0, 0, false
	}
	if _, ok = r.ue(); !ok {
		return 0, 0, false
	}
	chroma := uint(1)
	if highProfile(profile) {
		chroma, ok = r.ue()
		if !ok {
			return 0, 0, false
		}
		if chroma == 3 {
			if _, ok = r.u(1); !ok {
				return 0, 0, false
			}
		}
		if _, ok = r.ue(); !ok {
			return 0, 0, false
		}
		if _, ok = r.ue(); !ok {
			return 0, 0, false
		}
		if _, ok = r.u(1); !ok {
			return 0, 0, false
		}
		present, ok := r.u(1)
		if !ok {
			return 0, 0, false
		}
		if present == 1 && !skipScaling(r, chroma) {
			return 0, 0, false
		}
	}
	if _, ok = r.ue(); !ok {
		return 0, 0, false
	}
	poc, ok := r.ue()
	if !ok {
		return 0, 0, false
	}
	switch poc {
	case 0:
		if _, ok = r.ue(); !ok {
			return 0, 0, false
		}
	case 1:
		if _, ok = r.u(1); !ok {
			return 0, 0, false
		}
		if _, ok = r.se(); !ok {
			return 0, 0, false
		}
		if _, ok = r.se(); !ok {
			return 0, 0, false
		}
		n, ok := r.ue()
		if !ok {
			return 0, 0, false
		}
		for i := uint(0); i < n; i++ {
			if _, ok = r.se(); !ok {
				return 0, 0, false
			}
		}
	}
	if _, ok = r.ue(); !ok {
		return 0, 0, false
	}
	if _, ok = r.u(1); !ok {
		return 0, 0, false
	}
	mbW, ok := r.ue()
	if !ok {
		return 0, 0, false
	}
	mbH, ok := r.ue()
	if !ok {
		return 0, 0, false
	}
	frame, ok := r.u(1)
	if !ok {
		return 0, 0, false
	}
	w := int(mbW+1) * 16
	h := int(mbH+1) * 16
	if frame == 0 {
		h *= 2
		if _, ok = r.u(1); !ok { // mb_adaptive_frame_field_flag
			return 0, 0, false
		}
	}
	if _, ok = r.u(1); !ok { // direct_8x8_inference_flag
		return 0, 0, false
	}
	crop, ok := r.u(1)
	if !ok {
		return 0, 0, false
	}
	if crop == 1 {
		left, ok1 := r.ue()
		right, ok2 := r.ue()
		top, ok3 := r.ue()
		bottom, ok4 := r.ue()
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return 0, 0, false
		}
		ux, uy := cropUnits(chroma, frame)
		w -= int(left+right) * ux
		h -= int(top+bottom) * uy
	}
	if w < 16 || h < 16 {
		return 0, 0, false
	}
	return w, h, true
}

// cropUnits is the H.264 frame-crop sample size for a chroma format.
func cropUnits(chroma, frame uint) (int, int) {
	if chroma == 0 {
		return 1, 2 - int(frame)
	}
	subW, subH := 1, 1
	switch chroma {
	case 1:
		subW, subH = 2, 2
	case 2:
		subW, subH = 2, 1
	}
	return subW, subH * (2 - int(frame))
}

func (s *scanBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.b = append(s.b, p...)
	// A late header only needs the recent GOP. Keep the capture aligned so
	// the packet walker still sees 188-byte cells.
	const capBytes = 4 << 20
	if len(s.b) > capBytes {
		drop := len(s.b) - capBytes
		drop -= drop % 188
		if drop > 0 {
			s.b = append([]byte(nil), s.b[drop:]...)
		}
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (s *scanBuf) Close() error { return nil }
