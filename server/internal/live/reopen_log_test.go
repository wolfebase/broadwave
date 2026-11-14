package live

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/store"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A tuner that didn't answer a reopen in time was logged as "context
// canceled", once for every try of one silence. It says what happened, once.
func TestAReopenTheTunerDoesntAnswerIsLoggedOnce(t *testing.T) {
	var out lockedBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	prevWithin := reopenWithin
	reopenWithin = 100 * time.Millisecond
	t.Cleanup(func() { reopenWithin = prevWithin })

	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(quiet.Close)
	const freq = 210000000
	h := &Hub{channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	m := &mux{freq: freq, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}, reopen: quiet.URL + "/auto/v8.1"}
	// A recording keeps a reopen trying past a viewer's budget.
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "8.1"}, FrequencyHz: freq}, recordings: []*recording{{id: 1}}}
	m.feeds["8.1"] = f
	h.muxes[freq] = m
	h.channels[1] = f

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	if h.reopenInput(ctx, m, time.Now()) {
		t.Fatal("a tuner that never answered was reopened")
	}
	log := out.String()
	if n := strings.Count(log, "reopen:"); n != 1 {
		t.Fatalf("%d reopen lines, want 1:\n%s", n, log)
	}
	if !strings.Contains(log, "the tuner didn't answer in 0.1 s") || strings.Contains(log, "context canceled") {
		t.Fatalf("log:\n%s", log)
	}
}
