package dvr

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestJoinOffsetMovesWithTheClock(t *testing.T) {
	loc := time.FixedZone("plus1", 3600)
	recs := []store.Recording{{ID: 7, Title: "Night Shift", Duration: 30 * 60, Status: "complete"}}
	ids := []int64{7}
	for _, tc := range []struct {
		hour, min int
		want      float64
	}{
		{12, 0, 0},
		{12, 10, 600},
		{13, 10, 600},
	} {
		now := time.Date(2026, 10, 9, tc.hour, tc.min, 0, 0, loc)
		_, off, ok := Join("", ids, recs, now)
		if !ok || off != tc.want {
			t.Fatalf("%02d:%02d offset %v ok %v", tc.hour, tc.min, off, ok)
		}
		// A window that starts six hours ago lands on this show at offset 0.
		// Six hours divides 30 minutes, so that clock never moves.
		got := -1.0
		for _, slot := range Slots("", ids, recs, now.Add(-6*time.Hour), now.Add(time.Hour)) {
			if !now.Before(slot.Start) && now.Before(slot.End) {
				got = now.Sub(slot.Start).Seconds()
			}
		}
		if got != 0 {
			t.Fatalf("%02d:%02d window offset %v", tc.hour, tc.min, got)
		}
	}
}

func TestJoinUsesLocalMidnight(t *testing.T) {
	loc := time.FixedZone("plus1", 3600)
	noon := time.Date(2026, 10, 9, 12, 0, 0, 0, loc)
	recs := []store.Recording{{ID: 7, Title: "Night Shift", Duration: 50 * 60, Status: "complete"}}
	_, off, ok := Join("", []int64{7}, recs, noon)
	if !ok || off != 1200 {
		t.Fatalf("offset %v ok %v", off, ok)
	}
	got := -1.0
	for _, slot := range Slots("", []int64{7}, recs, noon.Add(-6*time.Hour), noon.Add(time.Hour)) {
		if !noon.Before(slot.Start) && noon.Before(slot.End) {
			got = noon.Sub(slot.Start).Seconds()
		}
	}
	if got != 600 {
		t.Fatalf("window offset %v", got)
	}
}

func TestScheduleStartsOnTheRecordingThatIsOn(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 40, 0, 0, time.UTC)
	recs := []store.Recording{
		{ID: 1, Title: "Night Shift", Duration: 30 * 60, Status: "complete"},
		{ID: 2, Title: "Harbor Watch", Duration: 30 * 60, Status: "complete"},
	}
	id, off, ok := Join("", []int64{1, 2}, recs, at)
	if !ok || id != 2 || off != 600 {
		t.Fatalf("join %d %v %v", id, off, ok)
	}
	slots := ScheduleFrom("", []int64{1, 2}, recs, at, at.Add(2*time.Hour))
	if len(slots) < 2 {
		t.Fatalf("slots %+v", slots)
	}
	if slots[0].RecordingID != 2 || !slots[0].Start.Equal(at.Add(-10*time.Minute)) {
		t.Fatalf("first %+v", slots[0])
	}
	if slots[1].RecordingID != 1 || !slots[1].Start.Equal(at.Add(20*time.Minute)) {
		t.Fatalf("next %+v", slots[1])
	}
}

func TestADayOfShortRecordingsStaysOnTheSchedule(t *testing.T) {
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	half := []store.Recording{{ID: 1, Title: "Night Shift", Duration: 30 * 60, Status: "complete"}}
	slots := ScheduleFrom("", []int64{1}, half, at, at.Add(48*time.Hour))
	if len(slots) < 96 {
		t.Fatalf("half-hour slots %d, want the 48 hours", len(slots))
	}
	if !slots[len(slots)-1].End.After(at.Add(47 * time.Hour)) {
		t.Fatalf("last %+v", slots[len(slots)-1])
	}
	minute := []store.Recording{{ID: 1, Title: "Night Shift", Duration: 60, Status: "complete"}}
	slots = ScheduleFrom("", []int64{1}, minute, at, at.Add(48*time.Hour))
	if len(slots) < 2800 {
		t.Fatalf("one-minute slots %d, want the 48 hours", len(slots))
	}
}

func TestARecordingUnderAMinuteCountsAsHalfAnHour(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 10, 0, 0, time.UTC)
	short := []store.Recording{{ID: 1, Title: "Night Shift", Duration: 59, Status: "complete"}}
	_, off, ok := Join("", []int64{1}, short, at)
	if !ok || off != 600 {
		t.Fatalf("under a minute %v ok %v", off, ok)
	}
	minute := []store.Recording{{ID: 1, Title: "Night Shift", Duration: 60, Status: "complete"}}
	_, off, ok = Join("", []int64{1}, minute, at)
	if !ok || off != 0 {
		t.Fatalf("one minute %v ok %v", off, ok)
	}
}
