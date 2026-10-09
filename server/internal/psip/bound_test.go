package psip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
)

// A short extended text table has a legal CRC and a length that used to
// make the text slice run backwards.
func TestShortETTDoesNotPanic(t *testing.T) {
	for _, n := range []int{14, 15, 16} {
		g, err := Parse(bytes.NewReader(ettPacket(n)))
		if err != nil {
			t.Fatalf("len %d: %v", n, err)
		}
		if len(g.Texts) != 0 {
			t.Fatalf("len %d parsed a text %+v", n, g.Texts)
		}
	}
}

// Parse stops reading once it has maxCapture bytes.
func TestParseStopsAtTheCaptureCap(t *testing.T) {
	r := &stopReader{max: maxCapture + 100}
	if _, err := Parse(r); err != nil {
		t.Fatal(err)
	}
	if r.n > maxCapture {
		t.Fatalf("read %d, cap %d", r.n, maxCapture)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(ettPacket(14))
	f.Add(ettPacket(16))
	if raw, err := os.ReadFile("testdata/kbwv.ts"); err == nil && len(raw) > 0 {
		if len(raw) > 4096 {
			raw = raw[:4096]
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			data = data[:64<<10]
		}
		_, _ = Parse(bytes.NewReader(data))
	})
}

func ettPacket(n int) []byte {
	sec := make([]byte, n)
	sec[0] = tableETT
	seclen := n - 3
	sec[1] = byte((seclen >> 8) & 0x0F)
	sec[2] = byte(seclen)
	binary.BigEndian.PutUint32(sec[n-4:], mpegCRC(sec[:n-4]))
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = 0x40 | byte((pidBase>>8)&0x1F)
	pkt[2] = byte(pidBase & 0xFF)
	pkt[3] = 0x10
	copy(pkt[5:], sec)
	return pkt
}

// stopReader returns zeros, then an error once max bytes have been read.
type stopReader struct {
	n, max int
}

func (r *stopReader) Read(p []byte) (int, error) {
	if r.n >= r.max {
		return 0, fmt.Errorf("read past the cap")
	}
	n := r.max - r.n
	if n > len(p) {
		n = len(p)
	}
	r.n += n
	return n, nil
}
