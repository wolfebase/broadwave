package live

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// partTicks is the shortest segment. A segment starts on a keyframe and
	// stays open until the next one once it has reached this length.
	partTicks = 45000 // 0.5s at 90 kHz
	// longGroup is past the one-second group of pictures most stations send,
	// also with a short keyframe fragment in front of it.
	longGroup = 135000
	// windowTicks is how much media a live playlist keeps, about 90 minutes.
	windowTicks = 90 * 60 * 90000
	// jumpTicks is a presentation-time step that is not one more group of
	// pictures. Backwards past half a second, or forwards past this, starts
	// a new timeline. ptsDiff already folds a 33-bit wrap of one group into
	// a normal step, so that wrap is not a jump.
	jumpTicks = 10 * 90000
	// minPartTicks is the shortest fragment published as its own part. An
	// interlaced keyframe can come as a one-field fragment (17 ms) that holds
	// no sound frame, and AVPlayer fails a sound view with an empty part.
	minPartTicks = 9000
	// maxOpenBytes bounds the fragments held for the segment that is still
	// open. A backwards timestamp never meets the keyframe close.
	maxOpenBytes = 32 << 20
	// maxBox is the largest incomplete fMP4 box kept in the read buffer.
	// A size field that asks for more is corrupt.
	maxBox = 32 << 20
)

// playlistGate is how a playlist request waits for the next part.
// msn is the media sequence of the open segment. part is the index inside it.
type playlistGate struct {
	mu        sync.Mutex
	cond      *sync.Cond
	origin    int
	openMSN   int
	openParts int
	// firstPart and firstSegment are when the playlist first listed one.
	firstPart, firstSegment stamp
	// moved is when the playlist last grew, in Unix ns; gap is the longest
	// wait between two growths, so the output watchdog learns the pace.
	moved, gap atomic.Int64
	// target is the TARGETDURATION the playlist advertises, in ns.
	target atomic.Int64
	// closedParts is how many parts each recent closed segment had, the
	// last one being openMSN-1.
	closedParts []int
}

// blockFloor is the least a blocking playlist request waits.
const blockFloor = 1500 * time.Millisecond

// holdFor is how long a blocking playlist request may wait for what it asked
// for: three advertised targets, as the protocol allows. A server must not
// answer one without that segment or part. A copied broadcast is cut at its
// own keyframes, so a segment can take 1.5 s or more.
func (g *playlistGate) holdFor() time.Duration {
	if g == nil {
		return blockFloor
	}
	return max(blockFloor, 3*time.Duration(g.target.Load()))
}

func newPlaylistGate() *playlistGate {
	g := &playlistGate{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

func (g *playlistGate) publish(origin, openMSN, openParts int, closedParts ...int) {
	if g == nil {
		return
	}
	if openParts > 0 || openMSN > origin {
		g.firstPart.mark()
	}
	if openMSN > origin {
		g.firstSegment.mark()
	}
	g.mu.Lock()
	if openMSN != g.openMSN || openParts != g.openParts {
		now := time.Now().UnixNano()
		if last := g.moved.Load(); last != 0 && now-last > g.gap.Load() {
			g.gap.Store(now - last)
		}
		g.moved.Store(now)
	}
	g.origin = origin
	g.openMSN = openMSN
	g.openParts = openParts
	g.closedParts = closedParts
	g.cond.Broadcast()
	g.mu.Unlock()
}

// stuck reports a playlist that has not grown for limit since it last grew
// or since from, whichever is later, or for three of its own longest gaps
// when a broadcast's picture groups run longer.
func (g *playlistGate) stuck(now, from time.Time, limit time.Duration) bool {
	if g == nil || g.moved.Load() == 0 {
		return false
	}
	since := time.Unix(0, g.moved.Load())
	if from.After(since) {
		since = from
	}
	// A tuner silence also counts as a gap; the cap keeps one from
	// loosening the watch for the rest of the tune.
	return now.Sub(since) > max(limit, 3*min(time.Duration(g.gap.Load()), 10*time.Second))
}

// ready reports whether the playlist holds the segment, or the part, asked
// for. A part past a closed segment's last one is the next segment's first.
func (g *playlistGate) ready(msn, part int) bool {
	for msn < g.openMSN {
		back := g.openMSN - 1 - msn
		if part < 0 || back >= len(g.closedParts) {
			return true
		}
		n := g.closedParts[len(g.closedParts)-1-back]
		if n == 0 || part < n {
			return true
		}
		msn, part = msn+1, 0
	}
	return msn == g.openMSN && part >= 0 && g.openParts > part
}

// wait blocks until the playlist contains that segment or part, or the timeout.
// A negative msn returns immediately.
// block holds a blocking playlist request until the playlist lists what it
// asked for, for up to holdFor. The hold is read again whenever the playlist
// changes: a request that comes before the first playlist would otherwise
// get only the floor.
func (g *playlistGate) block(msn, part int) {
	if g == nil || msn < 0 {
		return
	}
	began := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	for !g.ready(msn, part) {
		left := g.holdFor() - time.Since(began)
		if left <= 0 {
			return
		}
		timer := time.AfterFunc(left, func() {
			g.mu.Lock()
			g.cond.Broadcast()
			g.mu.Unlock()
		})
		g.cond.Wait()
		timer.Stop()
	}
}

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
	parts []partRef
	gap   bool
}

// partRef is a part of a closed segment, listed until it is three target
// durations from the end of the playlist.
type partRef struct {
	name string
	dur  int64
	sync bool
}

// partsKept reports whether a closed segment that ends this far before the
// end of the playlist still lists its parts, and whether their files stay.
// A player may still fetch a part for three target durations after it is
// no longer listed.
func partsKept(fromEnd int64, target int) (listed, kept bool) {
	t := int64(max(target, 2*90000))
	return fromEnd < 3*t, fromEnd < 6*t
}

// packPipe is ffmpeg's stdout. cmd.StdoutPipe would be closed by Wait, which
// can run before the packager has read the last fragment.
type packPipe struct {
	*os.File
	w *os.File
}

// started closes the parent's copy of the write end, so the read end ends
// when ffmpeg does.
func (p *packPipe) started() { _ = p.w.Close() }

func (p *packPipe) Close() error {
	_ = p.w.Close()
	return p.File.Close()
}

// packOutput is called before cmd.Start. startPack runs after Start succeeds.
func packOutput(cmd *exec.Cmd) (*packPipe, *playlistGate, chan struct{}, error) {
	p, err := newPackPipe(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	return p, newPlaylistGate(), make(chan struct{}), nil
}

func newPackPipe(cmd *exec.Cmd) (*packPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = w
	return &packPipe{File: r, w: w}, nil
}

func startPack(dir string, stdout *packPipe, gate *playlistGate, done chan struct{}, line func() int) *packInput {
	stdout.started()
	in := &packInput{cur: stdout, line: line}
	go func() {
		defer close(done)
		defer in.close()
		if err := Pack(dir, in, gate); err != nil && !os.IsNotExist(err) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, os.ErrClosed) {
			slog.Error(fmt.Sprintf("pack %s: %v", dir, err))
		}
	}()
	return in
}

// packInput is the output of each ffmpeg that has fed one packager, in turn.
// An encode started again after a timestamp break continues the same
// playlist, so a player sees a discontinuity rather than a new stream.
type packInput struct {
	mu   sync.Mutex
	cur  io.ReadCloser
	next []*readAhead
	done bool
	// offset is the broadcast time, in 90 kHz ticks, of fragment time zero
	// for the latest encode. known is true only while one encode has fed
	// the playlist; encodeAt has each encode's own.
	offset  int64
	known   bool
	encodes int
	startOK bool
	// line is the captions' timeline when an encode writes its first
	// segment. Nil without captions.
	line  func() int
	spans []encodeSpan
	// firstRead is when the first encode first wrote.
	firstRead stamp
}

// encodeSpan is one encode's place on the playlist: the sequence number of
// its first segment, its broadcast offset, and the captions' timeline.
type encodeSpan struct {
	seq    int
	offset int64
	line   int
}

// spanKeep bounds the encodes remembered. At most breakLimit start a minute,
// 450 over the playlist's 90 minutes.
const spanKeep = 1024

// noteEncodeStart records where the next encode's fragment clock starts on
// the broadcast clock, in seconds; ok is false when its header did not say.
func (in *packInput) noteEncodeStart(first float64, ok bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.encodes++
	in.known = ok && in.encodes == 1
	in.startOK = ok
	in.offset = int64(math.Round(first*90000)) % ptsWrap
}

// noteFirstSegment is told the sequence number of the segment the latest
// encode's first fragment opens.
// A second backward break within respawnGap can move the captions on before
// a queued encode writes, and that encode's segments then get no cues.
func (in *packInput) noteFirstSegment(seq int) {
	// line never changes; it takes the caption track's lock, not this one.
	n := 0
	if in.line != nil {
		n = in.line()
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if !in.startOK {
		// Its segments cannot be placed on the broadcast clock.
		n = -1
	}
	in.spans = append(in.spans, encodeSpan{seq: seq, offset: in.offset, line: n})
	if len(in.spans) > spanKeep {
		in.spans = append(in.spans[:0], in.spans[len(in.spans)-spanKeep:]...)
	}
}

// encodeAt is the encode that wrote segment seq.
func (in *packInput) encodeAt(seq int) (encodeSpan, bool) {
	if in == nil {
		return encodeSpan{}, false
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	for i := len(in.spans) - 1; i >= 0; i-- {
		if in.spans[i].seq <= seq {
			return in.spans[i], in.spans[i].line >= 0
		}
	}
	return encodeSpan{}, false
}

// broadcastOffset is what to add to a segment's time to get the broadcast
// timestamp of its first picture.
func (in *packInput) broadcastOffset() (int64, bool) {
	if in == nil {
		return 0, false
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.offset, in.known
}

// encodeStarts is the packager's reader when a hub owns it.
type encodeStarts interface {
	noteEncodeStart(first float64, ok bool)
	noteFirstSegment(seq int)
}

// errNextEncode is where one encode's output ends and the next one's begins.
var errNextEncode = errors.New("next encode")

func (in *packInput) Read(b []byte) (int, error) {
	in.mu.Lock()
	cur := in.cur
	in.mu.Unlock()
	n, err := cur.Read(b)
	if n > 0 {
		in.firstRead.mark()
	}
	if err == nil || n > 0 {
		return n, nil
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if len(in.next) == 0 {
		return 0, err
	}
	_ = cur.Close()
	in.cur, in.next = in.next[0], in.next[1:]
	return 0, errNextEncode
}

// follow queues the next encode's output. False means the packager has
// already stopped reading.
func (in *packInput) follow(p *packPipe) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.done {
		return false
	}
	p.started()
	in.next = append(in.next, newReadAhead(p))
	return true
}

// readAheadCap is how much of the next encode's output is held while the
// previous encode finishes. About a minute of a broadcast picture.
const readAheadCap = 64 << 20

// readAhead reads an encode's output from the moment it starts. Until the
// packager gets to it, ffmpeg would otherwise block on a full pipe and stop
// reading the tuner.
type readAhead struct {
	src  *packPipe
	mu   sync.Mutex
	cond *sync.Cond
	buf  []byte
	err  error
}

func newReadAhead(p *packPipe) *readAhead {
	r := &readAhead{src: p}
	r.cond = sync.NewCond(&r.mu)
	go func() {
		chunk := make([]byte, 64<<10)
		for {
			n, err := p.Read(chunk)
			r.mu.Lock()
			if n > 0 && len(r.buf)+n <= readAheadCap {
				r.buf = append(r.buf, chunk[:n]...)
			} else if n > 0 {
				err = fmt.Errorf("pack: the next encode is %d MB ahead of the packager", readAheadCap>>20)
			}
			if err != nil {
				r.err = err
			}
			r.cond.Broadcast()
			r.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return r
}

func (r *readAhead) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.buf) == 0 && r.err == nil {
		r.cond.Wait()
	}
	if len(r.buf) == 0 {
		return 0, r.err
	}
	n := copy(b, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *readAhead) Close() error { return r.src.Close() }

func (in *packInput) close() {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.done = true
	_ = in.cur.Close()
	for _, p := range in.next {
		_ = p.Close()
	}
	in.next = nil
}

// Pack turns an fMP4 fragment stream into init.mp4 and one segment per
// source group of pictures. hls.js will not fetch a part until its buffer is
// already at the live edge, so the first picture waits for a closed segment.
// A segment closes at the end of the keyframe fragment that brings it to half
// a second. Until then its fragments are parts.
func Pack(dir string, r io.Reader, gate *playlistGate) error {
	var init []byte
	var shifts map[uint32]int64
	var scales map[uint32]uint32
	leadIn := true
	var track uint32
	var scale uint32
	var haveInit bool
	// reinit is the header of a later encode on the same playlist. Its edit
	// lists are that encode's own track starts.
	var reinit []byte
	var moof []byte
	var open []packedPart
	var closed []packedSeg
	var partN int
	var msn int
	var segGap bool
	var lastPTS, lastDur int64
	var haveLast bool
	// handover is set at an encode boundary. The next fragment starts a new
	// timeline even when its step looks like one more group of pictures.
	var handover bool
	// fresh is set by each encode's header. That encode's first fragment
	// opens a segment, and the reader learns its sequence number.
	var fresh bool
	allSync := true

	var hold playlistCeiling
	flush := func() error {
		return writePacked(dir, init, closed, open, msn-len(closed), allSync, segGap, &hold, gate)
	}
	closeSeg := func(end int64) error {
		if len(open) == 0 {
			return nil
		}
		name := fmt.Sprintf("seg%05d.m4s", msn)
		var body []byte
		names := make([]partRef, len(open))
		for i, p := range open {
			body = append(body, p.body...)
			names[i] = partRef{name: p.name, dur: p.dur, sync: p.sync}
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			return err
		}
		dur := ptsDiff(end, open[0].pts)
		if dur <= 0 {
			dur = partTicks
		}
		closed = append(closed, packedSeg{name: name, pts: open[0].pts, dur: dur, parts: names, gap: segGap})
		segGap = false
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
				_ = os.Remove(filepath.Join(dir, n.name))
			}
		}
		// The next playlist's target counts this segment, and lists parts
		// by it.
		target := max(hold.target, int(dur))
		var fromEnd int64
		for i := len(closed) - 1; i >= 0; i-- {
			if _, keep := partsKept(fromEnd, target); !keep {
				for _, n := range closed[i].parts {
					_ = os.Remove(filepath.Join(dir, n.name))
				}
				closed[i].parts = nil
				if i > 0 && closed[i-1].parts == nil {
					break
				}
			}
			fromEnd += closed[i].dur
		}
		return nil
	}
	place := func(frag []byte, pts, dur int64, sync bool) error {
		jumped := false
		if haveLast {
			expected := lastPTS
			if lastDur > 0 {
				expected = lastPTS + lastDur
			}
			d := ptsDiff(pts, expected)
			if d < -int64(partTicks) || d > int64(jumpTicks) || handover {
				jumped = true
				slog.Warn(fmt.Sprintf("pack %s: timestamp jump %.3fs", dir, float64(d)/90000))
				if len(open) > 0 {
					if err := closeSeg(expected); err != nil {
						return err
					}
				}
				segGap = true
			}
		}
		handover = false
		if fresh {
			fresh = false
			// A later encode's first fragment always jumped, so the open
			// segment is closed and this fragment is the next one's first.
			if n, ok := r.(encodeStarts); ok && len(open) == 0 {
				n.noteFirstSegment(msn)
			}
		}
		if !jumped && len(open) > 0 {
			if open[len(open)-1].dur == 0 {
				step := ptsDiff(pts, open[len(open)-1].pts)
				if step > 0 {
					open[len(open)-1].dur = step
				}
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
		// A run with no keyframe, or a jump the step check missed, must not
		// keep every fragment. Close on the previous fragment's own end.
		if len(open) > 0 && openBytes(open)+len(frag) > maxOpenBytes {
			end := open[len(open)-1].pts + int64(partTicks)
			if open[len(open)-1].dur > 0 {
				end = open[len(open)-1].pts + open[len(open)-1].dur
			}
			if err := closeSeg(end); err != nil {
				return err
			}
		}
		if len(open) == 0 && !sync {
			allSync = false
		}
		lastPTS, lastDur, haveLast = pts, dur, true
		name := fmt.Sprintf("part%05d.m4s", partN)
		partN++
		if err := os.WriteFile(filepath.Join(dir, name), frag, 0o644); err != nil {
			return err
		}
		open = append(open, packedPart{name: name, pts: pts, dur: dur, body: frag, sync: sync})
		// Fragments are cut at keyframes, so this one ends where the next
		// group starts. Closing there now, once the segment is long enough,
		// is the cut the next group would make, a group sooner: a source that
		// keys twice in a few frames gives a short fragment and then the rest
		// of the group, and the segment waited a whole group for nothing.
		if sync && dur > 0 && ptsDiff(pts+dur, open[0].pts) >= int64(partTicks) {
			if err := closeSeg(pts + dur); err != nil {
				return err
			}
		}
		return flush()
	}
	// short is a keyframe fragment under minPartTicks. It goes out in front
	// of the next fragment, in one part, with the encode marks it came with.
	type held struct {
		body            []byte
		pts, dur        int64
		fresh, handover bool
	}
	var short *held
	placeShort := func() error {
		if short == nil {
			return nil
		}
		s := short
		short = nil
		f, h := fresh, handover
		fresh, handover = s.fresh, s.handover
		err := place(s.body, s.pts, s.dur, true)
		fresh, handover = f, h
		return err
	}
	take := func(frag []byte) error {
		if !haveInit {
			return nil
		}
		pts, ok := fragmentStart(frag, track, scale)
		if !ok {
			return nil
		}
		dur := int64(0)
		if d, known := fragmentDuration(frag, track, scale); known {
			dur = d
		}
		sync := fragmentIndependent(frag, track)
		if short != nil {
			d := ptsDiff(pts, short.pts+short.dur)
			if fresh || handover || d < -int64(partTicks) || d > int64(jumpTicks) {
				if err := placeShort(); err != nil {
					return err
				}
			} else {
				frag = append(short.body, frag...)
				if dur > 0 {
					dur += ptsDiff(pts, short.pts)
				}
				pts, sync = short.pts, true
				fresh, handover = short.fresh, short.handover
				short = nil
			}
		}
		if sync && dur > 0 && dur < minPartTicks {
			short = &held{body: frag, pts: pts, dur: dur, fresh: fresh, handover: handover}
			fresh, handover = false, false
			return nil
		}
		return place(frag, pts, dur, sync)
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
				shiftTracks(frag, shifts)
				if _, ok := fragmentPTS(frag, track, scale); leadIn && ok {
					frag = trimLeadIn(frag, track, scales)
					leadIn = false
				}
				if err := take(frag); err != nil {
					return err
				}
			default:
				if moof == nil && haveInit && (kind == "ftyp" || reinit != nil) {
					reinit = append(reinit, box...)
					if kind == "moov" {
						next, nextShifts, first := flattenEdits(reinit)
						if n, ok := r.(encodeStarts); ok {
							n.noteEncodeStart(first, nextShifts != nil)
						}
						fresh = true
						reinit = nil
						// Players keep init.mp4. A new encode that describes its
						// streams differently needs a new playlist.
						if !bytes.Equal(next, init) {
							return fmt.Errorf("pack: the next encode's header differs")
						}
						shifts = nextShifts
						scales = trackScales(next)
						leadIn = true
					}
				}
				if moof == nil && !haveInit {
					init = append(init, box...)
					if kind == "moov" {
						var first float64
						init, shifts, first = flattenEdits(init)
						if n, ok := r.(encodeStarts); ok {
							n.noteEncodeStart(first, shifts != nil)
						}
						fresh = true
						id, sc, ok := videoTrack(init)
						if !ok {
							return fmt.Errorf("pack: no video track")
						}
						track, scale = id, sc
						scales = trackScales(init)
						if err := os.WriteFile(filepath.Join(dir, "init.mp4"), init, 0o644); err != nil {
							return err
						}
						haveInit = true
					}
				}
			}
		}
		if len(buf) > maxBox {
			return fmt.Errorf("pack: fragment larger than %d bytes", maxBox)
		}
		if errors.Is(err, errNextEncode) {
			buf, moof, reinit = buf[:0], nil, nil
			handover = true
			continue
		}
		if err != nil {
			if err == io.EOF {
				if err := placeShort(); err != nil {
					return err
				}
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

func openBytes(open []packedPart) int {
	n := 0
	for _, p := range open {
		n += len(p.body)
	}
	return n
}

// playlistCeiling is the longest target this rendition has advertised.
// AVPlayer drops the stream on a reload that changes TARGETDURATION or
// PART-TARGET (CoreMedia -12642), so both stick.
type playlistCeiling struct {
	target int
	part   int
}

// longGroups holds each playlist folder (one channel and rendition) whose
// station has sent a group of pictures past longGroup.
var longGroups sync.Map

func writePacked(dir string, init []byte, closed []packedSeg, open []packedPart, origin int, allSync, segGap bool, hold *playlistCeiling, gate *playlistGate) error {
	if len(init) == 0 {
		return nil
	}
	longest := partTicks
	longestPart := partTicks
	for _, s := range closed {
		longest = max(longest, int(s.dur))
	}
	for _, p := range open {
		longest = max(longest, int(p.dur))
		longestPart = max(longestPart, int(p.dur))
	}
	// A part can outlast every closed segment. Two seconds covers the
	// one-second segments this packager writes.
	const floor = 2 * 90000
	target := max(longest, floor)
	if longest > longGroup {
		longGroups.Store(dir, true)
	}
	if hold != nil {
		// A broadcast's groups of pictures vary: 2.4 s with 3.3 s now and
		// then on one station, and an encode's first group can be cut short.
		// A station whose groups run past longGroup gets four, so the
		// target is set once.
		if hold.target == 0 && len(closed)+len(open) > 0 {
			if _, long := longGroups.Load(dir); long {
				target = max(target, 4*90000)
			}
		}
		target = max(target, hold.target)
		if len(closed)+len(open) > 0 {
			hold.target = target
		}
	}
	targetSec := max((target+89999)/90000, 1)
	// One millisecond under the advertised target, so it moves only when
	// TARGETDURATION does, unless a part is already longer.
	partTarget := min(max(longestPart, targetSec*90000-90), targetSec*90000)
	if hold != nil {
		partTarget = max(partTarget, hold.part)
		hold.part = partTarget
	}
	var b []byte
	// Parts, part-inf, and server-control require version 9.
	b = append(b, "#EXTM3U\n#EXT-X-VERSION:9\n"...)
	b = append(b, "#EXT-X-TARGETDURATION:"+strconv.Itoa(targetSec)+"\n"...)
	// Part hold-back is three part targets. Full hold-back is three target
	// durations; the part value has to stay under it.
	partHold := 3 * float64(partTarget) / 90000
	if minHold := float64(targetSec); partHold < minHold {
		partHold = minHold
	}
	fullHold := float64(targetSec * 3)
	if partHold >= fullHold {
		partHold = fullHold - 0.001
	}
	b = append(b, fmt.Sprintf("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,HOLD-BACK=%.3f,PART-HOLD-BACK=%.3f,CAN-SKIP-UNTIL=%d.000\n", fullHold, partHold, targetSec*6)...)
	b = append(b, "#EXT-X-PART-INF:PART-TARGET="+fmtDur(int64(partTarget))+"\n"...)
	if allSync {
		b = append(b, "#EXT-X-INDEPENDENT-SEGMENTS\n"...)
	}
	b = append(b, "#EXT-X-MEDIA-SEQUENCE:"+strconv.Itoa(origin)+"\n"...)
	b = append(b, "#EXT-X-MAP:URI=\"init.mp4\"\n"...)
	// A recent segment keeps its parts in the list. A player that was
	// playing them finds where they went when the segment closes.
	var openDur int64
	for _, p := range open {
		openDur += p.dur
	}
	fromEnd := make([]int64, len(closed))
	end := openDur
	for i := len(closed) - 1; i >= 0; i-- {
		fromEnd[i] = end
		end += closed[i].dur
	}
	partLine := func(name string, dur int64, sync bool) {
		if dur <= 0 {
			dur = partTicks
		}
		line := "#EXT-X-PART:DURATION=" + fmtDur(dur)
		if sync {
			line += ",INDEPENDENT=YES"
		}
		b = append(b, line+",URI=\""+name+"\"\n"...)
	}
	for i, s := range closed {
		if s.gap {
			b = append(b, "#EXT-X-DISCONTINUITY\n"...)
		}
		if listed, _ := partsKept(fromEnd[i], target); listed {
			for _, p := range s.parts {
				partLine(p.name, p.dur, p.sync)
			}
		}
		b = append(b, "#EXTINF:"+fmtDur(s.dur)+",\n"+s.name+"\n"...)
	}
	if segGap && len(open) > 0 {
		b = append(b, "#EXT-X-DISCONTINUITY\n"...)
	}
	for _, p := range open {
		partLine(p.name, p.dur, p.sync)
	}
	tmp := filepath.Join(dir, "index.m3u8.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, "index.m3u8")); err != nil {
		return err
	}
	if gate != nil {
		gate.target.Store(int64(targetSec) * int64(time.Second))
		recent := make([]int, 0, 4)
		for _, c := range closed[max(0, len(closed)-4):] {
			recent = append(recent, len(c.parts))
		}
		gate.publish(origin, origin+len(closed), len(open), recent...)
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
// keyframe. Missing flags are not treated as one: closing on a guess would
// cut a copy and a transcode at different frames.
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
			return false
		}
		return flags&0x10000 == 0
	}
	return false
}

func firstSampleFlags(tfhd, trun []byte) (uint32, bool) {
	if len(trun) >= 8 {
		trFlags := uint32(trun[1])<<16 | uint32(trun[2])<<8 | uint32(trun[3])
		off := 8
		if trFlags&0x1 != 0 {
			off += 4
		}
		if trFlags&0x4 != 0 {
			if off+4 <= len(trun) {
				return binary.BigEndian.Uint32(trun[off : off+4]), true
			}
		} else if trFlags&0x400 != 0 && binary.BigEndian.Uint32(trun[4:8]) > 0 {
			// The first sample's own flags, after its duration and size.
			if trFlags&0x100 != 0 {
				off += 4
			}
			if trFlags&0x200 != 0 {
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
