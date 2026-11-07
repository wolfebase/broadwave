package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
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

func (b *lockedBuffer) count(what string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Count(b.buf.String(), what)
}

// One screen every 10 s used to be most of the log. A screen's first report,
// a change of state, and one every 10 minutes reach it. The rest are Debug.
func TestSyncReportsDoNotFillTheLog(t *testing.T) {
	logged := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logged, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	bus := NewBus()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	bus.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return at
	}
	step := func(d time.Duration) {
		mu.Lock()
		at = at.Add(d)
		mu.Unlock()
	}
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send := func(kind string, v any) {
		t.Helper()
		data, _ := json.Marshal(v)
		raw, _ := json.Marshal(Message{Type: kind, Data: data})
		if err := c.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
	}
	// The clock echo comes back after the report before it has been handled.
	settle := func() {
		t.Helper()
		send("clock", map[string]float64{"t0": 1})
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var m Message
			_ = json.Unmarshal(raw, &m)
			if m.Type == "clock" {
				return
			}
		}
	}
	report := func(state string) int {
		t.Helper()
		send("sync.report", map[string]any{"state": state, "drift": 12})
		settle()
		return logged.count("sync report")
	}

	if n := report("locked"); n != 1 {
		t.Fatalf("the first report: %d lines", n)
	}
	for range 10 {
		step(10 * time.Second)
		if n := report("locked"); n != 1 {
			t.Fatalf("routine reports reached the log: %d lines", n)
		}
	}
	step(10 * time.Second)
	if n := report("correcting"); n != 2 {
		t.Fatalf("a change of state after a while: %d lines", n)
	}
	// A state flapping every report is one line a minute, not one every 10 s.
	step(10 * time.Second)
	if n := report("locked"); n != 2 {
		t.Fatalf("a quick flap: %d lines", n)
	}
	step(syncReportEvery)
	if n := report("locked"); n != 3 {
		t.Fatalf("the periodic line: %d lines", n)
	}
}

// A test harness measures sync from these lines, one every 10 s.
func TestEveryReportReachesTheLogWhenAsked(t *testing.T) {
	bus := NewBus()
	bus.LogEveryReport = true
	c := &client{}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for range 3 {
		if bus.reportLevel(c, "locked", now) != slog.LevelInfo {
			t.Fatal("a report stayed out of the log")
		}
	}
}
