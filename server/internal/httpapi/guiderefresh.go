package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"broadwave/internal/guide"
	"broadwave/internal/store"
)

// guideFeed is what one listing source brought to a refresh.
type guideFeed struct {
	source string
	rows   []store.Airing
	// err is a fetch that failed: the source's listings stay as they are.
	err error
	// off is a source with nothing configured: its listings go.
	off bool
}

// guideDropFloor and the half below it are a feed that suddenly lists far
// less than it did (an outage page parsed as a guide, a lineup that lost its
// stations). Its listings stay until a refresh brings them back.
const guideDropFloor = 100

// guideMerge is the guide a refresh writes: the rows, the channels whose
// listings they replace, and the sources whose listings were kept.
type guideMerge struct {
	rows    []store.Airing
	replace []int64
	held    map[string]string
}

// mergeGuide puts the feeds together in order of preference. A channel takes
// all its listings from the first feed that has any for it; a later feed only
// fills channels the earlier ones left empty. A feed that failed, or whose
// rows fell by more than half, keeps the channels it listed, unless an
// earlier feed lists them now.
func mergeGuide(channels []int64, feeds []guideFeed, prev store.GuideCoverage, now time.Time) guideMerge {
	m := guideMerge{held: map[string]string{}}
	usable := make([]bool, len(feeds))
	for i, feed := range feeds {
		switch {
		case feed.off:
		case feed.err != nil:
			m.held[feed.source] = feed.err.Error()
		default:
			n := 0
			for _, row := range feed.rows {
				if row.End.After(now) {
					n++
				}
			}
			if was := prev.Rows[feed.source]; was >= guideDropFloor && n*2 < was {
				m.held[feed.source] = fmt.Sprintf("%d listings, down from %d", n, was)
				continue
			}
			usable[i] = true
		}
	}
	claimed := map[int64]bool{}
	for i, feed := range feeds {
		if !usable[i] {
			continue
		}
		mine := map[int64]bool{}
		for _, row := range feed.rows {
			if !claimed[row.ChannelID] {
				mine[row.ChannelID] = true
			}
		}
		for _, row := range feed.rows {
			if mine[row.ChannelID] {
				m.rows = append(m.rows, row)
			}
		}
		for id := range mine {
			claimed[id] = true
		}
	}
	for _, id := range channels {
		if _, held := m.held[prev.Channels[id]]; held && !claimed[id] {
			continue
		}
		m.replace = append(m.replace, id)
	}
	return m
}

// RefreshGuide reads every listing source, merges them, and writes the guide
// once, in one transaction. A source that fails keeps what it listed before.
func (s *Server) RefreshGuide(ctx context.Context) (int, error) {
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, errors.New("no tuner")
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
	tmdbKey := strings.TrimSpace(settings["tmdbKey"])
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}
	var bases []string
	for _, d := range devices {
		if !strings.HasPrefix(d.DeviceID, "src-") && d.TunerCount > 0 {
			bases = append(bases, d.BaseURL)
		}
	}

	silicon := guideFeed{source: "silicondust"}
	raw, pullErr := guide.Pull(ctx, s.HDHR, bases...)
	if pullErr == nil {
		var rows []store.Airing
		var art map[int64]string
		rows, art, pullErr = guide.Parse(raw, antenna)
		if pullErr == nil {
			rows = guide.ShareSame(guide.ShareTwins(rows, art, antenna), art, antenna)
			_ = s.Store.SetChannelArt(ctx, art)
			_ = s.Store.SetNetworks(ctx, guide.Networks(raw, antenna))
			rows = guide.FillImages(ctx, tmdbKey, rows)
			silicon.rows = tagGuideSource(rows, "silicondust")
		}
	}
	silicon.err = pullErr

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
	direct := guideFeed{source: "schedules-direct", off: user == "" || pass == "" || lineup == ""}
	if !direct.off {
		extra, _, err := guide.SchedulesDirect(ctx, antenna, user, pass, lineup)
		direct.err = err
		if err == nil {
			direct.rows = tagGuideSource(guide.ShareSame(guide.ShareTwins(extra, nil, antenna), nil, antenna), "schedules-direct")
		}
	}

	rawURL := strings.TrimSpace(settings["guideUrl"])
	address := guideFeed{source: "xmltv", off: rawURL == ""}
	if !address.off {
		body, err := guide.PullURL(ctx, rawURL)
		var extra []store.Airing
		var extraArt map[int64]string
		if err == nil {
			extra, extraArt, err = guide.Parse(body, antenna)
		}
		address.err = err
		if err == nil {
			extra = guide.ShareSame(guide.ShareTwins(extra, extraArt, antenna), extraArt, antenna)
			_ = s.Store.SetChannelArt(ctx, extraArt)
			_ = s.Store.SetNetworks(ctx, guide.Networks(body, antenna))
			address.rows = tagGuideSource(guide.FillImages(ctx, tmdbKey, extra), "xmltv")
		}
	}

	now := time.Now().UTC()
	prev, err := s.Store.FeedCoverage(ctx, now)
	if err != nil {
		return 0, err
	}
	merged := mergeGuide(ids, []guideFeed{silicon, direct, address}, prev, now)
	if pullErr != nil {
		_ = s.Store.SetGuideError(ctx, pullErr.Error(), now)
		if len(merged.rows) == 0 {
			return 0, pullErr
		}
	}
	changes, err := s.Store.ApplyAirings(ctx, merged.replace, merged.rows)
	if err != nil {
		return 0, err
	}
	for source, why := range merged.held {
		if source == silicon.source && pullErr != nil {
			continue
		}
		slog.Warn(fmt.Sprintf("guide: %s kept its listings: %s", source, why))
	}
	if pruned, err := s.Store.PruneAirings(ctx, now); err == nil && pruned > 0 {
		slog.Debug(fmt.Sprintf("guide: removed %d listings that ended days ago", pruned))
	}
	n := len(merged.rows)
	if pullErr != nil {
		// The other sources filled the guide. SiliconDust is tried again soon.
		slog.Warn(fmt.Sprintf("guide: SiliconDust: %v; %d airings from the other sources", pullErr, n))
		s.DeferGuide(ctx, guide.RetryAfterError)
		_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", n))
		s.LinkGames(ctx)
		return n, nil
	}
	span := int64(guide.PullMax - guide.PullMin)
	jitter := time.Duration(rand.Int64N(span + 1))
	next := guide.NextPull(now, jitter)
	if err := s.Store.SetGuideSchedule(ctx, now, next); err != nil {
		return n, err
	}
	listed := map[int64]struct{}{}
	for _, row := range merged.rows {
		listed[row.ChannelID] = struct{}{}
	}
	slog.Info(fmt.Sprintf("guide: airings=%d channels=%d changed=%d next=%s", n, len(listed), changes, next.Format(time.RFC3339)))
	_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", n))
	s.LinkGames(ctx)
	return n, nil
}
