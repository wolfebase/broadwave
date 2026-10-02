package live

import (
	"context"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

type countWriter struct{ n atomic.Int64 }

func (w *countWriter) Write(p []byte) (int, error) {
	w.n.Add(int64(len(p)))
	return len(p), nil
}

func streamOpens(srv *fake.Server) int {
	n := 0
	for _, path := range srv.Requests() {
		if strings.Contains(path, "/tuner") || strings.Contains(path, "/auto/") {
			n++
		}
	}
	return n
}

func waitUntil(t *testing.T, limit time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// exportOf watches a channel through an export, the lightest viewer.
func exportOf(t *testing.T, h *Hub, id int64) (*countWriter, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := &countWriter{}
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		done <- h.Export(ctx, id, w)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
	})
	waitUntil(t, 20*time.Second, "the first bytes", func() bool { return w.n.Load() > 0 })
	return w, done
}

func TestASilentTunerIsOpenedAgain(t *testing.T) {
	h, st, srv := liveHub(t)
	h.stall = time.Second
	w, done := exportOf(t, h, idOf(t, st, "4.1"))
	opens := streamOpens(srv)
	srv.Dark("4.1")
	time.Sleep(2500 * time.Millisecond)
	srv.Light("4.1")
	if got := streamOpens(srv); got <= opens {
		t.Fatalf("stream opened %d times, %d before the silence", got, opens)
	}
	at := w.n.Load()
	waitUntil(t, 10*time.Second, "bytes after the silence", func() bool { return w.n.Load() > at+188*100 })
	select {
	case err := <-done:
		t.Fatalf("the export ended: %v", err)
	default:
	}
}

func TestATunerThatStaysSilentEndsTheChannel(t *testing.T) {
	h, st, srv := liveHub(t)
	h.stall = 500 * time.Millisecond
	id := idOf(t, st, "4.1")
	_, done := exportOf(t, h, id)
	srv.Dark("4.1")
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a silent channel kept its viewer waiting")
	}
	h.mu.Lock()
	tuned := h.channels[id] != nil
	h.mu.Unlock()
	if tuned {
		t.Fatal("the silent channel is still tuned")
	}
}

func TestASilentTunerKeepsItsRecording(t *testing.T) {
	h, st, srv := liveHub(t)
	h.stall = 500 * time.Millisecond
	id := idOf(t, st, "4.1")
	rec, err := h.RecordMeta(context.Background(), 1, store.Recording{ChannelID: id, Title: "News", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.StopRecord(rec.ID)
	waitUntil(t, 20*time.Second, "the first bytes", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		f := h.channels[id]
		return f != nil && muxOf(h, f) != nil && muxOf(h, f).got.Load()
	})
	opens := streamOpens(srv)
	srv.Dark("4.1")
	time.Sleep(3500 * time.Millisecond)
	srv.Light("4.1")
	if got := streamOpens(srv) - opens; got <= stallReopens {
		t.Fatalf("%d reopens in a silence that outlasts a viewer's budget", got)
	}
	got, err := st.Recording(context.Background(), rec.ID)
	if err != nil || got.Status != "recording" {
		t.Fatalf("status %q err %v", got.Status, err)
	}
}

// A real tuner with no signal answers a reopen with an error, and another app
// can take a tuner the moment it is free. Neither may fail a recording.
func TestARecordingOutlastsARefusedReopen(t *testing.T) {
	h, st, srv := liveHub(t)
	h.stall = 500 * time.Millisecond
	id := idOf(t, st, "4.1")
	rec, err := h.RecordMeta(context.Background(), 1, store.Recording{ChannelID: id, Title: "News", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.StopRecord(rec.ID)
	var m *mux
	waitUntil(t, 20*time.Second, "the first bytes", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		if f := h.channels[id]; f != nil {
			m = muxOf(h, f)
		}
		return m != nil && m.got.Load()
	})
	srv.Refuse()
	srv.Dark("4.1")
	time.Sleep(6 * time.Second)
	if got, err := st.Recording(context.Background(), rec.ID); err != nil || got.Status != "recording" {
		t.Fatalf("status %q err %v while the tuner refused", got.Status, err)
	}
	at := m.heard.Load()
	srv.Accept()
	srv.Light("4.1")
	waitUntil(t, 30*time.Second, "bytes after the tuner came back", func() bool { return m.heard.Load() > at && !m.recovering.Load() && m.got.Load() })
	if got, err := st.Recording(context.Background(), rec.ID); err != nil || got.Status != "recording" {
		t.Fatalf("status %q err %v after the tuner came back", got.Status, err)
	}
}

func TestAnEncodeThatStopsWritingIsStartedAgain(t *testing.T) {
	h, st, _ := liveHub(t)
	h.outStall = 2 * time.Second
	id := idOf(t, st, "4.1")
	session, err := h.Watch(context.Background(), id, Rendition{Video: "copy", Audio: "copy"}, false)
	if err != nil {
		t.Fatal(err)
	}
	rendition := func() *rendition {
		h.mu.Lock()
		defer h.mu.Unlock()
		if f := h.channels[id]; f != nil {
			return f.renditions[session.Rendition]
		}
		return nil
	}
	waitUntil(t, 20*time.Second, "a growing playlist", func() bool {
		r := rendition()
		return r != nil && r.gate.moved.Load() != 0
	})
	r := rendition()
	h.mu.Lock()
	frozen := r.cmd
	h.mu.Unlock()
	// Alive and writing nothing, as ffmpeg was on a live channel.
	if err := frozen.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 20*time.Second, "a new encode with a growing playlist", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return r.cmd != nil && r.cmd != frozen && !r.unfroze.IsZero() && r.gate.moved.Load() != 0
	})
}

func TestAPlaylistIsJudgedOnlyWhileItIsFed(t *testing.T) {
	g := newPlaylistGate()
	now := time.Now()
	if g.stuck(now, time.Time{}, time.Second) {
		t.Fatal("a playlist that never grew is not stuck; the start has its own limits")
	}
	g.publish(0, 0, 1)
	g.moved.Store(now.Add(-5 * time.Second).UnixNano())
	if !g.stuck(now, time.Time{}, 2*time.Second) {
		t.Fatal("5 s without a part while fed is stuck at a 2 s limit")
	}
	if g.stuck(now, now.Add(-time.Second), 2*time.Second) {
		t.Fatal("the tuner only came back 1 s ago")
	}
	g.gap.Store(int64(2 * time.Second))
	if g.stuck(now, time.Time{}, 2*time.Second) {
		t.Fatal("a broadcast with 2 s gaps gets three of them")
	}
	g.gap.Store(int64(time.Minute))
	g.moved.Store(now.Add(-31 * time.Second).UnixNano())
	if !g.stuck(now, time.Time{}, 2*time.Second) {
		t.Fatal("a minute-long silence loosened the watch past 30 s")
	}
}
