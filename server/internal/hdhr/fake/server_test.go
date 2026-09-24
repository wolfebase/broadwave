package fake

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"waveguide/internal/hdhr"
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
