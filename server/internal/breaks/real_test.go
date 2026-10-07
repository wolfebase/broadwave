package breaks

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// realTruth is the hand-labelled breaks of two real 75-minute recordings
// (local morning news on two networks). Their cues are not in the repo;
// BREAKS_REAL names a folder with <name>.log and <name>.gray from scanArgs.
var realTruth = map[string][]span{
	"news-a": {{130, 255}, {655, 810}, {1010, 1170}, {1555, 1680}, {1930, 2060}, {2450, 2570}, {2775, 2970}, {3450, 3610}, {3705, 3840}},
	"news-b": {{0, 90}, {225, 345}, {485, 635}, {760, 880}, {1085, 1220}, {1655, 1765}, {1885, 2045}, {2630, 2730}, {2820, 2950}, {3700, 3880}, {4260, 4440}},
}

func loadReal(t *testing.T, name string) Cues {
	dir := os.Getenv("BREAKS_REAL")
	if dir == "" {
		t.Skip("BREAKS_REAL is not set")
	}
	var c Cues
	logf, err := os.Open(filepath.Join(dir, name+".log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logf.Close()
	c.readLog(logf)
	gray, err := os.Open(filepath.Join(dir, name+".gray"))
	if err != nil {
		t.Fatal(err)
	}
	defer gray.Close()
	frames, err := readFrames(gray)
	if err != nil {
		t.Fatal(err)
	}
	c.addFrames(frames)
	return c
}

func inside(spans []span, t float64) bool {
	for _, s := range spans {
		if t >= s.Start && t < s.End {
			return true
		}
	}
	return false
}

func TestRealLogo(t *testing.T) {
	for name, truth := range realTruth {
		c := loadReal(t, name)
		if c.Logo == nil {
			t.Logf("%s: no logo", name)
			continue
		}
		var adOn, adOff, showOn, showOff int
		for i, on := range c.Logo {
			ad := inside(truth, float64(i))
			switch {
			case ad && on:
				adOn++
			case ad:
				adOff++
			case on:
				showOn++
			default:
				showOff++
			}
		}
		t.Logf("%s: ads logo on %d off %d, show logo on %d off %d", name, adOn, adOff, showOn, showOff)
		line := ""
		for i := 0; i < len(c.Logo); i += 10 {
			ch := "."
			if c.Logo[i] {
				ch = "L"
			}
			if inside(truth, float64(i)) {
				if ch == "L" {
					ch = "x"
				} else {
					ch = "_"
				}
			}
			line += ch
		}
		t.Log(line)
	}
}

func TestRealScore(t *testing.T) {
	cues := map[string]Cues{}
	for name := range realTruth {
		cues[name] = loadReal(t, name)
	}
	for name, truth := range realTruth {
		// Alone, then after the other recording was scanned.
		var other []Spot
		for o := range realTruth {
			if o != name {
				other = score(cues[o], nil, nil, false).Spots
				for i := range other {
					other[i].Start = -1
				}
			}
		}
		for _, known := range [][]Spot{nil, other} {
			res := score(cues[name], known, nil, false)
			t.Logf("%s, %d known spots: %s; sure only: %s; new spots %d", name, len(known),
				measure(truth, res.Breaks, cues[name].Length), measure(truth, sure(res.Breaks), cues[name].Length), len(res.Spots))
			for _, b := range res.Breaks {
				t.Logf("  %7.1f %7.1f %.2f %s", b.Start, b.End, b.Confidence, hitMark(truth, b))
			}
		}
	}
}

func sure(found []Break) []Break {
	var out []Break
	for _, b := range found {
		if b.Confidence >= AutoSkip {
			out = append(out, b)
		}
	}
	return out
}

func hitMark(truth []span, b Break) string {
	for _, s := range truth {
		if min(s.End, b.End)-max(s.Start, b.Start) > 0 {
			return ""
		}
	}
	return "FALSE"
}

// measure reads like the J0.136 measurement: per-second recall and
// precision, show seconds skipped, and breaks found.
func measure(truth []span, found []Break, length float64) string {
	var tp, ad, det int
	for t := 0.0; t < length; t++ {
		a := inside(truth, t)
		d := false
		for _, b := range found {
			if t >= b.Start && t < b.End {
				d = true
			}
		}
		if a {
			ad++
		}
		if d {
			det++
		}
		if a && d {
			tp++
		}
	}
	hits := 0
	for _, s := range truth {
		for _, b := range found {
			if min(s.End, b.End)-max(s.Start, b.Start) > 0.5*(s.End-s.Start) {
				hits++
				break
			}
		}
	}
	return fmt.Sprintf("recall %.3f, show skipped %d s, ads missed %d s, breaks %d/%d", float64(tp)/float64(max(ad, 1)), det-tp, ad-tp, hits, len(truth))
}
