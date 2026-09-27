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
