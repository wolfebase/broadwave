package guide

import (
	"sort"
	"strings"

	"broadwave/internal/network"
	"broadwave/internal/store"
)

// assign pairs each XMLTV channel with one lineup channel.
// A guide key wins. Otherwise the channel number wins, then the call sign,
// including a call sign that only differs by a network suffix (WTSTDT and WTSTDT1).
// When two stations share a number (a home between two markets), the number
// counts only where the name or call sign agrees too.
func assign(xmlCh []channel, lineup []store.Channel) map[string]int64 {
	out := map[string]int64{}
	shared := sharedNumbers(lineup)
	usedXML := map[string]bool{}
	usedLine := map[int64]bool{}
	for _, line := range lineup {
		key := strings.TrimSpace(line.GuideKey)
		if key == "" {
			continue
		}
		for _, ch := range xmlCh {
			if usedXML[ch.ID] {
				continue
			}
			if same(ch.ID, key) || nameIs(ch, key) {
				out[ch.ID] = line.ID
				usedXML[ch.ID] = true
				usedLine[line.ID] = true
				break
			}
		}
	}
	type pair struct {
		xml  string
		line int64
		rank int
	}
	var pairs []pair
	for _, ch := range xmlCh {
		if usedXML[ch.ID] {
			continue
		}
		for _, line := range lineup {
			if usedLine[line.ID] {
				continue
			}
			if rank := rankMatch(ch, line, shared[strings.TrimSpace(line.GuideNumber)]); rank > 0 {
				pairs = append(pairs, pair{ch.ID, line.ID, rank})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].rank != pairs[j].rank {
			return pairs[i].rank > pairs[j].rank
		}
		if pairs[i].xml != pairs[j].xml {
			return pairs[i].xml < pairs[j].xml
		}
		return pairs[i].line < pairs[j].line
	})
	for _, p := range pairs {
		if usedXML[p.xml] || usedLine[p.line] {
			continue
		}
		out[p.xml] = p.line
		usedXML[p.xml] = true
		usedLine[p.line] = true
	}
	return out
}

func rankMatch(ch channel, line store.Channel, sharedNumber bool) int {
	call := normCall(line.GuideName)
	named := same(ch.ID, line.GuideName) || nameIs(ch, line.GuideName) || nameIs(ch, line.DisplayName)
	called := len(call) >= 3 && (normCall(ch.ID) == call || namesCall(ch, call))
	if same(ch.ID, line.GuideNumber) || tokenHas(ch, line.GuideNumber) {
		if !sharedNumber {
			return 100
		}
		if named || called {
			return 110
		}
		// The number alone can't tell the two stations apart.
		return 0
	}
	if named {
		return 90
	}
	if called {
		return 80
	}
	return 0
}

// sharedNumbers are guide numbers two different stations use in one lineup.
// The same station on two tuners is not a shared number.
func sharedNumbers(lineup []store.Channel) map[string]bool {
	names := map[string]map[string]bool{}
	for _, line := range lineup {
		number := strings.TrimSpace(line.GuideNumber)
		if number == "" {
			continue
		}
		if names[number] == nil {
			names[number] = map[string]bool{}
		}
		names[number][normCall(line.GuideName)] = true
	}
	out := map[string]bool{}
	for number, set := range names {
		if len(set) > 1 {
			out[number] = true
		}
	}
	return out
}

func same(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func nameIs(ch channel, want string) bool {
	for _, name := range ch.Names {
		if same(name, want) {
			return true
		}
	}
	return false
}

func tokenHas(ch channel, number string) bool {
	number = strings.TrimSpace(number)
	if number == "" {
		return false
	}
	for _, name := range append([]string{ch.ID}, ch.Names...) {
		for _, tok := range strings.Fields(name) {
			if same(tok, number) {
				return true
			}
		}
	}
	return false
}

func namesCall(ch channel, call string) bool {
	if normCall(ch.ID) == call {
		return true
	}
	for _, name := range ch.Names {
		for _, tok := range strings.Fields(name) {
			if normCall(tok) == call {
				return true
			}
		}
	}
	return false
}

func normCall(s string) string {
	return network.NormCall(s)
}
