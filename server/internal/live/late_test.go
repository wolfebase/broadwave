package live

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestALateEncodeGapIsLoggedOnceWhenItEnds(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	g := newPlaylistGate()
	g.target.Store(int64(2 * time.Second))
	r := &rendition{spec: Rendition{Video: "360", Audio: "aac2", Mode: "broadcast"}, gate: g}
	t0 := time.Now()
	fed := t0.Add(-time.Minute)
	g.moved.Store(t0.UnixNano())

	noteLate(r, "41.1", t0.Add(3*time.Second), fed)
	noteLate(r, "41.1", t0.Add(4500*time.Millisecond), fed)
	if r.late == 0 {
		t.Fatal("a 4.5 s gap on a 2 s target was not marked late")
	}
	noteLate(r, "41.1", t0.Add(5*time.Second), fed)
	if out.Len() != 0 {
		t.Fatalf("logged before the gap ended: %s", out.String())
	}
	g.moved.Store(t0.Add(6 * time.Second).UnixNano())
	noteLate(r, "41.1", t0.Add(6200*time.Millisecond), fed)
	noteLate(r, "41.1", t0.Add(7*time.Second), fed)
	if got := strings.Count(out.String(), "wrote nothing for 6.0 s"); got != 1 {
		t.Fatalf("want one line for the 6.0 s gap, got %d: %s", got, out.String())
	}
	if !strings.Contains(out.String(), "360.aac2.broadcast on 41.1") {
		t.Fatalf("line does not name the rendition and channel: %s", out.String())
	}

	// A gap that began in a tuner pause counts only from when the tuner resumed.
	out.Reset()
	g.moved.Store(t0.Add(10 * time.Second).UnixNano())
	noteLate(r, "41.1", t0.Add(15*time.Second), t0.Add(13*time.Second))
	if r.late != 0 {
		t.Fatal("time while the tuner was silent counted as the encode's gap")
	}

	// A long-group broadcast's target sets the bar.
	g.target.Store(int64(5 * time.Second))
	noteLate(r, "41.1", t0.Add(18*time.Second), fed)
	if r.late != 0 {
		t.Fatal("an 8 s gap on a 5 s target was marked late")
	}
}
