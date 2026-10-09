package dvr

import (
	"math"
	"sort"
	"strings"
	"time"

	"broadwave/internal/store"
)

// Slot is one program on a library channel's clock.
type Slot struct {
	RecordingID int64     `json:"recordingId"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

// Slots lays recordings onto a looping clock. Unknown lengths count as 30 minutes.
// Item 0 starts at from. A guide that should open on whatever is airing uses ScheduleFrom.
func Slots(order string, ids []int64, recs []store.Recording, from, to time.Time) []Slot {
	list := lineup(order, ids, recs, from)
	if len(list) == 0 || !to.After(from) {
		return nil
	}
	var out []Slot
	cursor := from
	i := 0
	// A day of one-minute recordings is a few thousand rows. The cap only
	// stops a slot length of zero from spinning.
	for cursor.Before(to) && len(out) < 4096 {
		rec := list[i%len(list)]
		dur := slotSeconds(rec)
		if dur < 1 {
			break
		}
		end := cursor.Add(time.Duration(dur * float64(time.Second)))
		if end.After(from) {
			out = append(out, Slot{RecordingID: rec.ID, Title: rec.Title, Start: cursor, End: end})
		}
		cursor = end
		i++
	}
	return out
}

// Join is the recording on at now, and how many seconds into it that is.
// One cycle of the lineup is anchored at local midnight, so the spot does not
// follow the width of a window passed to Slots.
func Join(order string, ids []int64, recs []store.Recording, now time.Time) (int64, float64, bool) {
	list, offset, ok := cycle(order, ids, recs, now)
	if !ok {
		return 0, 0, false
	}
	return list[0].ID, offset, true
}

// ScheduleFrom lays the guide from the recording that is on at from, backed
// up to that file's start. The lineup is already ordered; Slots must not sort it again.
func ScheduleFrom(order string, ids []int64, recs []store.Recording, from, to time.Time) []Slot {
	list, offset, ok := cycle(order, ids, recs, from)
	if !ok || !to.After(from) {
		return nil
	}
	rotated := make([]int64, len(list))
	for i, rec := range list {
		rotated[i] = rec.ID
	}
	start := from.Add(-time.Duration(offset * float64(time.Second)))
	return Slots("", rotated, list, start, to)
}

// cycle rotates the lineup so item 0 is what is on at now. offset is how far
// into that recording the local clock is. Order is fixed for the local day.
func cycle(order string, ids []int64, recs []store.Recording, now time.Time) ([]store.Recording, float64, bool) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	list := lineup(order, ids, recs, midnight)
	if len(list) == 0 {
		return nil, 0, false
	}
	var sum float64
	for _, rec := range list {
		sum += slotSeconds(rec)
	}
	if sum <= 0 {
		return nil, 0, false
	}
	pos := math.Mod(now.Sub(midnight).Seconds(), sum)
	if pos < 0 {
		pos += sum
	}
	for i, rec := range list {
		dur := slotSeconds(rec)
		if pos < dur {
			if i > 0 {
				list = append(append([]store.Recording{}, list[i:]...), list[:i]...)
			}
			return list, pos, true
		}
		pos -= dur
	}
	return list, 0, true
}

func lineup(order string, ids []int64, recs []store.Recording, at time.Time) []store.Recording {
	byID := map[int64]store.Recording{}
	for _, rec := range recs {
		byID[rec.ID] = rec
	}
	var list []store.Recording
	for _, id := range ids {
		if rec, ok := byID[id]; ok && rec.Status != "failed" {
			list = append(list, rec)
		}
	}
	return orderRecordings(order, list, at)
}

func slotSeconds(rec store.Recording) float64 {
	if rec.Duration < 60 {
		return 30 * 60
	}
	return rec.Duration
}

func orderRecordings(order string, list []store.Recording, from time.Time) []store.Recording {
	switch strings.ToLower(order) {
	case "release":
		sort.SliceStable(list, func(i, j int) bool { return list[i].StartedAt.Before(list[j].StartedAt) })
	case "random":
		sort.SliceStable(list, func(i, j int) bool {
			return mix(list[i].ID, from) < mix(list[j].ID, from)
		})
	case "season", "show":
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Title == list[j].Title {
				return list[i].Subtitle < list[j].Subtitle
			}
			return list[i].Title < list[j].Title
		})
	case "marathon":
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Title == list[j].Title {
				return list[i].Subtitle < list[j].Subtitle
			}
			return list[i].Title < list[j].Title
		})
		return marathon(list)
	}
	return list
}

func marathon(list []store.Recording) []store.Recording {
	groups := [][]store.Recording{}
	for _, rec := range list {
		if len(groups) == 0 || groups[len(groups)-1][0].Title != rec.Title {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], rec)
	}
	var out []store.Recording
	left := true
	for left {
		left = false
		for i := range groups {
			n := 0
			for n < 3 && len(groups[i]) > 0 {
				out = append(out, groups[i][0])
				groups[i] = groups[i][1:]
				n++
				left = true
			}
		}
	}
	return out
}

func mix(id int64, from time.Time) int64 {
	return id*131 + int64(from.YearDay())
}
