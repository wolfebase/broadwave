package live

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// partTicks is the shortest segment. A segment starts on a keyframe and
	// stays open until the next one once it has reached this length.
	partTicks = 45000 // 0.5s at 90 kHz
	// windowTicks is how much media a live playlist keeps, about 90 minutes.
	windowTicks = 90 * 60 * 90000
)

// playlistGate is how a playlist request waits for the next part.
// msn is the media sequence of the open segment. part is the index inside it.
type playlistGate struct {
	mu        sync.Mutex
	cond      *sync.Cond
	origin    int
	openMSN   int
	openParts int
}

func newPlaylistGate() *playlistGate {
	g := &playlistGate{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

func (g *playlistGate) publish(origin, openMSN, openParts int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.origin = origin
	g.openMSN = openMSN
	g.openParts = openParts
	g.cond.Broadcast()
	g.mu.Unlock()
}

func (g *playlistGate) ready(msn, part int) bool {
	if msn < g.openMSN {
		return true
	}
	return msn == g.openMSN && part >= 0 && g.openParts > part
}

// wait blocks until the playlist contains that segment or part, or the timeout.
// A negative msn returns immediately.
func (g *playlistGate) wait(msn, part int, d time.Duration) {
	if g == nil || msn < 0 {
		return
	}
	deadline := time.Now().Add(d)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ready(msn, part) {
		return
	}
	timer := time.AfterFunc(d, func() {
		g.mu.Lock()
		g.cond.Broadcast()
		g.mu.Unlock()
	})
	defer timer.Stop()
	for !g.ready(msn, part) {
		if !time.Now().Before(deadline) {
			return
		}
		g.cond.Wait()
	}
}

type packedPart struct {
	name string
	pts  int64
	dur  int64
	body []byte
	sync bool
}

type packedSeg struct {
	name  string
	pts   int64
	dur   int64
	parts []string
}

// packOutput is called before cmd.Start. startPack runs after Start succeeds.
func packOutput(cmd *exec.Cmd) (io.ReadCloser, *playlistGate, chan struct{}, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	return stdout, newPlaylistGate(), make(chan struct{}), nil
}

func startPack(dir string, stdout io.Reader, gate *playlistGate, done chan struct{}) {
	go func() {
		defer close(done)
		if err := Pack(dir, stdout, gate); err != nil && !os.IsNotExist(err) && !errors.Is(err, io.ErrClosedPipe) {
			slog.Error(fmt.Sprintf("pack %s: %v", dir, err))
		}
	}()
}

// Pack turns an fMP4 fragment stream into init.mp4 and one segment per
// fragment. hls.js will not fetch a part until its buffer is already at the
// live edge, so a short segment followed by a longer gap stalls. The newest
// fragment stays a part until the next one arrives and gives it a duration.
func Pack(dir string, r io.Reader, gate *playlistGate) error {
	var init []byte
	var track uint32
	var scale uint32
	var haveInit bool
	var moof []byte
	var open []packedPart
	var closed []packedSeg
	var partN int
	var msn int
	allSync := true

	flush := func() error {
		return writePacked(dir, init, closed, open, msn-len(closed), allSync, gate)
	}
	closeSeg := func(end int64) error {
		if len(open) == 0 {
			return nil
		}
		name := fmt.Sprintf("seg%05d.m4s", msn)
		var body []byte
		names := make([]string, len(open))
		for i, p := range open {
			body = append(body, p.body...)
			names[i] = p.name
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			return err
		}
		dur := end - open[0].pts
		if dur <= 0 {
			dur = partTicks
		}
		closed = append(closed, packedSeg{name: name, pts: open[0].pts, dur: dur, parts: names})
		msn++
		open = nil
		var kept int64
		for _, s := range closed {
			kept += s.dur
		}
		for len(closed) > 3 && kept > windowTicks {
			old := closed[0]
			kept -= old.dur
			closed = closed[1:]
			_ = os.Remove(filepath.Join(dir, old.name))
			for _, n := range old.parts {
				_ = os.Remove(filepath.Join(dir, n))
			}
		}
		if len(closed) >= 3 {
			for _, n := range closed[len(closed)-3].parts {
				_ = os.Remove(filepath.Join(dir, n))
			}
		}
		return nil
	}
	take := func(frag []byte) error {
		if !haveInit {
			return nil
		}
		pts, ok := fragmentPTS(frag, track, scale)
		if !ok {
			return nil
		}
		sync := fragmentIndependent(frag, track)
		if len(open) > 0 {
			if open[len(open)-1].dur == 0 {
				open[len(open)-1].dur = ptsDiff(pts, open[len(open)-1].pts)
			}
			// Hold a short run of frames until the next keyframe, once the
			// open segment has at least half a second. Closing on a frame
			// that is not a keyframe would start the next segment mid-GOP,
			// and a copy and a transcode would no longer share the cut.
			if sync && ptsDiff(pts, open[0].pts) >= int64(partTicks) {
				if err := closeSeg(pts); err != nil {
					return err
				}
			}
		}
		if len(open) == 0 && !sync {
			allSync = false
		}
		name := fmt.Sprintf("part%05d.m4s", partN)
		partN++
		if err := os.WriteFile(filepath.Join(dir, name), frag, 0o644); err != nil {
			return err
		}
		open = append(open, packedPart{name: name, pts: pts, dur: 0, body: frag, sync: sync})
		return flush()
	}

	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32<<10)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		for {
			box, rest, ok := peelBox(buf)
			if !ok {
				break
			}
			buf = rest
			kind := string(box[4:8])
			switch kind {
			case "moof":
				moof = box
			case "mdat":
				if moof == nil {
					break
				}
				frag := append(append([]byte{}, moof...), box...)
				moof = nil
				if err := take(frag); err != nil {
					return err
				}
			default:
				if moof == nil && !haveInit {
					init = append(init, box...)
					if kind == "moov" {
						id, sc, ok := videoTrack(init)
						if !ok {
							return fmt.Errorf("pack: no video track")
						}
						track, scale = id, sc
						if err := os.WriteFile(filepath.Join(dir, "init.mp4"), init, 0o644); err != nil {
							return err
						}
						haveInit = true
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				if len(open) > 0 {
					end := open[len(open)-1].pts + partTicks
					if open[len(open)-1].dur > 0 {
						end = open[len(open)-1].pts + open[len(open)-1].dur
					}
					if err := closeSeg(end); err != nil {
						return err
					}
					return flush()
				}
				return nil
			}
			return err
		}
	}
}

func peelBox(b []byte) (box, rest []byte, ok bool) {
	if len(b) < 8 {
		return nil, b, false
	}
	size := int(binary.BigEndian.Uint32(b[:4]))
	head := 8
	if size == 1 {
		if len(b) < 16 {
			return nil, b, false
		}
		size = int(binary.BigEndian.Uint64(b[8:16]))
		head = 16
	}
	if size < head || size > len(b) {
		return nil, b, false
	}
	return b[:size], b[size:], true
}

func writePacked(dir string, init []byte, closed []packedSeg, open []packedPart, origin int, allSync bool, gate *playlistGate) error {
	if len(init) == 0 {
		return nil
	}
	partTarget := partTicks
	target := partTicks
	for _, s := range closed {
		if s.dur > int64(target) {
			target = int(s.dur)
		}
	}
	for _, p := range open {
		if p.dur > int64(partTarget) {
			partTarget = int(p.dur)
		}
	}
	var b []byte
	b = append(b, "#EXTM3U\n#EXT-X-VERSION:6\n"...)
	targetSec := (target + 89999) / 90000
	if targetSec < 1 {
		targetSec = 1
	}
	b = append(b, "#EXT-X-TARGETDURATION:"+strconv.Itoa(targetSec)+"\n"...)
	// Six target durations is the shortest skip boundary the playlist spec allows.
	b = append(b, "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=1.000,CAN-SKIP-UNTIL="+strconv.Itoa(targetSec*6)+".000\n"...)
	b = append(b, "#EXT-X-PART-INF:PART-TARGET="+fmtDur(int64(partTarget))+"\n"...)
	if allSync {
		b = append(b, "#EXT-X-INDEPENDENT-SEGMENTS\n"...)
	}
	b = append(b, "#EXT-X-MEDIA-SEQUENCE:"+strconv.Itoa(origin)+"\n"...)
	b = append(b, "#EXT-X-MAP:URI=\"init.mp4\"\n"...)
	for _, s := range closed {
		b = append(b, "#EXTINF:"+fmtDur(s.dur)+",\n"+s.name+"\n"...)
	}
	for i, p := range open {
		dur := p.dur
		if dur <= 0 {
			dur = partTicks
		}
		line := "#EXT-X-PART:DURATION=" + fmtDur(dur)
		if p.sync && (i == 0) {
			line += ",INDEPENDENT=YES"
		}
		line += ",URI=\"" + p.name + "\"\n"
		b = append(b, line...)
	}
	tmp := filepath.Join(dir, "index.m3u8.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, "index.m3u8")); err != nil {
		return err
	}
	if gate != nil {
		gate.publish(origin, origin+len(closed), len(open))
	}
	return nil
}

func fmtDur(ticks int64) string {
	if ticks <= 0 {
		ticks = partTicks
	}
	return strconv.FormatFloat(float64(ticks)/90000, 'f', 3, 64)
}

// fragmentIndependent reports whether the fragment's first video sample is a
// keyframe. Unknown flags count as independent so a segment still closes on
// the two-second grid.
func fragmentIndependent(seg []byte, track uint32) bool {
	moof := child(seg, "moof")
	for _, t := range boxes(moof) {
		if t.kind != "traf" {
			continue
		}
		tfhd := child(t.body, "tfhd")
		if len(tfhd) < 8 || binary.BigEndian.Uint32(tfhd[4:8]) != track {
			continue
		}
		flags, ok := firstSampleFlags(tfhd, child(t.body, "trun"))
		if !ok {
			return true
		}
		return flags&0x10000 == 0
	}
	return true
}

func firstSampleFlags(tfhd, trun []byte) (uint32, bool) {
	if len(trun) >= 8 {
		trFlags := uint32(trun[1])<<16 | uint32(trun[2])<<8 | uint32(trun[3])
		if trFlags&0x4 != 0 {
			off := 8
			if trFlags&0x1 != 0 {
				off += 4
			}
			if off+4 <= len(trun) {
				return binary.BigEndian.Uint32(trun[off : off+4]), true
			}
		}
	}
	if len(tfhd) < 8 {
		return 0, false
	}
	tfFlags := uint32(tfhd[1])<<16 | uint32(tfhd[2])<<8 | uint32(tfhd[3])
	if tfFlags&0x20 == 0 {
		return 0, false
	}
	off := 8
	if tfFlags&0x1 != 0 {
		off += 8
	}
	if tfFlags&0x2 != 0 {
		off += 4
	}
	if tfFlags&0x8 != 0 {
		off += 4
	}
	if tfFlags&0x10 != 0 {
		off += 4
	}
	if off+4 > len(tfhd) {
		return 0, false
	}
	return binary.BigEndian.Uint32(tfhd[off : off+4]), true
}
