package breaks

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// logoOff is a per-second logo timeline with the logo off in the given spans.
func logoOff(length int, off ...span) []bool {
	logo := make([]bool, length)
	for i := range logo {
		logo[i] = !inside(off, float64(i))
	}
	return logo
}

// spotCuts are black-and-silent cuts at the given times.
func spotCuts(times ...float64) Cues {
	var c Cues
	for _, at := range times {
		c.Blacks = append(c.Blacks, span{at - 0.2, at + 0.2})
		c.Silences = append(c.Silences, span{at - 0.25, at + 0.25})
	}
	return c
}

func TestALogoGapTimedLikeSpotsSkips(t *testing.T) {
	c := spotCuts(600, 630, 645, 675, 705)
	c.Logo = logoOff(1200, span{601, 704})
	got := Find(c)
	if len(got) != 1 || got[0].Start != 600 || got[0].End != 705 {
		t.Fatalf("%+v", got)
	}
	if got[0].Confidence < AutoSkip {
		t.Fatalf("confidence %v", got[0].Confidence)
	}
}

// A show's own scenes can be cut a whole number of 5 s apart. With the logo on
// through them, they are not a break.
func TestSpotTimingWithTheLogoOnIsTheShow(t *testing.T) {
	c := spotCuts(300, 330, 345, 375, 405)
	c.Logo = logoOff(1200)
	if got := Find(c); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// The logo comes back 20 s after the last spot, over the show's open: the
// break ends at the cut, not where the logo returns.
func TestALateLogoEndsTheBreakAtTheCut(t *testing.T) {
	c := spotCuts(600, 630, 660)
	c.Logo = logoOff(1200, span{601, 680})
	got := Find(c)
	if len(got) != 1 || got[0].End != 660 {
		t.Fatalf("%+v", got)
	}
}

// A long stretch with no logo and no cuts is a show without a logo, like a
// network hour, not a break.
func TestNoLogoAndNoCutsIsNotABreak(t *testing.T) {
	c := spotCuts(100, 400)
	c.Logo = logoOff(1200, span{101, 399})
	if got := Find(c); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestComskipAndTheScanTogether(t *testing.T) {
	skipped := []Break{{Start: 100, End: 220}, {Start: 1000, End: 1100}}
	own := []Break{
		{Start: 101, End: 219, Confidence: 0.8},   // agrees
		{Start: 600, End: 700, Confidence: 0.9},   // comskip missed it
		{Start: 1090, End: 1130, Confidence: 0.5}, // runs past comskip's break
	}
	got := combine(skipped, own)
	want := []Break{
		{Start: 100, End: 220, Confidence: bothAgree},
		{Start: 600, End: 700, Confidence: scanOverruled},
		{Start: 1000, End: 1100, Confidence: comskipAlone},
	}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if comskipAlone < AutoSkip || scanOverruled >= AutoSkip {
		t.Fatal("comskip alone skips; the scan against comskip asks")
	}
}

// A real ffmpeg scan of a made-up recording: two minutes of show with a logo,
// three spots with black and silence between them, and the show again.
func TestScanFindsABreakInARecording(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "show.ts")
	show := "testsrc2=s=320x180:r=10:d=150,drawbox=x=270:y=140:w=36:h=24:color=white:t=4"
	spot := func(color string) string { return "color=" + color + ":s=320x180:r=10:d=14.5,noise=alls=40:allf=t" }
	black := "color=black:s=320x180:r=10:d=0.5"
	videos := []string{show, black, spot("red"), black, spot("blue"), black, spot("green"), black, show}
	sounds := []string{"sine=f=300:d=150", "anullsrc=r=48000:cl=mono:d=0.5", "sine=f=500:d=14.5", "anullsrc=r=48000:cl=mono:d=0.5",
		"sine=f=600:d=14.5", "anullsrc=r=48000:cl=mono:d=0.5", "sine=f=700:d=14.5", "anullsrc=r=48000:cl=mono:d=0.5", "sine=f=300:d=150"}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	graph := ""
	for i := range videos {
		args = append(args, "-f", "lavfi", "-i", videos[i], "-f", "lavfi", "-i", sounds[i]+",aformat=sample_rates=48000:channel_layouts=mono")
		graph += "[" + strconv.Itoa(2*i) + ":v][" + strconv.Itoa(2*i+1) + ":a]"
	}
	graph += "concat=n=" + strconv.Itoa(len(videos)) + ":v=1:a=1[v][a]"
	args = append(args, "-filter_complex", graph, "-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-g", "10", "-c:a", "aac", "-f", "mpegts", path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	cues, err := Scan("", path)
	if err != nil {
		t.Fatal(err)
	}
	if cues.Logo == nil {
		t.Fatal("no logo found")
	}
	got := Find(cues)
	if len(got) != 1 || got[0].Start < 149 || got[0].Start > 151 || got[0].End < 194 || got[0].End > 196 {
		t.Fatalf("%+v (blacks %v)", got, cues.Blacks)
	}
	if got[0].Confidence < AutoSkip {
		t.Fatalf("confidence %v", got[0].Confidence)
	}
}

// With no logo, a busy show has silent cuts a whole 5 s apart by chance.
// Only black frames time spots then, so they are not a break.
func TestNoLogoSilentCutsAreTheShow(t *testing.T) {
	var c Cues
	for at := 100.0; at < 1000; at += 15 {
		c.Silences = append(c.Silences, span{at - 0.2, at + 0.2})
		c.Cuts = append(c.Cuts, at)
	}
	if got := Find(c); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// Black frames 30 s apart for twenty minutes are not one twenty-minute break.
func TestABlackFrameChainStopsAtALongBreak(t *testing.T) {
	var times []float64
	for at := 100.0; at <= 1300; at += 30 {
		times = append(times, at)
	}
	c := spotCuts(times...)
	for _, b := range Find(c) {
		if b.End-b.Start > longestBreak+maxSpot {
			t.Fatalf("%+v", b)
		}
	}
}

// A recording that ends in a break keeps that break.
func TestABreakAtTheEndOfARecording(t *testing.T) {
	c := spotCuts(1100, 1130, 1160)
	c.Logo = logoOff(1180, span{1101, 1180})
	c.Length = 1180
	got := Find(c)
	if len(got) != 1 || got[0].Start != 1100 || got[0].End != 1180 {
		t.Fatalf("%+v", got)
	}
}

// Two breaks comskip found close together stay two: the show between them
// is not skipped.
func TestCombineKeepsNearBreaksApart(t *testing.T) {
	got := combine([]Break{{Start: 100, End: 160}}, []Break{{Start: 190, End: 250, Confidence: 0.9}})
	if len(got) != 2 || got[0].End != 160 || got[1].Start != 190 || got[1].Confidence != scanOverruled {
		t.Fatalf("%+v", got)
	}
}

// A recording with no sound is scanned for its picture.
func TestScanWithoutSound(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "quiet.ts")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=10:d=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-f", "mpegts", path).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	cues, err := Scan("", path)
	if err != nil || cues.Length < 19 {
		t.Fatalf("%v %v", cues.Length, err)
	}
}
