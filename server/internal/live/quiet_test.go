package live

import (
	"testing"
	"time"
)

// A feed that sent bytes before its station went off the air is silent now,
// so its tuner's "no lock" counts as no signal.
func TestHeardWithinCountsOnlyRecentBytes(t *testing.T) {
	m := &mux{}
	if heardWithin(m, quietFor) {
		t.Fatal("a feed that never sent a byte was heard")
	}
	m.got.Store(true)
	m.heard.Store(time.Now().UnixNano())
	if !heardWithin(m, quietFor) {
		t.Fatal("a byte just now was not heard")
	}
	m.heard.Store(time.Now().Add(-5 * time.Second).UnixNano())
	if heardWithin(m, quietFor) {
		t.Fatal("a feed silent for 5 s still counted as heard")
	}
}
