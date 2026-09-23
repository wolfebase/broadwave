package dvr

import (
	"testing"
	"time"

	"ota-viewer/internal/store"
)

func TestStartDecisionPadding(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	airing := store.Airing{ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)}
	exact := store.Pass{PadBefore: 0, PadAfter: 0}
	early := store.Pass{PadBefore: 3, PadAfter: 5}

	if ok, _ := StartDecision(exact, airing, start.Add(-2*time.Minute)); ok {
		t.Fatal("a pass with no padding waits until the last 90 seconds")
	}
	ok, minutes := StartDecision(exact, airing, start.Add(-30*time.Second))
	if !ok || minutes < 60 || minutes > 62 {
		t.Fatalf("exact start %v %d", ok, minutes)
	}
	ok, minutes = StartDecision(early, airing, start.Add(-2*time.Minute))
	if !ok || minutes < 65 {
		t.Fatalf("three minutes early plus five after: %v %d", ok, minutes)
	}
	if ok, _ := StartDecision(early, airing, start.Add(-4*time.Minute)); ok {
		t.Fatal("four minutes is earlier than a 3 minute pad")
	}
	if ok, _ := StartDecision(early, airing, start.Add(2*time.Minute)); ok {
		t.Fatal("does not join a show that is already underway")
	}
}
