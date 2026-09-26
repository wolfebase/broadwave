package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/logbuf"
)

func TestPrometheusTextCountsFeeds(t *testing.T) {
	zero := prometheusText(nil)
	if !strings.Contains(zero, "broadwave_feeds 0\n") || !strings.Contains(zero, "broadwave_viewers 0\n") || !strings.Contains(zero, "broadwave_ffmpeg_processes 0\n") {
		t.Fatalf("%s", zero)
	}
	if strings.Contains(zero, "broadwave_feed_viewers") {
		t.Fatalf("%s", zero)
	}
	body := prometheusText([]live.FeedStat{{
		ChannelID: 7, GuideNumber: "4.1\"\n", Viewers: 3, FFmpeg: 2, Recording: true, Exports: 4,
	}})
	for _, line := range []string{
		"broadwave_feeds 1\n",
		"broadwave_viewers 3\n",
		"broadwave_ffmpeg_processes 2\n",
		"broadwave_feed_viewers{channel=\"7\",guide=\"4.1\\\"\\n\"} 3\n",
		"broadwave_feed_ffmpeg{channel=\"7\",guide=\"4.1\\\"\\n\"} 2\n",
		"broadwave_feed_exports{channel=\"7\",guide=\"4.1\\\"\\n\"} 4\n",
		"broadwave_feed_recording{channel=\"7\",guide=\"4.1\\\"\\n\"} 1\n",
	} {
		if !strings.Contains(body, line) {
			t.Fatalf("missing %q in %s", line, body)
		}
	}
}

func TestMetricsLeavesOutSecrets(t *testing.T) {
	st := testStore(t)
	const (
		password = "g8-fixture-password"
		auth     = "g8-device-auth-token"
	)
	playlist := "http://g8user:" + password + "@playlist.example/pl.m3u?password=" + password
	if _, err := st.AddSource(t.Context(), "xtream", "IPTV", playlist, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(t.Context(), map[string]string{"sdPassword": password}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(t.Context(), hdhr.Device{
		DeviceID: "DUO", FriendlyName: "HDHomeRun", TunerCount: 2,
		BaseURL: "http://tuner.example", LineupURL: "http://tuner.example/lineup.json?DeviceAuth=" + auth,
	}, nil); err != nil {
		t.Fatal(err)
	}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
	})
	logbuf.Install(io.Discard)
	slog.Info(fmt.Sprintf("source %s DeviceAuth=%s", playlist, auth))
	slog.Info("login", "password", password, "DeviceAuth", auth)

	api := &Server{
		Store: st,
		CountFeeds: func() []live.FeedStat {
			return []live.FeedStat{
				{ChannelID: 4, GuideNumber: password, Name: auth, Viewers: 2, FFmpeg: 1, Recording: true, Exports: 1},
				{ChannelID: 5, GuideNumber: "9.1", Name: "KMBC", Viewers: 1, FFmpeg: 0},
			}
		},
	}
	rec := get(t, api.Handler(), "/api/v1/metrics")
	if rec.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content type %s", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if strings.Contains(body, password) || strings.Contains(body, auth) || strings.Contains(body, "DeviceAuth") {
		t.Fatalf("metrics %s", body)
	}
	for _, line := range []string{"broadwave_feeds 2\n", "broadwave_viewers 3\n", "broadwave_ffmpeg_processes 1\n", "broadwave_feed_viewers{channel=\"5\",guide=\"9.1\"} 1\n"} {
		if !strings.Contains(body, line) {
			t.Fatalf("missing %q in %s", line, body)
		}
	}
	if text := logbuf.Text(); strings.Contains(text, password) || strings.Contains(text, auth) || strings.Contains(text, "DeviceAuth") {
		t.Fatalf("ring %s", text)
	}
	if !strings.Contains(logbuf.Text(), "playlist.example") {
		t.Fatalf("ring %s", logbuf.Text())
	}

	diag := get(t, api.Handler(), "/api/v1/diagnostics")
	var payload struct {
		Logs  []string        `json:"logs"`
		Feeds []live.FeedStat `json:"feeds"`
	}
	if err := json.Unmarshal(diag.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(payload.Logs, "\n")
	if strings.Contains(joined, password) || strings.Contains(joined, auth) || strings.Contains(joined, "DeviceAuth") {
		t.Fatalf("logs %s", joined)
	}
	feedJSON, _ := json.Marshal(payload.Feeds)
	if strings.Contains(string(feedJSON), password) || strings.Contains(string(feedJSON), auth) {
		t.Fatalf("feeds %s", feedJSON)
	}
	if payload.Feeds[0].GuideNumber != "••••" || payload.Feeds[0].Name != "••••" {
		t.Fatalf("feed labels %+v", payload.Feeds[0])
	}
	if !strings.Contains(joined, "playlist.example") {
		t.Fatalf("logs %s", joined)
	}
	if len(payload.Feeds) != 2 || payload.Feeds[0].Viewers != 2 || payload.Feeds[0].FFmpeg != 1 || payload.Feeds[1].GuideNumber != "9.1" {
		t.Fatalf("%+v", payload.Feeds)
	}
}
