package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// sameKey groups one channel carried by two tuners on the same antenna: the
// same guide number and the same station. Playlist rows have their own numbers.
func sameKey(ch Channel) string {
	if strings.HasPrefix(ch.DeviceID, "src-") || ch.Protected || !ch.Present {
		return ""
	}
	name := callKey(ch.GuideName)
	if name == "" {
		name = strings.ToUpper(strings.TrimSpace(ch.GuideName))
	}
	return ch.GuideNumber + "|" + name
}

// SameChannels groups the rows of one channel carried by several tuners on
// the same antenna. Each group has rows from at least two devices.
func SameChannels(chs []Channel) [][]int64 {
	var out [][]int64
	for _, idx := range sameGroups(chs) {
		ids := make([]int64, len(idx))
		for i, j := range idx {
			ids[i] = chs[j].ID
		}
		out = append(out, ids)
	}
	return out
}

func sameGroups(chs []Channel) [][]int {
	groups := map[string][]int{}
	var keys []string
	for i, ch := range chs {
		if key := sameKey(ch); key != "" {
			if groups[key] == nil {
				keys = append(keys, key)
			}
			groups[key] = append(groups[key], i)
		}
	}
	var dupes [][]int
	for _, key := range keys {
		idx := groups[key]
		devices := map[string]bool{}
		for _, i := range idx {
			devices[chs[i].DeviceID] = true
		}
		if len(devices) > 1 {
			dupes = append(dupes, idx)
		}
	}
	return dupes
}

// markSameAs leaves one row per channel on the guide when several tuners carry
// it, and points the others at it with SameAs. Nothing is stored, so removing a
// tuner or changing a priority just picks again. Watching a SameAs row still
// works: a channel number plays from any tuner that has it.
func (s *Store) markSameAs(ctx context.Context, chs []Channel) error {
	dupes := sameGroups(chs)
	if len(dupes) == 0 {
		return nil
	}
	priority, answering, err := s.deviceRanks(ctx)
	if err != nil {
		return err
	}
	listed, err := s.listedChannels(ctx)
	if err != nil {
		return err
	}
	for _, idx := range dupes {
		// A row the viewer hid stays hidden, so its twin on the other tuner is
		// the one to show. Then a tuner that still answers, the tuner tried
		// first, and the one with listings.
		sort.SliceStable(idx, func(a, b int) bool {
			x, y := chs[idx[a]], chs[idx[b]]
			if (x.Enabled && !x.Hidden) != (y.Enabled && !y.Hidden) {
				return x.Enabled && !x.Hidden
			}
			if answering[x.DeviceID] != answering[y.DeviceID] {
				return answering[x.DeviceID]
			}
			if priority[x.DeviceID] != priority[y.DeviceID] {
				return priority[x.DeviceID] < priority[y.DeviceID]
			}
			if listed[x.ID] != listed[y.ID] {
				return listed[x.ID]
			}
			if x.DeviceID != y.DeviceID {
				return x.DeviceID < y.DeviceID
			}
			return x.ID < y.ID
		})
		shown := &chs[idx[0]]
		for _, i := range idx[1:] {
			other := &chs[i]
			other.SameAs = shown.ID
			// A favorite or a name set on either row belongs to the channel.
			shown.Favorite = shown.Favorite || other.Favorite
			if shown.DisplayName == shown.GuideName && other.DisplayName != other.GuideName {
				shown.DisplayName = other.DisplayName
			}
			if shown.DisplayNumber == shown.GuideNumber && other.DisplayNumber != other.GuideNumber {
				shown.DisplayNumber = other.DisplayNumber
			}
		}
	}
	return nil
}

// sameChannel is id and every other tuner's row for the same channel.
func (s *Store) sameChannel(ctx context.Context, id int64) ([]int64, error) {
	chs, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	key := ""
	for _, ch := range chs {
		if ch.ID == id {
			key = sameKey(ch)
		}
	}
	out := []int64{id}
	if key == "" {
		return out, nil
	}
	for _, ch := range chs {
		if ch.ID != id && sameKey(ch) == key {
			out = append(out, ch.ID)
		}
	}
	return out, nil
}

// staleAfter is three missed rounds of the five-minute tuner refresh.
const staleAfter = 30 * time.Minute

// deviceRanks is each device's priority, and whether it answered the tuner
// refresh lately. A device never seen yet counts as answering.
func (s *Store) deviceRanks(ctx context.Context) (map[string]int, map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id, priority, last_seen FROM devices`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	priority := map[string]int{}
	answering := map[string]bool{}
	for rows.Next() {
		var id, seen string
		var p int
		if err := rows.Scan(&id, &p, &seen); err != nil {
			return nil, nil, err
		}
		priority[id] = p
		at, err := time.Parse(time.RFC3339, seen)
		answering[id] = err != nil || time.Since(at) < staleAfter
	}
	return priority, answering, rows.Err()
}

// listedChannels is every channel with an airing that hasn't ended.
func (s *Store) listedChannels(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT channel_id FROM airings WHERE ends_at > ?`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
