package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Prints round-trip, and a recording deleted while it was listened to gets
// none: SQLite gives its id to the next recording.
func TestEpisodePrintsNeedTheRecording(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateRecording(ctx, Recording{Title: "Show", Path: "/r/a.ts", Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	want := EpisodePrints{Head: []uint32{1, 0xfffffffe}, Tail: []uint32{7}, TailFrom: 1200, ShowStart: 60, ShowEnd: 1860}
	if err := st.SaveEpisodePrints(ctx, id, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.EpisodePrints(ctx, id)
	if err != nil || !ok || got.Head[1] != 0xfffffffe || got.Tail[0] != 7 || got.TailFrom != 1200 || got.ShowStart != 60 || got.ShowEnd != 1860 {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	if rec, _ := st.Recording(ctx, id); !rec.Listened {
		t.Fatal("not listened")
	}
	if err := st.DeleteRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEpisodePrints(ctx, id, EpisodePrints{}); err != nil {
		t.Fatal(err)
	}
	next, err := st.CreateRecording(ctx, Recording{Title: "Next", Path: "/r/b.ts", Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if rec, _ := st.Recording(ctx, next); rec.Listened {
		t.Fatalf("recording %d (was %d) came listened", next, id)
	}
}
