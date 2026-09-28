package dvr

import (
	"testing"
	"time"

	"broadwave/internal/store"
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

func TestJoinDecisionNeedsTheBufferFromTheStart(t *testing.T) {
	start := time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)
	airing := store.Airing{ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)}
	pass := store.Pass{PadAfter: 2}
	now := start.Add(10 * time.Minute)
	if ok, minutes := JoinDecision(pass, airing, now, start.Add(-5*time.Minute)); !ok || minutes != 53 {
		t.Fatalf("tuned since before the start: %v %d", ok, minutes)
	}
	if ok, _ := JoinDecision(pass, airing, now, start.Add(time.Minute)); ok {
		t.Fatal("a buffer that starts after the show cannot hold its beginning")
	}
	if ok, _ := JoinDecision(pass, airing, now, time.Time{}); ok {
		t.Fatal("an untuned channel has no buffer")
	}
	if ok, _ := JoinDecision(pass, airing, start.Add(time.Minute), start.Add(-time.Hour)); ok {
		t.Fatal("inside the start lead, StartDecision decides")
	}
	if ok, _ := JoinDecision(pass, airing, start.Add(61*time.Minute), start.Add(-time.Hour)); ok {
		t.Fatal("an airing that ended is not joined")
	}
}

func TestAttemptedCountsEveryRecordingOfTheAiring(t *testing.T) {
	start := time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)
	airing := store.Airing{ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)}
	stopped := store.Recording{ChannelID: 1, Title: "news", Status: "complete", StartedAt: start.Add(-time.Minute)}
	if !attempted([]store.Recording{stopped}, airing) {
		t.Fatal("a recording the viewer stopped must not be joined again")
	}
	yesterday := stopped
	yesterday.StartedAt = start.Add(-24 * time.Hour)
	other := stopped
	other.ChannelID = 2
	if attempted([]store.Recording{yesterday, other}, airing) {
		t.Fatal("another airing or channel does not count")
	}
}
