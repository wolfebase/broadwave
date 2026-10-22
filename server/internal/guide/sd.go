package guide

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"broadwave/internal/fetchguard"
	"broadwave/internal/store"
)

// SDBase is the Schedules Direct JSON API.
const SDBase = "https://json.schedulesdirect.org/20141201"

// SDGuide is what one Schedules Direct pull brought: the listings, and the
// station logos and networks of the channels they matched.
type SDGuide struct {
	Rows     []store.Airing
	Art      map[int64]string
	Networks map[int64]string
	// Channels are the lineup channels that got listings.
	Channels []int64
}

// SDChannel is a lineup channel and the RF channel it was last tuned on
// (zero when unknown), which tells two stations with one number apart.
type SDChannel struct {
	store.Channel
	RF int
}

// SchedulesDirect pulls two weeks of listings for the lineup's channels.
// Empty account fields mean skip. The password is passed in and is not logged.
func SchedulesDirect(ctx context.Context, channels []SDChannel, user, pass, lineup string) (SDGuide, error) {
	user = strings.TrimSpace(user)
	lineup = strings.TrimSpace(lineup)
	if user == "" || pass == "" || lineup == "" {
		return SDGuide{}, nil
	}
	return NewSD(user, pass).Fetch(ctx, lineup, channels)
}

// SD is one Schedules Direct account. Its token is shared with every other
// SD for the same account, for as long as Schedules Direct honors it.
type SD struct {
	client *http.Client
	base   string
	user   string
	hash   string
}

// NewSD is an account on the public API. The password is kept only as the
// SHA-1 hash Schedules Direct asks for.
func NewSD(user, pass string) *SD {
	return newSD(fetchguard.Client(20*time.Second), SDBase, user, pass)
}

// NewSDWith is an account on another API address, for tests.
func NewSDWith(client *http.Client, base, user, pass string) *SD {
	return newSD(client, base, user, pass)
}

func newSD(client *http.Client, base, user, pass string) *SD {
	sum := sha1.Sum([]byte(pass))
	return &SD{client: client, base: base, user: strings.TrimSpace(user), hash: hex.EncodeToString(sum[:])}
}

// A token is good for 24 hours. One a little younger than that is reused,
// so a daily pull or a lineup search does not sign in again each time.
const sdTokenLife = 23 * time.Hour

type sdToken struct {
	value string
	until time.Time
}

var sdTokens = struct {
	sync.Mutex
	m map[string]sdToken
}{m: map[string]sdToken{}}

func (c *SD) tokenKey() string { return c.base + "|" + c.user + "|" + c.hash }

func (c *SD) token(ctx context.Context) (string, error) {
	key := c.tokenKey()
	sdTokens.Lock()
	tok, ok := sdTokens.m[key]
	sdTokens.Unlock()
	if ok && time.Now().Before(tok.until) {
		return tok.value, nil
	}
	raw, err := c.do(ctx, http.MethodPost, "/token", "", map[string]string{"username": c.user, "password": c.hash})
	if err != nil {
		return "", err
	}
	var got struct {
		Token   string `json:"token"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.Token == "" {
		if got.Message != "" {
			return "", fmt.Errorf("Schedules Direct: %s", got.Message)
		}
		return "", fmt.Errorf("Schedules Direct did not sign in")
	}
	sdTokens.Lock()
	sdTokens.m[key] = sdToken{value: got.Token, until: time.Now().Add(sdTokenLife)}
	sdTokens.Unlock()
	return got.Token, nil
}

func (c *SD) forget() {
	sdTokens.Lock()
	delete(sdTokens.m, c.tokenKey())
	sdTokens.Unlock()
}

// call sends one signed-in request. A token Schedules Direct no longer
// honors is dropped and the request is sent once more with a new one.
func (c *SD) call(ctx context.Context, method, path string, body any) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		tok, err := c.token(ctx)
		if err != nil {
			return nil, err
		}
		raw, err := c.do(ctx, method, path, tok, body)
		var sdErr *SDError
		if attempt == 0 && errors.As(err, &sdErr) && sdErr.tokenRefused() {
			c.forget()
			continue
		}
		return raw, err
	}
}

// SDError is a request Schedules Direct turned down. Message is its own
// words, which are plain enough to show.
type SDError struct {
	Status  int
	Code    int
	Message string
}

func (e *SDError) Error() string {
	if e.Message != "" {
		return "Schedules Direct: " + e.Message
	}
	return fmt.Sprintf("schedules direct returned %d %s", e.Status, http.StatusText(e.Status))
}

// tokenRefused is a token Schedules Direct no longer honors (code 4006 is an
// expired one). A refused account is not: signing in again would not help,
// and repeated sign-ins can lock it.
func (e *SDError) tokenRefused() bool {
	return e.Status == http.StatusUnauthorized || e.Code == 4006 || strings.Contains(strings.ToLower(e.Message), "token")
}

func (c *SD) do(ctx context.Context, method, path, token string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	if token != "" {
		req.Header.Set("token", token)
	}
	return sdDo(c.client, req)
}

func sdDo(client *http.Client, req *http.Request) ([]byte, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, requestError(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		out := &SDError{Status: res.StatusCode}
		var doc struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &doc) == nil {
			out.Code, out.Message = doc.Code, strings.TrimSpace(doc.Message)
		}
		return nil, out
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, sdMaxResponse+1))
	if err != nil {
		return nil, err
	}
	// A response cut at the cap would fail to parse with a JSON error that
	// names no cause.
	if len(body) > sdMaxResponse {
		return nil, fmt.Errorf("schedules direct sent more than %d MB in one response", sdMaxResponse>>20)
	}
	return body, nil
}

type sdLineupDoc struct {
	Map []struct {
		StationID string `json:"stationID"`
		Channel   string `json:"channel"`
		UHFVHF    int    `json:"uhfVhf"`
		ATSCMajor int    `json:"atscMajor"`
		ATSCMinor int    `json:"atscMinor"`
	} `json:"map"`
	Stations []struct {
		StationID string `json:"stationID"`
		Name      string `json:"name"`
		Callsign  string `json:"callsign"`
		Affiliate string `json:"affiliate"`
		Logo      *struct {
			URL string `json:"URL"`
		} `json:"logo"`
		StationLogo []struct {
			URL string `json:"URL"`
		} `json:"stationLogo"`
	} `json:"stations"`
}

// sdMatch pairs lineup stations with channels. A cable or satellite lineup
// names the channel number; an antenna lineup gives the ATSC major and
// minor. Two stations with one number (two markets in reach) are told apart
// by the RF channel, then by the call sign; a number only one station has
// matches even when its RF channel moved.
func sdMatch(doc sdLineupDoc, channels []SDChannel) map[string][]int64 {
	byNumber := map[string][]SDChannel{}
	for _, ch := range channels {
		if key := numberKey(ch.GuideNumber); key != "" {
			byNumber[key] = append(byNumber[key], ch)
		}
	}
	calls := map[string]string{}
	for _, st := range doc.Stations {
		calls[st.StationID] = st.Callsign
	}
	type claim struct {
		station string
		score   int
	}
	best := map[int64]claim{}
	for _, row := range doc.Map {
		if row.StationID == "" {
			continue
		}
		key := numberKey(row.Channel)
		if row.ATSCMajor > 0 {
			key = fmt.Sprintf("%d.%d", row.ATSCMajor, row.ATSCMinor)
		}
		for _, ch := range byNumber[key] {
			score := 1
			if ch.RF > 0 && row.UHFVHF > 0 {
				if ch.RF == row.UHFVHF {
					score += 4
				} else {
					score -= 2
				}
			}
			if sameCall(calls[row.StationID], ch.GuideName) {
				score += 2
			}
			if was, ok := best[ch.ID]; !ok || score > was.score {
				best[ch.ID] = claim{row.StationID, score}
			}
		}
	}
	out := map[string][]int64{}
	for id, c := range best {
		out[c.station] = append(out[c.station], id)
	}
	for _, ids := range out {
		slices.Sort(ids)
	}
	return out
}

// numberKey is a channel number as major.minor without leading zeros, so
// "004.1", "4-1", and "4.1" are one channel. A number with no minor stays whole.
func numberKey(raw string) string {
	raw = strings.TrimSpace(raw)
	major, minor, found := strings.Cut(strings.ReplaceAll(raw, "-", "."), ".")
	a, err := strconv.Atoi(major)
	if err != nil {
		return raw
	}
	if !found {
		return strconv.Itoa(a)
	}
	b, err := strconv.Atoi(minor)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%d.%d", a, b)
}

func sameCall(callsign, guideName string) bool {
	a, b := normCall(callsign), normCall(guideName)
	return len(a) >= 3 && a == b
}

// RFChannel is the US and Canadian broadcast channel whose band holds this
// frequency, or zero.
func RFChannel(hz int) int {
	mhz := float64(hz) / 1e6
	switch {
	case mhz >= 54 && mhz < 72:
		return 2 + int((mhz-54)/6)
	case mhz >= 76 && mhz < 88:
		return 5 + int((mhz-76)/6)
	case mhz >= 174 && mhz < 216:
		return 7 + int((mhz-174)/6)
	case mhz >= 470 && mhz < 608:
		return 14 + int((mhz-470)/6)
	}
	return 0
}

// Fetch pulls two weeks of the lineup's listings for these channels.
func (c *SD) Fetch(ctx context.Context, lineup string, channels []SDChannel) (SDGuide, error) {
	body, err := c.call(ctx, http.MethodGet, "/lineups/"+lineup, nil)
	if err != nil {
		return SDGuide{}, err
	}
	var doc sdLineupDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return SDGuide{}, err
	}
	stationChannels := sdMatch(doc, channels)
	out := SDGuide{Art: map[int64]string{}, Networks: map[int64]string{}}
	byID := map[int64]SDChannel{}
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	for _, st := range doc.Stations {
		logo := ""
		if st.Logo != nil {
			logo = cleanIcon(st.Logo.URL)
		}
		for _, l := range st.StationLogo {
			if logo == "" {
				logo = cleanIcon(l.URL)
			}
		}
		affiliate := ""
		if first, _, _ := strings.Cut(strings.TrimSpace(st.Affiliate), " "); first != "" {
			affiliate = first
		}
		for _, id := range stationChannels[st.StationID] {
			if logo != "" {
				out.Art[id] = logo
			}
			line := byID[id]
			if net := Affiliation([]string{affiliate}, st.Callsign, line.GuideName, line.DisplayName); net != "" {
				out.Networks[id] = net
			}
		}
	}
	var stations []string
	asked := map[string]bool{}
	for _, row := range doc.Map {
		if _, ok := stationChannels[row.StationID]; ok && !asked[row.StationID] {
			asked[row.StationID] = true
			stations = append(stations, row.StationID)
		}
	}
	if len(stations) == 0 {
		return out, nil
	}
	blocks, err := c.schedules(ctx, stations)
	if err != nil {
		return SDGuide{}, err
	}
	details := c.programs(ctx, blocks)
	seen := map[int64]bool{}
	for _, block := range blocks {
		for _, id := range stationChannels[block.StationID] {
			for _, prog := range block.Programs {
				start, err := time.Parse(time.RFC3339, prog.AirDateTime)
				if err != nil || prog.Duration <= 0 {
					continue
				}
				row := sdAiring(prog, details[prog.ProgramID])
				row.ChannelID = id
				row.Start = start
				row.End = start.Add(time.Duration(prog.Duration) * time.Second)
				out.Rows = append(out.Rows, row)
				seen[id] = true
			}
		}
	}
	for id := range seen {
		out.Channels = append(out.Channels, id)
	}
	slices.Sort(out.Channels)
	return out, nil
}

type sdScheduled struct {
	ProgramID          string `json:"programID"`
	AirDateTime        string `json:"airDateTime"`
	Duration           int    `json:"duration"`
	MD5                string `json:"md5"`
	New                bool   `json:"new"`
	LiveTapeDelay      string `json:"liveTapeDelay"`
	Premiere           bool   `json:"premiere"`
	IsPremiereOrFinale string `json:"isPremiereOrFinale"`
	Ratings            []struct {
		Body string `json:"body"`
		Code string `json:"code"`
	} `json:"ratings"`
	// VideoProperties and AudioProperties describe this airing: "hdtv",
	// "uhdtv"; "cc", "stereo", "dd 5.1", "Dolby", "dvs", "sap".
	VideoProperties []string `json:"videoProperties"`
	AudioProperties []string `json:"audioProperties"`
}

type sdBlock struct {
	StationID string        `json:"stationID"`
	Programs  []sdScheduled `json:"programs"`
}

// schedules asks for fourteen days of a whole lineup. That is far past one
// response's cap, so the stations are asked for a few at a time. An account
// that refuses the longer request gets one day.
func (c *SD) schedules(ctx context.Context, stations []string) ([]sdBlock, error) {
	dates := make([]string, 14)
	for i := range dates {
		dates[i] = time.Now().AddDate(0, 0, i).Format("2006-01-02")
	}
	var blocks []sdBlock
	days := dates
	for part := range slices.Chunk(stations, sdStationsPerRequest) {
		var req []map[string]any
		for _, id := range part {
			req = append(req, map[string]any{"stationID": id, "date": days})
		}
		raw, err := c.call(ctx, http.MethodPost, "/schedules", req)
		if err != nil && len(days) > 1 {
			days = dates[:1]
			for i := range req {
				req[i]["date"] = days
			}
			raw, err = c.call(ctx, http.MethodPost, "/schedules", req)
		}
		if err != nil {
			return nil, err
		}
		var got []sdBlock
		if err := json.Unmarshal(raw, &got); err != nil {
			return nil, err
		}
		blocks = append(blocks, got...)
	}
	return blocks, nil
}

type sdProgram struct {
	ProgramID       string `json:"programID"`
	MD5             string `json:"md5"`
	EpisodeTitle150 string `json:"episodeTitle150"`
	Titles          []struct {
		Title string `json:"title120"`
	} `json:"titles"`
	Descriptions struct {
		Long  []sdText `json:"description1000"`
		Short []sdText `json:"description100"`
	} `json:"descriptions"`
	OriginalAirDate string   `json:"originalAirDate"`
	Genres          []string `json:"genres"`
	ShowType        string   `json:"showType"`
	EntityType      string   `json:"entityType"`
	Metadata        []map[string]struct {
		Season  int `json:"season"`
		Episode int `json:"episode"`
	} `json:"metadata"`
	Cast []struct {
		Name         string `json:"name"`
		Role         string `json:"role"`
		BillingOrder string `json:"billingOrder"`
	} `json:"cast"`
	ContentRating []struct {
		Body string `json:"body"`
		Code string `json:"code"`
	} `json:"contentRating"`
	Movie *struct {
		Year string `json:"year"`
	} `json:"movie"`
}

type sdText struct {
	Language    string `json:"descriptionLanguage"`
	Description string `json:"description"`
}

// Program details change rarely and each costs a lookup, so they are kept
// between pulls by their md5, which Schedules Direct changes with them.
var sdPrograms = struct {
	sync.Mutex
	m map[string]sdProgram
}{m: map[string]sdProgram{}}

const sdProgramsKept = 200_000

// programs reads the details of every scheduled program, from the kept
// copies where the md5 still matches. A batch that fails leaves its
// programs with the id for a title.
func (c *SD) programs(ctx context.Context, blocks []sdBlock) map[string]sdProgram {
	out := map[string]sdProgram{}
	var want []string
	wanted := map[string]bool{}
	sdPrograms.Lock()
	for _, block := range blocks {
		for _, prog := range block.Programs {
			if prog.ProgramID == "" {
				continue
			}
			if _, ok := out[prog.ProgramID]; ok || wanted[prog.ProgramID] {
				continue
			}
			wanted[prog.ProgramID] = true
			if kept, ok := sdPrograms.m[prog.ProgramID]; ok && kept.MD5 == prog.MD5 && prog.MD5 != "" {
				out[prog.ProgramID] = kept
				continue
			}
			want = append(want, prog.ProgramID)
		}
	}
	sdPrograms.Unlock()
	for part := range slices.Chunk(want, sdProgramsPerRequest) {
		meta, err := c.call(ctx, http.MethodPost, "/programs", part)
		if err != nil {
			continue
		}
		var got []sdProgram
		if json.Unmarshal(meta, &got) != nil {
			continue
		}
		sdPrograms.Lock()
		if len(sdPrograms.m)+len(got) > sdProgramsKept {
			sdPrograms.m = map[string]sdProgram{}
		}
		for _, prog := range got {
			out[prog.ProgramID] = prog
			sdPrograms.m[prog.ProgramID] = prog
		}
		sdPrograms.Unlock()
	}
	return out
}

// sdAiring is one scheduled airing with its program's details, in the
// shape an XMLTV guide gives.
func sdAiring(s sdScheduled, p sdProgram) store.Airing {
	row := store.Airing{ProgramID: sdProgramID(s.ProgramID), SeriesID: sdSeriesID(s.ProgramID)}
	if len(p.Titles) > 0 {
		row.Title = strings.TrimSpace(p.Titles[0].Title)
	}
	if row.Title == "" {
		row.Title = s.ProgramID
	}
	row.Subtitle = strings.TrimSpace(p.EpisodeTitle150)
	row.Description = sdDescription(p.Descriptions.Long)
	if row.Description == "" {
		row.Description = sdDescription(p.Descriptions.Short)
	}
	row.Category = sdCategory(p)
	for _, meta := range p.Metadata {
		if m, ok := meta["Gracenote"]; ok && (m.Season > 0 || m.Episode > 0) {
			row.Season, row.Episode = m.Season, m.Episode
			break
		}
	}
	if row.Season == 0 && row.Episode == 0 {
		for _, meta := range p.Metadata {
			for _, m := range meta {
				if m.Season > 0 || m.Episode > 0 {
					row.Season, row.Episode = m.Season, m.Episode
				}
			}
		}
	}
	if row.Season > 0 && row.Episode > 0 {
		row.EpisodeLabel = fmt.Sprintf("S%dE%d", row.Season, row.Episode)
	}
	row.OriginalAir = originalAir(strings.ReplaceAll(p.OriginalAirDate, "-", ""))
	if row.OriginalAir == "" && p.Movie != nil {
		row.OriginalAir = originalAir(p.Movie.Year)
	}
	row.New = s.New
	row.Live = strings.EqualFold(s.LiveTapeDelay, "Live")
	row.Premiere = s.Premiere || strings.Contains(s.IsPremiereOrFinale, "Premiere")
	row.Finale = strings.Contains(s.IsPremiereOrFinale, "Finale")
	ratings := s.Ratings
	if len(ratings) == 0 {
		ratings = p.ContentRating
	}
	row.Rating = sdRating(ratings)
	row.Cast = sdCast(p)
	for _, v := range s.VideoProperties {
		row.Video = richerVideo(row.Video, VideoQuality(v))
	}
	for _, a := range s.AudioProperties {
		if strings.EqualFold(strings.TrimSpace(a), "cc") {
			row.CC = true
			continue
		}
		row.Audio = richerAudio(row.Audio, AudioLayout(a))
	}
	return row
}

// sdProgramID writes a 14-character id the way XMLTV feeds do, EP01234567.0003,
// so the same episode from either source is one episode.
func sdProgramID(id string) string {
	if len(id) == 14 && !strings.Contains(id, ".") {
		return id[:10] + "." + id[10:]
	}
	return id
}

// sdSeriesID is the show an episode belongs to: SH and the program root.
func sdSeriesID(id string) string {
	if len(id) < 10 {
		return ""
	}
	switch id[:2] {
	case "EP", "SH":
		return "SH" + id[2:10]
	case "SP":
		return id[:10]
	}
	return ""
}

func sdDescription(list []sdText) string {
	for _, d := range list {
		if strings.HasPrefix(strings.ToLower(d.Language), "en") && strings.TrimSpace(d.Description) != "" {
			return strings.TrimSpace(d.Description)
		}
	}
	for _, d := range list {
		if text := strings.TrimSpace(d.Description); text != "" {
			return text
		}
	}
	return ""
}

// sdCategory is the program's genres, led by Sports or Movie when the show
// type says so, as XMLTV categories read.
func sdCategory(p sdProgram) string {
	var cats []string
	add := func(cat string) {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			return
		}
		for _, have := range cats {
			if strings.EqualFold(have, cat) {
				return
			}
		}
		cats = append(cats, cat)
	}
	kind := strings.ToLower(p.ShowType + " " + p.EntityType)
	switch {
	case strings.Contains(kind, "sport"):
		add("Sports")
	case strings.Contains(kind, "feature film") || strings.Contains(kind, "movie"):
		add("Movie")
	case strings.Contains(kind, "series"):
		add("Series")
	}
	for _, g := range p.Genres {
		add(g)
	}
	return strings.Join(cats, ", ")
}

// sdRating prefers the US TV rating, then the film rating, then any.
// TV14 is written TV-14, as in XMLTV feeds.
func sdRating(list []struct {
	Body string `json:"body"`
	Code string `json:"code"`
}) string {
	pick := ""
	for _, want := range []string{"USA Parental Rating", "Motion Picture Association of America", ""} {
		for _, r := range list {
			if pick == "" && (want == "" || r.Body == want) {
				pick = strings.TrimSpace(r.Code)
			}
		}
	}
	if len(pick) > 2 && strings.HasPrefix(pick, "TV") && pick[2] != '-' {
		pick = "TV-" + pick[2:]
	}
	return pick
}

func sdCast(p sdProgram) string {
	cast := slices.Clone(p.Cast)
	slices.SortStableFunc(cast, func(a, b struct {
		Name         string `json:"name"`
		Role         string `json:"role"`
		BillingOrder string `json:"billingOrder"`
	}) int {
		return strings.Compare(a.BillingOrder, b.BillingOrder)
	})
	var names []string
	for _, person := range cast {
		if person.Role != "" && !strings.Contains(person.Role, "Actor") && !strings.Contains(person.Role, "Host") {
			continue
		}
		if name := strings.TrimSpace(person.Name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
		if len(names) == 8 {
			break
		}
	}
	return strings.Join(names, ", ")
}

// Schedules Direct takes up to 5,000 stations or programs a request; these
// are smaller so each response stays well under sdMaxResponse. Fourteen days
// of one station is about 150 KB of schedule, and a program's details about
// 2 KB.
const (
	sdStationsPerRequest = 20
	sdProgramsPerRequest = 500
	sdMaxResponse        = 8 << 20
)
