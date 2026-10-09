package live

import (
	"bytes"
	"strconv"
	"sync"
)

// DeltaPlaylist answers an LL-HLS delta request. Segments further than
// CAN-SKIP-UNTIL from the end are replaced with EXT-X-SKIP, so a reload of a
// long half-second playlist stays small. A playlist that is already inside
// the skip boundary is returned unchanged, the same slice the caller passed.
func DeltaPlaylist(body []byte) []byte {
	if !bytes.Contains(body, []byte("#EXTINF")) {
		return body
	}
	skipUntil := skipUntilSeconds(body)
	lp := linePool.Get().(*[]lineRef)
	bp := blockPool.Get().(*[]deltaBlock)
	sp := segPool.Get().(*[]int)
	lines := (*lp)[:0]
	blocks := (*bp)[:0]
	segAt := (*sp)[:0]
	defer func() {
		*lp = lines[:0]
		*bp = blocks[:0]
		*sp = segAt[:0]
		linePool.Put(lp)
		blockPool.Put(bp)
		segPool.Put(sp)
	}()

	for i := 0; i <= len(body); {
		j := i
		for j < len(body) && body[j] != '\n' {
			j++
		}
		lines = append(lines, lineRef{a: i, b: j})
		if j >= len(body) {
			break
		}
		i = j + 1
		if i == len(body) {
			lines = append(lines, lineRef{i, i})
			break
		}
	}

	cur := 0
	for i := range lines {
		trim := bytes.TrimSpace(body[lines[i].a:lines[i].b])
		if bytes.HasPrefix(trim, []byte("#EXTINF:")) {
			split := i
			for split > cur {
				prev := bytes.TrimSpace(body[lines[split-1].a:lines[split-1].b])
				// DISCONTINUITY-SEQUENCE shares the prefix, so a trailing one
				// rides with the segment, as it did when the playlist was split.
				if bytes.HasPrefix(prev, []byte("#EXT-X-PROGRAM-DATE-TIME:")) || bytes.HasPrefix(prev, []byte("#EXT-X-DISCONTINUITY")) {
					split--
					continue
				}
				break
			}
			if split > cur {
				blocks = append(blocks, deltaBlock{a: cur, b: split})
			}
			blocks = append(blocks, deltaBlock{a: split, b: i + 1, dur: extinfSeconds(trim), seg: true})
			cur = i + 1
			continue
		}
		if n := len(blocks); n > 0 && blocks[n-1].seg && !blockHasURI(body, lines, blocks[n-1]) && len(trim) > 0 && trim[0] != '#' {
			blocks[n-1].b = i + 1
			cur = i + 1
			continue
		}
	}
	if cur < len(lines) {
		blocks = append(blocks, deltaBlock{a: cur, b: len(lines)})
	}

	for i := range blocks {
		if blocks[i].seg {
			segAt = append(segAt, i)
		}
	}
	if len(segAt) == 0 {
		return body
	}
	acc := 0.0
	start := len(segAt)
	for start > 0 && acc < skipUntil {
		start--
		acc += blocks[segAt[start]].dur
	}
	if start == 0 {
		return body
	}
	keep := segAt[start]
	if !deltaHasSequence(body, lines, blocks, keep) {
		return body
	}

	out := make([]byte, 0, 4096)
	wrote := false
	for i, block := range blocks {
		if block.seg && i < keep {
			continue
		}
		for n := block.a; n < block.b; n++ {
			if lines[n].a == lines[n].b && i == len(blocks)-1 {
				continue
			}
			line := body[lines[n].a:lines[n].b]
			trim := bytes.TrimSpace(line)
			if bytes.HasPrefix(trim, []byte("#EXT-X-VERSION:")) {
				out = append(out, "#EXT-X-VERSION:9\n"...)
			} else {
				out = append(out, line...)
				out = append(out, '\n')
			}
			if !wrote && bytes.HasPrefix(trim, []byte("#EXT-X-MEDIA-SEQUENCE:")) {
				out = append(out, "#EXT-X-SKIP:SKIPPED-SEGMENTS="...)
				out = strconv.AppendInt(out, int64(start), 10)
				out = append(out, '\n')
				wrote = true
			}
		}
	}
	if !wrote {
		return body
	}
	return out
}

type lineRef struct {
	a, b int
}

type deltaBlock struct {
	a, b int
	dur  float64
	seg  bool
}

var (
	linePool = sync.Pool{New: func() any {
		s := make([]lineRef, 0, 256)
		return &s
	}}
	blockPool = sync.Pool{New: func() any {
		s := make([]deltaBlock, 0, 128)
		return &s
	}}
	segPool = sync.Pool{New: func() any {
		s := make([]int, 0, 128)
		return &s
	}}
)

func skipUntilSeconds(playlist []byte) float64 {
	const key = "CAN-SKIP-UNTIL="
	i := bytes.Index(playlist, []byte(key))
	if i < 0 {
		return 6
	}
	rest := playlist[i+len(key):]
	end := 0
	for end < len(rest) && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	v := parseDec(rest[:end])
	if v <= 0 {
		return 6
	}
	return v
}

func extinfSeconds(line []byte) float64 {
	v := line[len("#EXTINF:"):]
	if i := bytes.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return parseDec(bytes.TrimSpace(v))
}

// parseDec reads a short decimal the way the playlist durations are written.
// A bad token is zero, which is what a failed parse used to return.
func parseDec(v []byte) float64 {
	if len(v) == 0 {
		return 0
	}
	neg := false
	if v[0] == '-' {
		neg = true
		v = v[1:]
	}
	var whole, frac uint64
	var digits int
	i := 0
	saw := false
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		whole = whole*10 + uint64(v[i]-'0')
		i++
		saw = true
	}
	if i < len(v) && v[i] == '.' {
		i++
		for i < len(v) && v[i] >= '0' && v[i] <= '9' && digits < 6 {
			frac = frac*10 + uint64(v[i]-'0')
			digits++
			i++
			saw = true
		}
	}
	if !saw || i != len(v) {
		return 0
	}
	div := 1.0
	for range digits {
		div *= 10
	}
	n := float64(whole) + float64(frac)/div
	if neg {
		return -n
	}
	return n
}

func blockHasURI(body []byte, lines []lineRef, block deltaBlock) bool {
	for n := block.a; n < block.b; n++ {
		trim := bytes.TrimSpace(body[lines[n].a:lines[n].b])
		if len(trim) > 0 && trim[0] != '#' {
			return true
		}
	}
	return false
}

func deltaHasSequence(body []byte, lines []lineRef, blocks []deltaBlock, keep int) bool {
	for i, block := range blocks {
		if block.seg && i < keep {
			continue
		}
		for n := block.a; n < block.b; n++ {
			trim := bytes.TrimSpace(body[lines[n].a:lines[n].b])
			if bytes.HasPrefix(trim, []byte("#EXT-X-MEDIA-SEQUENCE:")) {
				return true
			}
		}
	}
	return false
}
