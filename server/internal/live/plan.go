package live

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PlanChannel is one channel a multiview might show.
type PlanChannel struct {
	ID          int64
	FrequencyHz int
	Number      string
	Name        string
	// Direct is a link or file. It does not take an antenna tuner.
	Direct bool
}

func (c PlanChannel) label() string {
	if c.Number != "" {
		return c.Number
	}
	if c.Name != "" {
		return c.Name
	}
	return strconv.FormatInt(c.ID, 10)
}

// TunedFreq is a frequency this server already holds, with the channels on it.
type TunedFreq struct {
	FrequencyHz int
	Labels      []string
}

// Playable is a channel the plan can start.
type Playable struct {
	ChannelID   int64 `json:"channelId"`
	FrequencyHz int   `json:"frequencyHz"`
	Shared      bool  `json:"shared"`
}

// Blocked is a channel that would need a tuner the plan does not have.
type Blocked struct {
	ChannelID int64    `json:"channelId"`
	Reason    string   `json:"reason"`
	Holders   []string `json:"holders"`
}

// Offer is one row in the add-a-channel picker.
// Cost is "same", "tuner", "none", or "on".
type Offer struct {
	ChannelID int64  `json:"channelId"`
	Cost      string `json:"cost"`
	Label     string `json:"label"`
}

// Stop is a tile a scheduled recording will take. The tile keeps playing until At.
type Stop struct {
	ChannelID int64     `json:"channelId"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
}

// Reservation is a recording that needs a tuner at At. It wins over a live tile.
type Reservation struct {
	Channel PlanChannel
	Title   string
	At      time.Time
}

// MultiviewPlan says which of the requested channels can play together.
type MultiviewPlan struct {
	Playable     []Playable `json:"playable"`
	Blocked      []Blocked  `json:"blocked"`
	TunersNeeded int        `json:"tunersNeeded"`
	TunersFree   int        `json:"tunersFree"`
	Note         string     `json:"note,omitempty"`
	Offers       []Offer    `json:"offers,omitempty"`
	Stops        []Stop     `json:"stops,omitempty"`
}

type group struct {
	freq     int
	channels []PlanChannel
	known    bool
	direct   bool
	index    int
}

// PlanMultiview budgets tuners for a set of channels.
// Channels on one known frequency share a tuner. An unknown frequency costs one tuner each.
// ours are frequencies this server already has tuned. foreign are labels of tuners held by someone else.
func PlanMultiview(want []PlanChannel, tunerCount int, ours []TunedFreq, foreign []string) MultiviewPlan {
	if tunerCount < 0 {
		tunerCount = 0
	}
	seen := map[int64]bool{}
	var ordered []PlanChannel
	for _, c := range want {
		if c.ID == 0 || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		ordered = append(ordered, c)
	}
	groups := map[string]*group{}
	var keys []string
	for i, c := range ordered {
		key := "id:" + strconv.FormatInt(c.ID, 10)
		known := c.FrequencyHz > 0
		if c.Direct {
			key = "direct:" + strconv.FormatInt(c.ID, 10)
		} else if known {
			key = "hz:" + strconv.Itoa(c.FrequencyHz)
		}
		g := groups[key]
		if g == nil {
			g = &group{freq: c.FrequencyHz, known: known, direct: c.Direct, index: i}
			groups[key] = g
			keys = append(keys, key)
		}
		g.channels = append(g.channels, c)
	}
	oursFreq := map[int][]string{}
	occupied := 0
	for _, t := range ours {
		if t.FrequencyHz <= 0 {
			occupied++
			continue
		}
		if _, ok := oursFreq[t.FrequencyHz]; ok {
			oursFreq[t.FrequencyHz] = append(oursFreq[t.FrequencyHz], t.Labels...)
			continue
		}
		oursFreq[t.FrequencyHz] = append([]string{}, t.Labels...)
		occupied++
	}
	occupied += len(foreign)
	free := tunerCount - occupied
	if free < 0 {
		free = 0
	}
	plan := MultiviewPlan{Playable: []Playable{}, Blocked: []Blocked{}}
	var notes []string
	var taken []string
	taken = append(taken, foreign...)
	for _, labels := range oursFreq {
		taken = append(taken, labels...)
	}
	needed := occupied
	for _, key := range keys {
		g := groups[key]
		onAir := g.known && oursFreq[g.freq] != nil
		shared := len(g.channels) > 1 || onAir
		if g.direct || onAir || free > 0 {
			if !g.direct && !onAir {
				free--
				needed++
			}
			for _, c := range g.channels {
				plan.Playable = append(plan.Playable, Playable{ChannelID: c.ID, FrequencyHz: c.FrequencyHz, Shared: shared})
				taken = append(taken, c.label())
			}
			if len(g.channels) > 1 {
				notes = append(notes, englishList(labelsOf(g.channels))+" share one tuner.")
			}
			continue
		}
		reason := busyReason(tunerCount, unique(taken))
		holders := unique(taken)
		for _, c := range g.channels {
			plan.Blocked = append(plan.Blocked, Blocked{ChannelID: c.ID, Reason: reason, Holders: holders})
		}
	}
	plan.TunersNeeded = needed
	plan.TunersFree = free
	plan.Note = strings.Join(notes, " ")
	return plan
}

func labelsOf(channels []PlanChannel) []string {
	out := make([]string, len(channels))
	for i, c := range channels {
		out[i] = c.label()
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func busyReason(tunerCount int, holders []string) string {
	var lead string
	switch tunerCount {
	case 0:
		return "No tuner is available."
	case 1:
		lead = "The tuner is busy."
	case 2:
		lead = "Both tuners are busy."
	default:
		lead = "Every tuner is busy."
	}
	if len(holders) == 0 {
		return lead
	}
	verb := "is"
	if len(holders) > 1 {
		verb = "are"
	}
	return lead + " " + englishList(holders) + " " + verb + " on."
}

type channelSlot struct {
	freq     int
	known    bool
	direct   bool
	channels []PlanChannel
	maxID    int64
}

// Picker prices every candidate against the channels already chosen.
// A reservation keeps its tuner: a later recording drops the highest-id tile that is not on that station.
// now is the clock for "3:00 PM" versus "Friday at 3:00 PM".
func Picker(current, candidates []PlanChannel, tunerCount int, ours []TunedFreq, foreign []string, reserved []Reservation, now time.Time) ([]Offer, []Stop) {
	current = dedupePlans(current)
	base := PlanMultiview(current, tunerCount, ours, foreign)
	on := map[int64]bool{}
	for _, p := range base.Playable {
		on[p.ChannelID] = true
	}
	var onAir []PlanChannel
	for _, c := range current {
		if on[c.ID] {
			onAir = append(onAir, c)
		}
	}
	slots := slotsOf(onAir)
	heldFreq := map[int]bool{}
	heldID := map[int64]bool{}
	for _, slot := range slots {
		if slot.known {
			heldFreq[slot.freq] = true
		}
		for _, c := range slot.channels {
			heldID[c.ID] = true
		}
	}
	for _, t := range ours {
		if t.FrequencyHz > 0 {
			heldFreq[t.FrequencyHz] = true
		}
	}
	free := base.TunersFree
	reserved = append([]Reservation(nil), reserved...)
	sort.SliceStable(reserved, func(i, j int) bool { return reserved[i].At.Before(reserved[j].At) })
	var stops []Stop
	for _, r := range reserved {
		if r.Channel.Direct || heldID[r.Channel.ID] || (r.Channel.FrequencyHz > 0 && heldFreq[r.Channel.FrequencyHz]) {
			if r.Channel.FrequencyHz > 0 {
				heldFreq[r.Channel.FrequencyHz] = true
			}
			heldID[r.Channel.ID] = true
			continue
		}
		if free > 0 {
			free--
		} else if idx := victimSlot(slots, r); idx >= 0 {
			slot := slots[idx]
			slots = append(slots[:idx], slots[idx+1:]...)
			for _, c := range slot.channels {
				delete(heldID, c.ID)
			}
			if slot.known {
				delete(heldFreq, slot.freq)
			}
			reason := StopWarning(labelsOf(slot.channels), r.At, r.Title, now)
			for _, c := range slot.channels {
				stops = append(stops, Stop{ChannelID: c.ID, Reason: reason, At: r.At})
			}
		} else {
			continue
		}
		if r.Channel.FrequencyHz > 0 {
			heldFreq[r.Channel.FrequencyHz] = true
		}
		heldID[r.Channel.ID] = true
	}
	if len(candidates) == 0 {
		if len(stops) == 0 {
			return nil, nil
		}
		return nil, stops
	}
	offers := make([]Offer, 0, len(candidates))
	for _, c := range candidates {
		offers = append(offers, offerFor(c, onAir, ours, reserved, free))
	}
	if len(stops) == 0 {
		return offers, nil
	}
	return offers, stops
}

// StopWarning is the line shown before a recording takes a tile.
func StopWarning(numbers []string, at time.Time, title string, now time.Time) string {
	if strings.TrimSpace(title) == "" {
		title = "A show"
	}
	verb := "stops"
	if len(numbers) != 1 {
		verb = "stop"
	}
	return englishList(numbers) + " " + verb + " at " + clockLabel(at, now) + ". " + title + " is recording."
}

func clockLabel(at, now time.Time) string {
	local := at.In(time.Local)
	today := now.In(time.Local)
	hour := local.Hour()
	suffix := "AM"
	if hour >= 12 {
		suffix = "PM"
	}
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	tod := fmt.Sprintf("%d:%02d %s", hour, local.Minute(), suffix)
	if local.Year() == today.Year() && local.YearDay() == today.YearDay() {
		return tod
	}
	return local.Weekday().String() + " at " + tod
}

func dedupePlans(want []PlanChannel) []PlanChannel {
	seen := map[int64]bool{}
	var out []PlanChannel
	for _, c := range want {
		if c.ID == 0 || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out
}

func slotsOf(channels []PlanChannel) []channelSlot {
	index := map[string]int{}
	var slots []channelSlot
	for _, c := range channels {
		key := "id:" + strconv.FormatInt(c.ID, 10)
		known := c.FrequencyHz > 0
		if c.Direct {
			key = "direct:" + strconv.FormatInt(c.ID, 10)
		} else if known {
			key = "hz:" + strconv.Itoa(c.FrequencyHz)
		}
		i, ok := index[key]
		if !ok {
			index[key] = len(slots)
			slots = append(slots, channelSlot{freq: c.FrequencyHz, known: known && !c.Direct, direct: c.Direct, maxID: c.ID})
			i = len(slots) - 1
		}
		slots[i].channels = append(slots[i].channels, c)
		if c.ID > slots[i].maxID {
			slots[i].maxID = c.ID
		}
	}
	return slots
}

func victimSlot(slots []channelSlot, r Reservation) int {
	best := -1
	for i, slot := range slots {
		if slot.direct || slotClaims(slot, r) {
			continue
		}
		if best < 0 || slot.maxID > slots[best].maxID {
			best = i
		}
	}
	return best
}

func slotClaims(slot channelSlot, r Reservation) bool {
	if r.Channel.FrequencyHz > 0 && slot.known && slot.freq == r.Channel.FrequencyHz {
		return true
	}
	for _, c := range slot.channels {
		if c.ID == r.Channel.ID {
			return true
		}
	}
	return false
}

func offerFor(c PlanChannel, onAir []PlanChannel, ours []TunedFreq, reserved []Reservation, free int) Offer {
	if c.Direct {
		return Offer{ChannelID: c.ID, Cost: "on"}
	}
	if label, ok := partnerLabel(c, onAir, ours, reserved); ok {
		return Offer{ChannelID: c.ID, Cost: "same", Label: "Same tune as " + label}
	}
	for _, cur := range onAir {
		if cur.ID == c.ID {
			return Offer{ChannelID: c.ID, Cost: "on"}
		}
	}
	if matchesReservation(c, reserved) || free > 0 {
		return Offer{ChannelID: c.ID, Cost: "tuner", Label: "Uses a tuner"}
	}
	return Offer{ChannelID: c.ID, Cost: "none", Label: "No tuner free"}
}

func partnerLabel(c PlanChannel, onAir []PlanChannel, ours []TunedFreq, reserved []Reservation) (string, bool) {
	if c.FrequencyHz <= 0 {
		return "", false
	}
	for _, other := range onAir {
		if other.ID != c.ID && other.FrequencyHz == c.FrequencyHz {
			return other.label(), true
		}
	}
	for _, t := range ours {
		if t.FrequencyHz != c.FrequencyHz {
			continue
		}
		for _, label := range t.Labels {
			if label != "" && label != c.label() {
				return label, true
			}
		}
	}
	for _, r := range reserved {
		if r.Channel.ID != c.ID && r.Channel.FrequencyHz == c.FrequencyHz {
			return r.Channel.label(), true
		}
	}
	return "", false
}

func matchesReservation(c PlanChannel, reserved []Reservation) bool {
	for _, r := range reserved {
		if r.Channel.Direct {
			continue
		}
		if r.Channel.ID == c.ID {
			return true
		}
		if c.FrequencyHz > 0 && r.Channel.FrequencyHz == c.FrequencyHz {
			return true
		}
	}
	return false
}

func englishList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}
