package live

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// BenchmarkFanout is ten viewers on each of four channels, two tuners.
// The timed section reloads the playlists those viewers share. Profiles
// come from go test -cpuprofile, not from an HTTP endpoint.
func BenchmarkFanout(b *testing.B) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		b.Skip("ffmpeg is not installed")
	}
	sample := b.TempDir() + "/sample.ts"
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-f", "mpegts", sample)
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("sample: %v %s", err, out)
	}
	srv := &fake.Server{Source: sample, Channels: fake.QuadLineup()}
	base, port, err := srv.Start()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(srv.Close)
	b.Setenv("HDHR_CONTROL_PORT", port)

	ctx := context.Background()
	st, err := store.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		b.Fatal(err)
	}
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		b.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		b.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		b.Fatal(err)
	}
	h := New(st, b.TempDir(), "ffmpeg", "libx264")
	h.Host.Tiles = 0
	h.RenditionIdle = time.Hour
	b.Cleanup(h.Shutdown)

	want := Rendition{Video: "copy", Audio: "copy"}
	var ids []int64
	freqs := map[int]bool{}
	for _, number := range []string{"4.1", "4.2", "5.1", "5.2"} {
		id := benchChannel(b, st, number)
		var last Session
		for range 10 {
			last, err = h.Watch(ctx, id, want, false)
			if err != nil {
				b.Fatal(err)
			}
		}
		if last.Viewers != 10 {
			b.Fatalf("%s has %d viewers", number, last.Viewers)
		}
		freqs[last.Frequency] = true
		ids = append(ids, id)
	}
	if !freqs[593000000] || !freqs[533000000] || len(freqs) != 2 {
		b.Fatalf("frequencies %v", freqs)
	}
	deadline := time.Now().Add(20 * time.Second)
	for _, id := range ids {
		for {
			body, err := h.Playlist(id, "copy.copy")
			if err == nil && len(body) > 0 {
				break
			}
			if time.Now().After(deadline) {
				b.Fatalf("channel %d playlist: %v", id, err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, id := range ids {
			body, err := h.Playlist(id, "copy.copy")
			if err != nil || len(body) == 0 {
				b.Fatal(err)
			}
		}
	}
}

func benchChannel(b *testing.B, st *store.Store, number string) int64 {
	b.Helper()
	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		b.Fatal(err)
	}
	for _, ch := range channels {
		if ch.GuideNumber == number {
			return ch.ID
		}
	}
	b.Fatalf("missing %s", number)
	return 0
}
