package live

import (
	"strconv"
	"strings"
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

// MultiviewPlan says which of the requested channels can play together.
type MultiviewPlan struct {
	Playable     []Playable `json:"playable"`
	Blocked      []Blocked  `json:"blocked"`
	TunersNeeded int        `json:"tunersNeeded"`
	TunersFree   int        `json:"tunersFree"`
	Note         string     `json:"note,omitempty"`
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
