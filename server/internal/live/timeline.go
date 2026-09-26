package live

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ptsWrap = int64(1) << 33

// firstSegments are never served. It holds decoder warm-up, and its audio starts
// before its video; with broadcast timestamps kept, browsers reject it.
var firstSegments = map[string]bool{"seg00000.ts": true, "seg00000.m4s": true}

// SegmentPTS returns the earliest video presentation time in the start of an
// MPEG-TS segment, in 90 kHz ticks. It falls back to audio when no video PES
// header is found.
func SegmentPTS(path string) (int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	buf := make([]byte, 188*4096)
	n, _ := io.ReadFull(f, buf)
	return scanPTS(buf[:n])
}

func scanPTS(data []byte) (int64, bool) {
	start := bytes.IndexByte(data, 0x47)
	if start < 0 {
		return 0, false
	}
	best, bestAudio := int64(-1), int64(-1)
	for off := start; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		if pkt[0] != 0x47 || pkt[1]&0x40 == 0 {
			continue
		}
		payload := 4
		if afc := (pkt[3] >> 4) & 0x3; afc == 2 || afc == 0 {
			continue
		} else if afc == 3 {
			payload += 1 + int(pkt[4])
		}
		if payload+14 > 188 {
			continue
		}
		pes := pkt[payload:]
		if pes[0] != 0 || pes[1] != 0 || pes[2] != 1 || pes[7]&0x80 == 0 {
			continue
		}
		pts := int64(pes[9]>>1&0x07)<<30 | int64(pes[10])<<22 | int64(pes[11]>>1)<<15 | int64(pes[12])<<7 | int64(pes[13]>>1)
		switch id := pes[3]; {
		case id >= 0xE0 && id <= 0xEF:
			if best < 0 || ptsDiff(pts, best) < 0 {
				best = pts
			}
		case bestAudio < 0:
			bestAudio = pts
		}
	}
	if best >= 0 {
		return best, true
	}
	return bestAudio, bestAudio >= 0
}

// ptsDiff is a-b across the 33-bit wrap.
func ptsDiff(a, b int64) int64 {
	d := (a - b) % ptsWrap
	if d >= ptsWrap/2 {
		d -= ptsWrap
	} else if d < -ptsWrap/2 {
		d += ptsWrap
	}
	return d
}

// Timeline maps a channel's broadcast clock to wall time. Every rendition of the
// channel keeps broadcast timestamps, so they all share one Timeline and every
// client computes the same wall time for the same frame.
type Timeline struct {
	mu       sync.Mutex
	pts      int64
	wall     time.Time
	earliest time.Time
	set      bool
	now      func() time.Time
	behind   time.Duration
}

func NewTimeline() *Timeline {
	return &Timeline{now: time.Now, behind: 4 * time.Second}
}

// Wall returns the wall time for a presentation timestamp, anchoring on first use.
// A jump of more than an hour means the station reset its clock, so it re-anchors.
func (t *Timeline) Wall(pts int64) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set {
		d := ptsDiff(pts, t.pts)
		if d > -90000*3600 && d < 90000*3600 {
			return t.wall.Add(time.Duration(d) * time.Second / 90000)
		}
	}
	t.pts = pts
	t.wall = t.now().Add(-t.behind)
	t.earliest = t.wall
	t.set = true
	return t.wall
}

// Earliest is the program time of the first segment this encode stamped.
// A fresh tune's first frame is only a few seconds old; a long-running one
// keeps the original anchor so a deep buffer can still sit at the latency target.
func (t *Timeline) Earliest() (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.set {
		return time.Time{}, false
	}
	return t.earliest, true
}

// playlistStamper rewrites an ffmpeg live playlist with program date-times from
// the shared Timeline. Segment timestamps are cached; a segment never changes.
type playlistStamper struct {
	mu    sync.Mutex
	cache map[string]int64
}

// reset forgets segment times. A restarted encode reuses seg00001.m4s for a
// new file, and the cached time would stamp that file with the old one.
func (p *playlistStamper) reset() {
	p.mu.Lock()
	p.cache = nil
	p.mu.Unlock()
}

func (p *playlistStamper) stamp(dir string, src []byte, tl *Timeline) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cache == nil {
		p.cache = map[string]int64{}
	}
	lines := strings.Split(string(src), "\n")
	var out bytes.Buffer
	seen := map[string]bool{}
	pending := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#EXT-X-PROGRAM-DATE-TIME"):
			continue
		case strings.HasPrefix(trimmed, "#EXT-X-MEDIA-SEQUENCE:") && (strings.Contains(string(src), "\nseg00000.ts") || strings.Contains(string(src), "\nseg00000.m4s")):
			// The first segment is withheld below, so the sequence starts one later.
			n, _ := strconv.Atoi(strings.TrimPrefix(trimmed, "#EXT-X-MEDIA-SEQUENCE:"))
			out.WriteString("#EXT-X-MEDIA-SEQUENCE:" + strconv.Itoa(n+1) + "\n")
			continue
		case firstSegments[trimmed]:
			pending = pending[:0]
			continue
		case strings.HasPrefix(trimmed, "#EXTINF"):
			pending = append(pending, line)
			continue
		case trimmed != "" && !strings.HasPrefix(trimmed, "#"):
			name := filepath.Base(trimmed)
			seen[name] = true
			pts, ok := p.cache[name]
			if !ok {
				if v, found := segmentStart(dir, name); found {
					pts, ok = v, true
					p.cache[name] = v
				}
			}
			if ok && tl != nil {
				out.WriteString("#EXT-X-PROGRAM-DATE-TIME:" + tl.Wall(pts).UTC().Format("2006-01-02T15:04:05.000Z") + "\n")
			}
			for _, l := range pending {
				out.WriteString(l + "\n")
			}
			pending = pending[:0]
			out.WriteString(line + "\n")
			continue
		}
		for _, l := range pending {
			out.WriteString(l + "\n")
		}
		pending = pending[:0]
		if line != "" {
			out.WriteString(line + "\n")
		}
	}
	for name := range p.cache {
		if !seen[name] {
			delete(p.cache, name)
		}
	}
	return out.Bytes()
}

// readPlaylist is split out so tests can stamp a playlist without ffmpeg.
func readPlaylist(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(bufio.NewReader(f))
}
