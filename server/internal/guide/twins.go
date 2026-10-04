package guide

import "broadwave/internal/store"

// ShareTwins gives each ATSC 3.0 channel with no listings of its own the
// listings and logo of its 1.0 twin (104.1 WDAF carries 4.1 WDAF-DT's guide),
// and the other way when only the 3.0 channel was listed.
func ShareTwins(rows []store.Airing, art map[int64]string, lineup []store.Channel) []store.Airing {
	have := map[int64]bool{}
	for _, r := range rows {
		have[r.ChannelID] = true
	}
	n := len(rows)
	for c3, ones := range store.Twins(lineup) {
		if have[c3] {
			if !anyHave(have, ones) {
				rows = copyRows(rows, n, c3, ones[0])
				if art != nil && art[ones[0]] == "" && art[c3] != "" {
					art[ones[0]] = art[c3]
				}
			}
			continue
		}
		for _, from := range ones {
			if !have[from] {
				continue
			}
			rows = copyRows(rows, n, from, c3)
			if art != nil && art[c3] == "" && art[from] != "" {
				art[c3] = art[from]
			}
			break
		}
	}
	return rows
}

// copyRows appends a copy of the first n rows on channel from, moved to channel to.
func copyRows(rows []store.Airing, n int, from, to int64) []store.Airing {
	for _, r := range rows[:n] {
		if r.ChannelID == from {
			r.ID = 0
			r.ChannelID = to
			rows = append(rows, r)
		}
	}
	return rows
}

func anyHave(have map[int64]bool, ids []int64) bool {
	for _, id := range ids {
		if have[id] {
			return true
		}
	}
	return false
}

// ShareSame gives every tuner's row of one channel the listings and logo of
// the row the guide matched, so the row shown on the guide is listed whichever
// tuner it comes from.
func ShareSame(rows []store.Airing, art map[int64]string, lineup []store.Channel) []store.Airing {
	have := map[int64]bool{}
	for _, r := range rows {
		have[r.ChannelID] = true
	}
	n := len(rows)
	for _, ids := range store.SameChannels(lineup) {
		var from int64
		for _, id := range ids {
			if have[id] {
				from = id
				break
			}
		}
		if from == 0 {
			continue
		}
		for _, to := range ids {
			if have[to] {
				continue
			}
			rows = copyRows(rows, n, from, to)
			if art != nil && art[to] == "" && art[from] != "" {
				art[to] = art[from]
			}
		}
	}
	return rows
}
