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

// A library file's modification time is not a broadcast start. A listing of
// the same title that happens to fall then is not inside the file, and the
// intro the episodes share is kept.
func TestALibraryFileIgnoresAListingOfTheSameName(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	modified := time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)
	if err := st.ReplaceAirings(ctx, []store.Airing{{
		ChannelID: 4, Title: "Harbor", Start: modified.Add(10 * time.Minute), End: modified.Add(25 * time.Minute),
	}}); err != nil {
		t.Fatal(err)
	}
	theme := prints(1, 30, nil, 0)
	heads := map[string][]uint32{
		"/lib/one.ts": prints(3, 1200, theme, 90),
		"/lib/two.ts": prints(4, 1200, theme, 200),
	}
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path]}, nil
	}
	t.Cleanup(func() { listenFile = old })
	add := func(path string, episode int) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Harbor", Path: path, Status: "complete",
			Season: 1, Episode: episode, StartedAt: modified,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 1800); err != nil {
			t.Fatal(err)
		}
		return id
	}
	one := add("/lib/one.ts", 1)
	if from, to := showBounds(ctx, st, store.Recording{Title: "Harbor", StartedAt: modified}, 1800); from != 0 || to != 0 {
		t.Fatalf("library file took a listing: %v %v", from, to)
	}
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, one)); err != nil {
		t.Fatal(err)
	}
	two := add("/lib/two.ts", 2)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, two)); err != nil {
		t.Fatal(err)
	}
	for id, start := range map[int64]float64{one: 90, two: 200} {
		got, _ := st.Recording(ctx, id)
		if math.Abs(got.IntroStart-start) > 1 || math.Abs(got.IntroEnd-start-30) > 1 {
			t.Fatalf("recording %d: intro %v-%v, want %v", id, got.IntroStart, got.IntroEnd, start)
		}
	}
}

// The episodes share a closing theme and nothing else. The one listened to
// first has no intro, and the end titles are still marked on it.
func TestFindEpisodeEndsFillsCreditsOnTheEarlierEpisode(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	outro := prints(9, 30, nil, 0)
	heads := map[string][]uint32{
		"/lib/one.ts": prints(3, 200, nil, 0),
		"/lib/two.ts": prints(4, 200, nil, 0),
	}
	tails := map[string][]uint32{
		"/lib/one.ts": prints(7, 120, outro, 0),
		"/lib/two.ts": prints(8, 120, outro, 0),
	}
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path], Tail: tails[path], TailFrom: 3000}, nil
	}
	t.Cleanup(func() { listenFile = old })
	add := func(path string, episode int) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Harbor", Path: path, Status: "complete",
			Season: 1, Episode: episode, StartedAt: time.Now().AddDate(0, 0, -episode),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 3600); err != nil {
			t.Fatal(err)
		}
		return id
	}
	one := add("/lib/one.ts", 1)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, one)); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Recording(ctx, one); got.CreditsStart != 0 || got.IntroEnd != 0 {
		t.Fatalf("first listen: intro %v credits %v", got.IntroEnd, got.CreditsStart)
	}
	two := add("/lib/two.ts", 2)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, two)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{one, two} {
		got, _ := st.Recording(ctx, id)
		if math.Abs(got.CreditsStart-3000) > 1 {
			t.Fatalf("recording %d: credits %v, want 3000", id, got.CreditsStart)
		}
		if got.IntroEnd != 0 {
			t.Fatalf("recording %d: intro %v, want none", id, got.IntroEnd)
		}
	}
}

// An episode that already has its intro still gets the end titles when a
// later episode shares only the closing theme. The intro stays where it was.
func TestFindEpisodeEndsFillsCreditsAfterTheIntroIsSet(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	theme := prints(1, 30, nil, 0)
	outro := prints(9, 30, nil, 0)
	heads := map[string][]uint32{
		"/lib/one.ts":   prints(3, 1200, theme, 90),
		"/lib/two.ts":   prints(4, 1200, theme, 200),
		"/lib/three.ts": prints(5, 200, nil, 0),
	}
	tails := map[string][]uint32{
		"/lib/one.ts":   prints(7, 120, outro, 0),
		"/lib/two.ts":   prints(8, 120, nil, 0),
		"/lib/three.ts": prints(6, 120, outro, 0),
	}
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path], Tail: tails[path], TailFrom: 3000}, nil
	}
	t.Cleanup(func() { listenFile = old })
	add := func(path string, episode int) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Harbor", Path: path, Status: "complete",
			Season: 1, Episode: episode, StartedAt: time.Now().AddDate(0, 0, -episode),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 3600); err != nil {
			t.Fatal(err)
		}
		return id
	}
	one := add("/lib/one.ts", 1)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, one)); err != nil {
		t.Fatal(err)
	}
	two := add("/lib/two.ts", 2)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, two)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Recording(ctx, one)
	if math.Abs(got.IntroStart-90) > 1 || math.Abs(got.IntroEnd-120) > 1 || got.CreditsStart != 0 {
		t.Fatalf("before the closing theme: intro %v-%v credits %v", got.IntroStart, got.IntroEnd, got.CreditsStart)
	}
	three := add("/lib/three.ts", 3)
	if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, three)); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Recording(ctx, one)
	if math.Abs(got.IntroStart-90) > 1 || math.Abs(got.IntroEnd-120) > 1 {
		t.Fatalf("intro moved: %v-%v", got.IntroStart, got.IntroEnd)
	}
	if math.Abs(got.CreditsStart-3000) > 1 {
		t.Fatalf("recording %d: credits %v, want 3000", one, got.CreditsStart)
	}
	got, _ = st.Recording(ctx, two)
	if math.Abs(got.IntroStart-200) > 1 || math.Abs(got.IntroEnd-230) > 1 || got.CreditsStart != 0 {
		t.Fatalf("second episode: intro %v-%v credits %v", got.IntroStart, got.IntroEnd, got.CreditsStart)
	}
	got, _ = st.Recording(ctx, three)
	if got.IntroEnd != 0 || math.Abs(got.CreditsStart-3000) > 1 {
		t.Fatalf("third episode: intro %v credits %v", got.IntroEnd, got.CreditsStart)
	}
}

// Two copies of one episode are one vote. A logo only those copies share
// does not beat the intro the other episodes share.
func TestFindEpisodeEndsCountsOneCopyOfAnEpisode(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	logo := prints(1, 30, nil, 0)
	theme := prints(2, 30, nil, 0)
	selfHead := prints(3, 400, logo, 0)
	themeAt := 100.0
	copy(selfHead[int(themeAt/breaks.HopSeconds):], theme)
	heads := map[string][]uint32{
		"/lib/a.ts": prints(4, 400, logo, 0),
		"/lib/b.ts": prints(5, 400, logo, 0),
		"/lib/c.ts": prints(6, 400, theme, 100),
		"/lib/d.ts": prints(7, 400, theme, 100),
		"/lib/e.ts": selfHead,
	}
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path]}, nil
	}
	t.Cleanup(func() { listenFile = old })
	when := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	add := func(path string, episode, day int) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Harbor", Path: path, Status: "complete",
			Season: 1, Episode: episode, StartedAt: when.AddDate(0, 0, day),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 1800); err != nil {
			t.Fatal(err)
		}
		return id
	}
	listen := func(id int64) {
		t.Helper()
		if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, id)); err != nil {
			t.Fatal(err)
		}
	}
	listen(add("/lib/a.ts", 1, -4))
	listen(add("/lib/b.ts", 1, -3))
	listen(add("/lib/c.ts", 2, -2))
	listen(add("/lib/d.ts", 3, -1))
	self := add("/lib/e.ts", 5, 0)
	listen(self)
	got, _ := st.Recording(ctx, self)
	if math.Abs(got.IntroStart-100) > 1 || math.Abs(got.IntroEnd-130) > 1 {
		t.Fatalf("intro %v-%v, want 100-130", got.IntroStart, got.IntroEnd)
	}
}

// Two copies of one episode were listened to before any other episode. The
// next episode fills in the intro on both copies, not only the closer one.
func TestFindEpisodeEndsFillsEveryCopyOfAnEpisode(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	theme := prints(1, 30, nil, 0)
	heads := map[string][]uint32{
		"/lib/a.ts": prints(3, 1200, theme, 90),
		"/lib/b.ts": prints(3, 1200, theme, 90),
		"/lib/c.ts": prints(4, 1200, theme, 200),
	}
	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path]}, nil
	}
	t.Cleanup(func() { listenFile = old })
	when := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	listen := func(path string, episode, day int) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Harbor", Path: path, Status: "complete",
			Season: 1, Episode: episode, StartedAt: when.AddDate(0, 0, day),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 1800); err != nil {
			t.Fatal(err)
		}
		if err := FindEpisodeEnds(ctx, st, "", mustRec(t, st, id)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := listen("/lib/a.ts", 1, -3)
	b := listen("/lib/b.ts", 1, -2)
	c := listen("/lib/c.ts", 2, 0)
	for id, start := range map[int64]float64{a: 90, b: 90, c: 200} {
		got, _ := st.Recording(ctx, id)
		if math.Abs(got.IntroStart-start) > 1 || math.Abs(got.IntroEnd-start-30) > 1 {
			t.Fatalf("recording %d: intro %v-%v, want %v", id, got.IntroStart, got.IntroEnd, start)
		}
	}
}

func mustRec(t *testing.T, st *store.Store, id int64) store.Recording {
	t.Helper()
	rec, err := st.Recording(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return rec
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
	got, copies := showSiblings([]store.Recording{a, b, c, d}, a)
	if len(got) != 1 || got[0].ID != 4 || len(copies) != 0 {
		t.Fatalf("%+v %+v", got, copies)
	}
	if got, copies := showSiblings([]store.Recording{e, f}, e); len(got) != 0 || len(copies) != 0 {
		t.Fatalf("%+v %+v", got, copies)
	}
}

// Two copies of one episode are one vote. The closest copy votes, the copies
// do not take every slot from another episode, and the rest are still listed
// to be filled in, up to maxEpisodes.
func TestShowSiblingsCountsOneCopy(t *testing.T) {
	now := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	self := store.Recording{ID: 5, Title: "Sitcom", Season: 1, Episode: 5, Listened: true, Status: "complete", StartedAt: now}
	recs := []store.Recording{self}
	for i := int64(1); i <= 9; i++ {
		recs = append(recs, store.Recording{
			ID: 10 + i, Title: "Sitcom", Season: 1, Episode: 1,
			Listened: true, Status: "complete", StartedAt: now.Add(time.Duration(i) * time.Hour),
		})
	}
	recs = append(recs, store.Recording{
		ID: 2, Title: "Sitcom", Season: 1, Episode: 2,
		Listened: true, Status: "complete", StartedAt: now.Add(30 * time.Minute),
	})
	got, copies := showSiblings(recs, self)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 11 {
		t.Fatalf("%+v", got)
	}
	if len(copies) != maxEpisodes || copies[0].ID != 12 || copies[maxEpisodes-1].ID != 19 {
		t.Fatalf("copies %+v", copies)
	}
}
