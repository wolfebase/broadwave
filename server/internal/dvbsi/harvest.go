package dvbsi

import "bytes"

// Harvester collects the SDT and EIT packets of a live mux and parses them
// when a new window of those packets has arrived.
type Harvester struct {
	buf      bytes.Buffer
	reported int
	// since counts the mux bytes since the last parse. A station that sends
	// its listings slowly is still read every few seconds.
	since int
}

const (
	harvestStep = 32 << 10
	harvestSlow = 16 << 20
	// harvestKeep is whole packets, so the window still starts on one.
	harvestKeep  = 188 * 700
	harvestLimit = 188 * 1400
)

// Add copies the transport packets that carry the SDT and EIT. It returns a
// guide after each new 32 KB of them, or after 16 MB of the mux when any
// arrived, once the window holds a listing.
func (h *Harvester) Add(chunk []byte) (Guide, bool) {
	h.since += len(chunk)
	for i := 0; i+188 <= len(chunk); i += 188 {
		if chunk[i] != 0x47 {
			continue
		}
		pid := int(chunk[i+1]&0x1F)<<8 | int(chunk[i+2])
		if pid != pidSDT && pid != pidEIT {
			continue
		}
		_, _ = h.buf.Write(chunk[i : i+188])
	}
	fresh := h.buf.Len() - h.reported
	if fresh < harvestStep && (fresh == 0 || h.since < harvestSlow) {
		return Guide{}, false
	}
	g := Parse(h.buf.Bytes())
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
