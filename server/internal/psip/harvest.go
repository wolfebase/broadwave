package psip

import "bytes"

// Harvester collects the guide packets from a live mux and parses them when
// a new window of those packets has arrived.
type Harvester struct {
	buf      bytes.Buffer
	reported int
}

// Add copies transport packets that can carry the guide. It returns a guide
// after each new 32 KB of those packets.
func (h *Harvester) Add(chunk []byte) (Guide, bool) {
	for i := 0; i+188 <= len(chunk); i += 188 {
		if chunk[i] != 0x47 {
			continue
		}
		pid := int(chunk[i+1]&0x1F)<<8 | int(chunk[i+2])
		if pid != pidBase && (pid < 0x1300 || pid > 0x1500) {
			continue
		}
		_, _ = h.buf.Write(chunk[i : i+188])
	}
	if h.buf.Len() < h.reported+32*1024 {
		return Guide{}, false
	}
	g, err := Parse(bytes.NewReader(h.buf.Bytes()))
	h.reported = h.buf.Len()
	if h.buf.Len() > 256*1024 {
		h.buf.Next(h.buf.Len() - 128*1024)
		h.reported = h.buf.Len()
	}
	if err != nil || len(g.Events) == 0 {
		return Guide{}, false
	}
	return g, true
}
