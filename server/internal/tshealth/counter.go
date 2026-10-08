// Package tshealth counts MPEG-TS damage as the bytes of a recording are written.
// A Counter is an io.Writer: writes may split a packet, and the count is what
// has been confirmed so far. Sync is acquired on three 0x47 bytes 188 apart,
// and one lost lock is one event until the next lock.
package tshealth

// A PCR that moves ahead by more than gapTicks (90 kHz) with no discontinuity
// flag, across packets lost on the way, is time the tuner never delivered:
// the signal dropped. A jump past maxGapTicks is a damaged clock, not a gap.
const (
	gapTicks    = 2 * 90000
	maxGapTicks = 10 * 60 * 90000
)

// Packet is the MPEG-TS packet size.
const Packet = 188

const (
	syncByte = 0x47
	nullPID  = 0x1FFF
)

// Summary is the damage counted so far.
// WorstPID is the PID with the most continuity errors, then the most
// transport-error packets, then the lowest PID. WorstContinuity and
// WorstTransport are both zero when every PID was clean.
type Summary struct {
	ContinuityErrors int
	TransportErrors  int
	SyncLosses       int
	Packets          int
	WorstPID         uint16
	WorstContinuity  int
	WorstTransport   int
	// Gaps are the times the stream stopped and LostSeconds how long in all,
	// read from the clock (PCR) of the PID that carries it most.
	Gaps        int
	LostSeconds float64
}

type pidStat struct {
	packets int
	ccErr   int
	tei     int
	have    bool
	cc      int
	dup     bool // one repeat of cc is allowed; a second is an error
	pcrs    int
	pcr     uint64
	gaps    int
	lost    uint64
	// lostAt is the counter's continuity errors at this PID's last PCR.
	lostAt int
}

// Counter counts continuity-counter errors, transport-error packets, and lost
// sync. Null packets, packets with no payload, and a packet that sets the
// discontinuity indicator do not produce a continuity error.
type Counter struct {
	buf       []byte
	synced    bool
	notedLoss bool
	syncLoss  int
	pids      map[uint16]*pidStat
	// ccErrs counts continuity errors on every PID, for the clock's gaps.
	ccErrs int
}

// Write implements io.Writer. It always accepts the bytes.
func (c *Counter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.buf = append(c.buf, p...)
	c.drain()
	if cap(c.buf) > 64<<10 && len(c.buf) < Packet*4 {
		c.buf = append([]byte(nil), c.buf...)
	}
	return len(p), nil
}

// Summary returns the totals and the worst PID.
func (c *Counter) Summary() Summary {
	var s Summary
	s.SyncLosses = c.syncLoss
	clock, clockPID := 0, uint16(0)
	for pid, st := range c.pids {
		s.Packets += st.packets
		s.ContinuityErrors += st.ccErr
		s.TransportErrors += st.tei
		// The PID whose clock ticked most tells; a tie goes to the lower PID.
		if st.pcrs > clock || (st.pcrs == clock && st.pcrs > 0 && pid < clockPID) {
			clock, clockPID = st.pcrs, pid
			s.Gaps, s.LostSeconds = st.gaps, float64(st.lost)/90000
		}
		if st.ccErr == 0 && st.tei == 0 {
			continue
		}
		if worse(pid, st, s) {
			s.WorstPID = pid
			s.WorstContinuity = st.ccErr
			s.WorstTransport = st.tei
		}
	}
	return s
}

// worse reports whether pid should replace the worst PID chosen so far.
// Continuity errors rank above transport errors. An equal pair keeps the lower PID.
func worse(pid uint16, st *pidStat, s Summary) bool {
	if s.WorstContinuity == 0 && s.WorstTransport == 0 {
		return true
	}
	if st.ccErr != s.WorstContinuity {
		return st.ccErr > s.WorstContinuity
	}
	if st.tei != s.WorstTransport {
		return st.tei > s.WorstTransport
	}
	return pid < s.WorstPID
}

func (c *Counter) drain() {
	for {
		if !c.synced {
			if !c.acquire() {
				return
			}
		}
		if len(c.buf) < Packet {
			return
		}
		if c.buf[0] != syncByte {
			if !c.notedLoss {
				c.syncLoss++
				c.notedLoss = true
			}
			c.synced = false
			continue
		}
		c.packet(c.buf[:Packet])
		c.buf = c.buf[Packet:]
		c.notedLoss = false
	}
}

// acquire locks when three sync bytes sit 188 apart. Bytes before that lock
// are one lost sync, unless the loss was already counted on the way out of
// a lock. A start whose third sync byte is not buffered yet is kept.
func (c *Counter) acquire() bool {
	const tail = Packet * 2
	if len(c.buf) < tail+1 {
		return false
	}
	limit := len(c.buf) - tail
	for i := 0; i < limit; i++ {
		if c.buf[i] == syncByte && c.buf[i+Packet] == syncByte && c.buf[i+Packet*2] == syncByte {
			if i > 0 && !c.notedLoss {
				c.syncLoss++
			}
			c.notedLoss = false
			if i > 0 {
				c.buf = c.buf[i:]
			}
			c.synced = true
			return true
		}
	}
	if limit > 0 {
		c.buf = append([]byte(nil), c.buf[limit:]...)
	}
	return false
}

func (c *Counter) packet(pkt []byte) {
	if c.pids == nil {
		c.pids = make(map[uint16]*pidStat)
	}
	pid := uint16(pkt[1]&0x1f)<<8 | uint16(pkt[2])
	st := c.pids[pid]
	if st == nil {
		st = &pidStat{}
		c.pids[pid] = st
	}
	st.packets++
	damaged := pkt[1]&0x80 != 0
	if damaged {
		st.tei++
	}
	if pid == nullPID {
		return
	}
	afc := pkt[3] >> 4 & 0x3
	payload := afc&1 == 1
	disc := false
	if afc&2 == 2 && pkt[4] > 0 {
		disc = pkt[5]&0x80 != 0
		// A packet flagged in error carries a clock no one can trust.
		if pkt[4] >= 7 && pkt[5]&0x10 != 0 && !damaged {
			c.clock(st, pkt[6:11], disc)
		}
	}
	if !payload {
		if disc {
			st.have = false
			st.dup = false
		}
		return
	}
	cc := int(pkt[3] & 0x0f)
	if disc || !st.have {
		st.have = true
		st.cc = cc
		st.dup = false
		return
	}
	if cc == st.cc {
		// The stream may repeat one packet. The next one still follows the first.
		if st.dup {
			st.ccErr++
			c.ccErrs++
			return
		}
		st.dup = true
		return
	}
	if cc != (st.cc+1)&0x0f {
		st.ccErr++
		c.ccErrs++
	}
	st.cc = cc
	st.dup = false
}

// clock reads a PCR base and counts a jump ahead as lost time when packets
// went missing since the last one. A jump back, a jump with nothing lost (a
// splice that forgot its flag), and one too long to be a dropout are not.
func (c *Counter) clock(st *pidStat, b []byte, disc bool) {
	pcr := uint64(b[0])<<25 | uint64(b[1])<<17 | uint64(b[2])<<9 | uint64(b[3])<<1 | uint64(b[4])>>7
	if st.pcrs > 0 && !disc && c.ccErrs > st.lostAt {
		ahead := (pcr - st.pcr) & (1<<33 - 1)
		if ahead > gapTicks && ahead <= maxGapTicks {
			st.gaps++
			st.lost += ahead
		}
	}
	st.pcrs++
	st.pcr = pcr
	st.lostAt = c.ccErrs
}
