package dvr

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/sports"
	"broadwave/internal/store"
)

// Planned is one airing a series pass will record inside the window.
type Planned struct {
	PassID    int64        `json:"passId"`
	Airing    store.Airing `json:"airing"`
	Priority  int          `json:"priority"`
	PadBefore int          `json:"padBefore"`
	PadAfter  int          `json:"padAfter"`
	Conflict  bool         `json:"conflict"`
	Skipped   bool         `json:"skipped"`
	Reason    string       `json:"reason,omitempty"`
}

// Plan matches passes to airings and marks any span where more channels overlap than the tuner count.
// Two shows on the same channel count as one tuner. A show that ends as the next begins does not overlap.
func Plan(passes []store.Pass, airings []store.Airing, tunerCount int, from, to time.Time) []Planned {
	if tunerCount < 1 {
		tunerCount = 1
	}
	items := make([]Planned, 0)
	for _, airing := range airings {
		if !airing.End.After(from) || !airing.Start.Before(to) {
			continue
		}
		pass, ok := matchPass(passes, airing)
		if !ok {
			continue
		}
		items = append(items, Planned{PassID: pass.ID, Airing: airing, Priority: pass.Priority, PadBefore: pass.PadBefore, PadAfter: pass.PadAfter})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Airing.Start.Equal(items[j].Airing.Start) {
			return items[i].Airing.ID < items[j].Airing.ID
		}
		return items[i].Airing.Start.Before(items[j].Airing.Start)
	})
	resolvePriority(items, tunerCount)
	return items
}

func matchPass(passes []store.Pass, airing store.Airing) (store.Pass, bool) {
	for _, pass := range passes {
		if !passMatches(pass, airing) {
			continue
		}
		return pass, true
	}
	return store.Pass{}, false
}

func passMatches(pass store.Pass, airing store.Airing) bool {
	if pass.ChannelID != 0 && pass.ChannelID != airing.ChannelID {
		return false
	}
	if !inWindow(pass, airing.Start) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(pass.MatchKind))
	title := strings.ToLower(strings.TrimSpace(airing.Title))
	want := strings.ToLower(strings.TrimSpace(pass.Title))
	if strings.EqualFold(pass.Kind, "team") || kind == "team" {
		return sports.Mentions(airing.Title+" "+airing.Subtitle, pass.Title)
	}
	switch kind {
	case "category":
		if !categoryHas(airing.Category, want) {
			return false
		}
	case "contains":
		if want == "" || !strings.Contains(title, want) {
			return false
		}
	default:
		if title != want {
			return false
		}
	}
	if strings.EqualFold(pass.Episodes, "new") && !airing.New {
		return false
	}
	return true
}

func categoryHas(category, want string) bool {
	if want == "" {
		return false
	}
	for _, part := range strings.Split(category, ",") {
		if strings.EqualFold(strings.TrimSpace(part), want) {
			return true
		}
	}
	return strings.EqualFold(strings.TrimSpace(category), want)
}

func inWindow(pass store.Pass, start time.Time) bool {
	if strings.TrimSpace(pass.TimeStart) == "" && strings.TrimSpace(pass.TimeEnd) == "" {
		return true
	}
	local := start.In(time.Local)
	mins := local.Hour()*60 + local.Minute()
	a := clockMinutes(pass.TimeStart)
	b := clockMinutes(pass.TimeEnd)
	if a < 0 || b < 0 {
		return true
	}
	if a <= b {
		return mins >= a && mins < b
	}
	return mins >= a || mins < b
}

func clockMinutes(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return -1
	}
	hour, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
		return -1
	}
	return hour*60 + min
}

func resolvePriority(items []Planned, tunerCount int) {
	type event struct {
		at    time.Time
		index int
		start bool
	}
	events := make([]event, 0, len(items)*2)
	for i, item := range items {
		events = append(events, event{at: item.Airing.Start, index: i, start: true})
		events = append(events, event{at: item.Airing.End, index: i, start: false})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].at.Equal(events[j].at) && events[i].start != events[j].start {
			return !events[i].start
		}
		return events[i].at.Before(events[j].at)
	})
	active := map[int]int64{}
	for _, ev := range events {
		if !ev.start {
			delete(active, ev.index)
			continue
		}
		if items[ev.index].Skipped {
			continue
		}
		active[ev.index] = items[ev.index].Airing.ChannelID
		for uniqueChannels(active) > tunerCount && len(active) > 0 {
			loser := lowestPriority(items, active)
			items[loser].Skipped = true
			items[loser].Conflict = true
			delete(active, loser)
		}
	}
}

func lowestPriority(items []Planned, active map[int]int64) int {
	loser := -1
	for idx := range active {
		if loser < 0 || losesTo(items[idx], items[loser]) {
			loser = idx
		}
	}
	return loser
}

func losesTo(a, b Planned) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.Airing.Start.Equal(b.Airing.Start) {
		return a.Airing.Start.After(b.Airing.Start)
	}
	return a.Airing.ID > b.Airing.ID
}

func uniqueChannels(active map[int]int64) int {
	seen := map[int64]struct{}{}
	for _, channelID := range active {
		seen[channelID] = struct{}{}
	}
	return len(seen)
}
