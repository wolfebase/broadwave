package dvr

import (
	"context"
	"math"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/breaks"
	"broadwave/internal/store"
)

// prints stands in for a recording's sound: random prints, with theme
// copied in at the given second.
func prints(seed int64, seconds float64, theme []uint32, at float64) []uint32 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]uint32, int(seconds/breaks.HopSeconds))
	for i := range out {
		out[i] = rng.Uint32() | 1
	}
	copy(out[int(at/breaks.HopSeconds):], theme)
	return out
}

// The first episode of a show has nothing to compare with; the second finds
// the intro they share in both, and an ad they share inside a break is not it.
func TestFindEpisodeEndsComparesEpisodesOfAShow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	theme := prints(1, 30, nil, 0)
	ad := prints(2, 45, nil, 0)
	heads := map[string][]uint32{
		"/r/one.ts":   prints(3, 1200, theme, 90),
		"/r/two.ts":   prints(4, 1200, theme, 200),
		"/r/movie.ts": prints(5, 1200, theme, 300),
	}
	copy(heads["/r/one.ts"][int(600/breaks.HopSeconds):], ad)
	copy(heads["/r/two.ts"][int(704/breaks.HopSeconds):], ad)
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path]}, nil
	}
	t.Cleanup(func() { listenFile = old })

	add := func(title, path string, day int) store.Recording {
		id, err := st.CreateRecording(ctx, store.Recording{Title: title, Path: path, Status: "complete", StartedAt: time.Now().AddDate(0, 0, day)})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 1800); err != nil {
			t.Fatal(err)
		}
		rec, _ := st.Recording(ctx, id)
		return rec
	}
	one := add("Sitcom", "/r/one.ts", -7)
	if err := st.ReplaceMarkers(ctx, one.ID, []store.Marker{{Start: 590, End: 700, Confidence: 0.95}}); err != nil {
		t.Fatal(err)
	}
	if err := FindEpisodeEnds(ctx, st, "", one); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Recording(ctx, one.ID); got.IntroEnd != 0 || !got.Listened {
		t.Fatalf("first episode: %+v", got)
	}
	// Another show with the same theme is not compared.
	if err := FindEpisodeEnds(ctx, st, "", add("Movie", "/r/movie.ts", -3)); err != nil {
		t.Fatal(err)
	}
	two := add("sitcom ", "/r/two.ts", 0)
	if err := FindEpisodeEnds(ctx, st, "", two); err != nil {
		t.Fatal(err)
	}
	for id, start := range map[int64]float64{one.ID: 90, two.ID: 200} {
		got, _ := st.Recording(ctx, id)
		if math.Abs(got.IntroStart-start) > 1 || math.Abs(got.IntroEnd-start-30) > 1 {
			t.Fatalf("recording %d: intro %v-%v, want %v", id, got.IntroStart, got.IntroEnd, start)
		}
	}
	if got, _ := st.Recording(ctx, two.ID); got.CreditsStart != 0 {
		t.Fatalf("credits %v", got.CreditsStart)
	}
}

// The listing bounds the show in the file: a padded recording starts a minute
// before its show and ends two after it.
func TestShowBoundsComeFromTheListing(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	show := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	if err := st.ReplaceAirings(ctx, []store.Airing{{ChannelID: 1, Title: "Sitcom", Start: show, End: show.Add(30 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	rec := store.Recording{ChannelID: 1, Title: "Sitcom", StartedAt: show.Add(-time.Minute)}
	if from, to := showBounds(ctx, st, rec, 33*60); from != 60 || to != 31*60 {
		t.Fatalf("%v %v", from, to)
	}
	// Stopped before the show ended: no late padding to leave out.
	if from, to := showBounds(ctx, st, rec, 20*60); from != 60 || to != 0 {
		t.Fatalf("stopped early: %v %v", from, to)
	}
	// A game can run past its listing.
	rec.GameID = "401"
	if _, to := showBounds(ctx, st, rec, 33*60); to != 0 {
		t.Fatalf("game: %v", to)
	}
}

// A rerun of an episode shares the whole show; it is no other episode.
func TestSameEpisodeIsLeftOut(t *testing.T) {
	a := store.Recording{ID: 1, Title: "Sitcom", ProgramID: "EP1", Season: 2, Episode: 5, Listened: true, Status: "complete"}
	b := store.Recording{ID: 2, Title: "Sitcom", ProgramID: "EP1", Listened: true, Status: "complete"}
	c := store.Recording{ID: 3, Title: "Sitcom", Season: 2, Episode: 5, Listened: true, Status: "complete"}
	d := store.Recording{ID: 4, Title: "Sitcom", Season: 2, Episode: 6, Listened: true, Status: "complete"}
	e := store.Recording{ID: 5, Title: "Sitcom", Subtitle: "Pilot", Listened: true, Status: "complete"}
	f := store.Recording{ID: 6, Title: "Sitcom", Subtitle: "pilot ", Listened: true, Status: "complete"}
	got := showSiblings([]store.Recording{a, b, c, d}, a)
	if len(got) != 1 || got[0].ID != 4 {
		t.Fatalf("%+v", got)
	}
	if got := showSiblings([]store.Recording{e, f}, e); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}
