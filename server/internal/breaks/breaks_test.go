package breaks

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func blackLog(spans ...[2]float64) string {
	var b strings.Builder
	for _, s := range spans {
		fmt.Fprintf(&b, "[blackdetect @ 0x1] black_start:%g black_end:%g black_duration:%g\n", s[0], s[1], s[1]-s[0])
	}
	return b.String()
}

func findLog(log string) []Break {
	var c Cues
	c.readLog(strings.NewReader(log))
	return Find(c)
}

// Three spots between black frames, a 30, a 15, and a 30, inside a show.
func TestFindBlackOnlyFindsARunOfSpots(t *testing.T) {
	log := blackLog(
		[2]float64{12.0, 13.1}, // a fade in the show
		[2]float64{300.0, 300.3},
		[2]float64{330.0, 330.3},
		[2]float64{345.1, 345.3},
		[2]float64{375.0, 375.4},
		[2]float64{520.0, 521.5}, // another fade
	)
	got := findLog(log)
	if len(got) != 1 || math.Abs(got[0].Start-300.15) > 0.01 || math.Abs(got[0].End-375.2) > 0.01 {
		t.Fatalf("%+v", got)
	}
	// Timing alone with three spots is offered, not skipped.
	if got[0].Confidence >= AutoSkip {
		t.Fatalf("confidence %v", got[0].Confidence)
	}
}

// The black stretches a 5.2 movie clip had (recording 7 on a real tuner):
// fades and cuts, none of them a break. They were all skipped before.
func TestFindBlackOnlyIgnoresFadesInAShow(t *testing.T) {
	log := blackLog(
		[2]float64{21.454767, 24.9249}, [2]float64{30.463767, 34.667967}, [2]float64{35.168467, 35.902533},
		[2]float64{43.777067, 44.811433}, [2]float64{49.916533, 50.6506}, [2]float64{53.2532, 54.821433},
		[2]float64{67.100367, 68.101367}, [2]float64{77.2772, 78.111367}, [2]float64{81.514767, 82.649233},
		[2]float64{188.2881, 190.156633}, [2]float64{198.731867, 201.434567},
	)
	if got := findLog(log); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// Two spots in a row are not enough: a show's own scenes can line up that way.
func TestFindBlackOnlyNeedsThreeSpots(t *testing.T) {
	log := blackLog([2]float64{100, 100.2}, [2]float64{130, 130.2}, [2]float64{160, 160.2}, [2]float64{171, 171.2})
	if got := findLog(log); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// fakeComskip puts a comskip on PATH that checks it was asked for an EDL and
// writes one where it was told to, plus the .txt it writes by default.
func fakeComskip(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
for a in "$@"; do
  case "$a" in
    --ini=*) ini="${a#--ini=}" ;;
    --output=*) out="${a#--output=}" ;;
    -*) ;;
    *) in="$a" ;;
  esac
done
grep -q '^output_edl=1$' "$ini" || exit 3
[ -n "$out" ] || exit 4
base=$(basename "$in" .ts)
touch "$base.txt"
` + body
	if err := os.WriteFile(filepath.Join(bin, "comskip"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestComskipWritesItsEDLOutsideTheRecordings(t *testing.T) {
	fakeComskip(t, `printf '129.11\t256.31\t0\n1500.5\t1620\t0\n' > "$out/$base.edl"
`)
	recs := t.TempDir()
	path := filepath.Join(recs, "show.ts")
	if err := os.WriteFile(path, []byte("ts"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := comskipBreaks(path)
	if !ok || len(got) != 2 || got[0] != (Break{Start: 129.11, End: 256.31}) || got[1] != (Break{Start: 1500.5, End: 1620}) {
		t.Fatalf("breaks %v ok %v", got, ok)
	}
	entries, _ := os.ReadDir(recs)
	if len(entries) != 1 {
		t.Fatalf("comskip left files with the recording: %v", entries)
	}
}

func TestComskipThatFailsFallsBack(t *testing.T) {
	fakeComskip(t, "exit 1\n")
	path := filepath.Join(t.TempDir(), "show.ts")
	if err := os.WriteFile(path, []byte("ts"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := comskipBreaks(path); ok {
		t.Fatalf("a failed comskip gave %v", got)
	}
}
