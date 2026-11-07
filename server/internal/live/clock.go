package live

import (
	"io"
	"os"
	"sync"
)

const (
	// clockMaxStep is the longest step between two PCRs of a steady clock. A
	// broadcast sends one at least every 100 ms, so anything past this lost
	// time or changed clocks. ffmpeg closes up a large jump in its output, and
	// its seek reads timestamps, so the sum of segment times stops naming a
	// place it can seek to.
	clockMaxStep = 2 * 90000
	// clockMarkEvery is how often the clock notes which byte it was at.
	clockMarkEvery = 30 * 90000
)

type clockMark struct {
	off int64
	at  float64
}

// fileClock reads a recording's PCRs to tell whether its clock runs straight,
// and notes where in the file each stretch of it is. A recording whose clock
// doesn't, or one nothing is known about, has no place an encode can safely
// carry on from, so its encode is never stopped.
type fileClock struct {
	mu       sync.Mutex
	started  bool
	next     int64 // the file offset after the bytes in rest
	rest     []byte
	last     map[uint16]int64
	main     uint16
	have     bool
	elapsed  int64
	marked   int64
	base     float64
	marks    []clockMark
	broken   bool
	complete bool
	scanning bool
}

func newFileClock(base float64) *fileClock {
	return &fileClock{base: base, last: map[uint16]int64{}}
}

// feed reads bytes from offset off of the file. Bytes it has already read are
// passed over, so a copy that starts earlier than the last one picks up in step.
func (c *fileClock) feed(off int64, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken {
		return
	}
	if c.started {
		if off > c.next {
			c.broken = true
			return
		}
		if skip := c.next - off; skip > 0 {
			if skip >= int64(len(b)) {
				return
			}
			b = b[skip:]
			off = c.next
		}
	}
	at := off - int64(len(c.rest))
	data := b
	if len(c.rest) > 0 {
		data = append(c.rest, b...)
	}
	c.rest = nil
	if !c.started {
		align := syncAlign(data)
		if align < 0 {
			if len(data) > 1<<20 {
				c.broken = true
				return
			}
			c.rest = data
			return
		}
		data = data[align:]
		at += int64(align)
		c.started = true
	}
	for len(data) >= 188 {
		if data[0] != 0x47 {
			c.broken = true
			return
		}
		c.packet(at, data[:188])
		if c.broken {
			return
		}
		data = data[188:]
		at += 188
	}
	c.rest = append(c.rest[:0], data...)
	c.next = at + int64(len(data))
}

func (c *fileClock) packet(off int64, pkt []byte) {
	pcr, ok := readPCR(pkt)
	if !ok {
		return
	}
	pid := uint16(pkt[1]&0x1f)<<8 | uint16(pkt[2])
	last, seen := c.last[pid]
	c.last[pid] = pcr
	if !seen {
		if !c.have {
			c.have, c.main = true, pid
			c.marks = append(c.marks, clockMark{off: off, at: c.base})
		}
		return
	}
	step := (pcr - last) & (pcrMod - 1)
	if pkt[5]&0x80 != 0 || step > clockMaxStep {
		c.broken = true
		return
	}
	if pid != c.main {
		return
	}
	c.elapsed += step
	if c.elapsed-c.marked >= clockMarkEvery {
		c.marked = c.elapsed
		c.marks = append(c.marks, clockMark{off: off, at: c.base + float64(c.elapsed)/90000})
	}
}

// safe reports a clock an encode can carry on from: straight, all the way
// through a finished file, or in everything a follow copy has read so far.
func (c *fileClock) safe(follow bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.have && !c.broken && (follow || c.complete)
}

// end is how far the clock has been read, in seconds.
func (c *fileClock) end() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base + float64(c.elapsed)/90000
}

// readTo reports a clock that has read path to its end, not just as far as a
// follow copy got. Less than a packet can be left over.
func (c *fileClock) readTo(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.complete {
		return true
	}
	info, err := os.Stat(path)
	return err == nil && info.Size()-c.next < 188
}

// before is the last mark at or before at.
func (c *fileClock) before(at float64) (clockMark, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var found clockMark
	ok := false
	for _, m := range c.marks {
		if m.at > at {
			break
		}
		found, ok = m, true
	}
	return found, ok
}

// restartAt is for a follow copy that didn't reach its resume and starts over
// at the first byte.
func (c *fileClock) restartAt(base float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started, c.next, c.rest, c.last, c.main, c.have = false, 0, nil, map[uint16]int64{}, 0, false
	c.elapsed, c.marked, c.base, c.marks, c.broken, c.complete = 0, 0, base, nil, false, false
}

// scan reads the file from where the clock has got to. It stops when gone says
// nobody needs the answer, and takes up there next time.
func (c *fileClock) scan(path string, gone func() bool) {
	c.mu.Lock()
	if c.scanning || c.complete || c.broken {
		c.mu.Unlock()
		return
	}
	c.scanning = true
	off := c.next
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.scanning = false
		c.mu.Unlock()
	}()
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, 1<<20)
	for !gone() {
		n, err := f.ReadAt(buf, off)
		if n > 0 {
			c.feed(off, buf[:n])
			off += int64(n)
		}
		if err == io.EOF {
			c.mu.Lock()
			c.complete = true
			c.mu.Unlock()
			return
		}
		if err != nil {
			return
		}
	}
}
