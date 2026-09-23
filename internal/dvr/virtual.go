package dvr

import (
	"sort"
	"strings"
	"time"

	"ota-viewer/internal/store"
)

// Slot is one program on a library channel's clock.
type Slot struct {
	RecordingID int64     `json:"recordingId"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

// Slots lays recordings onto a looping clock. Unknown lengths count as 30 minutes.
func Slots(order string, ids []int64, recs []store.Recording, from, to time.Time) []Slot {
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
	list = orderRecordings(order, list, from)
	if len(list) == 0 || !to.After(from) {
		return nil
	}
	var out []Slot
	cursor := from
	i := 0
	for cursor.Before(to) && len(out) < 500 {
		rec := list[i%len(list)]
		dur := rec.Duration
		if dur < 60 {
			dur = 30 * 60
		}
		end := cursor.Add(time.Duration(dur * float64(time.Second)))
		if end.After(from) {
			out = append(out, Slot{RecordingID: rec.ID, Title: rec.Title, Start: cursor, End: end})
		}
		cursor = end
		i++
		if i > len(list)*40 {
			break
		}
	}
	return out
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
