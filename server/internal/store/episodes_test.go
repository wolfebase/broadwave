package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSetLibraryIdentityRenamesAndDropsTheOldIntro(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateRecording(ctx, Recording{
		Title: "S01", GuideNumber: "Library", Path: "/lib/a.mkv", Status: "complete",
		Season: 1, Episode: 2, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEpisodePrints(ctx, id, EpisodePrints{Head: []uint32{1}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetEpisodeEnds(ctx, id, 90, 120, 1700); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLibraryIdentity(ctx, id, Recording{Title: "Harbor Watch", Subtitle: "Harbor Watch - S01E02", Category: "Series", Season: 1, Episode: 2}, true); err != nil {
		t.Fatal(err)
	}
	got, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Harbor Watch" || got.Season != 1 || got.Episode != 2 || got.Listened || got.IntroEnd != 0 || got.CreditsStart != 0 {
		t.Fatalf("%+v", got)
	}
	if _, ok, err := st.EpisodePrints(ctx, id); err != nil || ok {
		t.Fatalf("prints kept: %v %v", ok, err)
	}
	aired, err := st.CreateRecording(ctx, Recording{
		ChannelID: 4, Title: "Evening News", GuideNumber: "4.1", Path: "/rec/news.ts", Status: "complete", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetLibraryIdentity(ctx, aired, Recording{Title: "Harbor Watch"}, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Recording(ctx, aired); got.Title != "Evening News" {
		t.Fatalf("recording renamed: %+v", got)
	}
}

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
