package live

import (
	"bytes"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type bufferCloser struct{ bytes.Buffer }

func (bufferCloser) Close() error { return nil }

func TestFollowFileReadsBytesWrittenLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "show.ts")
	if err := os.WriteFile(path, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bufferCloser
	var still atomic.Bool
	still.Store(true)
	done := make(chan struct{})
	go func() {
		followFile(path, &buf, still.Load)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("bbb")); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	time.Sleep(400 * time.Millisecond)
	still.Store(false)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("follower did not finish")
	}
	if buf.String() != "aaabbb" {
		t.Fatalf("read %q", buf.String())
	}
}
