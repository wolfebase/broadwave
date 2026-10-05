package live

import (
	"fmt"
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

// Three spots between black frames, a 30, a 15, and a 30, inside a show.
func TestParseBreaksFindsARunOfSpots(t *testing.T) {
	log := blackLog(
		[2]float64{12.0, 13.1}, // a fade in the show
		[2]float64{300.0, 300.3},
		[2]float64{330.0, 330.3},
		[2]float64{345.1, 345.3},
		[2]float64{375.0, 375.4},
		[2]float64{520.0, 521.5}, // another fade
	)
	got := ParseBreaks(log)
	if len(got) != 1 || got[0].Start != 300.0 || got[0].End != 375.4 {
		t.Fatalf("%+v", got)
	}
}

// The black stretches a 5.2 movie clip had (recording 7 on a real tuner):
// fades and cuts, none of them a break. They were all skipped before.
func TestParseBreaksIgnoresFadesInAShow(t *testing.T) {
	log := blackLog(
		[2]float64{21.454767, 24.9249}, [2]float64{30.463767, 34.667967}, [2]float64{35.168467, 35.902533},
		[2]float64{43.777067, 44.811433}, [2]float64{49.916533, 50.6506}, [2]float64{53.2532, 54.821433},
		[2]float64{67.100367, 68.101367}, [2]float64{77.2772, 78.111367}, [2]float64{81.514767, 82.649233},
		[2]float64{188.2881, 190.156633}, [2]float64{198.731867, 201.434567},
	)
	if got := ParseBreaks(log); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

// Two spots in a row are not enough: a show's own scenes can line up that way.
func TestParseBreaksNeedsThreeSpots(t *testing.T) {
	log := blackLog([2]float64{100, 100.2}, [2]float64{130, 130.2}, [2]float64{160, 160.2}, [2]float64{171, 171.2})
	if got := ParseBreaks(log); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}
