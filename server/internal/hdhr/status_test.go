package hdhr

import "testing"

func TestParseStatus(t *testing.T) {
	lock := ParseStatus("ch=8vsb:593000000 lock=8vsb ss=90 snq=88 seq=100 bps=1 pps=1")
	if !lock.Locked || lock.Strength != 90 || lock.Quality != 88 || lock.Symbol != 100 {
		t.Fatalf("%+v", lock)
	}
	if verdict, tip := lock.Verdict(); verdict != "Great" || tip != "" {
		t.Fatalf("verdict %s tip %q", verdict, tip)
	}
	weak := ParseStatus("ch=8vsb:593000000 lock=8vsb ss=40 snq=30 seq=80")
	if verdict, tip := weak.Verdict(); verdict != "Weak" || tip == "" {
		t.Fatalf("verdict %s tip %q", verdict, tip)
	}
	ok := ParseStatus("ch=8vsb:593000000 lock=8vsb ss=70 snq=60 seq=100")
	if verdict, _ := ok.Verdict(); verdict != "OK" {
		t.Fatal(verdict)
	}
	lost := ParseStatus("ch=8vsb:593000000 lock=none ss=0 snq=0 seq=0")
	if verdict, tip := lost.Verdict(); verdict != "Lost" || tip == "" {
		t.Fatalf("verdict %s tip %q", verdict, tip)
	}
}

func TestDemodulatedReadsEveryStandard(t *testing.T) {
	for status, want := range map[string]bool{
		"ch=8vsb:593000000 lock=8vsb ss=90 snq=88 seq=100":          true,
		"ch=qam:519000000 lock=qam256 ss=90 snq=88 seq=100":         true,
		"ch=auto:474000000 lock=t8qam64 ss=90 snq=88 seq=100":       true,
		"ch=auto:346000000 lock=a8qam256-6900 ss=90 snq=88 seq=100": true,
		"ch=auto:599000000 lock=atsc3 ss=90 snq=88 seq=100":         true,
		"ch=8vsb:593000000 lock=none ss=0 snq=0 seq=0":              false,
		"ch=auto:593000000 lock=(8vsb) ss=60 snq=0 seq=0":           false,
		"ch=none lock=none ss=0 snq=0 seq=0":                        false,
		"":                                                          false,
	} {
		if got := Demodulated(status); got != want {
			t.Errorf("Demodulated(%q) = %v, want %v", status, got, want)
		}
	}
}
