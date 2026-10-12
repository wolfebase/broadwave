package breaks

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// scorecard is one signal's precision and recall against labelled breaks.
// Seconds of the show that the signal called a break are showSkipped. autoShow
// is the part of that a player would skip on its own.
type scorecard struct {
	signal                string
	precision, recall, f1 float64
	showSkipped, autoShow int
	hits, truthN, predN   int
}

func (s scorecard) String() string {
	return fmt.Sprintf("%-9s precision %.3f  recall %.3f  f1 %.3f  show %ds  auto-skip show %ds  breaks %d/%d (%d proposed)",
		s.signal, s.precision, s.recall, s.f1, s.showSkipped, s.autoShow, s.hits, s.truthN, s.predN)
}

// scoreBreaks scores found breaks against labelled truth, one second at a time.
// A second counts only when a span covers most of it, so a cut a frame past
// the label is not a skipped second of the show.
func scoreBreaks(signal string, truth []span, found []Break, length float64) scorecard {
	tp, fp, fn := 0, 0, 0
	auto := 0
	for sec := 0; sec < int(length); sec++ {
		ad := coverSpan(truth, sec) >= 0.5
		det, sure := coverBreak(found, sec)
		switch {
		case ad && det:
			tp++
		case ad:
			fn++
		case det:
			fp++
			if sure {
				auto++
			}
		}
	}
	p, r := 1.0, 1.0
	if tp+fp > 0 {
		p = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		r = float64(tp) / float64(tp+fn)
	}
	f1 := 0.0
	if p+r > 0 {
		f1 = 2 * p * r / (p + r)
	}
	return scorecard{
		signal: signal, precision: p, recall: r, f1: f1,
		showSkipped: fp, autoShow: auto,
		hits: breakHits(truth, found), truthN: len(truth), predN: len(found),
	}
}

// breakHits is how many labelled breaks overlap a found break by half their length.
func coverSpan(spans []span, sec int) float64 {
	from, to := float64(sec), float64(sec+1)
	n := 0.0
	for _, s := range spans {
		n += max(0, min(to, s.End)-max(from, s.Start))
	}
	return n
}

func coverBreak(found []Break, sec int) (hit, sure bool) {
	from, to := float64(sec), float64(sec+1)
	var n, auto float64
	for _, b := range found {
		o := max(0, min(to, b.End)-max(from, b.Start))
		n += o
		if b.Confidence >= AutoSkip {
			auto += o
		}
	}
	return n >= 0.5, auto >= 0.5
}

func breakHits(truth []span, found []Break) int {
	n := 0
	for _, s := range truth {
		for _, b := range found {
			if min(s.End, b.End)-max(s.Start, b.Start) >= 0.5*(s.End-s.Start) {
				n++
				break
			}
		}
	}
	return n
}

// blackBreaks, silenceBreaks, cutBreaks, and logoBreaks are each signal on
// its own, before the combiner. The report is how far each one is from the
// labelled breaks, and what Find does with all of them.
func blackBreaks(c Cues) []Break {
	return Find(Cues{Blacks: c.Blacks, Length: c.Length})
}

func silenceBreaks(c Cues) []Break {
	return chainsOf(boundaries(Cues{Silences: c.Silences, Cuts: c.Cuts, Length: c.Length}))
}

func cutBreaks(c Cues) []Break {
	var cuts []boundary
	for _, at := range c.Cuts {
		cuts = append(cuts, boundary{At: at, Grid: at, Strength: 0.7})
	}
	return chainsOf(cuts)
}

func logoBreaks(c Cues) []Break {
	var out []Break
	for _, g := range logoGaps(c.Logo) {
		out = append(out, Break{Start: g.Start, End: g.End})
	}
	return out
}

func chainsOf(cuts []boundary) []Break {
	var spans []Break
	for _, chain := range spotChains(cuts) {
		spans = append(spans, Break{Start: chain[0], End: chain[len(chain)-1]})
	}
	return merge(spans, nil)
}

func TestScoreCountsAPartialBreak(t *testing.T) {
	truth := []span{{10, 20}}
	found := []Break{{Start: 9, End: 15, Confidence: 0.9}, {Start: 30, End: 32, Confidence: 0.5}}
	got := scoreBreaks("combined", truth, found, 40)
	// Seconds 10–14 are a hit, 9 and 30–31 are show, 15–19 are missed.
	if got.showSkipped != 3 || got.autoShow != 1 || got.hits != 1 {
		t.Fatalf("%s", got)
	}
	if got.precision < 0.62 || got.precision > 0.63 || got.recall < 0.49 || got.recall > 0.51 {
		t.Fatalf("%s", got)
	}
}

// A cue-level broadcast: two real breaks, a hard-cut break, a short return
// to the show, a logo-on run of black frames, and one dark fade. Each signal
// is scored on its own. The combiner has to beat every signal on F1 and must
// not auto-skip the show.
func TestSignalsOnACueBroadcast(t *testing.T) {
	const length = 900
	c := Cues{Length: length, Logo: logoOff(length)}
	addBlack := func(at float64) {
		c.Blacks = append(c.Blacks, span{at - 0.2, at + 0.2})
		c.Silences = append(c.Silences, span{at - 0.25, at + 0.25})
		c.Cuts = append(c.Cuts, at)
	}
	for _, at := range []float64{100, 130, 160, 190} {
		addBlack(at)
		for s := int(at); s < int(at)+30 && s < 190; s++ {
			c.Logo[s] = false
		}
	}
	// 30 s of show, logo on, then another pod.
	for _, at := range []float64{220, 250, 280, 310} {
		addBlack(at)
		for s := int(at); s < int(at)+30 && s < 310; s++ {
			c.Logo[s] = false
		}
	}
	// Hard cuts, logo off, no black and no silence.
	for _, at := range []float64{400, 415, 430, 445, 460} {
		c.Cuts = append(c.Cuts, at)
	}
	for s := 400; s < 460; s++ {
		c.Logo[s] = false
	}
	// Black frames on a 30 s grid through the show. The logo stays on.
	for at := 520.0; at <= 640; at += 30 {
		addBlack(at)
	}
	// One fade, logo off. Not a break.
	addBlack(700)
	addBlack(740)
	for s := 700; s < 740; s++ {
		c.Logo[s] = false
	}
	// A scene cut just after the logo returns must not extend the hard-cut break.
	c.Cuts = append(c.Cuts, 460.5)

	truth := []span{{100, 190}, {220, 310}, {400, 460}}
	report := map[string]scorecard{}
	for _, row := range []struct {
		name string
		fn   func(Cues) []Break
	}{
		{"black", blackBreaks},
		{"silence", silenceBreaks},
		{"cuts", cutBreaks},
		{"logo", logoBreaks},
		{"combined", Find},
	} {
		card := scoreBreaks(row.name, truth, row.fn(c), length)
		report[row.name] = card
		t.Log(card.String())
	}
	got := report["combined"]
	if got.hits != len(truth) {
		t.Fatalf("missed a labelled break: %s", got)
	}
	if got.autoShow != 0 || got.showSkipped != 0 {
		t.Fatalf("show inside a break: %s", got)
	}
	if got.f1 < 0.9 {
		t.Fatalf("combined f1 %.3f", got.f1)
	}
	for _, name := range []string{"black", "silence", "cuts"} {
		if got.f1+0.001 < report[name].f1 {
			t.Fatalf("combined f1 %.3f is below %s", got.f1, report[name])
		}
	}
	// The logo alone calls the dark fade a break. The combiner must not.
	if report["logo"].showSkipped <= got.showSkipped {
		t.Fatalf("logo show %d, combined show %d", report["logo"].showSkipped, got.showSkipped)
	}
	for _, at := range []float64{205, 580, 720} {
		if insideBreak(Find(c), at) {
			t.Fatalf("show at %.0f was called a break", at)
		}
	}
}

func insideBreak(found []Break, at float64) bool {
	for _, b := range found {
		if at >= b.Start && at < b.End {
			return true
		}
	}
	return false
}

// TestScanStaysOneLowPriorityPass locks the CPU budget: one decode, two
// threads, and the logo frames taken after the picture is down to 1 fps.
// blackdetect and scdet stay at full frame rate, in that same pass.
func TestScanStaysOneLowPriorityPass(t *testing.T) {
	for _, sound := range []bool{true, false} {
		args := scanArgs("clip.ts", sound)
		threads, inputs, graphs := 0, 0, 0
		var graph string
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "-threads", "-filter_threads":
				if i+1 >= len(args) || args[i+1] != "2" {
					t.Fatalf("%v", args)
				}
				threads++
			case "-i":
				inputs++
			case "-filter_complex":
				graphs++
				if i+1 < len(args) {
					graph = args[i+1]
				}
			}
		}
		if threads != 2 || inputs != 1 || graphs != 1 {
			t.Fatalf("sound %v threads %d inputs %d graphs %d (%v)", sound, threads, inputs, graphs, args)
		}
		fps := strings.Index(graph, "fps=1")
		scale := strings.Index(graph, "scale=160:90")
		black := strings.Index(graph, "blackdetect=")
		scene := strings.Index(graph, "scdet=")
		if black < 0 || scene < 0 || fps < 0 || scale < 0 || !(black < scene && scene < fps && fps < scale) {
			t.Fatalf("%s", graph)
		}
		if sound != strings.Contains(graph, "silencedetect=") {
			t.Fatalf("sound %v graph %s", sound, graph)
		}
	}
}

// clip is one piece of a made-up recording.
type clip struct {
	video, audio string
	dur          float64
	commercial   bool
}

// broadcast is a labelled recording built from clips.
type broadcast struct {
	clips                []clip
	truth                []span
	classic, tease, hard float64
	trap, dark, length   float64
	breakAt              float64
	inBreak              bool
}

func (b *broadcast) add(video, audio string, dur float64, commercial bool) {
	b.clips = append(b.clips, clip{video, audio, dur, commercial})
	if commercial && !b.inBreak {
		b.breakAt = b.length
		b.inBreak = true
	}
	if !commercial && b.inBreak {
		b.truth = append(b.truth, span{b.breakAt, b.length})
		b.inBreak = false
	}
	b.length += dur
}

func showClip(d float64) (string, string) {
	v := fmt.Sprintf("testsrc2=s=160x90:r=10:d=%.3f,drawbox=x=120:y=64:w=32:h=20:color=white:t=4,format=yuv420p", d)
	a := fmt.Sprintf("sine=f=300:d=%.3f,aformat=sample_rates=48000:channel_layouts=mono", d)
	return v, a
}

func colorClip(color string, freq, d float64, silent bool) (string, string) {
	v := fmt.Sprintf("color=c=%s:s=160x90:r=10:d=%.3f,format=yuv420p", color, d)
	a := fmt.Sprintf("sine=f=%.0f:d=%.3f,aformat=sample_rates=48000:channel_layouts=mono", freq, d)
	if silent {
		a = fmt.Sprintf("anullsrc=r=48000:cl=mono:d=%.3f", d)
	}
	return v, a
}

// madeBroadcast is about ten minutes: real breaks (black, and hard cuts), a
// short return to the show, black frames on a grid while the logo stays, and
// one dark fade. The logo box is on the show and off everywhere else.
func madeBroadcast() broadcast {
	var b broadcast
	addShow := func(d float64) {
		v, a := showClip(d)
		b.add(v, a, d, false)
	}
	addBlack := func(commercial bool) {
		v, a := colorClip("black", 0, 0.5, true)
		b.add(v, a, 0.5, commercial)
	}
	addSpot := func(color string, freq, d float64) {
		v, a := colorClip(color, freq, d, false)
		b.add(v, a, d, true)
	}
	addShow(40)
	addBlack(false) // a fade in the show
	addShow(39.5)

	// Break of three 15 s spots, black and silence between them.
	b.classic = b.length + 10
	addBlack(true)
	addSpot("red", 440, 14.5)
	addBlack(true)
	addSpot("blue", 550, 14.5)
	addBlack(true)
	addSpot("green", 660, 14.5)
	addBlack(true)

	addShow(30)
	b.tease = b.length - 15

	addBlack(true)
	addSpot("red", 470, 14.5)
	addBlack(true)
	addSpot("blue", 580, 14.5)
	addBlack(true)
	addSpot("yellow", 690, 14.5)
	addBlack(true)

	addShow(50)

	// Hard cuts: the picture changes and the logo is gone. No black, no silence.
	b.hard = b.length + 22
	addSpot("0xE23B2B", 700, 15)
	addSpot("0x2B6BE2", 720, 15)
	addSpot("0x2BA84A", 740, 15)
	addSpot("0xE2C12B", 760, 15)

	addShow(40)

	// The show itself, cut every 30 s with a black frame. The logo stays.
	b.trap = b.length + 45
	for i := 0; i < 4; i++ {
		addBlack(false)
		addShow(29.5)
	}
	addBlack(false)

	addShow(20)

	// A dim scene with black at each end. The logo is gone. Not a break.
	// 0x10 would be black to the scan (a pixel under a tenth of full range),
	// and the whole scene would become one black stretch.
	v, a := colorClip("0x505050", 180, 39, false)
	b.dark = b.length + 20
	addBlack(false)
	b.add(v, a, 39, false)
	addBlack(false)

	addShow(40)
	return b
}

func writeBroadcast(t *testing.T, path string, b broadcast) {
	t.Helper()
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	graph := ""
	for i, c := range b.clips {
		args = append(args, "-f", "lavfi", "-i", c.video, "-f", "lavfi", "-i", c.audio)
		graph += "[" + strconv.Itoa(2*i) + ":v][" + strconv.Itoa(2*i+1) + ":a]"
	}
	graph += "concat=n=" + strconv.Itoa(len(b.clips)) + ":v=1:a=1[v][a]"
	args = append(args, "-filter_complex", graph, "-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "10", "-c:a", "aac", "-f", "mpegts", path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
}

// TestSyntheticBroadcastScoresSignals scans one made-up recording and scores
// black, silence, scene cuts, the logo, and the combined breaks. The scan
// has to finish faster than the recording plays, and the combined breaks have
// to beat each signal without skipping the show.
func TestSyntheticBroadcastScoresSignals(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	b := madeBroadcast()
	if b.length < 300 {
		t.Fatalf("logo needs 300 s, broadcast is %.0f", b.length)
	}
	path := filepath.Join(t.TempDir(), "broadcast.ts")
	writeBroadcast(t, path, b)
	started := time.Now()
	cues, err := Scan("", path)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scan %.0f s of picture in %s (%.1f× realtime)", cues.Length, elapsed.Round(time.Millisecond), cues.Length/elapsed.Seconds())
	if elapsed > time.Duration(cues.Length*float64(time.Second)) {
		t.Fatalf("scan took %s, slower than the recording", elapsed.Round(time.Millisecond))
	}
	if cues.Logo == nil {
		t.Fatal("no logo found")
	}
	length := max(cues.Length, b.length)
	var cards []scorecard
	for _, row := range []struct {
		name string
		fn   func(Cues) []Break
	}{
		{"black", blackBreaks},
		{"silence", silenceBreaks},
		{"cuts", cutBreaks},
		{"logo", logoBreaks},
		{"combined", Find},
	} {
		card := scoreBreaks(row.name, b.truth, row.fn(cues), length)
		cards = append(cards, card)
		t.Log(card.String())
	}
	got := cards[len(cards)-1]
	if got.hits != len(b.truth) {
		t.Fatalf("missed a labelled break (%d): %s\ntruth %v", len(b.truth), got, b.truth)
	}
	if got.autoShow != 0 {
		t.Fatalf("auto-skipped %d s of the show\ntruth %v\nfound %v", got.autoShow, b.truth, Find(cues))
	}
	if got.f1 < 0.95 {
		t.Fatalf("combined f1 %.3f", got.f1)
	}
	var logo scorecard
	for _, card := range cards[:len(cards)-1] {
		if card.signal == "logo" {
			logo = card
			continue
		}
		if got.f1+0.02 < card.f1 {
			t.Fatalf("combined f1 %.3f is below %s %.3f", got.f1, card.signal, card.f1)
		}
	}
	// The logo alone includes the dim scene. The combiner leaves it in the show.
	if logo.showSkipped <= got.showSkipped {
		t.Fatalf("logo show %d s, combined %d s", logo.showSkipped, got.showSkipped)
	}
	found := Find(cues)
	for _, probe := range []struct {
		name string
		at   float64
		want bool
	}{
		{"classic break", b.classic, true},
		{"hard-cut break", b.hard, true},
		{"return to the show", b.tease, false},
		{"black frames in the show", b.trap, false},
		{"dark scene", b.dark, false},
	} {
		if insideBreak(found, probe.at) != probe.want {
			t.Fatalf("%s at %.0f s: in a break %v, want %v\n%v", probe.name, probe.at, !probe.want, probe.want, found)
		}
	}
}
