package live

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

// A tune that found no signal reads as dark, for its channel and for the
// others on its frequency, until darkFor passes or a tune of it works.
func TestADarkTuneIsRememberedForAWhile(t *testing.T) {
	h := &Hub{}
	ch := store.SourceChannel{}
	ch.ID, ch.FrequencyHz = 7, 503_000_000
	h.mu.Lock()
	h.noteDarkLocked(ch, ErrNoSignal)
	h.mu.Unlock()
	if !h.RecentlyDark(7, 0) || !h.RecentlyDark(8, 503_000_000) {
		t.Fatal("a fresh no-signal tune does not read dark")
	}
	if h.RecentlyDark(8, 509_000_000) || h.RecentlyDark(9, 0) {
		t.Fatal("another frequency reads dark")
	}

	h.mu.Lock()
	for key := range h.dark {
		h.dark[key] = time.Now().Add(-darkFor - time.Second)
	}
	h.mu.Unlock()
	if h.RecentlyDark(7, 503_000_000) {
		t.Fatal("a no-signal tune older than darkFor still reads dark")
	}

	h.mu.Lock()
	h.noteDarkLocked(ch, ErrNoSignal)
	h.noteDarkLocked(ch, nil)
	h.mu.Unlock()
	if h.RecentlyDark(7, 503_000_000) {
		t.Fatal("a tune that worked still reads dark")
	}
}
