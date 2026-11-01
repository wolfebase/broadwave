package psip

import "bytes"

// Harvester collects the guide packets from a live mux and parses them when
// a new window of those packets has arrived.
type Harvester struct {
	buf      bytes.Buffer
	reported int
	// rest is the start of a packet a read cut off. A tuner read is any
	// number of bytes, not whole packets.
	rest []byte
}

// keepWindow is about 128 KB of whole packets: a window that starts inside a
// packet parses nothing.
const keepWindow = 128 * 1024 / 188 * 188

// Add copies transport packets that can carry the guide. It returns a guide
// after each new 32 KB of those packets.
func (h *Harvester) Add(chunk []byte) (Guide, bool) {
	h.rest = Packets(h.rest, chunk, func(pkt []byte) {
		pid := int(pkt[1]&0x1F)<<8 | int(pkt[2])
		if pid == pidBase || (pid >= 0x1300 && pid <= 0x1500) {
			_, _ = h.buf.Write(pkt)
		}
	})
	if h.buf.Len() < h.reported+32*1024 {
		return Guide{}, false
	}
	g, err := Parse(bytes.NewReader(h.buf.Bytes()))
	h.reported = h.buf.Len()
	if h.buf.Len() > 256*1024 {
		h.buf.Next(h.buf.Len() - keepWindow)
		h.reported = h.buf.Len()
	}
	if err != nil || len(g.Events) == 0 {
		return Guide{}, false
	}
	return g, true
}

// Packets calls fn with each whole 188-byte packet of rest followed by chunk,
// finding the sync byte again where the stream lost it, and returns the
// bytes of a packet the chunk cut off, for the next call.
func Packets(rest, chunk []byte, fn func(pkt []byte)) []byte {
	data := chunk
	if len(rest) > 0 {
		data = append(rest, chunk...)
	}
	i := 0
	for i+188 <= len(data) {
		if !synced(data, i) {
			i++
			continue
		}
		fn(data[i : i+188])
		i += 188
	}
	if i >= len(data) {
		return rest[:0]
	}
	return append(rest[:0:0], data[i:]...)
}

// synced reports a sync byte at i that the next packet, when there is one in
// data, confirms.
func synced(data []byte, i int) bool {
	if data[i] != 0x47 {
		return false
	}
	return i+188 >= len(data) || data[i+188] == 0x47
}
