package live

import (
	"testing"
	"time"
)

func TestWatchingCountsViewersAndExports(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "4.1")
	r := addTestRendition(h, f, "copy.copy", 0, time.Now())
	if h.Watching() {
		t.Fatal("a rendition nobody watches counted as watching")
	}
	h.mu.Lock()
	r.viewers = 1
	h.mu.Unlock()
	if !h.Watching() {
		t.Fatal("a viewer did not count")
	}
	h.mu.Lock()
	r.viewers = 0
	f.exports = 1
	h.mu.Unlock()
	if !h.Watching() {
		t.Fatal("an emulated-tuner reader did not count")
	}
}
