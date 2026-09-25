package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestSignalSummaryAndReadyLine(t *testing.T) {
	if got := signalSummary(6, 0, 1, 0); got != "6 channels great, 1 weak." {
		t.Fatalf("summary %q", got)
	}
	if got := signalSummary(1, 0, 0, 0); got != "1 channel great." {
		t.Fatalf("one %q", got)
	}
	if got := signalSummary(0, 0, 0, 0); got != "No signal reading." {
		t.Fatalf("none %q", got)
	}
	if got := readyLine(27, 21, 2, "Intel GPU"); got != "Ready: 27 channels, guide for 21, 2 tuners, Intel GPU" {
		t.Fatalf("ready %q", got)
	}
	if got := readyLine(1, 0, 1, "Software"); got != "Ready: 1 channel, guide for 0, 1 tuner, Software" {
		t.Fatalf("one ready %q", got)
	}
	if got := favoriteLine([]map[string]any{{"network": "FOX"}, {"network": "ABC"}, {"network": "CBS"}}); got != "ABC, CBS, and FOX." {
		t.Fatalf("favorites %q", got)
	}
	if got := favoriteLine(nil); got != "No ABC, CBS, FOX, or NBC in this lineup." {
		t.Fatalf("no favorites %q", got)
	}
}

func TestNextSignalChannelLearnsAFrequency(t *testing.T) {
	rows := []store.ChannelSignal{
		{ChannelID: 1, GuideNumber: "4.1"},
		{ChannelID: 2, GuideNumber: "4.2"},
		{ChannelID: 3, GuideNumber: "5.1"},
	}
	tried := map[int64]bool{}
	seen := map[int]bool{}
	id, ok := nextSignalChannel(rows, tried, seen)
	if !ok || id != 1 {
		t.Fatalf("first %d %v", id, ok)
	}
	tried[1] = true
	rows[0].FrequencyHz = 563000000
	rows[1].FrequencyHz = 563000000
	seen[563000000] = true
	id, ok = nextSignalChannel(rows, tried, seen)
	if !ok || id != 3 {
		t.Fatalf("next %d %v", id, ok)
	}
}

func TestSetupFinishRunsItself(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "FAKE", FriendlyName: "Fake", ModelNumber: "HDHR4-2US", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF-DT"},
		{GuideNumber: "5.1", GuideName: "KCTV"},
		{GuideNumber: "9.1", GuideName: "ABC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var rows []store.Airing
	for _, ch := range channels {
		if ch.GuideName == "ABC" {
			continue
		}
		rows = append(rows, store.Airing{
			ChannelID: ch.ID, Title: "News", Start: now.Add(-time.Minute), End: now.Add(time.Hour),
		})
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	var benches atomic.Int32
	srv := &Server{
		Store:   st,
		Staging: true,
		Hub:     &live.Hub{Dir: t.TempDir(), Encoder: "h264_videotoolbox", FFmpeg: "ffmpeg"},
		SetupBench: func(context.Context, string, string) (float64, error) {
			benches.Add(1)
			time.Sleep(150 * time.Millisecond)
			return 11, nil
		},
		SetupSignal: func(context.Context) (int, int, int, int, error) {
			return 6, 0, 1, 0, nil
		},
	}
	h := srv.Handler()
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/finish", bytes.NewBufferString("{}"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("second start %d %s", rec.Code, rec.Body.String())
	}
	var status finishStatus
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/finish", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Ready != "" && !status.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if benches.Load() != 1 {
		t.Fatalf("encoder ran %d times", benches.Load())
	}
	if status.Ready != "Ready: 3 channels, guide for 2, 2 tuners, Apple GPU" {
		t.Fatalf("ready %q", status.Ready)
	}
	if status.ChannelID == 0 {
		t.Fatal("no channel to watch")
	}
	want := map[string]string{
		"scan":      "3 channels already in the lineup.",
		"guide":     "Listings for 2 channels. More listings arrive from the broadcast.",
		"favorites": "ABC, CBS, and FOX.",
		"encoder":   "Apple GPU found: 1080p60 at 11x real time.",
		"signal":    "6 channels great, 1 weak.",
	}
	for _, step := range status.Steps {
		if step.State != "done" {
			t.Fatalf("%s state %s", step.ID, step.State)
		}
		if expect, ok := want[step.ID]; ok && step.Detail != expect {
			t.Fatalf("%s detail %q", step.ID, step.Detail)
		}
		if step.ID == "folder" && step.Detail == "" {
			t.Fatal("folder detail empty")
		}
	}
	fav := map[string]bool{}
	channels, err = st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range channels {
		fav[ch.GuideName] = ch.Favorite
	}
	if !fav["WDAF-DT"] || !fav["KCTV"] || !fav["ABC"] {
		t.Fatalf("favorites %#v", fav)
	}
}
