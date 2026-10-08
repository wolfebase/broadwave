package breaks

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// tune is a made-up theme: notes a quarter second long, with overtones.
func tune(seed int64, seconds float64, rate int) []float64 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, int(seconds*float64(rate)))
	note := rate / 4
	for at := 0; at < len(out); at += note {
		f := 220 * math.Pow(2, float64(rng.Intn(24))/12)
		g := 330 * math.Pow(2, float64(rng.Intn(24))/12)
		for k := 0; k < note && at+k < len(out); k++ {
			t := float64(k) / float64(rate)
			env := math.Min(1, float64(k)/200) * math.Exp(-3*t)
			out[at+k] = 0.25 * env * (math.Sin(2*math.Pi*f*t) + 0.5*math.Sin(4*math.Pi*f*t) + 0.6*math.Sin(2*math.Pi*g*t))
		}
	}
	return out
}

// talk stands in for a show: noise that comes and goes like syllables.
func talk(seed int64, seconds float64, rate int) []float64 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, int(seconds*float64(rate)))
	prev := 0.0
	for i := range out {
		syllable := 0.5 + 0.5*math.Sin(2*math.Pi*4*float64(i)/float64(rate)+float64(i/rate))
		prev = 0.7*prev + 0.3*rng.NormFloat64()
		out[i] = 0.2 * syllable * prev
	}
	return out
}

func join(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func pcm(samples []float64, noise float64, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	out := make([]byte, 2*len(samples))
	for i, v := range samples {
		v += noise * rng.NormFloat64()
		binary.LittleEndian.PutUint16(out[2*i:], uint16(int16(max(-1, min(1, v))*32767)))
	}
	return out
}

func soundOf(t *testing.T, samples []float64, seed int64) []uint32 {
	t.Helper()
	prints, err := PrintSound(bytes.NewReader(pcm(samples, 0.003, seed)))
	if err != nil {
		t.Fatal(err)
	}
	return prints
}

// near is within a second; the ends of a stretch blur over a frame or two.
func near(got, want float64) bool { return math.Abs(got-want) <= 1 }

// Two episodes play the same 40 s theme after cold opens of different
// lengths, one a little off the other's hops.
func TestFindEndsFindsASharedIntro(t *testing.T) {
	theme := tune(1, 40, soundRate)
	a := soundOf(t, join(talk(2, 75, soundRate), theme, talk(3, 200, soundRate)), 10)
	b := soundOf(t, join(talk(4, 130.01, soundRate), theme, talk(5, 150, soundRate)), 11)
	got := FindEnds(Episode{Sound: Sound{Head: a}}, []Episode{{Sound: Sound{Head: b}}})
	if !near(got.IntroStart, 75) || !near(got.IntroEnd, 115) {
		t.Fatalf("%+v", got)
	}
	if got.Credits != 0 {
		t.Fatalf("credits %+v", got)
	}
}

func TestFindEndsFindsNothingInUnrelatedShows(t *testing.T) {
	a := soundOf(t, join(talk(2, 60, soundRate), tune(6, 40, soundRate), talk(3, 100, soundRate)), 10)
	b := soundOf(t, join(talk(4, 60, soundRate), tune(7, 40, soundRate), talk(5, 100, soundRate)), 11)
	if got := FindEnds(Episode{Sound: Sound{Head: a}}, []Episode{{Sound: Sound{Head: b}}}); got != (Ends{}) {
		t.Fatalf("%+v", got)
	}
}

// An ad both episodes carried is no intro, when the break scan found the break.
func TestFindEndsSkipsAnAdInABreak(t *testing.T) {
	ad := tune(8, 30, soundRate)
	a := soundOf(t, join(talk(2, 100, soundRate), ad, talk(3, 100, soundRate)), 10)
	b := soundOf(t, join(talk(4, 50, soundRate), ad, talk(5, 100, soundRate)), 11)
	self := Episode{Sound: Sound{Head: a}, Breaks: []Break{{Start: 95, End: 135}}}
	if got := FindEnds(self, []Episode{{Sound: Sound{Head: b}}}); got != (Ends{}) {
		t.Fatalf("%+v", got)
	}
	// The other episode's break counts as well.
	self.Breaks = nil
	other := Episode{Sound: Sound{Head: b}, Breaks: []Break{{Start: 45, End: 85}}}
	if got := FindEnds(self, []Episode{other}); got != (Ends{}) {
		t.Fatalf("%+v", got)
	}
}

// Three episodes: the intro plays in all of them, an earlier and longer ad
// in two.
func TestFindEndsPrefersWhatEveryEpisodeShares(t *testing.T) {
	theme, ad := tune(1, 20, soundRate), tune(8, 60, soundRate)
	a := soundOf(t, join(talk(2, 40, soundRate), ad, talk(3, 30, soundRate), theme, talk(9, 60, soundRate)), 10)
	b := soundOf(t, join(talk(4, 20, soundRate), ad, talk(5, 50, soundRate), theme, talk(12, 60, soundRate)), 11)
	c := soundOf(t, join(talk(6, 70, soundRate), theme, talk(7, 200, soundRate)), 12)
	others := []Episode{{Sound: Sound{Head: b}}, {Sound: Sound{Head: c}}}
	got := FindEnds(Episode{Sound: Sound{Head: a}}, others)
	if !near(got.IntroStart, 130) || !near(got.IntroEnd, 150) {
		t.Fatalf("%+v", got)
	}
	// With one other episode, the earlier stretch wins.
	got = FindEnds(Episode{Sound: Sound{Head: a}}, others[:1])
	if !near(got.IntroStart, 40) {
		t.Fatalf("%+v", got)
	}
}

// The closing theme marks the end titles; the next show's intro in the late
// padding does not, though both episodes carry it.
func TestFindEndsFindsEndTitlesBeforeTheNextShow(t *testing.T) {
	outro, next := tune(13, 25, soundRate), tune(14, 30, soundRate)
	a := soundOf(t, join(talk(2, 200, soundRate), outro, talk(3, 15, soundRate), next, talk(9, 60, soundRate)), 10)
	b := soundOf(t, join(talk(4, 150, soundRate), outro, talk(5, 15, soundRate), next, talk(12, 90, soundRate)), 11)
	self := Episode{Sound: Sound{Tail: a, TailFrom: 3000}, ShowEnd: 3240}
	other := Episode{Sound: Sound{Tail: b, TailFrom: 1000}, ShowEnd: 1190}
	got := FindEnds(self, []Episode{other})
	if !near(got.Credits, 3200) {
		t.Fatalf("%+v", got)
	}
	// With no end titles found, the show's end is where they would be.
	got = FindEnds(Episode{Sound: Sound{Tail: a[:3000], TailFrom: 3000}, ShowEnd: 3240}, []Episode{other})
	if got.Credits != 3240 {
		t.Fatalf("%+v", got)
	}
}

// The same theme through a broadcast codec at 48 kHz, read back by ffmpeg.
func TestListenEndsThroughACodec(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	const rate = 48000
	theme := tune(1, 30, rate)
	dir := t.TempDir()
	write := func(name string, samples []float64) string {
		path := filepath.Join(dir, name)
		cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "s16le", "-ar", "48000", "-ac", "1", "-i", "pipe:0",
			"-c:a", "ac3", "-b:a", "192k", "-f", "mpegts", path)
		cmd.Stdin = bytes.NewReader(pcm(samples, 0.003, int64(len(name))))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		return path
	}
	a := write("a.ts", join(talk(2, 45, rate), theme, talk(3, 60, rate)))
	b := write("b.ts", join(talk(4, 100.3, rate), theme, talk(5, 30, rate)))
	sa, err := ListenEnds(context.Background(), ffmpeg, a, 135)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := ListenEnds(context.Background(), ffmpeg, b, 160.3)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Head) < 2000 || len(sa.Tail) != 0 {
		t.Fatalf("head %d tail %d", len(sa.Head), len(sa.Tail))
	}
	got := FindEnds(Episode{Sound: sa}, []Episode{{Sound: sb}})
	// The codec's start-up delay shifts both files alike.
	if math.Abs(got.IntroStart-45) > 0.6 || math.Abs(got.IntroEnd-75) > 0.6 {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(a); err != nil {
		t.Fatal(err)
	}
}

// The show before ends in the early padding every week; it is not the intro.
func TestFindEndsLeavesOutTheEarlyPadding(t *testing.T) {
	before, theme := tune(15, 30, soundRate), tune(1, 20, soundRate)
	a := soundOf(t, join(talk(2, 20, soundRate), before, talk(3, 40, soundRate), theme, talk(9, 60, soundRate)), 10)
	b := soundOf(t, join(talk(4, 25, soundRate), before, talk(5, 70, soundRate), theme, talk(12, 60, soundRate)), 11)
	self := Episode{Sound: Sound{Head: a}, ShowStart: 60, ShowEnd: 1800}
	other := Episode{Sound: Sound{Head: b}, ShowStart: 60, ShowEnd: 1800}
	if got := FindEnds(self, []Episode{other}); !near(got.IntroStart, 90) || !near(got.IntroEnd, 110) {
		t.Fatalf("%+v", got)
	}
	// Without the listing, the earlier stretch wins.
	self.ShowEnd = 0
	if got := FindEnds(self, []Episode{other}); !near(got.IntroStart, 20) {
		t.Fatalf("%+v", got)
	}
}
