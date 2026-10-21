package httpapi

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestRecordSpanKeepsTheMinutesAskedFor(t *testing.T) {
	now := time.Date(2026, 10, 21, 20, 16, 0, 0, time.UTC)
	air := store.Airing{Title: "News", Start: now.Add(-16 * time.Minute), End: now.Add(14 * time.Minute)}
	minutes, from := recordSpan(2, air, now)
	if minutes != 2 || !from.IsZero() {
		t.Fatalf("asked 2: %d from %v", minutes, from)
	}
	minutes, from = recordSpan(0, air, now)
	if minutes != 17 || !from.Equal(air.Start) {
		t.Fatalf("whole show: %d from %v", minutes, from)
	}
	over := store.Airing{Title: "News", Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)}
	if minutes, _ := recordSpan(0, over, now); minutes != 0 {
		t.Fatalf("over: %d", minutes)
	}
}
