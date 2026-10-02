package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Twin choices. A pair shows its 3.0 channel, its 1.0 channel, or both.
const (
	TwinATSC3 = "atsc3"
	TwinATSC1 = "atsc1"
	TwinBoth  = "both"
)

// IsATSC3 reports an ATSC 3.0 row from its lineup codecs. A guide number is not a sign: 100.1 can be 1.0.
func IsATSC3(video, audio string) bool {
	v := strings.ToUpper(strings.TrimSpace(video))
	if v != "HEVC" && v != "H265" {
		return false
	}
	return strings.Contains(strings.ToUpper(strings.ReplaceAll(audio, "-", "")), "AC4")
}

// callKey is the station call sign in a guide name: WDAF-DT, KCPT-1, and WDAF are all WDAF.
// It is empty when the name doesn't look like one.
func callKey(name string) string {
	s := strings.ToUpper(strings.TrimSpace(name))
	s = strings.NewReplacer("-", "", " ", "", ".", "").Replace(s)
	s = strings.TrimRightFunc(s, unicode.IsDigit)
	for _, suf := range []string{"HD", "DT", "TV", "LD", "CD"} {
		if strings.HasSuffix(s, suf) && len(s)-len(suf) >= 3 {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	// North American call signs start with K, W, C, or X.
	if len(s) < 3 || len(s) > 4 || !strings.ContainsRune("KWCX", rune(s[0])) {
		return ""
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return s
}

// minor is the subchannel of a guide number, 0 when it has none.
func minor(number string) int {
	_, after, ok := strings.Cut(number, ".")
	if !ok {
		return 0
	}
	n := 0
	for _, r := range after {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Twins pairs each clear ATSC 3.0 antenna channel with the 1.0 channels of the
// same station. The first 1.0 id is the one whose listings the 3.0 channel
// shares: same device first, then the lowest subchannel. Other devices that
// carry the same 1.0 channel follow it.
func Twins(chs []Channel) map[int64][]int64 {
	return pairs(chs, false)
}

// StandIns maps each encrypted ATSC 3.0 channel to the clear 1.0 channel of the
// same station, which plays and records in its place.
func StandIns(chs []Channel) map[int64]int64 {
	out := map[int64]int64{}
	for c3, ones := range pairs(chs, true) {
		out[c3] = ones[0]
	}
	return out
}

// pairs matches 3.0 rows to their station's 1.0 main channels: the clear 3.0
// rows, or with encrypted the protected ones.
func pairs(chs []Channel, encrypted bool) map[int64][]int64 {
	byKey := map[string][]Channel{}
	for _, ch := range chs {
		if strings.HasPrefix(ch.DeviceID, "src-") || IsATSC3(ch.VideoCodec, ch.AudioCodec) {
			continue
		}
		if key := callKey(ch.GuideName); key != "" {
			byKey[key] = append(byKey[key], ch)
		}
	}
	out := map[int64][]int64{}
	for _, c3 := range chs {
		if strings.HasPrefix(c3.DeviceID, "src-") || c3.Protected != encrypted || !IsATSC3(c3.VideoCodec, c3.AudioCodec) {
			continue
		}
		cands := byKey[callKey(c3.GuideName)]
		if len(cands) == 0 {
			continue
		}
		// Only the station's lowest subchannel is its main program: 19.2 is not 119.1's twin.
		low := minor(cands[0].GuideNumber)
		for _, c := range cands {
			low = min(low, minor(c.GuideNumber))
		}
		var mains []Channel
		for _, c := range cands {
			if minor(c.GuideNumber) == low && !c.Protected {
				mains = append(mains, c)
			}
		}
		sort.SliceStable(mains, func(i, j int) bool {
			if (mains[i].DeviceID == c3.DeviceID) != (mains[j].DeviceID == c3.DeviceID) {
				return mains[i].DeviceID == c3.DeviceID
			}
			if mains[i].Present != mains[j].Present {
				return mains[i].Present
			}
			return mains[i].ID < mains[j].ID
		})
		for _, c := range mains {
			out[c3.ID] = append(out[c3.ID], c.ID)
		}
	}
	return out
}

// markTwins fills Standard, TwinID, TwinChoice, and PlaysAs on a full channel list.
func markTwins(chs []Channel, choices map[int64]string) {
	index := map[int64]int{}
	for i, ch := range chs {
		index[ch.ID] = i
		if IsATSC3(ch.VideoCodec, ch.AudioCodec) && !strings.HasPrefix(ch.DeviceID, "src-") {
			chs[i].Standard = TwinATSC3
		}
	}
	for c3, one := range StandIns(chs) {
		chs[index[c3]].PlaysAs = one
	}
	for c3, ones := range Twins(chs) {
		choice := choices[c3]
		i := index[c3]
		chs[i].TwinID = ones[0]
		chs[i].TwinChoice = choice
		for _, id := range ones {
			j := index[id]
			if chs[j].TwinID == 0 {
				chs[j].TwinID = c3
				chs[j].TwinChoice = choice
			}
		}
	}
}

// ErrNoTwin means the channel has no ATSC 1.0/3.0 twin.
var ErrNoTwin = errors.New("channel has no ATSC 1.0 and 3.0 pair")

// ErrTwinChoice is a choice other than atsc3, atsc1, or both.
var ErrTwinChoice = errors.New("choice must be atsc3, atsc1, or both")

// SetTwinChoice shows the 3.0 channel, the 1.0 channel, or both for the pair id belongs to.
// The channel that is hidden passes its favorite to the one that stays.
func (s *Store) SetTwinChoice(ctx context.Context, id int64, choice string) error {
	if choice != TwinATSC3 && choice != TwinATSC1 && choice != TwinBoth {
		return ErrTwinChoice
	}
	chs, err := s.Channels(ctx, false)
	if err != nil {
		return err
	}
	for c3, ones := range Twins(chs) {
		if c3 == id || contains(ones, id) {
			return s.applyTwinChoice(ctx, chs, c3, ones, choice)
		}
	}
	return ErrNoTwin
}

func (s *Store) applyTwinChoice(ctx context.Context, chs []Channel, c3 int64, ones []int64, choice string) error {
	byID := map[int64]Channel{}
	for _, ch := range chs {
		byID[ch.ID] = ch
	}
	fav := byID[c3].Favorite
	for _, id := range ones {
		fav = fav || byID[id].Favorite
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	set := func(id int64, hidden bool) error {
		f := byID[id].Favorite
		if !hidden && choice != TwinBoth {
			f = fav
		}
		_, err := tx.ExecContext(ctx, `UPDATE channels SET hidden=?, favorite=? WHERE id=?`, boolInt(hidden), boolInt(f), id)
		return err
	}
	if err := set(c3, choice == TwinATSC1); err != nil {
		return err
	}
	for _, id := range ones {
		if err := set(id, choice == TwinATSC3); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET twin_choice=? WHERE id=?`, choice, c3); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyTwinDefaults decides each new pair once: the 3.0 channel shows unless
// its 1.0 twin is known to carry a taller picture. Pairs the user set keep their choice.
func (s *Store) ApplyTwinDefaults(ctx context.Context) error {
	chs, err := s.Channels(ctx, false)
	if err != nil {
		return err
	}
	heights, err := s.pictureHeights(ctx)
	if err != nil {
		return err
	}
	for c3, ones := range Twins(chs) {
		var ch Channel
		for _, c := range chs {
			if c.ID == c3 {
				ch = c
			}
		}
		if ch.TwinChoice != "" {
			continue
		}
		choice := TwinATSC3
		if h1, h3 := heights[ones[0]], heights[c3]; h1 > 0 && h3 > 0 && h1 > h3 {
			choice = TwinATSC1
		}
		if err := s.applyTwinChoice(ctx, chs, c3, ones, choice); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pictureHeights(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, picture_height FROM channels WHERE picture_height > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var h int
		if err := rows.Scan(&id, &h); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// RecordingAirings is Airings with each simulcast copy marked, so a series pass
// records the broadcast once. A pair records on the channel that shows; when
// both show (or neither), on the 1.0 channel. An encrypted 3.0 channel's
// copy goes to its 1.0 twin.
func (s *Store) RecordingAirings(ctx context.Context, from, to time.Time) ([]Airing, error) {
	rows, err := s.Airings(ctx, from, to)
	if err != nil {
		return nil, err
	}
	chs, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	twins, stand := Twins(chs), StandIns(chs)
	if len(twins) == 0 && len(stand) == 0 {
		return rows, nil
	}
	hidden := map[int64]bool{}
	for _, ch := range chs {
		hidden[ch.ID] = ch.Hidden
	}
	group, atsc3 := map[int64]int64{}, map[int64]bool{}
	for c3, ones := range twins {
		group[c3], atsc3[c3] = c3, true
		for _, id := range ones {
			group[id] = c3
		}
	}
	// An encrypted channel's broadcast is its 1.0 twin's.
	for c3, one := range stand {
		if _, ok := group[one]; !ok {
			group[one] = one
		}
		group[c3], atsc3[c3] = group[one], true
	}
	// better ranks a shown channel first, then 1.0 over 3.0, then the lower id.
	better := func(a, b int64) bool {
		if hidden[a] != hidden[b] {
			return !hidden[a]
		}
		if atsc3[a] != atsc3[b] {
			return atsc3[b]
		}
		return a < b
	}
	type key struct {
		group int64
		start int64
		title string
	}
	best := map[key]int64{}
	for _, r := range rows {
		g, ok := group[r.ChannelID]
		if !ok {
			continue
		}
		k := key{g, r.Start.Unix(), strings.ToLower(strings.TrimSpace(r.Title))}
		if cur, ok := best[k]; !ok || better(r.ChannelID, cur) {
			best[k] = r.ChannelID
		}
	}
	for i, r := range rows {
		g, ok := group[r.ChannelID]
		if !ok {
			continue
		}
		if b := best[key{g, r.Start.Unix(), strings.ToLower(strings.TrimSpace(r.Title))}]; b != r.ChannelID {
			rows[i].Simulcast = b
		}
	}
	return rows, nil
}
