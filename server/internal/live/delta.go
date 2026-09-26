package live

import (
	"strconv"
	"strings"
)

// DeltaPlaylist answers an LL-HLS delta request. Segments further than
// CAN-SKIP-UNTIL from the end are replaced with EXT-X-SKIP, so a reload of a
// long half-second playlist stays small. A playlist that is already inside
// the skip boundary is returned unchanged.
func DeltaPlaylist(body []byte) []byte {
	text := string(body)
	if !strings.Contains(text, "#EXTINF") {
		return body
	}
	skipUntil := skipUntilSeconds(text)
	lines := strings.Split(text, "\n")
	type block struct {
		lines []string
		dur   float64
		seg   bool
	}
	var blocks []block
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		blocks = append(blocks, block{lines: cur})
		cur = nil
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#EXTINF:") {
			var pdt []string
			var head []string
			for _, l := range cur {
				if strings.HasPrefix(strings.TrimSpace(l), "#EXT-X-PROGRAM-DATE-TIME:") {
					pdt = append(pdt, l)
				} else {
					head = append(head, pdt...)
					pdt = nil
					head = append(head, l)
				}
			}
			cur = nil
			if len(head) > 0 {
				blocks = append(blocks, block{lines: head})
			}
			blocks = append(blocks, block{lines: append(pdt, line), dur: extinfSeconds(trim), seg: true})
			continue
		}
		if n := len(blocks); n > 0 && blocks[n-1].seg && !blockHasURI(blocks[n-1].lines) && trim != "" && !strings.HasPrefix(trim, "#") {
			blocks[n-1].lines = append(blocks[n-1].lines, line)
			continue
		}
		cur = append(cur, line)
	}
	flush()

	var segAt []int
	for i, b := range blocks {
		if b.seg {
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

	var b strings.Builder
	wroteSkip := false
	write := func(line string) {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#EXT-X-VERSION:") {
			line = "#EXT-X-VERSION:9"
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if !wroteSkip && strings.HasPrefix(trim, "#EXT-X-MEDIA-SEQUENCE:") {
			b.WriteString("#EXT-X-SKIP:SKIPPED-SEGMENTS=" + strconv.Itoa(start) + "\n")
			wroteSkip = true
		}
	}
	for i, block := range blocks {
		if block.seg && i < segAt[start] {
			continue
		}
		for _, line := range block.lines {
			if line == "" && i == len(blocks)-1 {
				continue
			}
			write(line)
		}
	}
	if !wroteSkip {
		return body
	}
	return []byte(b.String())
}

func skipUntilSeconds(playlist string) float64 {
	const key = "CAN-SKIP-UNTIL="
	i := strings.Index(playlist, key)
	if i < 0 {
		return 6
	}
	rest := playlist[i+len(key):]
	end := 0
	for end < len(rest) && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	v, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil || v <= 0 {
		return 6
	}
	return v
}

func extinfSeconds(line string) float64 {
	v := strings.TrimPrefix(line, "#EXTINF:")
	v = strings.TrimSuffix(v, ",")
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}

func blockHasURI(lines []string) bool {
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim != "" && !strings.HasPrefix(trim, "#") {
			return true
		}
	}
	return false
}
