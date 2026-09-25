package dvr

import (
	"strings"

	"broadwave/internal/store"
)

// ApplyLibrary marks airings a pass should not start: already recorded, over the limit, or skipped once.
func ApplyLibrary(items []Planned, passes []store.Pass, recs []store.Recording, seen map[string]bool, skips map[string]bool) []Planned {
	byID := map[int64]store.Pass{}
	for _, pass := range passes {
		byID[pass.ID] = pass
	}
	for i := range items {
		airing := items[i].Airing
		key, starts := SkipParts(airing)
		if key != "" && skips[key+"|"+starts] {
			items[i].Skipped = true
			items[i].Reason = "Skipped once"
			// The viewer already chose another airing, so don't offer this one again.
			items[i].Conflict = false
			items[i].Suggestion = nil
			continue
		}
		if items[i].Skipped {
			continue
		}
		pass := byID[items[i].PassID]
		if key != "" && haveEpisode(pass, recs, seen, key) {
			items[i].Skipped = true
			items[i].Reason = "Already recorded"
			continue
		}
		if pass.LimitCount > 0 && unwatchedCount(recs, pass) >= pass.LimitCount {
			items[i].Skipped = true
			items[i].Reason = "Limit reached"
		}
	}
	return items
}

func haveEpisode(pass store.Pass, recs []store.Recording, seen map[string]bool, key string) bool {
	for _, rec := range recs {
		if rec.Status == "failed" {
			continue
		}
		if store.EpisodeKey(rec.ProgramID, rec.Title, rec.Subtitle, rec.ChannelID) == key {
			return true
		}
	}
	deleted, ok := seen[key]
	if !ok {
		return false
	}
	if deleted && pass.Rerecord {
		return false
	}
	return true
}

func unwatchedCount(recs []store.Recording, pass store.Pass) int {
	n := 0
	for _, rec := range recs {
		if rec.Status == "failed" || !sameShow(pass, rec) {
			continue
		}
		if !rec.Played() {
			n++
		}
	}
	return n
}

func sameShow(pass store.Pass, rec store.Recording) bool {
	if pass.ChannelID != 0 && pass.ChannelID != rec.ChannelID {
		return false
	}
	switch strings.ToLower(pass.MatchKind) {
	case "category":
		return categoryHas(rec.Category, pass.Title)
	case "contains":
		return strings.Contains(strings.ToLower(rec.Title), strings.ToLower(pass.Title))
	default:
		return strings.EqualFold(rec.Title, pass.Title)
	}
}

// KeepVictims returns completed recording ids the pass's keep rule should remove.
func KeepVictims(pass store.Pass, recs []store.Recording) []int64 {
	var mine []store.Recording
	for _, rec := range recs {
		if rec.Status == "recording" || rec.Status == "failed" || !sameShow(pass, rec) {
			continue
		}
		mine = append(mine, rec)
	}
	mode := strings.ToLower(strings.TrimSpace(pass.KeepMode))
	var ids []int64
	switch mode {
	case "unwatched":
		for _, rec := range mine {
			if rec.Played() {
				ids = append(ids, rec.ID)
			}
		}
	case "last":
		n := pass.KeepCount
		if n < 1 {
			n = 1
		}
		if len(mine) <= n {
			return nil
		}
		// recordings arrive newest id first from the store, but don't assume that.
		for i := 0; i < len(mine); i++ {
			for j := i + 1; j < len(mine); j++ {
				if mine[j].StartedAt.After(mine[i].StartedAt) {
					mine[i], mine[j] = mine[j], mine[i]
				}
			}
		}
		for _, rec := range mine[n:] {
			ids = append(ids, rec.ID)
		}
	}
	return ids
}
