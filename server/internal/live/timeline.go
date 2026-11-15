package live

import (
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

// anchorStep is how far ahead of its anchor a timestamp gets before the
// anchor moves up to it.
const anchorStep = 90000 * 600

// stampSlack is the overlap a segment may have with the one before it
// and still keep its own date. A playlist duration is rounded to a
// millisecond, so the end worked out from it can sit a fraction of a
// millisecond past the next start. Pinning that moved the clock forward
// for good, by a different amount on each encode.
const stampSlack = 2 * time.Millisecond

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
	// seed, when seeded, is another encode's mapping moved onto this one's
	// timestamps: the first anchor follows it instead of the clock on the wall.
	seedPTS  int64
	seedWall time.Time
	seeded   bool
	// carried is a seed from the encode this one replaced (Carry). It gives
	// way when it would date the first picture far from the wall clock.
	carried bool
	// startWall, when set, dates the first anchor instead of the wall clock.
	startWall time.Time
	// carry is input silence (Shift) not yet stamped on a segment. A
	// re-anchor on a timestamp break adds it, so the break lands on the wall
	// clock instead of right after the last segment.
	carry time.Duration
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
			// The anchor follows the broadcast in whole seconds, which
			// convert exactly. Left where the tune began, an hour of
			// television would read as a station clock reset.
			if d >= anchorStep {
				whole := d / 90000
				t.pts = (t.pts + whole*90000) % ptsWrap
				t.wall = t.wall.Add(time.Duration(whole) * time.Second)
				d -= whole * 90000
			}
			return t.wall.Add(time.Duration(d) * time.Second / 90000)
		}
	}
	t.pts = pts
	t.wall = t.now().Add(-t.behind)
	if !t.set && !t.startWall.IsZero() {
		t.wall = t.startWall
	}
	if d := ptsDiff(pts, t.seedPTS); t.seeded && !t.set && d > -90000*3600 && d < 90000*3600 {
		if w := t.seedWall.Add(time.Duration(d) * time.Second / 90000); !t.carried || (w.Sub(t.wall) <= carryWithin && t.wall.Sub(w) <= carryWithin) {
			t.wall = w
		}
	}
	t.earliest = t.wall
	t.set = true
	return t.wall
}

// Seed makes the first anchor put pts at wall. After the first anchor it
// does nothing.
func (t *Timeline) Seed(pts int64, wall time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set {
		return
	}
	t.seedPTS = pts
	t.seedWall = wall
	t.seeded = true
	t.carried = false
}

// carryWithin is how far a carried seed may date the first picture from
// the clock on the wall. Further off, the broadcast clock changed while the
// encode was down (a station clock reset), and the old mapping is wrong.
const carryWithin = 10 * time.Second

// Carry seeds the first anchor from the encode this one replaced on the same
// broadcast clock, so a frame keeps its date across a restart. Unlike a
// sibling's seed, it is dropped when it lands more than carryWithin from the
// wall clock.
func (t *Timeline) Carry(pts int64, wall time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set {
		return
	}
	t.seedPTS = pts
	t.seedWall = wall
	t.seeded = true
	t.carried = true
}

// Start makes the first anchor put its picture at wall, for an encode whose
// first picture is older than the clock on the wall says. A seed wins.
func (t *Timeline) Start(wall time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set || t.seeded {
		return
	}
	t.startWall = wall
}

// hasDate reports whether the first stamp will follow a seed or an anchor
// instead of reading the wall clock.
func (t *Timeline) hasDate() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.set || t.seeded || !t.startWall.IsZero()
}

// Anchor is the timestamp the clock last anchored on and its wall time.
func (t *Timeline) Anchor() (int64, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pts, t.wall, t.set
}

// Reanchor keeps the program date-time on the wall clock across a timestamp
// jump. The next presentation time maps to wall, and Earliest stays put.
func (t *Timeline) Reanchor(pts int64, wall time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pts = pts
	t.wall = wall.Add(t.carry)
	t.carry = 0
	t.set = true
}

// Continue re-anchors on an encode that took over on the same broadcast
// clock: pts maps to wall exactly. A silence the tuner had is already in
// wall, since the broadcast clock ran on through it.
func (t *Timeline) Continue(pts int64, wall time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pts = pts
	t.wall = wall
	t.carry = 0
	t.set = true
}

// Shift moves the clock on by d after the tuner sent nothing for d. A source
// that pauses and then carries on with the next timestamp would otherwise put
// every later frame d in the past, and players would chase a live edge that
// is not there.
func (t *Timeline) Shift(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.set || d <= 0 {
		return
	}
	t.wall = t.wall.Add(d)
	t.carry += d
}

// Settle marks the shift as stamped: a segment after the silence carries it.
func (t *Timeline) Settle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.carry = 0
}

// Earliest is the program time of the first part or segment this encode stamped.
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
// the shared Timeline. A segment's timestamp and date-time are cached: a
// discontinuity moves the anchor, and a later stamp must not measure the
// segments from before that move against it.
type playlistStamper struct {
	mu    sync.Mutex
	cache map[string]int64
	walls map[string]time.Time
	// broke names the first part after each break, anchored once. It stays
	// listed while its segment is open and for a while after; anchoring it
	// again would drop the shift a silence added.
	broke map[string]bool
	// opens is the date the open segment was given, by its first part. A
	// player has it, so the segment keeps it when it closes, also when a
	// silence moved the clock on meanwhile.
	opens map[string]time.Time
	// lastSrc and lastOut are the previous playlist and the bytes handed
	// out for it. A reload of the same file skips the rebuild. They stay
	// empty when a part or segment had no timestamp yet, so the next reload
	// can date it once its file is on disk. lastTimed is whether that rebuild
	// had a timeline: a dated playlist is not an answer for an undated one.
	lastSrc   []byte
	lastOut   []byte
	lastTimed bool
}

// reset forgets segment times. A restarted encode reuses seg00001.m4s for a
// new file, and the cached time would stamp that file with the old one.
func (p *playlistStamper) reset() {
	p.mu.Lock()
	p.cache = nil
	p.walls = nil
	p.broke = nil
	p.opens = nil
	p.lastSrc = nil
	p.lastOut = nil
	p.lastTimed = false
	p.mu.Unlock()
}

func (p *playlistStamper) stamp(dir string, src []byte, tl *Timeline) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastOut != nil && p.lastTimed == (tl != nil) && bytes.Equal(p.lastSrc, src) {
		return p.lastOut
	}
	if p.cache == nil {
		p.cache = map[string]int64{}
	}
	if p.walls == nil {
		p.walls = map[string]time.Time{}
	}
	if p.broke == nil {
		p.broke = map[string]bool{}
	}
	if p.opens == nil {
		p.opens = map[string]time.Time{}
	}
	lines := strings.Split(string(src), "\n")
	var out bytes.Buffer
	seen := map[string]bool{}
	pending := []string{}
	var lastEnd time.Time
	var haveEnd bool
	// lastWall and lastPTS are the last segment's date and first picture.
	var lastWall time.Time
	var lastPTS int64
	var haveLast bool
	var breakNext bool
	// cont is the step a continuing encode's first picture comes after the
	// last end (#EXT-X-CONT), for the break it follows. same is a gap inside
	// one encode: its timestamps say where the next picture sits.
	var cont time.Duration
	var contNext, same bool
	// first is the first part listed since the last segment: the segment
	// that follows starts with it.
	first := ""
	// missed is a part or segment the timeline still cannot date. Caching
	// that playlist would keep the date off after the file shows up.
	missed := false
	// openAt is where in out the first part after the last segment starts,
	// and openWall its date: the open segment's date line goes there.
	// AVPlayer will not play low latency from a playlist with no date, and
	// a fresh tune's first playlist lists only parts.
	openAt, openWall := -1, time.Time{}
	noteBreak := func(pts int64) {
		if breakNext && haveEnd && tl != nil {
			if contNext && same && haveLast {
				tl.Continue(pts, lastWall.Add(time.Duration(ptsDiff(pts, lastPTS))*time.Second/90000))
			} else if contNext {
				tl.Continue(pts, lastEnd.Add(cont))
			} else {
				tl.Reanchor(pts, lastEnd)
			}
			breakNext, contNext = false, false
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#EXT-X-PROGRAM-DATE-TIME"):
			continue
		case strings.HasPrefix(trimmed, "#EXT-X-CONT:"):
			// The packer's note that the encode after this break continues
			// the broadcast clock. The line is not served.
			raw, mark, _ := strings.Cut(strings.TrimPrefix(trimmed, "#EXT-X-CONT:"), ",")
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && breakNext {
				cont, contNext, same = time.Duration(v)*time.Second/90000, true, mark == "SAME"
			}
			continue
		case strings.HasPrefix(trimmed, "#EXT-X-PTS:"):
			// The packer already measured this fragment. The line is not served.
			if name, pts, ok := packedPTS(trimmed); ok {
				p.cache[ownName(name)] = pts
			}
			continue
		case strings.HasPrefix(trimmed, "#EXT-X-PART:"):
			for _, l := range pending {
				out.WriteString(l + "\n")
			}
			pending = pending[:0]
			if name := ownName(partName(trimmed)); name != "" {
				seen[name] = true
				if first == "" {
					first = name
				}
				pts, ok := p.cache[name]
				if !ok {
					if v, found := segmentStart(dir, name); found {
						pts, ok = v, true
						p.cache[name] = v
					}
				}
				if tl != nil && !ok {
					missed = true
				}
				// The part anchors the clock. A date on every part is not written:
				// the tag belongs to the next media segment.
				if ok && tl != nil {
					if breakNext && p.broke[name] {
						breakNext = false
					} else if breakNext && haveEnd {
						p.broke[name] = true
					}
					noteBreak(pts)
					wall := tl.Wall(pts)
					if openAt < 0 {
						if haveEnd && wall.Before(lastEnd) {
							wall = lastEnd
						}
						if given, ok := p.opens[name]; ok {
							wall = given
						}
						p.opens[name] = wall
						openAt, openWall = out.Len(), wall
					}
				}
			}
			out.WriteString(line + "\n")
			continue
		case strings.HasPrefix(trimmed, "#EXTINF"):
			pending = append(pending, line)
			continue
		case trimmed != "" && !strings.HasPrefix(trimmed, "#"):
			name := ownName(filepath.Base(trimmed))
			seen[name] = true
			pts, ok := p.cache[name]
			if !ok {
				if v, found := segmentStart(dir, name); found {
					pts, ok = v, true
					p.cache[name] = v
				}
			}
			if wall, cached := p.walls[name]; cached {
				// Already assigned. Wall would measure this segment from the
				// anchor a discontinuity moved, and shift every date-time.
				out.WriteString("#EXT-X-PROGRAM-DATE-TIME:" + wall.UTC().Format("2006-01-02T15:04:05.000Z") + "\n")
				if d := pendingDur(pending); d > 0 {
					lastEnd = wall.Add(d)
					haveEnd = true
				}
				lastWall, lastPTS, haveLast = wall, pts, ok
				breakNext = false
			} else if ok && tl != nil {
				noteBreak(pts)
				wall := tl.Wall(pts)
				given, wasOpen := p.opens[first]
				switch {
				case wasOpen && given.After(wall.Add(stampSlack)):
					// Dated while open at the previous segment's end, below.
					tl.Reanchor(pts, given)
					wall = given
				case wasOpen && wall.Sub(given) > stampSlack:
					// Players have this date from its parts. A silence that
					// moved the clock on since lands on the next segment.
					wall = given
				case wasOpen:
					// The open date was this one, or the rounded end before it
					// a fraction of a millisecond later. The segment takes its
					// own: kept, those fractions add up until the clock is
					// pinned forward.
					tl.Settle()
				case haveEnd && lastEnd.Sub(wall) > stampSlack:
					// A short step the jump check missed overlaps the previous
					// segment. Pin it to that segment's end and re-anchor, so the
					// program timeline stays monotonic and the next segment continues.
					tl.Reanchor(pts, lastEnd)
					wall = tl.Wall(pts)
					tl.Settle()
				default:
					tl.Settle()
				}
				p.walls[name] = wall
				lastWall, lastPTS, haveLast = wall, pts, true
				out.WriteString("#EXT-X-PROGRAM-DATE-TIME:" + wall.UTC().Format("2006-01-02T15:04:05.000Z") + "\n")
				if d := pendingDur(pending); d > 0 {
					lastEnd = wall.Add(d)
					haveEnd = true
				}
			} else if tl != nil {
				missed = true
			}
			for _, l := range pending {
				out.WriteString(l + "\n")
			}
			pending = pending[:0]
			out.WriteString(line + "\n")
			openAt = -1
			first = ""
			continue
		}
		for _, l := range pending {
			out.WriteString(l + "\n")
		}
		pending = pending[:0]
		if strings.HasPrefix(trimmed, "#EXT-X-DISCONTINUITY") {
			breakNext, contNext, same = true, false, false
		}
		if line != "" {
			out.WriteString(line + "\n")
		}
	}
	for name := range p.cache {
		if !seen[name] {
			delete(p.cache, name)
			delete(p.walls, name)
			delete(p.broke, name)
			delete(p.opens, name)
		}
	}
	body := out.Bytes()
	if openAt >= 0 {
		date := "#EXT-X-PROGRAM-DATE-TIME:" + openWall.UTC().Format("2006-01-02T15:04:05.000Z") + "\n"
		body = append(append(append([]byte{}, body[:openAt]...), date...), body[openAt:]...)
	}
	if missed {
		return body
	}
	p.lastSrc = make([]byte, len(src))
	copy(p.lastSrc, src)
	p.lastOut = make([]byte, len(body))
	copy(p.lastOut, body)
	p.lastTimed = tl != nil
	return p.lastOut
}

func pendingDur(lines []string) time.Duration {
	for _, l := range lines {
		v, ok := strings.CutPrefix(strings.TrimSpace(l), "#EXTINF:")
		if !ok {
			continue
		}
		v = strings.TrimSuffix(v, ",")
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f <= 0 {
			continue
		}
		return time.Duration(f * float64(time.Second))
	}
	return 0
}

// packedPTS is the start time the packer wrote for one part or segment,
// in 90 kHz ticks. The name is the file, with no directory.
func packedPTS(line string) (string, int64, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXT-X-PTS:")
	if !ok {
		return "", 0, false
	}
	name, raw, ok := strings.Cut(rest, ":")
	if !ok || name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return "", 0, false
	}
	pts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "", 0, false
	}
	return name, pts, true
}

// ownName copies a token out of the playlist text. playlistStamper stores
// the first name it sees for each segment, and that name stays until the
// segment leaves the live window.
func ownName(name string) string {
	if name == "" {
		return ""
	}
	return strings.Clone(name)
}

func partName(line string) string {
	const key = `URI="`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return filepath.Base(rest[:j])
}

// readPlaylist is split out so tests can stamp a playlist without ffmpeg.
func readPlaylist(path string) ([]byte, error) {
	// A buffered reader would allocate a 4 KiB block on top of the file
	// itself, and a live playlist is read again for every viewer.
	return os.ReadFile(path)
}
