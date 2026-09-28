package psip

import (
	"os"
	"testing"
	"time"
)

func TestKBWVSample(t *testing.T) {
	f, err := os.Open("testdata/kbwv.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	var kbwv Channel
	found := false
	for _, ch := range g.Channels {
		if ch.Major == 4 && ch.Minor == 1 {
			kbwv = ch
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no 4.1 in %+v", g.Channels)
	}
	if kbwv.ShortName != "KBWV-DT" {
		t.Fatalf("short name %q", kbwv.ShortName)
	}
	if len(g.EITPIDs) == 0 {
		t.Fatal("MGT listed no EIT")
	}
	if g.Offset != 18*time.Second {
		t.Fatalf("GPS offset %s", g.Offset)
	}
	var listed int
	for _, ev := range g.Events {
		if ev.SourceID == kbwv.SourceID && ev.Title != "" && ev.End.After(ev.Start) {
			listed++
		}
	}
	if listed == 0 {
		t.Fatalf("4.1 source %d has no titled events (%d events total)", kbwv.SourceID, len(g.Events))
	}
	if g.Events[0].Start.Before(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("event start looks wrong: %s", g.Events[0].Start)
	}
}
