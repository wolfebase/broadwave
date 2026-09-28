// Package ring keeps the last stretch of one tuned multiplex on disk, so a
// recording can start from a show's beginning when the tuner was already on it.
package ring

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ErrGone is a read of bytes the ring no longer holds.
var ErrGone = errors.New("ring: bytes are no longer held")

const (
	// pendingCap is how far the disk may fall behind the tuner. Past it the
	// ring starts over rather than hold the multiplex in memory.
	pendingCap = 32 << 20
	// markEvery is the spacing of the time index.
	markEvery = time.Second
	// roomEvery is how often free space is checked.
	roomEvery = 10 * time.Second
	// resumeRoom is how far above its floor the disk must be before a ring
	// that stopped for space writes again.
	resumeRoom = 1 << 30
)

// Ring is safe for one writer (Append) and any number of readers. No file
// is created, closed, or removed while mu is held: Append runs in the
// tuner's read loop and must never wait on the disk.
type Ring struct {
	dir      string
	windowFn func() time.Duration
	span     time.Duration
	room     func(held int64) int64
	now      func() time.Time

	mu       sync.Mutex
	gen      int
	start    int64 // first position still held
	flushed  int64 // bytes [start, flushed) are on disk
	received int64 // bytes [flushed, received) are in pending
	pending  [][]byte
	queued   int
	segs     []*segment
	marks    []mark
	lastMark time.Time
	lastAt   time.Time // when the newest byte arrived
	window   time.Duration
	low      bool
	off      bool
	closed   bool
	wake     chan struct{}
	done     chan struct{}
}

type segment struct {
	path   string
	first  int64
	size   int64
	opened time.Time
}

type mark struct {
	at  time.Time
	pos int64
}

// Options configure a ring. Window is read when the ring starts and at each
// room check, so a new setting applies within roomEvery; zero turns the ring
// off and empties it. Span is the length of one file on disk. Room is how
// many more bytes the ring may hold, given what it holds now; below zero the
// ring gives up its oldest files, and it stops writing if that is not enough.
type Options struct {
	Window func() time.Duration
	Span   time.Duration
	Room   func(held int64) int64
	Now    func() time.Time
}

// Open starts a ring in dir, which it owns: the folder is emptied first and
// removed on Close. Open itself does not touch the disk.
func Open(dir string, opt Options) *Ring {
	if opt.Span <= 0 {
		opt.Span = time.Minute
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Window == nil {
		opt.Window = func() time.Duration { return 0 }
	}
	r := &Ring{dir: dir, windowFn: opt.Window, span: opt.Span, room: opt.Room, now: opt.Now,
		wake: make(chan struct{}, 1), done: make(chan struct{})}
	go r.flushLoop()
	return r
}

// Append adds the next bytes of the multiplex. It never waits on the disk.
func (r *Ring) Append(chunk []byte) {
	if r == nil || len(chunk) == 0 {
		return
	}
	r.mu.Lock()
	if r.closed || r.low || r.off || r.queued+len(chunk) > pendingCap {
		var drop []string
		if !r.closed && !r.low && !r.off {
			slog.Warn(fmt.Sprintf("ring: the disk fell %d MB behind the tuner; starting the buffer over", r.queued>>20))
			drop = r.resetLocked()
		}
		r.received += int64(len(chunk))
		r.flushed, r.start = r.received, r.received
		r.mu.Unlock()
		if len(drop) > 0 {
			go removeFiles(drop)
		}
		return
	}
	now := r.now()
	if r.lastMark.IsZero() || now.Sub(r.lastMark) >= markEvery {
		r.marks = append(r.marks, mark{at: now, pos: r.received})
		r.lastMark = now
	}
	r.lastAt = now
	r.pending = append(r.pending, chunk)
	r.queued += len(chunk)
	r.received += int64(len(chunk))
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Received is the position just past the last appended byte.
func (r *Ring) Received() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.received
}

// Since is the earliest time the ring holds, or zero when it holds nothing.
func (r *Ring) Since() time.Time {
	if r == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.marks {
		if m.pos >= r.start {
			return m.at
		}
	}
	return time.Time{}
}

// Position is the first indexed position at or after t, and its time. A time
// before the ring's start gives the oldest position held.
func (r *Ring) Position(t time.Time) (int64, time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, time.Time{}, false
	}
	for _, m := range r.marks {
		if m.pos < r.start || m.at.Before(t) {
			continue
		}
		return m.pos, m.at, true
	}
	return 0, time.Time{}, false
}

// At is the position of the byte that arrived at t, between the index
// marks around it; the multiplex arrives at a steady rate over one mark. A
// time past the newest byte is Received. It is not ok when t is before the
// oldest byte held.
func (r *Ring) At(t time.Time) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, false
	}
	// A paced reader asks many times a second, under the lock the tuner's
	// read loop needs, and a long window holds thousands of marks.
	lo := sort.Search(len(r.marks), func(i int) bool { return r.marks[i].pos >= r.start })
	a := lo + sort.Search(len(r.marks)-lo, func(i int) bool { return r.marks[lo+i].at.After(t) }) - 1
	if a < lo {
		return 0, false
	}
	from := r.marks[a]
	to := mark{at: r.lastAt, pos: r.received}
	if a+1 < len(r.marks) {
		to = r.marks[a+1]
	}
	if !t.Before(r.lastAt) {
		return r.received, true
	}
	span := to.at.Sub(from.at)
	if span <= 0 {
		return from.pos, true
	}
	return from.pos + int64(float64(to.pos-from.pos)*float64(t.Sub(from.at))/float64(span)), true
}

// PendingFrom copies [pos, Received) when all of it is still in memory. A
// caller holding a lock the read loop needs uses it instead of Copy, so it
// never waits on a file.
func (r *Ring) PendingFrom(pos int64) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || pos < r.flushed || pos > r.received {
		return nil, false
	}
	out := make([]byte, r.received-pos)
	copyPending(out, r.pending, pos-r.flushed)
	return out, true
}

// Copy writes bytes [from, to) to w, opening each file once.
func (r *Ring) Copy(w io.Writer, from, to int64) (int64, error) {
	return r.CopyBuffer(w, from, to, make([]byte, 1<<20))
}

// CopyBuffer is Copy through buf, for a reader that copies many times a
// second.
func (r *Ring) CopyBuffer(w io.Writer, from, to int64, buf []byte) (int64, error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return 0, ErrGone
	}
	var total int64
	for from < to {
		r.mu.Lock()
		if r.closed || from < r.start || from >= r.received {
			r.mu.Unlock()
			return total, ErrGone
		}
		if from >= r.flushed {
			n := copyPending(buf[:min(int64(len(buf)), to-from)], r.pending, from-r.flushed)
			r.mu.Unlock()
			if n == 0 {
				return total, ErrGone
			}
			wrote, err := w.Write(buf[:n])
			total += int64(wrote)
			from += int64(wrote)
			if err != nil {
				return total, err
			}
			continue
		}
		var seg segment
		found := false
		for _, s := range r.segs {
			if from >= s.first && from < s.first+s.size {
				seg, found = *s, true
				break
			}
		}
		r.mu.Unlock()
		if !found {
			return total, ErrGone
		}
		n, err := copyFile(w, seg.path, from-seg.first, min(seg.first+seg.size, to)-from, buf)
		total += n
		from += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// copyFile writes length bytes of path from off. A file the ring removed
// meanwhile is ErrGone; a failed write is the writer's error.
func copyFile(w io.Writer, path string, off, length int64, buf []byte) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ErrGone
	}
	defer f.Close()
	var total int64
	for total < length {
		n, err := f.ReadAt(buf[:min(int64(len(buf)), length-total)], off+total)
		if n > 0 {
			wrote, werr := w.Write(buf[:n])
			total += int64(wrote)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil && total < length {
			return total, ErrGone
		}
	}
	return total, nil
}

func copyPending(p []byte, pending [][]byte, skip int64) int {
	n := 0
	for _, chunk := range pending {
		if skip >= int64(len(chunk)) {
			skip -= int64(len(chunk))
			continue
		}
		n += copy(p[n:], chunk[skip:])
		skip = 0
		if n == len(p) {
			break
		}
	}
	return n
}

// Close stops the ring and removes its files.
func (r *Ring) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()
	close(r.done)
}

func (r *Ring) flushLoop() {
	var file *os.File
	defer func() {
		if file != nil {
			_ = file.Close()
		}
		r.mu.Lock()
		r.resetLocked()
		r.flushed, r.start = r.received, r.received
		r.mu.Unlock()
		_ = os.RemoveAll(r.dir)
	}()
	if err := os.RemoveAll(r.dir); err == nil {
		err = os.MkdirAll(r.dir, 0o755)
		if err != nil {
			r.stop(err)
		}
	} else {
		r.stop(err)
	}
	tick := time.NewTicker(roomEvery)
	defer tick.Stop()
	r.checkWindow()
	r.checkRoom()
	for {
		select {
		case <-r.done:
			return
		case <-r.wake:
		case <-tick.C:
			r.checkWindow()
			r.checkRoom()
		}
		for {
			more := false
			file, more = r.flushOnce(file)
			if !more {
				break
			}
		}
	}
}

// stop turns the ring off for good after a folder it cannot use.
func (r *Ring) stop(err error) {
	slog.Warn("ring: " + err.Error())
	r.mu.Lock()
	drop := r.resetLocked()
	r.flushed, r.start = r.received, r.received
	r.low = true
	r.mu.Unlock()
	removeFiles(drop)
	r.room = func(int64) int64 { return -1 }
}

// flushOnce writes what is pending to the current file, starting a new file
// when the current one spans its time. It returns the open file and whether
// more arrived meanwhile.
func (r *Ring) flushOnce(file *os.File) (*os.File, bool) {
	r.mu.Lock()
	if r.closed || len(r.pending) == 0 {
		r.mu.Unlock()
		return file, false
	}
	gen, batch, first, now := r.gen, r.pending, r.flushed, r.now()
	rotate := file == nil || len(r.segs) == 0 || now.Sub(r.segs[len(r.segs)-1].opened) >= r.span
	r.mu.Unlock()

	var seg *segment
	if rotate {
		if file != nil {
			_ = file.Close()
			file = nil
		}
		path := filepath.Join(r.dir, fmt.Sprintf("%016d.ts", first))
		f, err := os.Create(path)
		if err != nil {
			r.fail(gen, err)
			return nil, false
		}
		file = f
		seg = &segment{path: path, first: first, opened: now}
	}
	var n int64
	var werr error
	for _, chunk := range batch {
		w, err := file.Write(chunk)
		n += int64(w)
		if err != nil {
			werr = err
			break
		}
	}

	r.mu.Lock()
	if r.gen != gen {
		r.mu.Unlock()
		// A reset dropped these bytes; the file it did not know about goes too.
		if seg != nil {
			_ = file.Close()
			_ = os.Remove(seg.path)
			file = nil
		}
		return file, false
	}
	if seg != nil {
		r.segs = append(r.segs, seg)
	}
	if werr != nil {
		r.mu.Unlock()
		r.fail(gen, werr)
		_ = file.Close()
		return nil, false
	}
	r.segs[len(r.segs)-1].size += n
	r.flushed += n
	r.pending = r.pending[len(batch):]
	if len(r.pending) == 0 {
		r.pending = nil
	}
	r.queued -= int(n)
	drop := r.trimLocked(now)
	more := len(r.pending) > 0
	r.mu.Unlock()
	removeFiles(drop)
	return file, more
}

// fail empties the ring after a file error and stops it until the next
// room check, so a disk that keeps failing costs one attempt every roomEvery.
func (r *Ring) fail(gen int, err error) {
	slog.Warn("ring: " + err.Error() + "; the buffer is off for now")
	r.mu.Lock()
	var drop []string
	if r.gen == gen {
		drop = r.resetLocked()
		r.flushed, r.start = r.received, r.received
		r.low = true
	}
	r.mu.Unlock()
	removeFiles(drop)
}

// trimLocked forgets files that end before the window and returns their paths.
func (r *Ring) trimLocked(now time.Time) []string {
	var drop []string
	for len(r.segs) > 1 && r.window > 0 && now.Sub(r.segs[1].opened) >= r.window {
		drop = append(drop, r.dropOldestLocked())
	}
	r.trimMarksLocked()
	return drop
}

func (r *Ring) dropOldestLocked() string {
	old := r.segs[0]
	r.segs = r.segs[1:]
	r.start = old.first + old.size
	if len(r.segs) > 0 {
		r.start = r.segs[0].first
	}
	return old.path
}

func (r *Ring) trimMarksLocked() {
	i := 0
	for i < len(r.marks) && r.marks[i].pos < r.start {
		i++
	}
	if i > 0 {
		r.marks = append(r.marks[:0], r.marks[i:]...)
	}
}

// checkWindow applies the current window. Zero empties the ring and keeps it
// off until the window is set again.
func (r *Ring) checkWindow() {
	w := r.windowFn()
	r.mu.Lock()
	var drop []string
	r.window = w
	switch {
	case w <= 0 && !r.off:
		drop = r.resetLocked()
		r.flushed, r.start = r.received, r.received
		r.off = true
	case w > 0 && r.off:
		r.off = false
	}
	if w > 0 {
		drop = append(drop, r.trimLocked(r.now())...)
	}
	r.mu.Unlock()
	removeFiles(drop)
}

// Stats is what the ring holds now: bytes on disk, the earliest time held,
// and whether it is off by setting or paused for disk space.
type Stats struct {
	Bytes int64
	Since time.Time
	Off   bool
	Low   bool
}

// Stats reports what the ring holds.
func (r *Ring) Stats() Stats {
	if r == nil {
		return Stats{Off: true}
	}
	since := r.Since()
	r.mu.Lock()
	defer r.mu.Unlock()
	return Stats{Bytes: r.flushed - r.start, Since: since, Off: r.off, Low: r.low}
}

// checkRoom gives up the oldest files while the disk is under its floor. If
// the file being written is all that is left, the ring stops writing until
// the disk has resumeRoom again.
func (r *Ring) checkRoom() {
	if r.room == nil {
		return
	}
	r.mu.Lock()
	held := r.flushed - r.start
	r.mu.Unlock()
	room := r.room(held)
	r.mu.Lock()
	var drop []string
	switch {
	case room < 0 && !r.low:
		need := -room
		for len(r.segs) > 1 && need > 0 {
			need -= r.segs[0].size
			drop = append(drop, r.dropOldestLocked())
		}
		r.trimMarksLocked()
		if need > 0 {
			slog.Warn("ring: the disk is nearly full; the buffer is off until there is room")
			drop = append(drop, r.resetLocked()...)
			r.flushed, r.start = r.received, r.received
			r.low = true
		}
	case r.low && room >= resumeRoom:
		r.low = false
	}
	r.mu.Unlock()
	removeFiles(drop)
}

// resetLocked forgets every file, mark, and pending byte and returns the
// paths to remove. Positions keep counting; the caller moves start and
// flushed to received.
func (r *Ring) resetLocked() []string {
	r.gen++
	var drop []string
	for len(r.segs) > 0 {
		drop = append(drop, r.dropOldestLocked())
	}
	r.pending = nil
	r.queued = 0
	r.marks = nil
	r.lastMark = time.Time{}
	r.lastAt = time.Time{}
	return drop
}

func removeFiles(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}
