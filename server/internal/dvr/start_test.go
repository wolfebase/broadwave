package dvr

import (
	"context"
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
	pass := store.Pass{}
	stopped := store.Recording{ChannelID: 1, Title: "news", Status: "complete", StartedAt: start.Add(-time.Minute)}
	if !attempted([]store.Recording{stopped}, pass, airing) {
		t.Fatal("a recording the viewer stopped must not be joined again")
	}
	yesterday := stopped
	yesterday.StartedAt = start.Add(-24 * time.Hour)
	other := stopped
	other.ChannelID = 2
	if attempted([]store.Recording{yesterday, other}, pass, airing) {
		t.Fatal("another airing or channel does not count")
	}
}

func TestAFinishedEpisodeDoesNotBlockTheNextOne(t *testing.T) {
	first := time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)
	secondStart := first.Add(30 * time.Minute)
	second := store.Airing{ChannelID: 1, Title: "News", Start: secondStart, End: secondStart.Add(30 * time.Minute)}
	pass := store.Pass{PadBefore: 1, PadAfter: 2}
	done := store.Recording{
		ChannelID: 1, Title: "News", Status: "complete",
		StartedAt: first.Add(-time.Minute),
	}
	now := secondStart.Add(2 * time.Minute)
	held := first.Add(-time.Minute)
	start, _ := StartDecision(pass, second, now)
	if !start && !attempted([]store.Recording{done}, pass, second) {
		start, _ = JoinDecision(pass, second, now, held)
	}
	if !start {
		t.Fatal("second episode was not started")
	}
	// The episode before, still in its padding, used to block this one, which
	// then started only from the buffer, or never without one.
	open := done
	open.Status = "recording"
	if already([]store.Recording{open}, pass, second) {
		t.Fatal("the episode before must not block this one: the two record side by side")
	}
	mine := open
	mine.StartedAt = secondStart.Add(-time.Minute)
	if !already([]store.Recording{open, mine}, pass, second) {
		t.Fatal("this airing's own recording in progress blocks a second start")
	}
}

// A retry of one airing that started in its last 90 s fell in the next
// airing's start window and counted as that one's recording: the next airing
// never recorded. A recording names the airing it was made for.
func TestALateRetryIsNotTheNextAiringsRecording(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	first := time.Now().UTC().Truncate(time.Minute).Add(-29 * time.Minute)
	next := first.Add(30 * time.Minute)
	airing := store.Airing{ChannelID: 1, Title: "News", Start: next, End: next.Add(30 * time.Minute)}
	pass := store.Pass{PadBefore: 1, PadAfter: 2}
	ends := next.Add(2 * time.Minute)
	id, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 1, Title: "News", Path: "retry.ts", Status: "recording",
		StartedAt: next.Add(-time.Minute), EndsAt: &ends, AiringStart: &first,
	})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := st.Recording(ctx, id)
	if err != nil || retry.AiringStart == nil || !retry.AiringStart.Equal(first) {
		t.Fatalf("the row lost its airing: %+v %v", retry.AiringStart, err)
	}
	recs := []store.Recording{retry}
	if already(recs, pass, airing) || attempted(recs, pass, airing) {
		t.Fatal("the last airing's retry counted as the next airing's recording")
	}
	retry.Status = "failed"
	if tries, _ := failedTries([]store.Recording{retry}, pass, airing); tries != 0 {
		t.Fatalf("the last airing's failed retry counted as %d tries of the next", tries)
	}
	// A row with no airing named is still judged by its start.
	retry.Status, retry.AiringStart = "recording", nil
	if !already([]store.Recording{retry}, pass, airing) {
		t.Fatal("an older row in the window no longer counts")
	}
}
