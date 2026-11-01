package dvbsi

import (
	"testing"
	"time"
)

func carousel(c *carrier, s *carrier) []byte {
	raw := s.alone(sdtSection(0x42, 0x0101, 0x2001, sdtService(0x1001, []byte("Channel One HD"))))
	return append(raw, c.alone(eitSection(0x4E, 0x1001, 0x0101, 0x2001,
		event(1, evening, time.Hour, shortDesc("eng", []byte("Quiz Night"), nil)),
		event(2, evening.Add(time.Hour), time.Hour, shortDesc("eng", []byte("Harbour Lights"), nil)),
	))...)
}

func TestTheHarvesterReadsAWindow(t *testing.T) {
	eit, sdt := &carrier{pid: pidEIT}, &carrier{pid: pidSDT}
	var h Harvester
	var got Guide
	ready := 0
	for range 200 {
		chunk := append(carousel(eit, sdt), nullPackets(40)...)
		if g, ok := h.Add(chunk); ok {
			got = g
			ready++
		}
	}
	if ready == 0 {
		t.Fatal("no guide")
	}
	if len(got.Events) != 2 || len(got.Services) != 1 || got.Services[0].Name != "Channel One HD" {
		t.Fatalf("%+v", got)
	}
	if h.buf.Len() > harvestLimit || h.buf.Len()%188 != 0 {
		t.Fatalf("window %d bytes", h.buf.Len())
	}
	// The window was trimmed and still reads.
	if g, ok := h.Add(make([]byte, 0)); ok {
		t.Fatalf("an empty chunk gave %+v", g)
	}
}

// A station that sends its listings slowly is still read before 32 KB of them
// arrive, once 16 MB of the mux went by, and not again until more arrive.
func TestTheHarvesterReadsASlowStation(t *testing.T) {
	eit, sdt := &carrier{pid: pidEIT}, &carrier{pid: pidSDT}
	var h Harvester
	if _, ok := h.Add(carousel(eit, sdt)); ok {
		t.Fatal("parsed after one carousel")
	}
	filler := nullPackets(5000)
	sent, reads := 0, 0
	for sent < 3*harvestSlow {
		g, ok := h.Add(filler)
		sent += len(filler)
		if !ok {
			continue
		}
		reads++
		if sent < harvestSlow-len(filler) || len(g.Events) != 2 {
			t.Fatalf("read after %d bytes: %+v", sent, g)
		}
	}
	if reads != 1 {
		t.Fatalf("read %d times with one carousel", reads)
	}
}

// A tuner read is any number of bytes; a packet it cuts in two still counts.
func TestTheHarvesterJoinsPacketsSplitAcrossReads(t *testing.T) {
	eit, sdt := &carrier{pid: pidEIT}, &carrier{pid: pidSDT}
	var stream []byte
	for range 100 {
		stream = append(stream, carousel(eit, sdt)...)
	}
	stream = append([]byte{0x12, 0x47}, stream...)
	var h Harvester
	ok := false
	for len(stream) > 0 {
		n := min(777, len(stream))
		if g, ready := h.Add(stream[:n]); ready && len(g.Events) == 2 {
			ok = true
		}
		stream = stream[n:]
	}
	if !ok {
		t.Fatal("no guide from reads that split packets")
	}
}
