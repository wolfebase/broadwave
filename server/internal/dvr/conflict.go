package dvr

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"broadwave/internal/store"
)

// Suggestion is another airing of a skipped show that fits on the tuners already kept.
// Misses are other showings a one-shot fix will not record. A channel-pin move leaves this empty.
type Suggestion struct {
	ChannelID   int64           `json:"channelId"`
	GuideNumber string          `json:"guideNumber,omitempty"`
	Title       string          `json:"title"`
	Start       time.Time       `json:"start"`
	End         time.Time       `json:"end"`
	Misses      []MissedShowing `json:"misses,omitempty"`
}

// MissedShowing is one airing a one-shot would record and then drop.
type MissedShowing struct {
	ChannelID   int64     `json:"channelId"`
	GuideNumber string    `json:"guideNumber,omitempty"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
}

// Soon is a recording that has not started. Pad is how early its tuner is reserved.
type Soon struct {
	Title     string
	ChannelID int64
	Start     time.Time
	Pad       time.Duration
}

// FixPlan records a suggestion instead of a skipped airing.
// Priority is never raised. When the pass already matches the later airing,
// only that skipped showing is dropped. A title pass whose channel pin is the
// only mismatch takes the later channel. Any other pass gets a one-shot, which
// can skip other showings of that title for good.
type FixPlan struct {
	Skip       store.Airing
	SetChannel int64
	OneShot    *store.Pass
}

// SkipParts is the skip row for an airing. Shows with no episode id share one
// key and are told apart by the start time, matching POST /schedule/skip.
func SkipParts(air store.Airing) (key, starts string) {
	starts = air.Start.UTC().Format(time.RFC3339)
	key = store.EpisodeKey(air.ProgramID, air.Title, air.Subtitle, air.ChannelID)
	if key == "" {
		key = store.EpisodeKey("once", air.Title, starts, air.ChannelID)
	}
	return key, starts
}

// AttachSuggestions fills Suggestion on tuner conflicts. The airing has to be
// the same title, follow the pass apart from a channel pin, start later, and
// fit beside the shows that are still recording. Library skips are left alone.
func AttachSuggestions(items []Planned, passes []store.Pass, airings []store.Airing, tunerCount int, from, to time.Time, guides map[int64]string) []Planned {
	if tunerCount < 1 {
		tunerCount = 1
	}
	byID := map[int64]store.Pass{}
	for _, pass := range passes {
		byID[pass.ID] = pass
	}
	for i := range items {
		if !items[i].Skipped || !items[i].Conflict {
			continue
		}
		pass, ok := byID[items[i].PassID]
		if !ok {
			continue
		}
		alt, ok := laterAiring(items, items[i], pass, airings, tunerCount, from, to, guides)
		if !ok {
			continue
		}
		items[i].Suggestion = &alt
	}
	return items
}

func laterAiring(items []Planned, skipped Planned, pass store.Pass, airings []store.Airing, tunerCount int, from, to time.Time, guides map[int64]string) (Suggestion, bool) {
	var best store.Airing
	found := false
	for _, air := range airings {
		if !air.End.After(from) || !air.Start.Before(to) {
			continue
		}
		if strings.TrimSpace(air.Title) == "" || sameShowing(air, skipped.Airing) {
			continue
		}
		if !air.Start.After(skipped.Airing.Start) || !strings.EqualFold(strings.TrimSpace(air.Title), strings.TrimSpace(skipped.Airing.Title)) {
			continue
		}
		if !rulesMatch(pass, air) || librarySkip(items, air) || !fitsBeside(items, air, tunerCount) {
			continue
		}
		if !found || air.Start.Before(best.Start) || (air.Start.Equal(best.Start) && air.ChannelID < best.ChannelID) {
			best = air
			found = true
		}
	}
	if !found {
		return Suggestion{}, false
	}
	sug := Suggestion{
		ChannelID:   best.ChannelID,
		GuideNumber: guides[best.ChannelID],
		Title:       best.Title,
		Start:       best.Start,
		End:         best.End,
	}
	fix := PlanFix(pass, skipped.Airing, best)
	if fix.OneShot != nil {
		sug.Misses = MissedFrom(airings, *fix.OneShot, best, guides)
	}
	return sug, true
}

// rulesMatch applies the pass except its channel pin, so a later airing on
// another channel can be offered. The fix decides whether to move the pin.
func rulesMatch(pass store.Pass, airing store.Airing) bool {
	pass.ChannelID = 0
	return passMatches(pass, airing)
}

func librarySkip(items []Planned, air store.Airing) bool {
	for _, item := range items {
		if sameShowing(item.Airing, air) && item.Skipped && !item.Conflict {
			return true
		}
	}
	return false
}

func fitsBeside(items []Planned, air store.Airing, tunerCount int) bool {
	for _, item := range items {
		if !item.Skipped && sameShowing(item.Airing, air) {
			return true
		}
	}
	spans := make([]span, 0, len(items)+1)
	for _, item := range items {
		if item.Skipped || sameShowing(item.Airing, air) {
			continue
		}
		spans = append(spans, span{start: item.Airing.Start, end: item.Airing.End, channel: item.Airing.ChannelID})
	}
	spans = append(spans, span{start: air.Start, end: air.End, channel: air.ChannelID})
	return !overflows(spans, tunerCount)
}

type span struct {
	start, end time.Time
	channel    int64
}

func overflows(spans []span, tunerCount int) bool {
	if tunerCount < 1 {
		tunerCount = 1
	}
	type event struct {
		at      time.Time
		index   int
		start   bool
		channel int64
	}
	events := make([]event, 0, len(spans)*2)
	for i, sp := range spans {
		if !sp.end.After(sp.start) {
			continue
		}
		events = append(events, event{at: sp.start, index: i, start: true, channel: sp.channel})
		events = append(events, event{at: sp.end, index: i, start: false, channel: sp.channel})
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
		active[ev.index] = ev.channel
		if uniqueChannels(active) > tunerCount {
			return true
		}
	}
	return false
}

func sameShowing(a, b store.Airing) bool {
	if a.ID > 0 && a.ID == b.ID {
		return true
	}
	return a.ChannelID == b.ChannelID && !a.Start.IsZero() && a.Start.Equal(b.Start)
}

// PlanFix keeps the series on its channel when that pass already matches the
// later airing. It moves a title pass's channel pin only when the pin is the
// only mismatch. Everything else is a one-shot. It does not raise priority.
func PlanFix(pass store.Pass, skipped, suggestion store.Airing) FixPlan {
	fix := FixPlan{Skip: skipped}
	if passMatches(pass, suggestion) {
		return fix
	}
	if channelAdjust(pass, suggestion) {
		fix.SetChannel = suggestion.ChannelID
		return fix
	}
	shot := oneShotPass(pass, suggestion)
	fix.OneShot = &shot
	return fix
}

func channelAdjust(pass store.Pass, suggestion store.Airing) bool {
	if pass.ChannelID == 0 || suggestion.ChannelID == 0 || suggestion.ChannelID == pass.ChannelID {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(pass.MatchKind))
	if kind == "team" || strings.EqualFold(pass.Kind, "team") || (kind != "" && kind != "title") {
		return false
	}
	relaxed := pass
	relaxed.ChannelID = 0
	return passMatches(relaxed, suggestion)
}

func oneShotPass(pass store.Pass, suggestion store.Airing) store.Pass {
	shot := pass
	shot.ID = 0
	shot.Kind = "series"
	shot.Title = strings.TrimSpace(suggestion.Title)
	shot.ChannelID = suggestion.ChannelID
	shot.MatchKind = "title"
	shot.Episodes = "all"
	shot.LimitCount = 1
	shot.Priority = pass.Priority
	local := suggestion.Start.In(time.Local)
	shot.TimeStart = fmt.Sprintf("%02d:%02d", local.Hour(), local.Minute())
	end := local.Add(time.Minute)
	shot.TimeEnd = fmt.Sprintf("%02d:%02d", end.Hour(), end.Minute())
	return shot
}

// OneShotLimit leaves room for this airing on top of what is already unwatched.
func OneShotLimit(pass store.Pass, recs []store.Recording) int {
	n := unwatchedCount(recs, pass)
	if n < 0 {
		n = 0
	}
	return n + 1
}

// HaveOneShot reports a pass that already records this airing and then stops.
func HaveOneShot(passes []store.Pass, suggestion store.Airing) bool {
	for _, pass := range passes {
		if pass.LimitCount < 1 || pass.ChannelID != suggestion.ChannelID || strings.TrimSpace(pass.TimeStart) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pass.Title), strings.TrimSpace(suggestion.Title)) && passMatches(pass, suggestion) {
			return true
		}
	}
	return false
}

// OneShotSkips lists other airings the one-shot would grab besides the one the viewer chose.
func OneShotSkips(airings []store.Airing, shot store.Pass, keep store.Airing) []store.Airing {
	var out []store.Airing
	for _, air := range airings {
		if sameShowing(air, keep) || !passMatches(shot, air) {
			continue
		}
		out = append(out, air)
	}
	return out
}

// MissedFrom is the showings OneShotSkips would drop, in start order.
func MissedFrom(airings []store.Airing, shot store.Pass, keep store.Airing, guides map[int64]string) []MissedShowing {
	skipped := OneShotSkips(airings, shot, keep)
	if len(skipped) == 0 {
		return nil
	}
	out := make([]MissedShowing, 0, len(skipped))
	for _, other := range skipped {
		out = append(out, MissedShowing{
			ChannelID:   other.ChannelID,
			GuideNumber: guides[other.ChannelID],
			Title:       other.Title,
			Start:       other.Start,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].ChannelID < out[j].ChannelID
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

// MissedLine is the sentence shown before a one-shot fix is confirmed.
func MissedLine(items []MissedShowing) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = missedPart(item)
	}
	return joinList(parts) + " will not record."
}

func missedPart(item MissedShowing) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "A show"
	}
	when := item.Start.In(time.Local).Format("3:04 PM")
	if number := strings.TrimSpace(item.GuideNumber); number != "" {
		return fmt.Sprintf("%s at %s on %s", title, when, number)
	}
	return fmt.Sprintf("%s at %s", title, when)
}

func joinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
	}
}

// preemptWindow is how early a recording keeps the last tuner when the pass
// has no padding. The scheduler checks on that same interval.
const preemptWindow = 20 * time.Second

// LiveWatch reports whether a live tune that needs its own tuner would leave a
// recording without one. busyOthers is how many tuners are already in use on
// other channels. A recording counts while now is inside its start pad, or
// inside preemptWindow, and the recording has not started. allow is false when
// the viewer should confirm.
func LiveWatch(tunerCount, busyOthers int, upcoming []Soon, now time.Time) (allow bool, warning string) {
	if tunerCount < 1 {
		tunerCount = 1
	}
	if busyOthers < 0 {
		busyOthers = 0
	}
	due := make([]Soon, 0, len(upcoming))
	for _, item := range upcoming {
		if inPad(item, now) {
			due = append(due, item)
		}
	}
	needed := dueTuners(due)
	if needed == 0 || tunerCount-busyOthers-1 >= needed {
		return true, ""
	}
	soon := due[0]
	for _, item := range due[1:] {
		if item.Start.Before(soon.Start) {
			soon = item
		}
	}
	return false, liveWarningText(soon, needed)
}

func inPad(item Soon, now time.Time) bool {
	if item.Start.IsZero() || !now.Before(item.Start) {
		return false
	}
	pad := item.Pad
	if pad < preemptWindow {
		pad = preemptWindow
	}
	return !now.Before(item.Start.Add(-pad))
}

func dueTuners(due []Soon) int {
	seen := map[int64]struct{}{}
	anon := 0
	for _, item := range due {
		id := item.ChannelID
		if id == 0 {
			anon--
			id = int64(anon)
		}
		seen[id] = struct{}{}
	}
	return len(seen)
}

func liveWarningText(soon Soon, needed int) string {
	title := strings.TrimSpace(soon.Title)
	if title == "" {
		title = "A recording"
	}
	when := soon.Start.In(time.Local).Format("3:04 PM")
	if needed > 1 {
		return fmt.Sprintf("%s starts at %s. Watching now will miss %d recordings.", title, when, needed)
	}
	return fmt.Sprintf("%s starts at %s. Watching now will miss that recording.", title, when)
}
