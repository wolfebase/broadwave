package tshealth

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCounter(t *testing.T) {
	const pid = 0x100
	tests := []struct {
		name string
		pkts [][]byte
		want Summary
	}{
		{
			name: "clean",
			pkts: series(pid, 0, 1, 2, 3, 4),
			want: Summary{Packets: 5},
		},
		{
			name: "one dropped packet",
			pkts: series(pid, 0, 1, 3, 4),
			want: Summary{Packets: 4, ContinuityErrors: 1, WorstPID: pid, WorstContinuity: 1},
		},
		{
			name: "one duplicate",
			pkts: series(pid, 0, 1, 1, 2, 3),
			want: Summary{Packets: 5},
		},
		{
			name: "second duplicate",
			pkts: series(pid, 0, 1, 1, 1, 2),
			want: Summary{Packets: 5, ContinuityErrors: 1, WorstPID: pid, WorstContinuity: 1},
		},
		{
			name: "null pid skips the counter",
			pkts: series(nullPID, 0, 5, 9, 1),
			want: Summary{Packets: 4},
		},
		{
			name: "no payload",
			pkts: join(series(pid, 0, 1), one(adaptOnly(pid, 14, false)), series(pid, 2, 3)),
			want: Summary{Packets: 5},
		},
		{
			name: "discontinuity",
			pkts: join(series(pid, 0, 1), one(packet(pid, 9, true, true, false)), series(pid, 10, 11)),
			want: Summary{Packets: 5},
		},
		{
			name: "jump after discontinuity",
			pkts: join(series(pid, 0, 1), one(packet(pid, 9, true, true, false)), series(pid, 11, 12)),
			want: Summary{Packets: 5, ContinuityErrors: 1, WorstPID: pid, WorstContinuity: 1},
		},
		{
			name: "discontinuity without payload",
			pkts: join(series(pid, 0, 1), one(adaptOnly(pid, 4, true)), series(pid, 9, 10)),
			want: Summary{Packets: 5},
		},
		{
			name: "transport error",
			pkts: join(series(pid, 0, 1), one(withTEI(packet(pid, 2, true, false, false))), series(pid, 3, 4)),
			want: Summary{Packets: 5, TransportErrors: 1, WorstPID: pid, WorstTransport: 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c Counter
			n, err := c.Write(bytes.Join(tc.pkts, nil))
			if err != nil || n != Packet*len(tc.pkts) {
				t.Fatalf("write %d %v", n, err)
			}
			if got := c.Summary(); got != tc.want {
				t.Fatalf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestSplitWriteMatchesOneWrite(t *testing.T) {
	body := bytes.Join(series(0x100, 0, 1, 2, 3, 4, 5), nil)
	var one Counter
	if _, err := one.Write(body); err != nil {
		t.Fatal(err)
	}
	cuts := []int{1, 50, 100, 187, 188, 189, 400}
	for _, cut := range cuts {
		t.Run(fmt.Sprintf("cut-%d", cut), func(t *testing.T) {
			var c Counter
			if _, err := c.Write(body[:cut]); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Write(body[cut:]); err != nil {
				t.Fatal(err)
			}
			if got, want := c.Summary(), one.Summary(); got != want {
				t.Fatalf("cut %d: got %+v want %+v", cut, got, want)
			}
		})
	}
	var bytewise Counter
	for i := 0; i < len(body); i++ {
		if _, err := bytewise.Write(body[i : i+1]); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := bytewise.Summary(), one.Summary(); got != want {
		t.Fatalf("bytewise %+v want %+v", got, want)
	}
}

func TestGarbageBetweenPackets(t *testing.T) {
	const pid = 0x11
	var body []byte
	body = append(body, bytes.Join(series(pid, 0, 1, 2, 3), nil)...)
	body = append(body, 0x00, 0xFF)
	body = append(body, bytes.Join(series(pid, 4, 5, 6, 7), nil)...)
	var c Counter
	if _, err := c.Write(body); err != nil {
		t.Fatal(err)
	}
	got := c.Summary()
	want := Summary{Packets: 8, SyncLosses: 1}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestGarbageBeforeTheFirstPacket(t *testing.T) {
	body := append([]byte{0x00, 0x01, 0x02}, bytes.Join(series(0x100, 0, 1, 2, 3), nil)...)
	var c Counter
	if _, err := c.Write(body); err != nil {
		t.Fatal(err)
	}
	got := c.Summary()
	if got.SyncLosses != 1 || got.ContinuityErrors != 0 || got.Packets != 4 {
		t.Fatalf("%+v", got)
	}
}

func TestWorstPID(t *testing.T) {
	var body []byte
	// 0x31 drops one packet. 0x11 stays clean. 0x21 drops one and has a transport error.
	body = append(body, bytes.Join(series(0x11, 0, 1, 2, 3), nil)...)
	bad := series(0x21, 0, 1, 3, 4)
	bad[2] = withTEI(bad[2])
	body = append(body, bytes.Join(bad, nil)...)
	body = append(body, bytes.Join(series(0x31, 0, 2, 3, 4), nil)...)
	var c Counter
	if _, err := c.Write(body); err != nil {
		t.Fatal(err)
	}
	got := c.Summary()
	if got.WorstPID != 0x21 || got.WorstContinuity != 1 || got.WorstTransport != 1 {
		t.Fatalf("%+v", got)
	}
	if got.ContinuityErrors != 2 || got.TransportErrors != 1 || got.Packets != 12 {
		t.Fatalf("%+v", got)
	}
}

func TestTieBreaksToTheLowerPID(t *testing.T) {
	var body []byte
	body = append(body, bytes.Join(series(0x31, 0, 2, 3, 4), nil)...)
	body = append(body, bytes.Join(series(0x11, 0, 2, 3, 4), nil)...)
	var c Counter
	if _, err := c.Write(body); err != nil {
		t.Fatal(err)
	}
	got := c.Summary()
	if got.WorstPID != 0x11 || got.WorstContinuity != 1 || got.ContinuityErrors != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestPartialTailIsNotAPacket(t *testing.T) {
	body := append(bytes.Join(series(0x100, 0, 1, 2, 3), nil), 0x47, 0x01, 0x00)
	var c Counter
	if _, err := c.Write(body); err != nil {
		t.Fatal(err)
	}
	got := c.Summary()
	if got.Packets != 4 || got.SyncLosses != 0 || got.ContinuityErrors != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestEmptyWrite(t *testing.T) {
	var c Counter
	n, err := c.Write(nil)
	if n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	if got := c.Summary(); got != (Summary{}) {
		t.Fatalf("%+v", got)
	}
}

// live/testdata has no recording. A sample that shows up there must count
// without panicking; continuity on a real broadcast is not asserted.
func TestLiveSampleIfPresent(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "live", "testdata", "*.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("server/internal/live/testdata has no .ts sample")
	}
	for _, name := range matches {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var c Counter
		n, err := c.Write(body)
		if err != nil || n != len(body) {
			t.Fatalf("%s: %d %v", name, n, err)
		}
		s := c.Summary()
		if s.Packets == 0 {
			t.Fatalf("%s: no packets in %d bytes", name, len(body))
		}
		t.Logf("%s %+v", filepath.Base(name), s)
	}
}

func join(groups ...[][]byte) [][]byte {
	var out [][]byte
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func one(p []byte) [][]byte {
	return [][]byte{p}
}

func series(pid uint16, ccs ...int) [][]byte {
	out := make([][]byte, len(ccs))
	for i, cc := range ccs {
		out[i] = packet(pid, cc, true, false, false)
	}
	return out
}

func adaptOnly(pid uint16, cc int, disc bool) []byte {
	return packet(pid, cc, false, disc, false)
}

func withTEI(p []byte) []byte {
	p[1] |= 0x80
	return p
}

func packet(pid uint16, cc int, payload, disc, tei bool) []byte {
	p := make([]byte, Packet)
	p[0] = syncByte
	p[1] = byte(pid >> 8)
	p[2] = byte(pid)
	if tei {
		p[1] |= 0x80
	}
	afc := byte(0x10) // payload only
	if !payload {
		afc = 0x20
	} else if disc {
		afc = 0x30
	}
	p[3] = afc | byte(cc&0x0f)
	if !payload || disc {
		p[4] = 1
		if disc {
			p[5] = 0x80
		}
	}
	return p
}
