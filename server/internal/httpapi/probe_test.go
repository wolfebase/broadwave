package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// The library list does not wait on ffprobe: lengths are read once, in the
// background, and stored.
func TestProbeLaterStoresTheLengthOnce(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ffprobe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	release := make(chan struct{})
	prev := probeDuration
	probeDuration = func(_, path string) (float64, error) {
		calls.Add(1)
		<-release
		return 1800, nil
	}
	t.Cleanup(func() { probeDuration = prev })
	id, err := st.CreateRecording(ctx, store.Recording{ChannelID: 1, Title: "Film", Path: filepath.Join(dir, "film.ts"), Status: "imported", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Hub: &live.Hub{FFmpeg: filepath.Join(dir, "ffmpeg")}}
	s.probeLater(id, filepath.Join(dir, "film.ts"))
	s.probeLater(id, filepath.Join(dir, "film.ts"))
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec, _ := st.Recording(ctx, id)
		if rec.Duration == 1800 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("length not stored: %v", rec.Duration)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("probed %d times", calls.Load())
	}
}
