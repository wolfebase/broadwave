package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"broadwave/internal/store"
)

// While a start has made its row but not yet taken it, which row it is
// can't be told, so a stop of a row with no writer waits instead of closing
// a recording about to begin.
func TestAStopDuringAStartClosesNothing(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", GuideNumber: "4.1", Status: "recording", Path: "/r/news.ts", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 1}}}
	f.recordings = []*recording{{ready: make(chan struct{})}}
	h := &Hub{Store: st, Dir: t.TempDir(), channels: map[int64]*feed{1: f}}
	if err := h.EndRecording(ctx, id); !errors.Is(err, ErrRecordingStarting) {
		t.Fatalf("stop during a start: %v", err)
	}
	if rec, _ := st.Recording(ctx, id); rec.Status != "recording" {
		t.Fatalf("closed a row a start may own: %s", rec.Status)
	}
	f.recordings = nil
	if err := h.EndRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	if rec, _ := st.Recording(ctx, id); rec.Status != "interrupted" || !rec.Partial {
		t.Fatalf("after the start: %s partial=%v", rec.Status, rec.Partial)
	}
}
