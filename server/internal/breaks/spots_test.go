package breaks

import (
	"math/rand/v2"
	"testing"
)

// randomPrints are textured prints, as a moving picture gives.
func randomPrints(r *rand.Rand, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		for !textured(out[i]) {
			out[i] = r.Uint64()
		}
	}
	return out
}

// playAt copies a spot into a recording's prints with a few bits off per
// second and a few seconds lost, as a second airing looks.
func playAt(r *rand.Rand, prints []uint64, spot []uint64, at int) {
	for i, p := range spot {
		if i%5 == 4 {
			continue
		}
		prints[at+i] = p ^ (1 << r.IntN(64)) ^ (1 << r.IntN(64))
	}
}

// A 30 s ad seen in another recording plays with the logo off where the cues
// alone found no break: it is skipped.
func TestAKnownSpotMarksABreakTheCuesMissed(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	ad := randomPrints(r, 30)
	c := Cues{Length: 1200, Prints: randomPrints(r, 1200), Logo: logoOff(1200, span{400, 430})}
	if got := score(c, nil, nil, false).Breaks; len(got) != 0 {
		t.Fatalf("found without the spot: %+v", got)
	}
	playAt(r, c.Prints, ad, 400)
	res := score(c, []Spot{{Prints: ad, Start: -1}}, nil, false)
	if len(res.Breaks) != 1 || res.Breaks[0].Start != 400 || res.Breaks[0].End != 430 || res.Breaks[0].Confidence < AutoSkip {
		t.Fatalf("%+v", res.Breaks)
	}
	if len(res.Seen) != 1 || !res.Seen[0] {
		t.Fatalf("seen %v", res.Seen)
	}
}

// The same pictures under the station logo are the show (a recap, an open
// that matched): not a break. Without a logo to check, a lone known spot is
// only offered.
func TestAKnownSpotNeedsTheLogoToSkip(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	ad := randomPrints(r, 30)
	c := Cues{Length: 1200, Prints: randomPrints(r, 1200), Logo: logoOff(1200)}
	playAt(r, c.Prints, ad, 400)
	known := []Spot{{Prints: ad, Start: -1}}
	if got := score(c, known, nil, false).Breaks; len(got) != 0 {
		t.Fatalf("under the logo: %+v", got)
	}
	c.Logo = nil
	got := score(c, known, nil, false).Breaks
	if len(got) != 1 || got[0].Confidence >= AutoSkip {
		t.Fatalf("no logo: %+v", got)
	}
}

// Spots of a sure break are learned, once each, and an ad from it that runs
// again in the same recording marks that break too.
func TestSureBreaksTeachTheirSpots(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	c := spotCuts(600, 630, 645, 675, 705)
	c.Length = 1800
	c.Logo = logoOff(1800, span{601, 704}, span{1200, 1230})
	c.Prints = randomPrints(r, 1800)
	// Still black frames for the spot at 630-645: not worth keeping.
	for i := 631; i < 644; i++ {
		c.Prints[i] = 0
	}
	ad := append([]uint64(nil), c.Prints[601:629]...)
	playAt(r, c.Prints, ad, 1201)
	res := score(c, nil, nil, false)
	if len(res.Breaks) != 2 || res.Breaks[1].Start != 1201 || res.Breaks[1].Confidence < AutoSkip {
		t.Fatalf("%+v", res.Breaks)
	}
	if len(res.Spots) != 3 {
		t.Fatalf("learned %d spots", len(res.Spots))
	}
	for _, s := range res.Spots {
		if s.Start == 631 || s.Start == 1202 {
			t.Fatalf("kept %d", s.Start)
		}
	}
	// Scanned again with them in the library, nothing is new.
	again := score(c, asOther(res.Spots), nil, false)
	if len(again.Spots) != 0 {
		t.Fatalf("relearned %d", len(again.Spots))
	}
}

func TestSpotsDoNotMatchStillPictures(t *testing.T) {
	if usable(make([]uint64, 30)) {
		t.Fatal("black is usable")
	}
	r := rand.New(rand.NewPCG(7, 8))
	card := randomPrints(r, 1)[0]
	still := make([]uint64, 30)
	for i := range still {
		still[i] = card
	}
	if usable(still) {
		t.Fatal("a still card is usable")
	}
	if !usable(randomPrints(r, 30)) {
		t.Fatal("a moving picture is not usable")
	}
}

// A known spot that plays under the logo just after a sure break is the show:
// the break keeps its end.
func TestAKnownSpotUnderTheLogoDoesNotStretchABreak(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	c := spotCuts(600, 630, 645, 675, 705)
	c.Length = 1800
	c.Logo = logoOff(1800, span{601, 704})
	c.Prints = randomPrints(r, 1800)
	teaser := randomPrints(r, 20)
	playAt(r, c.Prints, teaser, 720)
	got := score(c, []Spot{{Prints: teaser, Start: -1}}, nil, false).Breaks
	if len(got) != 1 || got[0].End != 705 {
		t.Fatalf("%+v", got)
	}
}

// One ad matched twice over the same seconds counts once.
func TestKnownShareCountsASecondOnce(t *testing.T) {
	b := Break{Start: 0, End: 60}
	if got := knownShare([]span{{0, 30}, {0, 30}, {10, 30}}, b); got != 0.5 {
		t.Fatalf("share %v", got)
	}
}

// A break made sure only by known spots teaches nothing: a piece of the show
// that once got in cannot vouch for itself the next week.
func TestABreakSureOnlyByKnownSpotsTeachesNothing(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 14))
	// Titles at 400-430 that once got in, then 15 s of show cut like a spot,
	// with the logo back for the last 5 s: unsure on its own cues.
	c := spotCuts(400, 430, 445)
	c.Length = 1200
	c.Logo = logoOff(1200, span{400, 440})
	c.Prints = randomPrints(r, 1200)
	if got := score(c, nil, nil, false).Breaks; len(got) != 1 || got[0].Confidence >= AutoSkip {
		t.Fatalf("alone: %+v", got)
	}
	titles := append([]uint64(nil), c.Prints[401:429]...)
	res := score(c, []Spot{{Prints: titles, Start: -1}}, nil, false)
	if len(res.Breaks) != 1 || res.Breaks[0].Confidence < AutoSkip {
		t.Fatalf("%+v", res.Breaks)
	}
	if len(res.Spots) != 0 {
		t.Fatalf("learned %d spots", len(res.Spots))
	}
}
