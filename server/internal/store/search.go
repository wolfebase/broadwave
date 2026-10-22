package store

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"broadwave/internal/hdhr"
	"broadwave/internal/network"
)

// AiringHit is a guide listing that matched a search, with the channel it is on.
type AiringHit struct {
	Airing
	GuideNumber string `json:"guideNumber"`
	ChannelName string `json:"channelName"`
}

// Search finds upcoming listings and recordings. A blank query matches nothing.
// A query shaped like a channel number ("4.1", "4-1", "41") finds that channel
// first; "4.1" with a dot is only a channel, never the word 41.
func (s *Store) Search(ctx context.Context, query string, from time.Time, limit int) ([]AiringHit, []Recording, error) {
	query = strings.TrimSpace(query)
	number, dotted := channelNumber(query)
	match := ftsMatch(query)
	if dotted {
		match = ""
	}
	if match == "" && number == "" {
		return []AiringHit{}, []Recording{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	airings, err := s.searchAirings(ctx, searchQuery{text: query, number: number, match: match}, from, limit)
	if err != nil {
		return nil, nil, err
	}
	recordings := []Recording{}
	if match != "" {
		recordings, err = s.searchRecordings(ctx, match, queryWords(query), limit)
		if err != nil {
			return nil, nil, err
		}
	}
	return airings, recordings, nil
}

type searchQuery struct {
	text   string
	number string
	match  string
}

// How well a listing matches, best first. A listing below tierMention only
// mentions the words (in its description, or by a stem like "new" for
// "news"); those are left out when any listing matches better.
const (
	tierNumber       = iota // the channel with that number, and its 3.0 twin
	tierNumberPrefix        // 4.10 for 4.1, 4.2 for 4
	tierTitleExact          // the title is the words
	tierTitle               // every word is a word of the title
	tierTitlePart           // every word starts a word of the title
	tierEpisode             // the episode title, whole words
	tierEpisodePart         // the episode title, word starts
	tierChannel             // the channel's name or call sign
	tierPeople              // cast, whole words
	tierPeoplePart          // cast, word starts
	tierCategory            // the listing's category
	tierMention      = 20
	tierPaid         = 21 // paid programming the title does not name
)

// channelAirings is how many listings a channel found by its number or name
// returns: what is on now and what follows.
const channelAirings = 4

// textCandidates bounds the rows one search reads before ranking.
const textCandidates = 3000

// searchPick is a listing that matched and how well.
type searchPick struct {
	id      int64
	channel int64
	title   string
	game    string
	start   string
	tier    int
	order   int
}

const searchPickColumns = `a.id, a.channel_id, a.title, a.subtitle, a.cast_list, a.category, a.game_id, a.starts_at`

const searchWhere = `
FROM airing_search
JOIN airings a ON a.id = airing_search.rowid
WHERE airing_search MATCH ?
  AND a.ends_at > ?
  AND a.channel_id IN (SELECT CAST(value AS INTEGER) FROM json_each(?))`

// The picks read only what ranking needs, soonest first; the full rows are
// read for the ones kept.
const searchTextSQL = `SELECT ` + searchPickColumns + searchWhere + `
ORDER BY a.starts_at, a.id
LIMIT ?`

const searchCategorySQL = `SELECT ` + searchPickColumns + `
FROM airings a
WHERE a.category IN (SELECT value FROM json_each(?))
  AND a.ends_at > ?
  AND a.channel_id IN (SELECT CAST(value AS INTEGER) FROM json_each(?))
ORDER BY a.starts_at, a.id
LIMIT ?`

const searchChannelSQL = `SELECT ` + searchPickColumns + `
FROM airings a
WHERE a.channel_id = ? AND a.ends_at > ?
ORDER BY a.starts_at, a.id
LIMIT ?`

const searchHitColumns = `a.id, a.channel_id, a.title, a.subtitle, a.description, a.category, a.starts_at, a.ends_at,
	a.program_id, a.is_new, a.image_url, a.image_width, a.image_height, a.season, a.episode, a.episode_label, a.original_air, a.series_id,
	a.is_live, a.is_premiere, a.is_finale, a.rating, a.cast_list, a.game_id, c.guide_number, c.guide_name`

const searchRowsSQL = `SELECT ` + searchHitColumns + `
FROM airings a
JOIN channels c ON c.id = a.channel_id
WHERE a.id IN (SELECT CAST(value AS INTEGER) FROM json_each(?))`

// perShow is how many airings of one show a search returns. A two-week guide
// lists a daily show fourteen times and a news word matches hundreds of
// listings; without a cap the next few hours fill the results and a show on
// next week never appears. A game is its own show.
const perShow = 6

// searchScope is the channels a search reads: the ones the guide shows, one
// row per channel when two tuners carry it, and an encrypted 3.0 station,
// which records as its clear twin.
type searchScope struct {
	all     []Channel
	byID    map[int64]Channel
	in      map[int64]bool
	ids     []int64
	standIn map[int64]int64
}

func newSearchScope(all []Channel) searchScope {
	sc := searchScope{all: all, byID: map[int64]Channel{}, in: map[int64]bool{}, standIn: map[int64]int64{}}
	shown := map[int64]bool{}
	for i, ch := range all {
		if ch.Network == "" {
			// As the channel list fills it, so "fox" finds a station whose
			// guide never said FOX.
			names := []string{ch.GuideName}
			if ch.DisplayName != "" && ch.DisplayName != ch.GuideName {
				names = append(names, ch.DisplayName)
			}
			ch.Network = network.Of(names)
			all[i] = ch
		}
		sc.byID[ch.ID] = ch
		if ch.Present && ch.Enabled && !ch.Hidden && ch.SameAs == 0 {
			shown[ch.ID] = true
		}
	}
	for _, ch := range all {
		if !shown[ch.ID] && (ch.PlaysAs == 0 || !shown[ch.PlaysAs]) {
			continue
		}
		if !shown[ch.ID] {
			sc.standIn[ch.ID] = ch.PlaysAs
		}
		sc.in[ch.ID] = true
		sc.ids = append(sc.ids, ch.ID)
	}
	return sc
}

// route is where a channel's listings are searched: the channel, the row the
// guide shows for it, and its ATSC 3.0 or 1.0 twin. 4.1 hidden for its 3.0
// twin finds 104.1.
func (sc searchScope) route(ch Channel) []int64 {
	ids := []int64{ch.ID, ch.SameAs, ch.TwinID, sc.byID[ch.TwinID].SameAs, ch.PlaysAs}
	for _, other := range sc.all {
		if other.ID != ch.ID && (other.TwinID == ch.ID || other.PlaysAs == ch.ID) {
			ids = append(ids, other.ID)
		}
	}
	var out []int64
	seen := map[int64]bool{}
	for _, id := range ids {
		if id != 0 && sc.in[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// numbered is the channels a number names, the exact number first: "4.1" is
// 4.1, then 4.10 to 4.19; "4" is channel 4, then 4.1, 4.2, and on. 41.1 is
// never 4.1.
func (sc searchScope) numbered(number string) []searchPick {
	type found struct {
		ch   Channel
		tier int
	}
	var list []found
	for _, ch := range sc.all {
		tier := -1
		for _, n := range []string{ch.DisplayNumber, ch.GuideNumber} {
			switch {
			case n == number:
				tier = tierNumber
			case tier < 0 && strings.HasPrefix(n, number) && (strings.Contains(number, ".") || n[len(number)] == '.'):
				tier = tierNumberPrefix
			}
		}
		if tier >= 0 {
			list = append(list, found{ch, tier})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].tier != list[j].tier {
			return list[i].tier < list[j].tier
		}
		return hdhr.CompareGuide(list[i].ch.DisplayNumber, list[j].ch.DisplayNumber) < 0
	})
	var out []searchPick
	for _, f := range list {
		for _, id := range sc.route(f.ch) {
			out = append(out, searchPick{channel: id, tier: f.tier, order: len(out)})
		}
	}
	return out
}

// named is the channels every word names: the guide name, a custom name, the
// call sign, or the network. "tv" alone names no channel. With starts, a word
// of three letters or more may start a name ("kbw" for KBWV); those follow the
// whole-word matches.
func (sc searchScope) named(words []string, starts bool) []searchPick {
	generic := map[string]bool{"tv": true, "dt": true, "hd": true, "ld": true, "cd": true}
	specific := false
	for _, w := range words {
		specific = specific || !generic[w]
	}
	if len(words) == 0 || !specific {
		return nil
	}
	var whole, part []Channel
	for _, ch := range sc.all {
		names := foldWords(ch.GuideName + " " + ch.DisplayName + " " + ch.Network + " " + callKey(ch.GuideName))
		switch {
		case allWords(words, names, false):
			whole = append(whole, ch)
		case starts && nameStarts(words, names):
			part = append(part, ch)
		}
	}
	var out []searchPick
	for _, ch := range append(whole, part...) {
		for _, id := range sc.route(ch) {
			out = append(out, searchPick{channel: id, tier: tierChannel, order: len(out)})
		}
	}
	return out
}

// nameStarts is every word a whole name word, or the start of one when it has
// three letters or more.
func nameStarts(words, names []string) bool {
	for _, w := range words {
		m := wordMatch(w, names)
		if m == 0 || (m == 1 && utf8.RuneCountInString(w) < 3) {
			return false
		}
	}
	return true
}

// ChannelHit is a channel a search names by its number, name, call sign, or
// network, with what is on it now when the guide knows.
type ChannelHit struct {
	ID       int64   `json:"id"`
	Number   string  `json:"number"`
	Name     string  `json:"name"`
	Network  string  `json:"network,omitempty"`
	Logo     string  `json:"logo,omitempty"`
	Standard string  `json:"standard,omitempty"`
	OnNow    *Airing `json:"onNow,omitempty"`
}

// channelResults is how many channels a search lists.
const channelResults = 12

// SearchChannels finds the channels the guide shows that a query names: a
// channel number (exact, then prefix, each with its ATSC 3.0 or 1.0 twin), or
// every word in the channel's name, call sign, or network. It needs no
// listings, so a house with only broadcast listings, or none yet, still finds
// 5.1 or "pbs". An encrypted 3.0 station is listed as the clear channel it
// plays as.
func (s *Store) SearchChannels(ctx context.Context, query string, now time.Time) ([]ChannelHit, error) {
	query = strings.TrimSpace(query)
	number, dotted := channelNumber(query)
	words := queryWords(query)
	if number == "" && (dotted || ftsMatch(query) == "") {
		return []ChannelHit{}, nil
	}
	all, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	sc := newSearchScope(all)
	var picks []searchPick
	if number != "" {
		picks = sc.numbered(number)
	}
	if !dotted {
		picks = append(picks, sc.named(words, true)...)
	}
	out := []ChannelHit{}
	seen := map[int64]bool{}
	for _, p := range picks {
		id := p.channel
		if twin, ok := sc.standIn[id]; ok {
			id = twin
		}
		if seen[id] || len(out) == channelResults {
			continue
		}
		seen[id] = true
		ch := sc.byID[id]
		hit := ChannelHit{ID: ch.ID, Number: ch.DisplayNumber, Name: ch.DisplayName, Network: ch.Network, Standard: ch.Standard}
		if ch.ArtURL != "" {
			hit.Logo = "/media/art/channel/" + strconv.FormatInt(ch.ID, 10)
		}
		on, err := s.querySearchHits(ctx, searchOnNowSQL, ch.ID, now.UTC().Format(time.RFC3339))
		if err != nil {
			return nil, err
		}
		if len(on) > 0 {
			hit.OnNow = &on[0].Airing
		}
		out = append(out, hit)
	}
	return out, nil
}

const searchOnNowSQL = `SELECT ` + searchHitColumns + `
FROM airings a
JOIN channels c ON c.id = a.channel_id
WHERE a.channel_id = ?1 AND a.starts_at <= ?2 AND a.ends_at > ?2
ORDER BY a.starts_at DESC
LIMIT 1`

// searchAirings ranks listings by where the words are: title, then episode
// title, channel, cast, category, and last a mention in the description, which
// is left out when anything matches better. Whole words beat word starts, and
// paid programming sinks unless its title is the words. Within a rank, what is
// on now comes first and then the soonest.
func (s *Store) searchAirings(ctx context.Context, q searchQuery, from time.Time, limit int) ([]AiringHit, error) {
	all, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	sc := newSearchScope(all)
	if len(sc.ids) == 0 {
		return []AiringHit{}, nil
	}
	channels, err := json.Marshal(sc.ids)
	if err != nil {
		return nil, err
	}
	fromArg := from.UTC().Format(time.RFC3339)
	words := queryWords(q.text)
	picks := map[int64]searchPick{}
	keep := func(p searchPick) {
		if held, ok := picks[p.id]; !ok || p.tier < held.tier || (p.tier == held.tier && p.order < held.order) {
			picks[p.id] = p
		}
	}
	readChannels := func(list []searchPick) error {
		for _, c := range list {
			rows, err := s.readPicks(ctx, searchChannelSQL, c.channel, fromArg, channelAirings)
			if err != nil {
				return err
			}
			for _, p := range rows {
				p.tier, p.order = c.tier, c.order
				keep(p)
			}
		}
		return nil
	}
	if q.number != "" {
		if err := readChannels(sc.numbered(q.number)); err != nil {
			return nil, err
		}
	}
	if q.match != "" {
		if err := readChannels(sc.named(words, false)); err != nil {
			return nil, err
		}
		phrase := strings.Join(foldWords(q.text), " ")
		rank := func(rows []rankedRow) {
			for _, r := range rows {
				r.pick.tier = rankListing(words, phrase, r.title, r.subtitle, r.cast, r.category)
				keep(r.pick)
			}
		}
		// Title, episode title, and cast first; the description is read
		// only when none of those match.
		named, err := s.readRanked(ctx, searchTextSQL, `{title subtitle cast_list} : (`+q.match+`)`, fromArg, string(channels), textCandidates)
		if err != nil {
			return nil, err
		}
		rank(named)
		filed, err := s.categoriesNamed(ctx, words)
		if err != nil {
			return nil, err
		}
		if len(filed) > 0 {
			list, err := json.Marshal(filed)
			if err != nil {
				return nil, err
			}
			byCategory, err := s.readRanked(ctx, searchCategorySQL, string(list), fromArg, string(channels), textCandidates)
			if err != nil {
				return nil, err
			}
			rank(byCategory)
		}
		if !anyStrong(picks) {
			rest, err := s.readRanked(ctx, searchTextSQL, q.match, fromArg, string(channels), textCandidates)
			if err != nil {
				return nil, err
			}
			rank(rest)
		}
	}
	ordered := make([]searchPick, 0, len(picks))
	strong := anyStrong(picks)
	for _, p := range picks {
		if !strong || p.tier < tierMention {
			ordered = append(ordered, p)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if a.order != b.order {
			return a.order < b.order
		}
		if a.start != b.start {
			return a.start < b.start
		}
		return a.id < b.id
	})
	// A stand-in's duplicates are dropped below, so keep a few more.
	want := limit + len(sc.standIn)*4
	type show struct{ title, game string }
	count := map[show]int{}
	var ids []int64
	for _, p := range ordered {
		if len(ids) == want {
			break
		}
		key := show{strings.ToLower(p.title), p.game}
		if count[key] == perShow {
			continue
		}
		count[key]++
		ids = append(ids, p.id)
	}
	if len(ids) == 0 {
		return []AiringHit{}, nil
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	hits, err := s.querySearchHits(ctx, searchRowsSQL, string(list))
	if err != nil {
		return nil, err
	}
	place := make(map[int64]int, len(ids))
	for i, id := range ids {
		place[id] = i
	}
	sort.Slice(hits, func(i, j int) bool { return place[hits[i].ID] < place[hits[j].ID] })
	return withoutTwinRepeats(hits, sc.standIn, limit), nil
}

func anyStrong(picks map[int64]searchPick) bool {
	for _, p := range picks {
		if p.tier < tierMention {
			return true
		}
	}
	return false
}

// rankedRow is a pick with the fields it is ranked by.
type rankedRow struct {
	pick                            searchPick
	title, subtitle, cast, category string
}

func (s *Store) readRanked(ctx context.Context, sqlText string, args ...any) ([]rankedRow, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rankedRow
	for rows.Next() {
		var r rankedRow
		if err := rows.Scan(&r.pick.id, &r.pick.channel, &r.title, &r.subtitle, &r.cast, &r.category, &r.pick.game, &r.pick.start); err != nil {
			return nil, err
		}
		r.pick.title = r.title
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) readPicks(ctx context.Context, sqlText string, args ...any) ([]searchPick, error) {
	rows, err := s.readRanked(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	out := make([]searchPick, len(rows))
	for i, r := range rows {
		out[i] = r.pick
	}
	return out, nil
}

// rankListing is the tier of a listing for the folded query words. Each word
// counts where it matches best, and the listing ranks by its weakest word.
func rankListing(words []string, phrase, title, subtitle, cast, category string) int {
	if len(words) == 0 {
		return tierMention
	}
	titleWords := foldWords(title)
	tier := tierTitleExact
	if strings.Join(titleWords, " ") != phrase {
		tier = tierTitle
		fields := []struct {
			words       []string
			whole, part int
		}{
			{titleWords, tierTitle, tierTitlePart},
			{foldWords(subtitle), tierEpisode, tierEpisodePart},
			{foldWords(cast), tierPeople, tierPeoplePart},
			{foldWords(category), tierCategory, tierCategory},
		}
		for _, w := range words {
			best := tierMention
			for _, f := range fields {
				switch wordMatch(w, f.words) {
				case 2:
					best = min(best, f.whole)
				case 1:
					best = min(best, f.part)
				}
			}
			tier = max(tier, best)
		}
	}
	if tier > tierTitle && paidProgramming(title, category) {
		return tierPaid
	}
	return tier
}

// wordMatch is 2 when w is one of the words, 1 when it starts one, else 0.
func wordMatch(w string, words []string) int {
	best := 0
	for _, x := range words {
		if x == w {
			return 2
		}
		if strings.HasPrefix(x, w) {
			best = 1
		}
	}
	return best
}

func allWords(words, in []string, prefix bool) bool {
	for _, w := range words {
		m := wordMatch(w, in)
		if m == 0 || (m == 1 && !prefix) {
			return false
		}
	}
	return true
}

// paidProgramming is an infomercial or a shopping hour.
func paidProgramming(title, category string) bool {
	switch strings.Join(foldWords(title), " ") {
	case "paid programming", "paid program", "infomercial":
		return true
	}
	for part := range strings.SplitSeq(category, ",") {
		switch strings.Join(foldWords(part), " ") {
		case "shop", "shopping", "home shopping", "infomercial", "paid programming", "paid program":
			return true
		}
	}
	return false
}

// channelNumber is the query as a channel number, "4.1" for "4.1" or "4-1",
// and whether it had a dot or dash. Anything else is "".
func channelNumber(query string) (string, bool) {
	major, minor, dotted := strings.Cut(strings.Replace(query, "-", ".", 1), ".")
	digits := func(s string) bool {
		return s != "" && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) < 0
	}
	if !digits(major) || (dotted && !digits(minor)) || len(query) > 9 {
		return "", false
	}
	if dotted {
		return major + "." + minor, true
	}
	return major, false
}

// queryWords is the query folded into words the way the index splits them.
// One-letter words are dropped, as ftsMatch drops them, unless that is all
// there is.
func queryWords(query string) []string {
	all := foldWords(query)
	var out []string
	for _, w := range all {
		if utf8.RuneCountInString(w) >= 2 {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// categoryCache holds the guide's distinct categories for one guide version.
// Category is not in the search index, and reading every listing for each
// search cost tens of milliseconds on a two-week guide.
type categoryCache struct {
	mu      sync.Mutex
	version int64
	list    []string
}

// categoriesNamed is the guide's categories that every word names, whole or
// as a word start.
func (s *Store) categoriesNamed(ctx context.Context, words []string) ([]string, error) {
	if len(words) == 0 {
		return nil, nil
	}
	var version int64
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM data_versions WHERE name = 'guide'`).Scan(&version); err != nil {
		return nil, err
	}
	c := &s.categories
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.list == nil || c.version != version {
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT category FROM airings WHERE category != ''`)
		if err != nil {
			return nil, err
		}
		list := []string{}
		for rows.Next() {
			var cat string
			if err := rows.Scan(&cat); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, cat)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		c.list, c.version = list, version
	}
	var out []string
	for _, cat := range c.list {
		if allWords(words, foldWords(cat), true) {
			out = append(out, cat)
		}
	}
	return out, nil
}

// plainLetters maps an accented lowercase letter to the letter the index
// stores, as unicode61 removes diacritics.
var plainLetters = func() map[rune]rune {
	m := map[rune]rune{}
	for base, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćĉċč", 'd': "ďđ", 'e': "èéêëēĕėęě", 'g': "ĝğġģ", 'h': "ĥħ",
		'i': "ìíîïĩīĭįı", 'j': "ĵ", 'k': "ķ", 'l': "ĺļľŀł", 'n': "ñńņňŉ", 'o': "òóôõöøōŏő",
		'r': "ŕŗř", 's': "śŝşš", 't': "ţťŧ", 'u': "ùúûüũūŭůűų", 'w': "ŵ", 'y': "ýÿŷ", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = base
		}
	}
	return m
}()

// foldWords lowercases s, drops accents, and splits it into words of letters
// and digits.
func foldWords(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if plain, ok := plainLetters[r]; ok {
			r = plain
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func (s *Store) querySearchHits(ctx context.Context, sqlText string, args ...any) ([]AiringHit, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AiringHit
	for rows.Next() {
		var hit AiringHit
		var start, end string
		var isNew, isLive, isPremiere, isFinale int
		if err := rows.Scan(&hit.ID, &hit.ChannelID, &hit.Title, &hit.Subtitle, &hit.Description, &hit.Category, &start, &end,
			&hit.ProgramID, &isNew, &hit.ImageURL, &hit.ImageWidth, &hit.ImageHeight, &hit.Season, &hit.Episode, &hit.EpisodeLabel, &hit.OriginalAir, &hit.SeriesID,
			&isLive, &isPremiere, &isFinale, &hit.Rating, &hit.Cast, &hit.GameID, &hit.GuideNumber, &hit.ChannelName); err != nil {
			return nil, err
		}
		hit.New = isNew != 0
		hit.Live = isLive != 0
		hit.Premiere = isPremiere != 0
		hit.Finale = isFinale != 0
		hit.Start, _ = time.Parse(time.RFC3339, start)
		hit.End, _ = time.Parse(time.RFC3339, end)
		out = append(out, hit)
	}
	return out, rows.Err()
}

// withoutTwinRepeats drops an encrypted station's listing when its clear twin
// lists the same show at the same time.
func withoutTwinRepeats(hits []AiringHit, standIn map[int64]int64, limit int) []AiringHit {
	type key struct {
		channel int64
		title   string
		start   int64
	}
	listed := map[key]bool{}
	for _, hit := range hits {
		if _, ok := standIn[hit.ChannelID]; !ok {
			listed[key{hit.ChannelID, hit.Title, hit.Start.Unix()}] = true
		}
	}
	out := []AiringHit{}
	for _, hit := range hits {
		if twin, ok := standIn[hit.ChannelID]; ok && listed[key{twin, hit.Title, hit.Start.Unix()}] {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, hit)
	}
	return out
}

// searchRecordings finds recordings, newest first. A recording that only
// mentions the words in its description is left out when one is named by them.
func (s *Store) searchRecordings(ctx context.Context, match string, words []string, limit int) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT `+recordingColumns+`
FROM recording_search
JOIN recordings r ON r.id = recording_search.rowid
LEFT JOIN progress p ON p.recording_id = r.id
WHERE recording_search MATCH ?
ORDER BY r.started_at DESC
LIMIT ?`, match, limit*4)
	if err != nil {
		return nil, err
	}
	found, err := scanRecordings(rows)
	if err != nil {
		return nil, err
	}
	phrase := strings.Join(words, " ")
	named := func(rec Recording) bool {
		return rankListing(words, phrase, rec.Title, rec.Subtitle, "", "") < tierMention
	}
	strong := false
	for _, rec := range found {
		strong = strong || named(rec)
	}
	out := []Recording{}
	for _, rec := range found {
		if len(out) == limit {
			break
		}
		if !strong || named(rec) {
			out = append(out, rec)
		}
	}
	return out, nil
}

// ftsMatch turns typed words into a prefix query. Punctuation that FTS treats
// as syntax is dropped so a title with a quote cannot break the search.
// teamMatch is an FTS query for listings whose title or subtitle names one
// of the teams. Each name is one quoted phrase, so FTS splits it the way it
// split the listing (accents, "A&M", one-letter words) and no character is
// query syntax. Description and cast are not searched: a news item that
// mentions a club is not that club's game. An empty result means no name had
// a letter or digit.
func teamMatch(names []string) string {
	var parts []string
	for _, name := range names {
		if strings.IndexFunc(name, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		phrase := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		parts = append(parts, `(title : `+phrase+` OR subtitle : `+phrase+`)`)
	}
	return strings.Join(parts, " OR ")
}

// ftsMatch keeps letters and digits in any script. It kept only ASCII, so
// "Pokémon" searched for "Pokmon" and "Café" for "Caf". The index folds
// accents (unicode61 removes diacritics), and the quoted word is folded the
// same way, so "pokemon" and "Pokémon" find each other.
func ftsMatch(query string) string {
	fields := strings.Fields(query)
	var parts []string
	for _, field := range fields {
		var b strings.Builder
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || r == '-' {
				b.WriteRune(r)
			}
		}
		word := b.String()
		if utf8.RuneCountInString(word) < 2 {
			continue
		}
		parts = append(parts, `"`+word+`"*`)
	}
	return strings.Join(parts, " ")
}
