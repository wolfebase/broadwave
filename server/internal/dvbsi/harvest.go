package dvbsi

import (
	"bytes"

	"broadwave/internal/psip"
)

// Harvester collects the SDT and EIT packets of a live mux and parses them
// when a new window of those packets has arrived.
type Harvester struct {
	buf      bytes.Buffer
	reported int
	// since counts the mux bytes since the last parse. A station that sends
	// its listings slowly is still read every few seconds.
	since int
	// rest is the start of a packet a read cut off.
	rest []byte
	// pat is the latest packet that starts a PAT. It is not kept in the
	// window: it repeats ten times a second.
	pat []byte
}

const (
	harvestStep = 32 << 10
	harvestSlow = 16 << 20
	// harvestKeep is whole packets, so the window still starts on one.
	harvestKeep  = 188 * 700
	harvestLimit = 188 * 1400
)

// Add copies the transport packets that carry the SDT and EIT, and the PAT. It returns a
// guide after each new 32 KB of them, or after 16 MB of the mux when any
// arrived, once the window holds a listing.
func (h *Harvester) Add(chunk []byte) (Guide, bool) {
	h.since += len(chunk)
	h.rest = psip.Packets(h.rest, chunk, func(pkt []byte) {
		pid := int(pkt[1]&0x1F)<<8 | int(pkt[2])
		switch {
		case pid == pidSDT || pid == pidEIT:
			_, _ = h.buf.Write(pkt)
		case pid == pidPAT && pkt[1]&0x40 != 0:
			h.pat = append(h.pat[:0], pkt...)
		}
	})
	fresh := h.buf.Len() - h.reported
	if fresh < harvestStep && (fresh == 0 || h.since < harvestSlow) {
		return Guide{}, false
	}
	g := Parse(h.buf.Bytes())
	if h.pat != nil {
		g.Programs = Parse(h.pat).Programs
	}
	h.reported = h.buf.Len()
	h.since = 0
	if h.buf.Len() > harvestLimit {
		h.buf.Next(h.buf.Len() - harvestKeep)
		h.reported = h.buf.Len()
	}
	if len(g.Events) == 0 {
		return Guide{}, false
	}
	return g, true
}
