package live

import (
	"encoding/binary"
	"os"
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

// videoTrack returns the video track id and its timescale from an init segment.
func videoTrack(init []byte) (uint32, uint32, bool) {
	moov := child(init, "moov")
	for _, t := range boxes(moov) {
		if t.kind != "trak" {
			continue
		}
		mdia := child(t.body, "mdia")
		hdlr := child(mdia, "hdlr")
		if len(hdlr) < 12 || string(hdlr[8:12]) != "vide" {
			continue
		}
		tkhd := child(t.body, "tkhd")
		mdhd := child(mdia, "mdhd")
		if len(tkhd) < 24 || len(mdhd) < 24 {
			continue
		}
		var id, scale uint32
		if tkhd[0] == 1 {
			id = binary.BigEndian.Uint32(tkhd[20:24])
		} else {
			id = binary.BigEndian.Uint32(tkhd[12:16])
		}
		if mdhd[0] == 1 {
			if len(mdhd) < 24 {
				continue
			}
			scale = binary.BigEndian.Uint32(mdhd[20:24])
		} else {
			scale = binary.BigEndian.Uint32(mdhd[12:16])
		}
		return id, scale, scale > 0
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
