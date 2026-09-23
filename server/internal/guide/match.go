package guide

import (
	"sort"
	"strings"

	"waveguide/internal/store"
)

// assign pairs each XMLTV channel with one lineup channel.
// A guide key wins. Otherwise the channel number wins, then the call sign,
// including a call sign that only differs by a network suffix (KCTVDT and KCTVDT1).
func assign(xmlCh []channel, lineup []store.Channel) map[string]int64 {
	out := map[string]int64{}
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
			if rank := rankMatch(ch, line); rank > 0 {
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

func rankMatch(ch channel, line store.Channel) int {
	if same(ch.ID, line.GuideNumber) || tokenHas(ch, line.GuideNumber) {
		return 100
	}
	if same(ch.ID, line.GuideName) || nameIs(ch, line.GuideName) || nameIs(ch, line.DisplayName) {
		return 90
	}
	call := normCall(line.GuideName)
	if len(call) < 3 {
		return 0
	}
	if normCall(ch.ID) == call || namesCall(ch, call) {
		return 80
	}
	return 0
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
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	for _, suf := range []string{"DT1", "DT2", "DT3", "DT4", "HD", "DT", "TV", "LD"} {
		if strings.HasSuffix(s, suf) && len(s)-len(suf) >= 3 {
			s = strings.TrimSuffix(s, suf)
		}
	}
	return s
}
