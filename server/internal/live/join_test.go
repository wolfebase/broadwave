package live

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"broadwave/internal/ring"
)

type timedWriter struct {
	mu     sync.Mutex
	got    []byte
	closed bool
}

func (w *timedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.got = append(w.got, p...)
	return len(p), nil
}

func (w *timedWriter) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

func (w *timedWriter) len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.got)
}

// steadyMux feeds a mux with a ring the way readLoop does: numbered 1000-byte
// chunks every 10 ms, so a byte's value says where in the stream it was.
func steadyMux(t *testing.T) (*mux, func() int64, func()) {
	t.Helper()
	m := &mux{freq: 1, ring: ring.Open(t.TempDir(), ring.Options{Window: func() time.Duration { return time.Hour }})}
	var mu sync.Mutex
	var sent int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for n := uint32(0); ; n++ {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			chunk := make([]byte, 1000)
			for i := 0; i < len(chunk); i += 4 {
				binary.BigEndian.PutUint32(chunk[i:], n*250+uint32(i/4))
			}
			for _, s := range m.noteLead(chunk) {
				s.offer(chunk, m.freq)
			}
			mu.Lock()
			sent += int64(len(chunk))
			mu.Unlock()
		}
	}()
	total := func() int64 {
		mu.Lock()
		defer mu.Unlock()
		return sent
	}
	var once sync.Once
	end := func() {
		once.Do(func() {
			close(stop)
			<-done
			m.ring.Close()
		})
	}
	t.Cleanup(end)
	return m, total, end
}

// inOrder fails unless got is one unbroken run of the stream, from where it
// started: a byte lost or sent twice at the hand-over breaks the count.
func inOrder(t *testing.T, got []byte) {
	t.Helper()
	if len(got) < 8 || len(got)%4 != 0 {
		t.Fatalf("%d bytes", len(got))
	}
	first := binary.BigEndian.Uint32(got)
	for i := 4; i < len(got); i += 4 {
		if v := binary.BigEndian.Uint32(got[i:]); v != first+uint32(i/4) {
			t.Fatalf("word %d is %d, want %d", i/4, v, first+uint32(i/4))
		}
	}
}

func TestAnExportStartsWithTheBufferedSeconds(t *testing.T) {
	m, sent, end := steadyMux(t)
	h := &Hub{}
	time.Sleep(4 * time.Second)
	w := &timedWriter{}
	sub := h.attachExportLocked(m, w)
	time.Sleep(300 * time.Millisecond)
	// Three seconds of the stream (300 KB) arrive at once, not 30 KB.
	if n := w.len(); n < 250_000 {
		t.Fatalf("%d bytes after 300 ms", n)
	}
	end()
	deadline := time.Now().Add(2 * time.Second)
	for sub.queued.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := func() []byte { w.mu.Lock(); defer w.mu.Unlock(); return bytes.Clone(w.got) }()
	inOrder(t, got)
	if last := binary.BigEndian.Uint32(got[len(got)-4:]); int64(last+1)*4 != sent() {
		t.Fatalf("the export ends at byte %d, the tuner sent %d", int64(last+1)*4, sent())
	}
	m.detach(sub)
}

// An export still copying the ring when the tune ends must not join the
// ended tune: nothing would stop it, and the app on the other end would hang.
func TestAnExportCopyingWhenTheTuneEndsLetsGo(t *testing.T) {
	m, _, end := steadyMux(t)
	h := &Hub{}
	time.Sleep(4 * time.Second)
	w := &timedWriter{}
	slow := &slowWriter{w: w, wait: 50 * time.Millisecond}
	sub := h.attachExportLocked(m, slow)
	time.Sleep(20 * time.Millisecond)
	for _, s := range m.shut() {
		s.stop()
	}
	end()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w.mu.Lock()
		closed := w.closed
		w.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the export's input stayed open after the tune ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.pipeMu.Lock()
	defer m.pipeMu.Unlock()
	for _, s := range m.pipes {
		if s == sub {
			t.Fatal("the export joined a tune that had ended")
		}
	}
}

type slowWriter struct {
	w    *timedWriter
	wait time.Duration
}

func (s *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(s.wait)
	return s.w.Write(p)
}

func (s *slowWriter) Close() error { return s.w.Close() }
