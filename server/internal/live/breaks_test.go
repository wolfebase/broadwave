package live

import "testing"

func TestParseBreaks(t *testing.T) {
	log := "[blackdetect @ 0x1] black_start:12.4 black_end:42.8 black_duration:30.4\n[blackdetect @ 0x1] black_start:1.0 black_end:1.1 black_duration:0.1\n"
	got := ParseBreaks(log)
	if len(got) != 1 || got[0].Start != 12.4 || got[0].End != 42.8 {
		t.Fatalf("%+v", got)
	}
}
