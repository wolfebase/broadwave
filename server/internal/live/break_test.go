package live

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// A station splice or a looping source moves timestamps back for good. With
// -copyts, ffmpeg clamps every later packet of a copied track to the old high
// point and the playlist never grows again. The encode starts again instead,
// on the same playlist, and players see a discontinuity.
func TestCopiedTrackPlaysOnAcrossABackwardsBreak(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/loop.ts"
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "4", "-c:v", "libx264", "-preset", "ultrafast", "-g", "15", "-pix_fmt", "yuv420p",
		"-c:a", "ac3", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	srv := &fake.Server{TS: sample, Raw: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
	}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	defer h.Shutdown()
	id := idOf(t, st, "4.1")
	session, err := h.Watch(ctx, id, Rendition{Video: "copy", Audio: "copy"}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Two loops of the sample: each is a break back of about four seconds.
	var list string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		b, err := h.Playlist(id, session.Rendition)
		if err != nil {
			continue
		}
		list = string(b)
		if strings.Count(list, "#EXT-X-DISCONTINUITY") >= 2 && afterLast(list, "#EXT-X-DISCONTINUITY") >= 3 {
			break
		}
	}
	if n := strings.Count(list, "#EXT-X-DISCONTINUITY"); n < 2 {
		t.Fatalf("%d discontinuities after two loops:\n%s", n, list)
	}
	if n := afterLast(list, "#EXT-X-DISCONTINUITY"); n < 3 {
		t.Fatalf("%d segments after the last break:\n%s", n, list)
	}
	if !strings.Contains(list, "#EXT-X-MEDIA-SEQUENCE:0") {
		t.Fatalf("the playlist started over:\n%s", list)
	}
}

func afterLast(list, tag string) int {
	i := strings.LastIndex(list, tag)
	if i < 0 {
		return 0
	}
	return strings.Count(list[i:], "#EXTINF")
}
