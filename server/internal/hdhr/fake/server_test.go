package fake

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestStreamSourceKeepsEachFrequency(t *testing.T) {
	s := &Server{Source: "fast.ts", Source5: "slow.ts"}
	if got := s.streamSource("4.1"); got != "fast.ts" {
		t.Fatalf("4.1 -> %s", got)
	}
	if got := s.streamSource("4.2"); got != "fast.ts" {
		t.Fatalf("4.2 -> %s", got)
	}
	if got := s.streamSource("5.1"); got != "slow.ts" {
		t.Fatalf("5.1 -> %s", got)
	}
	s.Channels = antennaChannels()
	if got := s.streamSource("593000000"); got != "fast.ts" {
		t.Fatalf("593 MHz -> %s", got)
	}
	if got := s.streamSource("533000000"); got != "slow.ts" {
		t.Fatalf("533 MHz -> %s", got)
	}
	s.Source5 = ""
	if got := s.streamSource("5.1"); got != "fast.ts" {
		t.Fatalf("5.1 without a second file -> %s", got)
	}
}

func TestTuneStatusAndBusy(t *testing.T) {
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	if err := os.WriteFile(sample, append(pkt, pkt...), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{TS: sample}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	res, err := http.Get(base + "/discover.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "FAKEHDHR") {
		t.Fatalf("discover %s", body)
	}

	c := hdhr.Control{Addr: "127.0.0.1:" + port}
	if _, err := c.Set("/tuner0/vchannel", "4.1"); err != nil {
		t.Fatal(err)
	}
	status, err := c.Get("/tuner0/status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "lock=8vsb") || !strings.Contains(status, "593000000") {
		t.Fatal(status)
	}
	info, err := c.Get("/tuner0/streaminfo")
	if err != nil || !strings.Contains(info, "4.2") {
		t.Fatalf("info %q %v", info, err)
	}
	if _, err := c.Set("/tuner1/vchannel", "5.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Set("/tuner0/vchannel", "5.1"); err == nil || !strings.Contains(err.Error(), "805") {
		t.Fatalf("expected 805, got %v", err)
	}

	stream, err := http.Get(base + "/auto/v4.1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	buf := make([]byte, 188)
	if _, err := io.ReadFull(stream.Body, buf); err != nil || buf[0] != 0x47 {
		t.Fatalf("stream %v %x", err, buf[0])
	}
}

func TestHTTPTuneReportsLockWithoutAControlTune(t *testing.T) {
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	if err := os.WriteFile(sample, bytes.Repeat(pkt, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{TS: sample}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/tuner0/ch593000000", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	c := hdhr.Control{Addr: "127.0.0.1:" + port}
	status, err := c.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=8vsb") || !strings.Contains(status, "593000000") {
		t.Fatalf("streaming status %q %v", status, err)
	}
	srv.Dark("4.1")
	status, err = c.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=none") || strings.Contains(status, "lock=8vsb") {
		t.Fatalf("dark status %q %v", status, err)
	}
	srv.Light("4.1")
	status, err = c.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=8vsb") {
		t.Fatalf("restored status %q %v", status, err)
	}
}

func TestDarkChannelReportsNoLockAndSendsNothing(t *testing.T) {
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	if err := os.WriteFile(sample, bytes.Repeat(pkt, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{TS: sample}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c := hdhr.Control{Addr: "127.0.0.1:" + port}
	if _, err := c.Set("/tuner0/vchannel", "5.1"); err != nil {
		t.Fatal(err)
	}
	srv.Dark("5.1")
	status, err := c.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=none") || strings.Contains(status, "lock=8vsb") {
		t.Fatalf("dark status %q %v", status, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/tuner0/ch533000000", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %s", res.Status)
	}
	buf := make([]byte, 188)
	n, _ := res.Body.Read(buf)
	if n != 0 {
		t.Fatalf("dark channel sent %d bytes", n)
	}
	// Closing /tunerN/ch clears that tuner, after the handler sees the close.
	// Tuning again before that lands is undone by it.
	_ = res.Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, _ := c.Get("/tuner0/status")
		if strings.Contains(status, "ch=none") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	srv.Light("5.1")
	if _, err := c.Set("/tuner0/vchannel", "5.1"); err != nil {
		t.Fatal(err)
	}
	status, err = c.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=8vsb") {
		t.Fatalf("light status %q %v", status, err)
	}
	stream, err := http.Get(base + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if _, err := io.ReadFull(stream.Body, buf); err != nil || buf[0] != 0x47 {
		t.Fatalf("stream %v %x", err, buf[0])
	}
}

func TestHoldAndSilence(t *testing.T) {
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	if err := os.WriteFile(sample, append(pkt, pkt...), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{TS: sample}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	srv.HoldAll()
	res, err := http.Get(base + "/status.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Count(string(body), `"TargetIP":"127.0.0.1"`) != 2 {
		t.Fatalf("both tuners should look busy: %s", body)
	}
	srv.FreeAll()
	res, err = http.Get(base + "/status.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(body), `"TargetIP":"127.0.0.1"`) {
		t.Fatalf("tuners still held: %s", body)
	}

	srv.Silence()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	if _, err := client.Get(base + "/discover.json"); err == nil {
		t.Fatal("a silent tuner still answered discover")
	}
	c := hdhr.Control{Addr: "127.0.0.1:" + port}
	if _, err := c.Get("/sys/model"); err == nil {
		t.Fatal("a silent tuner still answered control")
	}
	srv.Answer()
	res, err = http.Get(base + "/discover.json")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("answer status %s", res.Status)
	}
}

func TestRawPlaysTheFileByteForByteAtItsOwnPace(t *testing.T) {
	pcr := func(ticks int64) []byte {
		pkt := make([]byte, 188)
		pkt[0], pkt[1], pkt[2], pkt[3] = 0x47, 0x01, 0x00, 0x20
		pkt[4], pkt[5] = 183, 0x10
		pkt[6] = byte(ticks >> 25)
		pkt[7] = byte(ticks >> 17)
		pkt[8] = byte(ticks >> 9)
		pkt[9] = byte(ticks >> 1)
		pkt[10] = byte(ticks<<7) | 0x7e
		return pkt
	}
	var data []byte
	data = append(data, pcr(1_000_000)...)
	for i := range 1998 {
		pkt := make([]byte, 188)
		pkt[0], pkt[1], pkt[2], pkt[3] = 0x47, 0x01, 0x01, 0x10|byte(i&0x0f)
		pkt[4] = byte(i)
		data = append(data, pkt...)
	}
	data = append(data, pcr(1_090_000)...)
	sample := filepath.Join(t.TempDir(), "raw.ts")
	if err := os.WriteFile(sample, data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := &Server{TS: sample, Raw: true}
	base, _, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	res := openStream(t, base+"/auto/v5.1")
	defer res.Body.Close()
	start := time.Now()
	got := make([]byte, 2*len(data))
	if _, err := io.ReadFull(res.Body, got[:len(data)]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:len(data)], data) {
		t.Fatal("one pass is not the file's bytes")
	}
	if _, err := io.ReadFull(res.Body, got[len(data):]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[len(data):], data) {
		t.Fatal("the loop is not the file's bytes")
	}
	// Two passes of a file whose PCR spans one second, plus one interval.
	if d := time.Since(start); d < 3500*time.Millisecond || d > 5000*time.Millisecond {
		t.Fatalf("two passes took %s", d)
	}
}
