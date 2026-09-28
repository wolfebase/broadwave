package ring

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func fixed(d time.Duration) func() time.Duration {
	return func() time.Duration { return d }
}

func chunk(i int) []byte {
	return bytes.Repeat([]byte{byte(i)}, 1000)
}

// settle waits until everything appended is on disk.
func settle(t *testing.T, r *Ring) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		done := len(r.pending) == 0
		r.mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("ring did not flush")
}

func TestRingReadsBackFromATime(t *testing.T) {
	c := &clock{t: time.Unix(1000, 0)}
	r := Open(t.TempDir(), Options{Window: fixed(time.Hour), Span: 10 * time.Second, Now: c.now})
	defer r.Close()
	start := c.now()
	for i := 0; i < 30; i++ {
		r.Append(chunk(i))
		settle(t, r)
		c.add(time.Second)
	}
	// Second 12 starts with chunk 12, and it spans three files by now.
	pos, at, ok := r.Position(start.Add(12 * time.Second))
	if !ok || pos != 12000 || !at.Equal(start.Add(12*time.Second)) {
		t.Fatalf("pos %d at %v ok %v", pos, at, ok)
	}
	var out bytes.Buffer
	if _, err := r.Copy(&out, pos, r.Received()); err != nil {
		t.Fatal(err)
	}
	var want []byte
	for i := 12; i < 30; i++ {
		want = append(want, chunk(i)...)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("read %d bytes, want %d", out.Len(), len(want))
	}
	if since := r.Since(); !since.Equal(start) {
		t.Fatalf("since %v", since)
	}
}

func TestRingReadsBytesNotYetOnDisk(t *testing.T) {
	r := Open(t.TempDir(), Options{Window: fixed(time.Hour)})
	defer r.Close()
	// Hold the flusher off by taking the lock while appending directly.
	r.mu.Lock()
	r.pending = append(r.pending, chunk(1), chunk(2))
	r.queued += 2000
	r.received += 2000
	r.mu.Unlock()
	var out bytes.Buffer
	if _, err := r.Copy(&out, 500, 2000); err != nil {
		t.Fatal(err)
	}
	want := append(chunk(1)[500:], chunk(2)...)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("got %d bytes", out.Len())
	}
	if tail, ok := r.PendingFrom(500); !ok || !bytes.Equal(tail, want) {
		t.Fatalf("pending %d bytes ok %v", len(tail), ok)
	}
}

func TestRingForgetsPastItsWindow(t *testing.T) {
	c := &clock{t: time.Unix(1000, 0)}
	dir := t.TempDir()
	r := Open(dir, Options{Window: fixed(30 * time.Second), Span: 10 * time.Second, Now: c.now})
	defer r.Close()
	start := c.now()
	for i := 0; i < 100; i++ {
		r.Append(chunk(i))
		settle(t, r)
		c.add(time.Second)
	}
	since := r.Since()
	held := c.now().Sub(since)
	if held < 30*time.Second || held > 41*time.Second {
		t.Fatalf("holds %s from %v", held, since.Sub(start))
	}
	if _, err := r.Copy(io.Discard, 0, 10); !errors.Is(err, ErrGone) {
		t.Fatalf("old bytes: %v", err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) > 5 {
		t.Fatalf("%d files for a 30 s window of 10 s files", len(files))
	}
	r.mu.Lock()
	marks := len(r.marks)
	r.mu.Unlock()
	if marks > 45 {
		t.Fatalf("%d marks kept", marks)
	}
}

func TestRingGivesUpItsOldestFilesForRoom(t *testing.T) {
	c := &clock{t: time.Unix(1000, 0)}
	var mu sync.Mutex
	var room int64 = 1 << 40
	setRoom := func(v int64) { mu.Lock(); room = v; mu.Unlock() }
	dir := t.TempDir()
	r := Open(dir, Options{Window: fixed(time.Hour), Span: 10 * time.Second, Now: c.now, Room: func(int64) int64 { mu.Lock(); defer mu.Unlock(); return room }})
	defer r.Close()
	for i := 0; i < 35; i++ {
		r.Append(chunk(i))
		settle(t, r)
		c.add(time.Second)
	}
	// Four files of ten seconds; the disk is 15 KB short.
	setRoom(-15000)
	r.checkRoom()
	pos, _, ok := r.Position(time.Time{})
	if !ok || pos != 20000 {
		t.Fatalf("oldest held %d ok %v; want the two oldest files gone", pos, ok)
	}
	if files, _ := os.ReadDir(dir); len(files) != 2 {
		t.Fatalf("%d files", len(files))
	}
	// Short by more than the ring holds: it stops and keeps nothing.
	setRoom(-1 << 30)
	r.checkRoom()
	r.Append(chunk(99))
	if !r.Since().IsZero() {
		t.Fatal("the ring should be off")
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatalf("%d files left", len(files))
	}
	// It waits for resumeRoom, not just the floor.
	setRoom(resumeRoom - 1)
	r.checkRoom()
	r.Append(chunk(100))
	if !r.Since().IsZero() {
		t.Fatal("resumed too early")
	}
	setRoom(resumeRoom)
	r.checkRoom()
	r.Append(chunk(101))
	settle(t, r)
	if pos, _, ok := r.Position(time.Time{}); !ok || pos != 37000 {
		t.Fatalf("resumed at %d ok %v", pos, ok)
	}
}

func TestRingStartsOverWhenTheDiskFallsBehind(t *testing.T) {
	r := Open(t.TempDir(), Options{Window: fixed(time.Hour)})
	defer r.Close()
	r.Append(chunk(1))
	settle(t, r)
	r.mu.Lock()
	r.queued = pendingCap
	r.mu.Unlock()
	r.Append(chunk(2))
	if !r.Since().IsZero() {
		t.Fatal("an overflow should empty the ring")
	}
	r.Append(chunk(3))
	settle(t, r)
	if pos, _, ok := r.Position(time.Time{}); !ok || pos != 2000 {
		t.Fatalf("restarted at %d ok %v", pos, ok)
	}
}

func TestRingReadsFailOnceClosed(t *testing.T) {
	r := Open(t.TempDir(), Options{Window: fixed(time.Hour)})
	r.Append(chunk(1))
	settle(t, r)
	r.mu.Lock()
	r.pending = append(r.pending, chunk(2))
	r.queued += 1000
	r.received += 1000
	r.mu.Unlock()
	r.Close()
	done := make(chan error, 1)
	go func() {
		_, err := r.Copy(io.Discard, 0, 2000)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrGone) {
			t.Fatalf("err %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a read after Close did not return")
	}
	if _, ok := r.PendingFrom(1000); ok {
		t.Fatal("pending bytes after Close")
	}
	// A reader that had caught up must hear about it too, or it spins.
	if _, err := r.Copy(io.Discard, 2000, 2000); !errors.Is(err, ErrGone) {
		t.Fatalf("empty read after Close: %v", err)
	}
}

func TestRingCloseRemovesItsFiles(t *testing.T) {
	dir := t.TempDir() + "/ring"
	r := Open(dir, Options{Window: fixed(time.Hour)})
	r.Append(chunk(1))
	settle(t, r)
	r.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("ring folder is still there")
}

func TestRingPausesAfterAFileError(t *testing.T) {
	dir := t.TempDir() + "/ring"
	r := Open(dir, Options{Window: fixed(time.Hour), Room: func(int64) int64 { return 1 << 40 }})
	defer r.Close()
	r.Append(chunk(1))
	settle(t, r)
	// The folder goes away under the ring, as a failed disk would take it.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.segs[len(r.segs)-1].opened = time.Time{}
	r.mu.Unlock()
	r.Append(chunk(2))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		low := r.low
		r.mu.Unlock()
		if low {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.Append(chunk(3))
	if !r.Since().IsZero() {
		t.Fatal("the ring should wait for the next room check")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r.checkRoom()
	r.Append(chunk(4))
	settle(t, r)
	if pos, _, ok := r.Position(time.Time{}); !ok || pos != 3000 {
		t.Fatalf("resumed at %d ok %v", pos, ok)
	}
}

func TestRingTurnsOffAndOnWithItsSetting(t *testing.T) {
	var mu sync.Mutex
	window := time.Hour
	dir := t.TempDir()
	r := Open(dir, Options{Window: func() time.Duration { mu.Lock(); defer mu.Unlock(); return window }})
	defer r.Close()
	r.Append(chunk(1))
	settle(t, r)
	if st := r.Stats(); st.Bytes != 1000 || st.Off || st.Since.IsZero() {
		t.Fatalf("%+v", st)
	}
	mu.Lock()
	window = 0
	mu.Unlock()
	r.checkWindow()
	r.Append(chunk(2))
	if st := r.Stats(); st.Bytes != 0 || !st.Off || !st.Since.IsZero() {
		t.Fatalf("off: %+v", st)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatalf("%d files while off", len(files))
	}
	mu.Lock()
	window = 30 * time.Minute
	mu.Unlock()
	r.checkWindow()
	r.Append(chunk(3))
	settle(t, r)
	if pos, _, ok := r.Position(time.Time{}); !ok || pos != 2000 {
		t.Fatalf("back on at %d ok %v", pos, ok)
	}
}
