package psip

import (
	"os"
	"testing"
	"time"
)

func TestHarvesterReadsTheSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/wdaf.ts")
	if err != nil {
		t.Fatal(err)
	}
	var h Harvester
	var got Guide
	ok := false
	for len(raw) > 0 {
		n := 188 * 7
		if n > len(raw) {
			n = len(raw)
		}
		if g, ready := h.Add(raw[:n]); ready {
			got = g
			ok = true
		}
		raw = raw[n:]
	}
	if !ok {
		t.Fatal("harvester did not yield a guide")
	}
	if len(got.Events) == 0 || got.Offset != 18*time.Second {
		t.Fatalf("events %d offset %s", len(got.Events), got.Offset)
	}
}
