package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"broadwave/internal/live"
)

// CountFeeds replaces hub feed stats in tests. Nil reads the hub.
func (s *Server) feedStats() []live.FeedStat {
	if s != nil && s.CountFeeds != nil {
		stats := s.CountFeeds()
		if stats == nil {
			return []live.FeedStat{}
		}
		return stats
	}
	if s == nil || s.Hub == nil {
		return []live.FeedStat{}
	}
	return s.Hub.FeedStats()
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	body := scrubBody(prometheusText(s.feedStats()), s.hiddenSecrets(r.Context()))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

func (s *Server) hiddenSecrets(ctx context.Context) []string {
	if s == nil || s.Store == nil {
		return nil
	}
	raw, err := s.Store.CredentialValues(ctx)
	if err != nil {
		return nil
	}
	devices, _ := s.Store.Devices(ctx)
	for _, device := range devices {
		raw = append(raw, device.BaseURL, device.LineupURL)
	}
	if settings, err := s.Store.Settings(ctx); err == nil {
		if v := strings.TrimSpace(settings["sportsdbKey"]); v != "" {
			raw = append(raw, v)
		}
	}
	return secretPieces(raw)
}

func prometheusText(stats []live.FeedStat) string {
	if stats == nil {
		stats = []live.FeedStat{}
	}
	feeds, viewers, procs := len(stats), 0, 0
	for _, st := range stats {
		viewers += st.Viewers
		procs += st.FFmpeg
	}
	var b strings.Builder
	writeGauge(&b, "broadwave_feeds", "Tuned channels.", feeds)
	writeGauge(&b, "broadwave_viewers", "Viewers across tuned channels.", viewers)
	writeGauge(&b, "broadwave_ffmpeg_processes", "ffmpeg processes on tuned channels.", procs)
	writeFeedGauge(&b, "broadwave_feed_viewers", "Viewers on one tuned channel.", stats, func(st live.FeedStat) int { return st.Viewers })
	writeFeedGauge(&b, "broadwave_feed_ffmpeg", "ffmpeg processes on one tuned channel.", stats, func(st live.FeedStat) int { return st.FFmpeg })
	writeFeedGauge(&b, "broadwave_feed_exports", "App streams on one tuned channel.", stats, func(st live.FeedStat) int { return st.Exports })
	writeFeedGauge(&b, "broadwave_feed_recording", "1 when the tuned channel is recording.", stats, func(st live.FeedStat) int {
		if st.Recording {
			return 1
		}
		return 0
	})
	return b.String()
}

func writeGauge(b *strings.Builder, name, help string, value int) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}

func writeFeedGauge(b *strings.Builder, name, help string, stats []live.FeedStat, value func(live.FeedStat) int) {
	if len(stats) == 0 {
		return
	}
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	for _, st := range stats {
		fmt.Fprintf(b, "%s{%s} %d\n", name, feedLabels(st), value(st))
	}
}

func feedLabels(st live.FeedStat) string {
	return fmt.Sprintf("channel=\"%d\",guide=\"%s\"", st.ChannelID, promEscape(st.GuideNumber))
}

func promEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scrubLogs(lines []string, secrets []string) []string {
	if lines == nil {
		return []string{}
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = scrubBody(line, secrets)
	}
	return out
}
