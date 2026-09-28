package live

import (
	"encoding/binary"
	"errors"
	"slices"
)

// One encode can carry every audio track of a program. Players get views of
// it: the picture alone, or one sound track alone. These cut an init segment
// and its fragments down to the tracks a view keeps, without touching the
// timing, so every view lines up sample for sample with the others.

var errSplit = errors.New("fmp4: layout the track split does not know")

// keepTracks returns the init segment with only the given tracks in its moov.
func keepTracks(init []byte, ids []uint32) ([]byte, error) {
	var out []byte
	found := 0
	for _, top := range boxes(init) {
		if top.kind != "moov" {
			out = append(out, putBox(top.kind, top.body)...)
			continue
		}
		var moov []byte
		for _, b := range boxes(top.body) {
			switch b.kind {
			case "trak":
				id, _, ok := trackScale(b.body)
				if !ok {
					return nil, errSplit
				}
				if slices.Contains(ids, id) {
					moov = append(moov, putBox("trak", b.body)...)
					found++
				}
			case "mvex":
				var mvex []byte
				for _, x := range boxes(b.body) {
					if x.kind == "trex" {
						if len(x.body) < 8 {
							return nil, errSplit
						}
						if !slices.Contains(ids, binary.BigEndian.Uint32(x.body[4:8])) {
							continue
						}
					}
					mvex = append(mvex, putBox(x.kind, x.body)...)
				}
				moov = append(moov, putBox("mvex", mvex)...)
			default:
				moov = append(moov, putBox(b.kind, b.body)...)
			}
		}
		out = append(out, putBox("moov", moov)...)
	}
	if found != len(ids) {
		return nil, errSplit
	}
	return out, nil
}

// keepTracksFrag returns a segment or part with only the given tracks in each
// moof and only their samples in each mdat. A fragment where a kept track has
// no samples keeps its moof with no traf for it.
func keepTracksFrag(frag []byte, ids []uint32) ([]byte, error) {
	var out []byte
	for len(frag) > 0 {
		moofLen, ok := boxLen(frag)
		if !ok {
			return nil, errSplit
		}
		if string(frag[4:8]) != "moof" {
			out = append(out, frag[:moofLen]...)
			frag = frag[moofLen:]
			continue
		}
		rest := frag[moofLen:]
		mdatLen, ok := boxLen(rest)
		if !ok || string(rest[4:8]) != "mdat" || binary.BigEndian.Uint32(rest[:4]) == 1 {
			return nil, errSplit
		}
		cut, err := keepMoof(frag[:moofLen], rest[8:mdatLen], ids)
		if err != nil {
			return nil, err
		}
		out = append(out, cut...)
		frag = rest[mdatLen:]
	}
	return out, nil
}

func keepMoof(moof, mdat []byte, ids []uint32) ([]byte, error) {
	if binary.BigEndian.Uint32(moof[:4]) == 1 {
		return nil, errSplit
	}
	moofSize := int64(len(moof))
	var mfhd []byte
	var runs []trackRun
	var data [][]byte
	for _, b := range boxes(moof[8:]) {
		switch b.kind {
		case "mfhd":
			mfhd = b.body
		case "traf":
			r, ok := parseRun(b.body)
			if !ok || !r.hasSizes {
				return nil, errSplit
			}
			if !slices.Contains(ids, r.id) {
				continue
			}
			var n int64
			for _, s := range r.samples {
				n += r.sampleSize(s)
			}
			start := r.dataOff - moofSize - 8
			if start < 0 || start+n > int64(len(mdat)) {
				return nil, errSplit
			}
			runs = append(runs, r)
			data = append(data, mdat[start:start+n])
		default:
			return nil, errSplit
		}
	}
	if mfhd == nil {
		return nil, errSplit
	}
	body := func(offsets []int64) []byte {
		b := putBox("mfhd", mfhd)
		for i := range runs {
			b = append(b, runs[i].build(offsets[i])...)
		}
		return b
	}
	offsets := make([]int64, len(runs))
	at := int64(8 + len(body(offsets)) + 8)
	var media []byte
	for i := range runs {
		offsets[i] = at
		at += int64(len(data[i]))
		media = append(media, data[i]...)
	}
	return append(putBox("moof", body(offsets)), putBox("mdat", media)...), nil
}

// boxLen is the size of the box at the front of b, header included.
func boxLen(b []byte) (int, bool) {
	if len(b) < 8 {
		return 0, false
	}
	size := int(binary.BigEndian.Uint32(b[:4]))
	head := 8
	if size == 1 {
		if len(b) < 16 {
			return 0, false
		}
		size = int(binary.BigEndian.Uint64(b[8:16]))
		head = 16
	}
	if size == 0 {
		size = len(b)
	}
	if size < head || size > len(b) {
		return 0, false
	}
	return size, true
}
