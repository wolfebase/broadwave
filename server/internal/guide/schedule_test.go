package guide

import (
	"testing"
	"time"
)

func TestNextPullStaysInsideSiliconDustWindow(t *testing.T) {
	success := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		jitter time.Duration
		want   time.Duration
	}{
		{0, 20 * time.Hour},
		{8 * time.Hour, 28 * time.Hour},
		{3 * time.Hour, 23 * time.Hour},
		{-time.Hour, 20 * time.Hour},
		{48 * time.Hour, 28 * time.Hour},
	}
	for _, tc := range cases {
		got := NextPull(success, tc.jitter).Sub(success)
		if got != tc.want {
			t.Fatalf("jitter %s: got %s, want %s", tc.jitter, got, tc.want)
		}
	}
}

func TestManualAllowed(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	if ok, _ := ManualAllowed(time.Time{}, now); !ok {
		t.Fatal("first manual refresh should be allowed")
	}
	ok, retry := ManualAllowed(now.Add(-20*time.Minute), now)
	if ok {
		t.Fatal("a refresh 20 minutes ago should be blocked")
	}
	if !retry.Equal(now.Add(40 * time.Minute)) {
		t.Fatalf("retry at %s", retry)
	}
	if ok, _ := ManualAllowed(now.Add(-ManualGap), now); !ok {
		t.Fatal("a refresh an hour ago should be allowed")
	}
}

func TestDelay(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	if Delay(time.Time{}, now) != 0 {
		t.Fatal("missing schedule should pull now")
	}
	if Delay(now.Add(-time.Minute), now) != 0 {
		t.Fatal("past schedule should pull now")
	}
	if got := Delay(now.Add(2*time.Hour), now); got != 2*time.Hour {
		t.Fatalf("delay %s", got)
	}
}
