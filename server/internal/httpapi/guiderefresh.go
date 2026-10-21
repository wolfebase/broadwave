package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"broadwave/internal/guide"
	"broadwave/internal/store"
)

// The listing sources, in order of preference when two are equally detailed.
const (
	sourceSilicon = "silicondust"
	sourceSD      = "schedules-direct"
	sourceXMLTV   = "xmltv"
)

var guideSourceOrder = []string{sourceSilicon, sourceSD, sourceXMLTV}

// Schedules Direct and XMLTV links are pulled once a day, up to an hour
// late so many servers don't ask at once. A pull that fails waits 30
// minutes, then twice as long each time it fails again, up to 6 hours.
const (
	guideDaily       = 24 * time.Hour
	guideDailySpread = time.Hour
	guideBackoffMax  = 6 * time.Hour
)

// guideBackoff is the wait after the fails-th failed pull in a row.
func guideBackoff(fails int) time.Duration {
	wait := guide.RetryAfterError
	for i := 1; i < fails && wait < guideBackoffMax; i++ {
		wait *= 2
	}
	return min(wait, guideBackoffMax)
}

// guideFeed is what one listing source brought to a refresh: listings it
// just pulled, or the ones it wrote last time when it was not pulled or the
// pull failed.
type guideFeed struct {
	source string
	rows   []store.Airing
}

// guideDropFloor and the half below it are a feed that suddenly lists far
// less than it did (an outage page parsed as a guide, a lineup that lost its
// stations). Its listings stay until a refresh brings them back.
const guideDropFloor = 100

// fellTooFar is a fresh pull that lists less than half of what its source
// had in the guide.
func fellTooFar(fresh []store.Airing, was int, now time.Time) error {
	n := 0
	for _, row := range fresh {
		if row.End.After(now) {
			n++
		}
	}
	if was >= guideDropFloor && n*2 < was {
		return fmt.Errorf("%d listings, down from %d", n, was)
	}
	return nil
}

// combineGuide puts the feeds together channel by channel. The feed with the
// most detail where they overlap keeps that stretch; the others add the
// airings it does not cover, such as the days past its end, without any two
// airings overlapping.
func combineGuide(feeds []guideFeed) []store.Airing {
	type pick struct{ lists [][]store.Airing }
	per := map[int64]*pick{}
	var ids []int64
	for i, feed := range feeds {
		for _, row := range feed.rows {
			p := per[row.ChannelID]
			if p == nil {
				p = &pick{lists: make([][]store.Airing, len(feeds))}
				per[row.ChannelID] = p
				ids = append(ids, row.ChannelID)
			}
			p.lists[i] = append(p.lists[i], row)
		}
	}
	slices.Sort(ids)
	var out []store.Airing
	for _, id := range ids {
		lists := per[id].lists
		for _, list := range lists {
			slices.SortStableFunc(list, func(a, b store.Airing) int { return a.Start.Compare(b.Start) })
		}
		base := richestFeed(lists)
		chosen := noOverlaps(lists[base], nil)
		for i, list := range lists {
			if i == base || len(list) == 0 {
				continue
			}
			chosen = mergeByStart(chosen, noOverlaps(list, chosen))
		}
		out = append(out, chosen...)
	}
	return out
}

// richestFeed is the feed with the most detail in the stretch every feed
// that lists this channel covers. With no shared stretch, or a tie, the
// earlier feed wins.
func richestFeed(lists [][]store.Airing) int {
	var from, to time.Time
	first := -1
	for i, list := range lists {
		if len(list) == 0 {
			continue
		}
		if first < 0 {
			first = i
			from, to = list[0].Start, list[len(list)-1].End
			continue
		}
		if list[0].Start.After(from) {
			from = list[0].Start
		}
		if end := list[len(list)-1].End; end.Before(to) {
			to = end
		}
	}
	if first < 0 || !to.After(from) {
		return max(first, 0)
	}
	best, bestScore := first, -1
	for i, list := range lists {
		if len(list) == 0 {
			continue
		}
		score := 0
		for _, row := range list {
			if !row.Start.Before(from) && row.Start.Before(to) {
				score += airingDetail(row)
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func airingDetail(a store.Airing) int {
	n := 1
	for _, field := range []string{a.Description, a.Subtitle, a.ImageURL, a.Category, a.ProgramID, a.SeriesID, a.OriginalAir, a.Rating, a.Cast} {
		if field != "" {
			n++
		}
	}
	if a.Season > 0 || a.Episode > 0 {
		n++
	}
	return n
}

// noOverlaps is the airings in list (sorted by start) that overlap nothing
// in taken (also sorted) and no earlier airing kept from list.
func noOverlaps(list, taken []store.Airing) []store.Airing {
	var out []store.Airing
	for _, row := range list {
		if !row.End.After(row.Start) {
			continue
		}
		if n := len(out); n > 0 && out[n-1].End.After(row.Start) {
			continue
		}
		// The first taken airing that ends after this one starts is the only
		// one that can overlap it from before; any later one starts later.
		i := sort.Search(len(taken), func(i int) bool { return taken[i].End.After(row.Start) })
		if i < len(taken) && taken[i].Start.Before(row.End) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func mergeByStart(a, b []store.Airing) []store.Airing {
	out := make([]store.Airing, 0, len(a)+len(b))
	for len(a) > 0 && len(b) > 0 {
		if b[0].Start.Before(a[0].Start) {
			out = append(out, b[0])
			b = b[1:]
		} else {
			out = append(out, a[0])
			a = a[1:]
		}
	}
	return append(append(out, a...), b...)
}

// ErrNoTuner is RefreshGuide before any tuner or source was added: nothing
// to pull a guide for yet, which a fresh server expects.
var ErrNoTuner = errors.New("no tuner")

// GuideSourceError is a refresh where every source that was pulled failed
// and nothing was listed. Each source already has its next try scheduled.
type GuideSourceError struct {
	Failed map[string]error
}

func (e *GuideSourceError) Error() string {
	var parts []string
	for _, source := range guideSourceOrder {
		if err := e.Failed[source]; err != nil {
			parts = append(parts, err.Error())
		}
	}
	return strings.Join(parts, "; ")
}

func (e *GuideSourceError) Unwrap() []error {
	var out []error
	for _, source := range guideSourceOrder {
		if err := e.Failed[source]; err != nil {
			out = append(out, err)
		}
	}
	return out
}

// guideSourceConfig is one source as the settings describe it now.
type guideSourceConfig struct {
	source string
	// fingerprint changes when the account or address does.
	fingerprint string
	user, pass  string
	lineup, url string
}

// guideConfigs lists the sources that are set up. SiliconDust is always
// there: it needs only a tuner, which the pull checks.
func guideConfigs(settings map[string]string) []guideSourceConfig {
	out := []guideSourceConfig{{source: sourceSilicon}}
	user := strings.TrimSpace(settings["sdUser"])
	pass := settings["sdPassword"]
	lineup := strings.TrimSpace(settings["sdLineup"])
	if user == "" {
		user = strings.TrimSpace(os.Getenv("SD_USERNAME"))
	}
	if pass == "" {
		pass = os.Getenv("SD_PASSWORD")
	}
	if lineup == "" {
		lineup = strings.TrimSpace(os.Getenv("SD_LINEUP"))
	}
	if user != "" && pass != "" && lineup != "" {
		out = append(out, guideSourceConfig{source: sourceSD, fingerprint: fingerprint(user, pass, lineup), user: user, pass: pass, lineup: lineup})
	}
	if raw := strings.TrimSpace(settings["guideUrl"]); raw != "" {
		out = append(out, guideSourceConfig{source: sourceXMLTV, fingerprint: fingerprint(raw), url: raw})
	}
	return out
}

func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// sourceDue is a source whose next pull time has come, or whose account or
// address changed since its last pull.
func sourceDue(conf guideSourceConfig, st store.GuideSource, siliconNext, now time.Time) bool {
	if conf.source == sourceSilicon {
		return !siliconNext.After(now)
	}
	return st.Config != conf.fingerprint || !st.Next.After(now)
}

// RefreshGuide pulls the sources that are due and rewrites the guide.
func (s *Server) RefreshGuide(ctx context.Context) (int, error) {
	return s.pullGuide(ctx, nil)
}

// pullGuide pulls each source that is due or forced, keeps the listings of
// the others as they are, combines them, and writes the guide once, in one
// transaction. Each source pulled gets its own next time: SiliconDust's 20
// to 28 hours, a day for the others, and a growing wait after a failure.
func (s *Server) pullGuide(ctx context.Context, force map[string]bool) (int, error) {
	s.guideMu.Lock()
	defer s.guideMu.Unlock()
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, ErrNoTuner
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return 0, err
	}
	var antenna []store.Channel
	var ids []int64
	for _, ch := range channels {
		if strings.HasPrefix(ch.DeviceID, "src-") {
			continue
		}
		antenna = append(antenna, ch)
		ids = append(ids, ch.ID)
	}
	settings, _ := s.Store.Settings(ctx)
	if settings == nil {
		settings = map[string]string{}
	}
	states, err := s.Store.GuideSources(ctx)
	if err != nil {
		return 0, err
	}
	_, siliconNext, _, _ := s.Store.GuideSchedule(ctx)
	now := time.Now().UTC()
	prev, err := s.Store.FeedCoverage(ctx, now)
	if err != nil {
		return 0, err
	}
	tmdbKey := strings.TrimSpace(settings["tmdbKey"])
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}

	configs := guideConfigs(settings)
	on := map[string]bool{}
	var feeds []guideFeed
	failed := map[string]error{}
	pulled := 0
	for _, conf := range configs {
		on[conf.source] = true
		st := states[conf.source]
		if !force[conf.source] && !sourceDue(conf, st, siliconNext, now) {
			kept, err := s.Store.FeedRows(ctx, conf.source, now)
			if err != nil {
				return 0, err
			}
			feeds = append(feeds, guideFeed{source: conf.source, rows: kept})
			continue
		}
		pulled++
		rows, err := s.pullSource(ctx, conf, devices, antenna, tmdbKey)
		if err == nil {
			err = fellTooFar(rows, prev.Rows[conf.source], now)
		}
		st.Config = conf.fingerprint
		if err != nil {
			failed[conf.source] = err
			st.Fails++
			st.Error, st.ErrorAt = err.Error(), now
			st.Next = now.Add(guideBackoff(st.Fails))
			if conf.source == sourceSilicon {
				_ = s.Store.SetGuideError(ctx, err.Error(), now)
				_ = s.Store.SetNextGuidePull(ctx, st.Next)
			}
			slog.Warn(fmt.Sprintf("guide: %s: %v; kept its listings, next try %s", conf.source, err, st.Next.Format(time.RFC3339)))
			kept, kerr := s.Store.FeedRows(ctx, conf.source, now)
			if kerr != nil {
				return 0, kerr
			}
			feeds = append(feeds, guideFeed{source: conf.source, rows: kept})
		} else {
			st.Fails, st.Error, st.ErrorAt = 0, "", time.Time{}
			st.OK, st.Rows = now, len(rows)
			if conf.source == sourceSilicon {
				span := int64(guide.PullMax - guide.PullMin)
				st.Next = guide.NextPull(now, time.Duration(rand.Int64N(span+1)))
				if err := s.Store.SetGuideSchedule(ctx, now, st.Next); err != nil {
					return 0, err
				}
			} else {
				st.Next = now.Add(guideDaily + time.Duration(rand.Int64N(int64(guideDailySpread)+1)))
			}
			feeds = append(feeds, guideFeed{source: conf.source, rows: tagGuideSource(rows, conf.source)})
		}
		states[conf.source] = st
	}
	// A source that was turned off loses its schedule and its listings.
	removed := false
	for source := range states {
		if !on[source] {
			delete(states, source)
			removed = true
		}
	}
	for source := range prev.Rows {
		if !on[source] && slices.Contains(guideSourceOrder, source) {
			removed = true
		}
	}
	if err := s.Store.SetGuideSources(ctx, states); err != nil {
		return 0, err
	}
	if pulled == 0 && !removed {
		return 0, nil
	}
	merged := combineGuide(feeds)
	if len(merged) == 0 && len(failed) > 0 {
		return 0, &GuideSourceError{Failed: failed}
	}
	changes, err := s.Store.ApplyAirings(ctx, ids, merged)
	if err != nil {
		return 0, err
	}
	if pruned, err := s.Store.PruneAirings(ctx, now); err == nil && pruned > 0 {
		slog.Debug(fmt.Sprintf("guide: removed %d listings that ended days ago", pruned))
	}
	n := len(merged)
	listed := map[int64]struct{}{}
	for _, row := range merged {
		listed[row.ChannelID] = struct{}{}
	}
	slog.Info(fmt.Sprintf("guide: airings=%d channels=%d changed=%d", n, len(listed), changes))
	_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", n))
	s.LinkGames(ctx)
	return n, nil
}

// pullSource reads one source's listings for the antenna lineup, and saves
// the channel logos and networks it names.
func (s *Server) pullSource(ctx context.Context, conf guideSourceConfig, devices []store.Device, antenna []store.Channel, tmdbKey string) ([]store.Airing, error) {
	switch conf.source {
	case sourceSilicon:
		var bases []string
		for _, d := range devices {
			if !strings.HasPrefix(d.DeviceID, "src-") && d.TunerCount > 0 {
				bases = append(bases, d.BaseURL)
			}
		}
		raw, err := guide.Pull(ctx, s.HDHR, bases...)
		if err != nil {
			return nil, err
		}
		rows, art, err := guide.Parse(raw, antenna)
		if errors.Is(err, guide.ErrNoPrograms) {
			return nil, errors.New("SiliconDust sent a guide with no programs")
		}
		if err != nil {
			return nil, err
		}
		rows = guide.ShareSame(guide.ShareTwins(rows, art, antenna), art, antenna)
		_ = s.Store.SetChannelArt(ctx, art)
		_ = s.Store.SetNetworks(ctx, guide.Networks(raw, antenna))
		return guide.FillImages(ctx, tmdbKey, rows), nil
	case sourceSD:
		rf := map[int64]int{}
		if signals, err := s.Store.ChannelSignals(ctx); err == nil {
			for _, sig := range signals {
				rf[sig.ChannelID] = guide.RFChannel(sig.FrequencyHz)
			}
		}
		lineup := make([]guide.SDChannel, 0, len(antenna))
		for _, ch := range antenna {
			lineup = append(lineup, guide.SDChannel{Channel: ch, RF: rf[ch.ID]})
		}
		got, err := s.schedulesDirect(conf.user, conf.pass).Fetch(ctx, conf.lineup, lineup)
		if err != nil {
			return nil, err
		}
		if len(got.Rows) == 0 {
			return nil, errors.New("the Schedules Direct lineup lists none of these channels")
		}
		_ = s.Store.SetChannelArt(ctx, got.Art)
		_ = s.Store.SetNetworks(ctx, got.Networks)
		rows := guide.ShareSame(guide.ShareTwins(got.Rows, got.Art, antenna), got.Art, antenna)
		return guide.FillImages(ctx, tmdbKey, rows), nil
	case sourceXMLTV:
		body, err := guide.PullURL(ctx, conf.url)
		if err != nil {
			return nil, err
		}
		rows, art, err := guide.Parse(body, antenna)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, errors.New("the XMLTV link lists none of these channels")
		}
		rows = guide.ShareSame(guide.ShareTwins(rows, art, antenna), art, antenna)
		_ = s.Store.SetChannelArt(ctx, art)
		_ = s.Store.SetNetworks(ctx, guide.Networks(body, antenna))
		return guide.FillImages(ctx, tmdbKey, rows), nil
	}
	return nil, fmt.Errorf("unknown guide source %q", conf.source)
}

// schedulesDirect is the account's API client; tests point it elsewhere.
func (s *Server) schedulesDirect(user, pass string) *guide.SD {
	if s.SD != nil {
		return s.SD(user, pass)
	}
	return guide.NewSD(user, pass)
}

// GuideDelay is how long the automatic refresh should wait. Zero means pull now.
func (s *Server) GuideDelay(now time.Time) time.Duration {
	ctx := context.Background()
	_, siliconNext, _, err := s.Store.GuideSchedule(ctx)
	if err != nil {
		return 0
	}
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return 0
	}
	states, err := s.Store.GuideSources(ctx)
	if err != nil {
		return 0
	}
	var wait time.Duration
	for _, conf := range guideConfigs(settings) {
		if sourceDue(conf, states[conf.source], siliconNext, now) {
			return 0
		}
		next := states[conf.source].Next
		if conf.source == sourceSilicon {
			next = siliconNext
		}
		if d := guide.Delay(next, now); wait == 0 || d < wait {
			wait = d
		}
	}
	return wait
}

// DeferGuide schedules another SiliconDust attempt after a failed pull
// without counting it as a success.
func (s *Server) DeferGuide(ctx context.Context, after time.Duration) {
	_ = s.Store.SetNextGuidePull(ctx, time.Now().Add(after))
}

// manualGuidePlan is what a Refresh press pulls: SiliconDust at most once an
// hour, as it asks, and each other source when an hour has passed since its
// last pull or its settings changed. retryAt is when a press would pull
// something, when this one pulls nothing.
func (s *Server) manualGuidePlan(ctx context.Context, now time.Time) (map[string]bool, time.Time, error) {
	_, siliconNext, lastManual, err := s.Store.GuideSchedule(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	states, err := s.Store.GuideSources(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	force := map[string]bool{}
	var retryAt time.Time
	later := func(at time.Time) {
		if retryAt.IsZero() || at.Before(retryAt) {
			retryAt = at
		}
	}
	for _, conf := range guideConfigs(settings) {
		st := states[conf.source]
		if conf.source == sourceSilicon {
			if ok, retry := guide.ManualAllowed(lastManual, now); ok {
				force[conf.source] = true
			} else {
				later(retry)
			}
			continue
		}
		last := st.OK
		if st.ErrorAt.After(last) {
			last = st.ErrorAt
		}
		if ok, retry := guide.ManualAllowed(last, now); ok || sourceDue(conf, st, siliconNext, now) {
			force[conf.source] = true
		} else {
			later(retry)
		}
	}
	return force, retryAt, nil
}

// kickGuide pulls the guide shortly after a listing source's settings
// change. A few fields saved one after another pull once.
func (s *Server) kickGuide() {
	if s.Staging {
		return
	}
	s.guideKickMu.Lock()
	defer s.guideKickMu.Unlock()
	if s.guideKick != nil {
		s.guideKick.Stop()
	}
	s.guideKick = time.AfterFunc(3*time.Second, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := s.RefreshGuide(ctx); err != nil && !errors.Is(err, ErrNoTuner) {
			slog.Warn(fmt.Sprintf("guide: after a settings change: %v", err))
		}
	})
}

// guideSourceStatus is one source's state for the settings page.
type guideSourceStatus struct {
	Source string     `json:"source"`
	On     bool       `json:"on"`
	OK     *time.Time `json:"lastOk,omitempty"`
	Next   *time.Time `json:"next,omitempty"`
	Error  string     `json:"error,omitempty"`
	Fails  int        `json:"fails,omitempty"`
	Rows   int        `json:"listings,omitempty"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}

// guideSourceList reports every source, set up or not, in order.
func (s *Server) guideSourceList(ctx context.Context) ([]guideSourceStatus, error) {
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.Store.GuideSources(ctx)
	if err != nil {
		return nil, err
	}
	_, siliconNext, _, _ := s.Store.GuideSchedule(ctx)
	on := map[string]bool{}
	for _, conf := range guideConfigs(settings) {
		on[conf.source] = true
	}
	out := make([]guideSourceStatus, 0, len(guideSourceOrder))
	for _, source := range guideSourceOrder {
		st := states[source]
		next := st.Next
		if source == sourceSilicon {
			next = siliconNext
		}
		row := guideSourceStatus{Source: source, On: on[source], Error: st.Error, Fails: st.Fails, Rows: st.Rows}
		if on[source] {
			row.OK, row.Next = timePtr(st.OK), timePtr(next)
		}
		out = append(out, row)
	}
	return out, nil
}
