package live

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// The delta scan reads bytes in place. It has to answer every playlist the
// packer and the stamper write exactly as the text version it replaced did.
func TestDeltaPlaylistMatchesTheTextVersion(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for n := range 3000 {
		in := randomPlaylist(rng)
		got := DeltaPlaylist([]byte(in))
		want := deltaPlaylistText([]byte(in))
		if !bytes.Equal(got, want) {
			t.Fatalf("case %d differs\ninput:\n%s\ngot:\n%s\nwant:\n%s", n, in, got, want)
		}
	}
}

func TestParseDecMatchesParseFloat(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	tokens := []string{"", ".", "-", "+1", "1e3", "2.", ".5", "0.500", "2.002", "12.000", "1.2.3", "007.25",
		"1.0000001", "123456789012345.6", "0.1234567890123456", "inf", "NaN", "0x1p-2", "1_0", " 2", "-1.5"}
	for range 20000 {
		whole := rng.IntN(100000)
		frac := rng.IntN(1000000)
		width := 1 + rng.IntN(6)
		tokens = append(tokens, fmt.Sprintf("%d.%0*d", whole, width, frac%int(pow10(width))))
	}
	for _, tok := range tokens {
		want, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			want = 0
		}
		if got := parseDec([]byte(tok)); math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("%q: %v, ParseFloat %v", tok, got, want)
		}
	}
}

func pow10(n int) float64 {
	v := 1.0
	for range n {
		v *= 10
	}
	return v
}

// randomPlaylist is a stamped live playlist: a header, segments with dates,
// discontinuities, parts, and an open tail. A URI always follows its EXTINF,
// as the packer writes it.
func randomPlaylist(rng *rand.Rand) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	if rng.IntN(5) > 0 {
		fmt.Fprintf(&b, "#EXT-X-VERSION:%d\n", 6+rng.IntN(4))
	}
	b.WriteString("#EXT-X-TARGETDURATION:2\n")
	switch rng.IntN(4) {
	case 0:
	case 1:
		b.WriteString("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,CAN-SKIP-UNTIL=6.000\n")
	default:
		fmt.Fprintf(&b, "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=1.500,CAN-SKIP-UNTIL=%s\n",
			[]string{"3.000", "12.000", "4.5", "0", "7.0000001", "x"}[rng.IntN(6)])
	}
	if rng.IntN(3) == 0 {
		fmt.Fprintf(&b, "#EXT-X-DISCONTINUITY-SEQUENCE:%d\n", rng.IntN(5))
	}
	if rng.IntN(8) > 0 {
		fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", rng.IntN(1000))
	}
	b.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")
	segs := rng.IntN(40)
	part := 0
	for i := range segs {
		if rng.IntN(10) == 0 {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		if rng.IntN(15) == 0 {
			b.WriteString("\n")
		}
		for range rng.IntN(4) {
			fmt.Fprintf(&b, "#EXT-X-PART:DURATION=0.500,URI=\"part%05d.m4s\"\n", part)
			part++
		}
		if rng.IntN(6) > 0 {
			fmt.Fprintf(&b, "#EXT-X-PROGRAM-DATE-TIME:2026-10-09T12:00:%02d.000Z\n", i%60)
		}
		dur := []string{"2.000", "0.500", "1.001", "0.533", "2.002", "1.5", "3"}[rng.IntN(7)]
		fmt.Fprintf(&b, "#EXTINF:%s,\nseg%05d.m4s\n", dur, i)
	}
	for range rng.IntN(4) {
		fmt.Fprintf(&b, "#EXT-X-PART:DURATION=0.500,URI=\"part%05d.m4s\"\n", part)
		part++
	}
	if rng.IntN(4) == 0 {
		b.WriteString("#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"next.m4s\"\n")
	}
	out := b.String()
	if rng.IntN(4) == 0 {
		out = strings.TrimSuffix(out, "\n")
	}
	if rng.IntN(8) == 0 {
		out += "\n"
	}
	return out
}

// deltaPlaylistText is the string version DeltaPlaylist replaced.
func deltaPlaylistText(body []byte) []byte {
	text := string(body)
	if !strings.Contains(text, "#EXTINF") {
		return body
	}
	skipUntil := 6.0
	if i := strings.Index(text, "CAN-SKIP-UNTIL="); i >= 0 {
		rest := text[i+len("CAN-SKIP-UNTIL="):]
		end := 0
		for end < len(rest) && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
			end++
		}
		if v, err := strconv.ParseFloat(rest[:end], 64); err == nil && v > 0 {
			skipUntil = v
		}
	}
	extinf := func(line string) float64 {
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
	hasURI := func(lines []string) bool {
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			if trim != "" && !strings.HasPrefix(trim, "#") {
				return true
			}
		}
		return false
	}
	type block struct {
		lines []string
		dur   float64
		seg   bool
	}
	var blocks []block
	var cur []string
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#EXTINF:") {
			var pdt, head []string
			for _, l := range cur {
				trimL := strings.TrimSpace(l)
				if strings.HasPrefix(trimL, "#EXT-X-PROGRAM-DATE-TIME:") || strings.HasPrefix(trimL, "#EXT-X-DISCONTINUITY") {
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
			blocks = append(blocks, block{lines: append(pdt, line), dur: extinf(trim), seg: true})
			continue
		}
		if n := len(blocks); n > 0 && blocks[n-1].seg && !hasURI(blocks[n-1].lines) && trim != "" && !strings.HasPrefix(trim, "#") {
			blocks[n-1].lines = append(blocks[n-1].lines, line)
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		blocks = append(blocks, block{lines: cur})
	}
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
	for i, block := range blocks {
		if block.seg && i < segAt[start] {
			continue
		}
		for _, line := range block.lines {
			if line == "" && i == len(blocks)-1 {
				continue
			}
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "#EXT-X-VERSION:") {
				line = "#EXT-X-VERSION:9"
			}
			b.WriteString(line + "\n")
			if !wroteSkip && strings.HasPrefix(trim, "#EXT-X-MEDIA-SEQUENCE:") {
				b.WriteString("#EXT-X-SKIP:SKIPPED-SEGMENTS=" + strconv.Itoa(start) + "\n")
				wroteSkip = true
			}
		}
	}
	if !wroteSkip {
		return body
	}
	return []byte(b.String())
}
