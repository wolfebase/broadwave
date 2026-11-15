package live

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Two days of the sweep's choices for one frequency, a minute at a time. The
// lead channels' programs start every grid minutes, and one channel on the
// frequency may never give a still (a sound-only service). A channel that can
// give a still is never left unlisted (older than SweepStaleAfter), and the
// frequency is tuned no more than the old 30-minute refresh did.
func TestSweepBudgetKeepsEveryStillListed(t *testing.T) {
	for _, grid := range []int{10, 15, 20, 30, 60} {
		t.Run(fmt.Sprintf("grid%d", grid), func(t *testing.T) {
			darkPlain, plain := simulateSweep(t, grid, false)
			if darkPlain > 0 {
				t.Errorf("%d minutes with no listed still", darkPlain)
			}
			dark, tunes := simulateSweep(t, grid, true)
			t.Logf("%d tunes a day", tunes)
			if dark > 0 {
				t.Errorf("a channel with a picture went %d minutes with no listed still", dark)
			}
			if tunes > sweepDailyTunes {
				t.Errorf("%d tunes on the second day, more than %d", tunes, sweepDailyTunes)
			}
			if tunes != plain {
				t.Errorf("a channel that never gives a still: %d tunes a day, %d without it", tunes, plain)
			}
		})
	}
}

// simulateSweep returns the second day's minutes in which channel 7 had no
// listed still, and its tunes. With mute, channel 9 never gives a still.
func simulateSweep(t *testing.T, grid int, mute bool) (dark, tunes int) {
	h := &Hub{Dir: t.TempDir(), channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	base := time.Now().Truncate(time.Hour).Add(-48 * time.Hour)
	if err := os.MkdirAll(filepath.Join(h.Dir, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(id int64, at time.Time) {
		path := FramePath(h.Dir, id, 0)
		if err := os.WriteFile(path, []byte{0xff, 0xd8}, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	channels := []int64{7, 8}
	if mute {
		channels = append(channels, 9)
	}
	for m := 0; m < 48*60; m++ {
		now := base.Add(time.Duration(m) * time.Minute)
		// Two lead channels; the second's programs start half a grid later.
		program := now.Add(-time.Duration(grid) * time.Minute / 2).Truncate(time.Duration(grid) * time.Minute).Add(time.Duration(grid) * time.Minute / 2)
		if first := now.Truncate(time.Duration(grid) * time.Minute); first.After(program) {
			program = first
		}
		target := sweepTarget{key: "k", channels: channels, lead: []int64{7, 8}, program: program}
		if h.sweepDue(target, now) {
			h.noteSweepTune("k", now)
			grabbed := now.Add(30 * time.Second)
			put(7, grabbed)
			put(8, grabbed)
			h.noteSweepResult(target, now)
			if m >= 24*60 {
				tunes++
			}
		}
		if m >= 24*60 {
			if at, ok := frameTime(h.Dir, 7); !ok || now.Sub(at) > SweepStaleAfter {
				dark++
			}
		}
	}
	return dark, tunes
}

// A channel the last tune brought back no still for waits for the refresh;
// one that never had a still stops counting after sweepGiveUp tunes, until
// the lineup changes.
func TestSweepBacksOffAChannelWithNoStill(t *testing.T) {
	h := &Hub{Dir: t.TempDir(), channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	now := time.Now()
	path := FramePath(h.Dir, 7, 0)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-40 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	target := sweepTarget{key: "k", channels: []int64{7}}
	h.noteSweepTune("k", now.Add(-15*time.Minute))
	h.noteSweepResult(target, now.Add(-15*time.Minute))
	if h.sweepDue(target, now) {
		t.Fatal("a channel that just came back without a still is due again before the refresh")
	}
	if !h.sweepDue(target, now.Add(16*time.Minute)) {
		t.Fatal("a channel that came back without a still is not tried on the refresh")
	}

	never := sweepTarget{key: "n", channels: []int64{9}}
	for i := range sweepGiveUp {
		at := now.Add(-time.Duration(3-i) * 2 * time.Hour)
		if !h.sweepDue(never, at) {
			t.Fatalf("try %d of a channel with no still is not due", i+1)
		}
		h.noteSweepTune("n", at)
		h.noteSweepResult(never, at)
	}
	if h.sweepDue(never, now) {
		t.Fatal("a channel that never had a still is still tuned for")
	}
	h.forgetSweepMisses()
	if !h.sweepDue(never, now) {
		t.Fatal("a new lineup does not try it again")
	}
	if sweepLineup([]sweepTarget{never}) == sweepLineup([]sweepTarget{target}) {
		t.Fatal("two lineups read the same")
	}
}
