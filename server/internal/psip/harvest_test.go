package psip

import (
	"os"
	"testing"
	"time"
)

func TestHarvesterReadsTheSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/kbwv.ts")
	if err != nil {
		t.Fatal(err)
	}
	var h Harvester
	var got Guide
	ok := false
	for len(raw) > 0 {
		n := 188 * 7
		if n > len(raw) {
			n = len(raw)
		}
		if g, ready := h.Add(raw[:n]); ready {
			got = g
			ok = true
		}
		raw = raw[n:]
	}
	if !ok {
		t.Fatal("harvester did not yield a guide")
	}
	if len(got.Events) == 0 || got.Offset != 18*time.Second {
		t.Fatalf("events %d offset %s", len(got.Events), got.Offset)
	}
}

// The window is trimmed once it passes 256 KB. It has to stay on packet
// boundaries, or nothing after the first trim parses and the station's
// listings stop arriving.
func TestHarvesterKeepsReadingAfterItsWindowIsTrimmed(t *testing.T) {
	raw, err := os.ReadFile("testdata/kbwv.ts")
	if err != nil {
		t.Fatal(err)
	}
	var h Harvester
	trims, after := 0, 0
	last := 0
	for pass := 0; pass < 40 && after < 3; pass++ {
		for chunk := raw; len(chunk) > 0; {
			n := min(188*7, len(chunk))
			_, ready := h.Add(chunk[:n])
			chunk = chunk[n:]
			if h.buf.Len() < last {
				trims++
			}
			last = h.buf.Len()
			if ready && trims > 0 {
				after++
			}
		}
	}
	if trims == 0 {
		t.Fatal("the window was never trimmed")
	}
	if after < 3 {
		t.Fatalf("%d guides after %d trims", after, trims)
	}
	if h.buf.Len()%188 != 0 {
		t.Fatalf("window of %d bytes is not whole packets", h.buf.Len())
	}
}

// A tuner read is any number of bytes. A packet split across two reads, and
// a stream that starts inside a packet, still parse.
func TestHarvesterReadsPacketsSplitAcrossReads(t *testing.T) {
	raw, err := os.ReadFile("testdata/kbwv.ts")
	if err != nil {
		t.Fatal(err)
	}
	stream := append([]byte{0x00, 0x47, 0x11, 0x47}, raw...)
	var h Harvester
	ok := false
	for len(stream) > 0 {
		n := min(1001, len(stream))
		if g, ready := h.Add(stream[:n]); ready && len(g.Events) > 0 && len(g.Channels) > 0 {
			ok = true
		}
		stream = stream[n:]
	}
	if !ok {
		t.Fatal("no guide from reads that split packets")
	}
}

func TestPacketsFindsTheSyncAgain(t *testing.T) {
	pkt := func(b byte) []byte {
		p := make([]byte, 188)
		p[0], p[1] = 0x47, b
		return p
	}
	var stream []byte
	stream = append(stream, 0x47, 0x00, 0x13) // a cut-off packet, then a false sync
	stream = append(stream, pkt(1)...)
	stream = append(stream, pkt(2)...)
	stream = append(stream, pkt(3)[:100]...)
	var got []byte
	rest := Packets(nil, stream, func(p []byte) { got = append(got, p[1]) })
	rest = Packets(rest, pkt(3)[100:], func(p []byte) { got = append(got, p[1]) })
	if string(got) != "\x01\x02\x03" || len(rest) != 0 {
		t.Fatalf("packets %v, rest %d", got, len(rest))
	}
}
