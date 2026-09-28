package live

import (
	"cmp"
	"encoding/binary"
	"math"
	"os"
	"slices"
)

// fMP4 segments carry their timing in boxes. This reads just enough of them
// to give the Timeline a presentation time for a segment's first video frame.

type box struct {
	kind string
	body []byte
}

func boxes(b []byte) []box {
	var out []box
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b[:4]))
		kind := string(b[4:8])
		head := 8
		if size == 1 && len(b) >= 16 {
			size = int(binary.BigEndian.Uint64(b[8:16]))
			head = 16
		}
		if size == 0 {
			size = len(b)
		}
		if size < head || size > len(b) {
			break
		}
		out = append(out, box{kind: kind, body: b[head:size]})
		b = b[size:]
	}
	return out
}

func child(b []byte, kind string) []byte {
	for _, x := range boxes(b) {
		if x.kind == kind {
			return x.body
		}
	}
	return nil
}

const maxTrackLead = 10.0

func putBox(kind string, body []byte) []byte {
	b := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(b[:4], uint32(8+len(body)))
	copy(b[4:8], kind)
	return append(b, body...)
}

// flattenEdits drops the edit lists from an init segment and returns how far
// to move each track's fragments so the tracks still line up, and the
// broadcast time in seconds that fragment time zero then stands for. With -copyts,
// ffmpeg starts every track's decode time at zero and keeps each track's
// broadcast start only in an empty edit. hls.js and Chrome ignore edits, so
// sound played as late as the audio led the first picture (over a second on
// a real channel), and Safari honors them, so its buffer sat hours away from
// where hls.js looked and the picture never started.
func flattenEdits(init []byte) ([]byte, map[uint32]int64, float64) {
	// at is when the track's first sample is shown, in seconds on the
	// broadcast clock; scale is the track's own timescale.
	type start struct {
		id, scale uint32
		at        float64
	}
	var out []byte
	var starts []start
	for _, b := range boxes(init) {
		if b.kind != "moov" {
			out = append(out, putBox(b.kind, b.body)...)
			continue
		}
		movie := mvhdScale(child(b.body, "mvhd"))
		var moov []byte
		for _, c := range boxes(b.body) {
			if c.kind != "trak" {
				moov = append(moov, putBox(c.kind, c.body)...)
				continue
			}
			var trak []byte
			for _, x := range boxes(c.body) {
				if x.kind != "edts" {
					trak = append(trak, putBox(x.kind, x.body)...)
				}
			}
			moov = append(moov, putBox("trak", trak)...)
			id, scale, ok := trackScale(c.body)
			elst := child(child(c.body, "edts"), "elst")
			if !ok || elst == nil {
				return init, nil, 0
			}
			starts = append(starts, start{id, scale, editStart(elst, movie, scale)})
		}
		out = append(out, putBox("moov", moov)...)
	}
	if len(starts) == 0 {
		return init, nil, 0
	}
	first := starts[0].at
	for _, s := range starts {
		first = min(first, s.at)
	}
	shifts := map[uint32]int64{}
	for _, s := range starts {
		// Tracks of one broadcast start seconds apart. More means a clock
		// wrap or a track that lost its start; moving it would put it hours off.
		if s.at-first > maxTrackLead {
			return init, nil, 0
		}
		shifts[s.id] = int64(math.Round((s.at - first) * float64(s.scale)))
	}
	return out, shifts, first
}

// editStart is when a track's first sample is shown, in seconds: leading
// empty edits delay it, and the first real edit skips its media time.
func editStart(elst []byte, movie, scale uint32) float64 {
	if len(elst) < 8 || movie == 0 || scale == 0 {
		return 0
	}
	wide := elst[0] == 1
	size := 12
	if wide {
		size = 20
	}
	var empty float64
	b := elst[8:]
	for n := binary.BigEndian.Uint32(elst[4:8]); n > 0 && len(b) >= size; n-- {
		var dur uint64
		var media int64
		if wide {
			dur, media = binary.BigEndian.Uint64(b[:8]), int64(binary.BigEndian.Uint64(b[8:16]))
		} else {
			dur, media = uint64(binary.BigEndian.Uint32(b[:4])), int64(int32(binary.BigEndian.Uint32(b[4:8])))
		}
		if media >= 0 {
			return empty - float64(media)/float64(scale)
		}
		empty += float64(dur) / float64(movie)
		b = b[size:]
	}
	return empty
}

func mvhdScale(mvhd []byte) uint32 {
	if len(mvhd) >= 24 && mvhd[0] == 1 {
		return binary.BigEndian.Uint32(mvhd[20:24])
	}
	if len(mvhd) >= 16 {
		return binary.BigEndian.Uint32(mvhd[12:16])
	}
	return 0
}

// shiftTracks moves each track's decode time in a fragment by its shift from
// flattenEdits. It edits frag in place.
func shiftTracks(frag []byte, shifts map[uint32]int64) {
	if len(shifts) == 0 {
		return
	}
	for _, t := range boxes(child(frag, "moof")) {
		if t.kind != "traf" {
			continue
		}
		tfhd := child(t.body, "tfhd")
		if len(tfhd) < 8 {
			continue
		}
		d := shifts[binary.BigEndian.Uint32(tfhd[4:8])]
		if d == 0 {
			continue
		}
		tfdt := child(t.body, "tfdt")
		switch {
		case len(tfdt) >= 12 && tfdt[0] == 1:
			binary.BigEndian.PutUint64(tfdt[4:12], uint64(int64(binary.BigEndian.Uint64(tfdt[4:12]))+d))
		case len(tfdt) >= 8:
			binary.BigEndian.PutUint32(tfdt[4:8], uint32(int64(binary.BigEndian.Uint32(tfdt[4:8]))+d))
		}
	}
}

// trackScale reads a trak's id and media timescale.
func trackScale(trak []byte) (uint32, uint32, bool) {
	tkhd := child(trak, "tkhd")
	mdhd := child(child(trak, "mdia"), "mdhd")
	if len(tkhd) < 24 || len(mdhd) < 24 {
		return 0, 0, false
	}
	var id, scale uint32
	if tkhd[0] == 1 {
		id = binary.BigEndian.Uint32(tkhd[20:24])
	} else {
		id = binary.BigEndian.Uint32(tkhd[12:16])
	}
	if mdhd[0] == 1 {
		scale = binary.BigEndian.Uint32(mdhd[20:24])
	} else {
		scale = binary.BigEndian.Uint32(mdhd[12:16])
	}
	return id, scale, scale > 0
}

// trackScales maps each track id in an init segment to its timescale.
func trackScales(init []byte) map[uint32]uint32 {
	out := map[uint32]uint32{}
	for _, t := range boxes(child(init, "moov")) {
		if t.kind != "trak" {
			continue
		}
		if id, scale, ok := trackScale(t.body); ok {
			out[id] = scale
		}
	}
	return out
}

// videoTrack returns the video track id and its timescale from an init segment.
func videoTrack(init []byte) (uint32, uint32, bool) {
	for _, t := range boxes(child(init, "moov")) {
		if t.kind != "trak" {
			continue
		}
		hdlr := child(child(t.body, "mdia"), "hdlr")
		if len(hdlr) < 12 || string(hdlr[8:12]) != "vide" {
			continue
		}
		if id, scale, ok := trackScale(t.body); ok {
			return id, scale, true
		}
	}
	return 0, 0, false
}

// fragmentPTS returns the first video sample's presentation time in 90 kHz ticks.
func fragmentPTS(seg []byte, track, scale uint32) (int64, bool) {
	moof := child(seg, "moof")
	for _, t := range boxes(moof) {
		if t.kind != "traf" {
			continue
		}
		tfhd := child(t.body, "tfhd")
		if len(tfhd) < 8 || binary.BigEndian.Uint32(tfhd[4:8]) != track {
			continue
		}
		tfdt := child(t.body, "tfdt")
		if len(tfdt) < 8 {
			return 0, false
		}
		var base int64
		if tfdt[0] == 1 && len(tfdt) >= 12 {
			base = int64(binary.BigEndian.Uint64(tfdt[4:12]))
		} else {
			base = int64(binary.BigEndian.Uint32(tfdt[4:8]))
		}
		base += firstCompositionOffset(child(t.body, "trun"))
		return base * 90000 / int64(scale), true
	}
	return 0, false
}

// fragmentDuration is the video samples in one fragment, in 90 kHz ticks.
// The playlist can name that length before the next fragment arrives.
func fragmentDuration(seg []byte, track, scale uint32) (int64, bool) {
	moof := child(seg, "moof")
	for _, t := range boxes(moof) {
		if t.kind != "traf" {
			continue
		}
		tfhd := child(t.body, "tfhd")
		if len(tfhd) < 8 || binary.BigEndian.Uint32(tfhd[4:8]) != track {
			continue
		}
		def, hasDef := defaultSampleDuration(tfhd)
		sum, ok := trunDuration(child(t.body, "trun"), def, hasDef)
		if !ok || sum <= 0 || scale == 0 {
			return 0, false
		}
		return sum * 90000 / int64(scale), true
	}
	return 0, false
}

func defaultSampleDuration(tfhd []byte) (uint32, bool) {
	if len(tfhd) < 8 {
		return 0, false
	}
	flags := uint32(tfhd[1])<<16 | uint32(tfhd[2])<<8 | uint32(tfhd[3])
	if flags&0x8 == 0 {
		return 0, false
	}
	off := 8
	if flags&0x1 != 0 {
		off += 8
	}
	if flags&0x2 != 0 {
		off += 4
	}
	if off+4 > len(tfhd) {
		return 0, false
	}
	return binary.BigEndian.Uint32(tfhd[off : off+4]), true
}

func trunDuration(trun []byte, def uint32, hasDef bool) (int64, bool) {
	if len(trun) < 8 {
		return 0, false
	}
	flags := uint32(trun[1])<<16 | uint32(trun[2])<<8 | uint32(trun[3])
	count := int(binary.BigEndian.Uint32(trun[4:8]))
	if count <= 0 {
		return 0, false
	}
	if flags&0x100 == 0 {
		if !hasDef {
			return 0, false
		}
		return int64(def) * int64(count), true
	}
	off := 8
	if flags&0x1 != 0 {
		off += 4
	}
	if flags&0x4 != 0 {
		off += 4
	}
	per := 4
	for _, f := range []uint32{0x200, 0x400, 0x800} {
		if flags&f != 0 {
			per += 4
		}
	}
	var sum int64
	for i := 0; i < count; i++ {
		if off+4 > len(trun) {
			return 0, false
		}
		sum += int64(binary.BigEndian.Uint32(trun[off : off+4]))
		off += per
	}
	return sum, sum > 0
}

func firstCompositionOffset(trun []byte) int64 {
	if len(trun) < 8 {
		return 0
	}
	version := trun[0]
	flags := uint32(trun[1])<<16 | uint32(trun[2])<<8 | uint32(trun[3])
	if flags&0x800 == 0 || binary.BigEndian.Uint32(trun[4:8]) == 0 {
		return 0
	}
	off := 8
	if flags&0x1 != 0 {
		off += 4
	}
	if flags&0x4 != 0 {
		off += 4
	}
	for _, f := range []uint32{0x100, 0x200, 0x400} {
		if flags&f != 0 {
			off += 4
		}
	}
	if len(trun) < off+4 {
		return 0
	}
	v := binary.BigEndian.Uint32(trun[off : off+4])
	if version == 1 {
		return int64(int32(v))
	}
	return int64(v)
}

// segmentStart reads a segment's first video presentation time, from MP4 boxes
// for fMP4 renditions or from PES headers for MPEG-TS.
func segmentStart(dir, name string) (int64, bool) {
	path := dir + string(os.PathSeparator) + name
	if len(name) > 4 && name[len(name)-4:] == ".m4s" {
		init, err := os.ReadFile(dir + string(os.PathSeparator) + "init.mp4")
		if err != nil {
			return 0, false
		}
		track, scale, ok := videoTrack(init)
		if !ok {
			return 0, false
		}
		seg, err := os.ReadFile(path)
		if err != nil {
			return 0, false
		}
		return fragmentPTS(seg, track, scale)
	}
	return SegmentPTS(path)
}

type trunSample struct{ dur, size, flags, cts uint32 }

// trackRun is one traf with a single trun, as ffmpeg writes them.
type trackRun struct {
	id       uint32
	tfhd     []byte
	tfdt     []byte
	version  byte
	flags    uint32
	dataOff  int64
	first    uint32
	samples  []trunSample
	defDur   uint32
	defSize  uint32
	hasSizes bool
	hasDurs  bool
}

func parseRun(traf []byte) (trackRun, bool) {
	var r trackRun
	runs := 0
	for _, b := range boxes(traf) {
		switch b.kind {
		case "tfhd":
			r.tfhd = b.body
		case "tfdt":
			r.tfdt = b.body
		case "trun":
			runs++
			if !r.readTrun(b.body) {
				return r, false
			}
		default:
			// Sample groups and encryption boxes describe samples by index
			// or point at bytes; a trim would leave them wrong.
			return r, false
		}
	}
	if runs != 1 || len(r.tfhd) < 8 || len(r.tfdt) < 8 || (r.tfdt[0] == 1 && len(r.tfdt) < 12) {
		return r, false
	}
	r.id = binary.BigEndian.Uint32(r.tfhd[4:8])
	tf := uint32(r.tfhd[1])<<16 | uint32(r.tfhd[2])<<8 | uint32(r.tfhd[3])
	// Data offsets must count from the moof (default-base-is-moof), not from
	// an explicit base.
	if tf&0x1 != 0 || tf&0x20000 == 0 {
		return r, false
	}
	off := 8
	if tf&0x2 != 0 {
		off += 4
	}
	if tf&0x8 != 0 && off+4 <= len(r.tfhd) {
		r.defDur, r.hasDurs = binary.BigEndian.Uint32(r.tfhd[off:off+4]), true
		off += 4
	}
	if tf&0x10 != 0 && off+4 <= len(r.tfhd) {
		r.defSize, r.hasSizes = binary.BigEndian.Uint32(r.tfhd[off:off+4]), true
	}
	if r.flags&0x100 != 0 {
		r.hasDurs = true
	}
	if r.flags&0x200 != 0 {
		r.hasSizes = true
	}
	return r, r.flags&0x1 != 0
}

func (r *trackRun) readTrun(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	r.version = b[0]
	r.flags = uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	n := int(binary.BigEndian.Uint32(b[4:8]))
	off := 8
	if r.flags&0x1 != 0 {
		if off+4 > len(b) {
			return false
		}
		r.dataOff = int64(int32(binary.BigEndian.Uint32(b[off : off+4])))
		off += 4
	}
	if r.flags&0x4 != 0 {
		if off+4 > len(b) {
			return false
		}
		r.first = binary.BigEndian.Uint32(b[off : off+4])
		off += 4
	}
	for i := 0; i < n; i++ {
		var s trunSample
		for _, f := range []struct {
			bit uint32
			v   *uint32
		}{{0x100, &s.dur}, {0x200, &s.size}, {0x400, &s.flags}, {0x800, &s.cts}} {
			if r.flags&f.bit == 0 {
				continue
			}
			if off+4 > len(b) {
				return false
			}
			*f.v = binary.BigEndian.Uint32(b[off : off+4])
			off += 4
		}
		r.samples = append(r.samples, s)
	}
	return true
}

func (r *trackRun) decodeTime() int64 {
	if r.tfdt[0] == 1 && len(r.tfdt) >= 12 {
		return int64(binary.BigEndian.Uint64(r.tfdt[4:12]))
	}
	return int64(binary.BigEndian.Uint32(r.tfdt[4:8]))
}

func (r *trackRun) sampleDur(s trunSample) int64 {
	if r.flags&0x100 != 0 {
		return int64(s.dur)
	}
	return int64(r.defDur)
}

func (r *trackRun) cts(s trunSample) int64 {
	if r.version == 1 {
		return int64(int32(s.cts))
	}
	return int64(s.cts)
}

func (r *trackRun) sampleSize(s trunSample) int64 {
	if r.flags&0x200 != 0 {
		return int64(s.size)
	}
	return int64(r.defSize)
}

func (r *trackRun) build(dataOff int64) []byte {
	tfdt := append([]byte{}, r.tfdt...)
	body := putBox("tfhd", r.tfhd)
	body = append(body, putBox("tfdt", tfdt)...)
	trun := []byte{r.version, byte(r.flags >> 16), byte(r.flags >> 8), byte(r.flags)}
	trun = binary.BigEndian.AppendUint32(trun, uint32(len(r.samples)))
	trun = binary.BigEndian.AppendUint32(trun, uint32(int32(dataOff)))
	if r.flags&0x4 != 0 {
		trun = binary.BigEndian.AppendUint32(trun, r.first)
	}
	for _, s := range r.samples {
		for _, f := range []struct {
			bit uint32
			v   uint32
		}{{0x100, s.dur}, {0x200, s.size}, {0x400, s.flags}, {0x800, s.cts}} {
			if r.flags&f.bit != 0 {
				trun = binary.BigEndian.AppendUint32(trun, f.v)
			}
		}
	}
	return putBox("traf", append(body, putBox("trun", trun)...))
}

// trimLeadIn drops the samples of every track but the video that start
// before the fragment's first picture. It is for an encode's first fragment:
// sound from before the first picture has nothing to play against, and
// hls.js anchors its timeline on whichever track starts first while the
// playlist dates the first picture, so a lead there paused and then seeked
// every new screen. A layout other than ffmpeg's comes back unchanged.
func trimLeadIn(frag []byte, video uint32, scales map[uint32]uint32) []byte {
	top := boxes(frag)
	if len(top) != 2 || top[0].kind != "moof" || top[1].kind != "mdat" ||
		binary.BigEndian.Uint32(frag[:4]) == 1 || len(frag) < 8+len(top[0].body)+8 ||
		binary.BigEndian.Uint32(frag[8+len(top[0].body):]) == 1 {
		return frag
	}
	moofSize := int64(8 + len(top[0].body))
	var mfhd []byte
	var runs []trackRun
	for _, b := range boxes(top[0].body) {
		switch b.kind {
		case "mfhd":
			mfhd = b.body
		case "traf":
			r, ok := parseRun(b.body)
			if !ok || !r.hasDurs || !r.hasSizes {
				return frag
			}
			runs = append(runs, r)
		default:
			return frag
		}
	}
	vs := scales[video]
	var picture int64 = -1
	for i := range runs {
		if runs[i].id == video && len(runs[i].samples) > 0 {
			picture = runs[i].decodeTime() + runs[i].cts(runs[i].samples[0])
		}
	}
	if picture < 0 || vs == 0 {
		return frag
	}
	type cut struct{ at, n int64 }
	var cuts []cut
	for i := range runs {
		r := &runs[i]
		scale := scales[r.id]
		if r.id == video || scale == 0 {
			continue
		}
		before := picture * int64(scale) / int64(vs)
		at, drop, k := r.decodeTime(), int64(0), 0
		for k < len(r.samples)-1 && at+r.cts(r.samples[k]) < before {
			at += r.sampleDur(r.samples[k])
			drop += r.sampleSize(r.samples[k])
			k++
		}
		if k == 0 {
			continue
		}
		cuts = append(cuts, cut{r.dataOff, drop})
		r.samples = r.samples[k:]
		if r.tfdt[0] == 1 && len(r.tfdt) >= 12 {
			r.tfdt = binary.BigEndian.AppendUint64(append([]byte{}, r.tfdt[:4]...), uint64(at))
		} else {
			r.tfdt = binary.BigEndian.AppendUint32(append([]byte{}, r.tfdt[:4]...), uint32(at))
		}
		r.dataOff += drop
	}
	if len(cuts) == 0 {
		return frag
	}
	moofBody := func(offsets []int64) []byte {
		body := putBox("mfhd", mfhd)
		for i := range runs {
			body = append(body, runs[i].build(offsets[i])...)
		}
		return body
	}
	newMoof := int64(8 + len(moofBody(make([]int64, len(runs)))))
	// A run's data moves up by what the moof lost and by every cut before it.
	offsets := make([]int64, len(runs))
	for i := range runs {
		o := runs[i].dataOff - (moofSize - newMoof)
		for _, c := range cuts {
			if c.at < runs[i].dataOff {
				o -= c.n
			}
		}
		offsets[i] = o
	}
	slices.SortFunc(cuts, func(a, b cut) int { return cmp.Compare(a.at, b.at) })
	mdat := append([]byte{}, top[1].body...)
	for j := len(cuts) - 1; j >= 0; j-- {
		start := cuts[j].at - moofSize - 8
		if start < 0 || start+cuts[j].n > int64(len(mdat)) {
			return frag
		}
		mdat = append(mdat[:start], mdat[start+cuts[j].n:]...)
	}
	return append(putBox("moof", moofBody(offsets)), putBox("mdat", mdat)...)
}
