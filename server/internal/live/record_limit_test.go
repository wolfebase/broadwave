package live

import (
	"math"
	"testing"
	"time"
)

func TestARecordingStopsWithinADay(t *testing.T) {
	if got := clampRecordMinutes(0); got != 60 {
		t.Fatalf("blank %d", got)
	}
	if got := clampRecordMinutes(90); got != 90 {
		t.Fatalf("short %d", got)
	}
	got := clampRecordMinutes(100_000)
	if got != maxRecordMinutes {
		t.Fatalf("long %d", got)
	}
	if clampRecordMinutes(math.MaxInt) != maxRecordMinutes {
		t.Fatal("huge")
	}
	d := time.Duration(got) * time.Minute
	if d <= 0 || d > 24*time.Hour {
		t.Fatalf("duration %s", d)
	}
}
