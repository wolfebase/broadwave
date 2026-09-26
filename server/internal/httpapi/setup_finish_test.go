package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestStagingSetupDoesNotTune(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		t.Errorf("staging called %s", r.URL.Path)
	}))
	defer srv.Close()
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "STAGE", FriendlyName: "Fake", BaseURL: srv.URL, TunerCount: 2,
	}, nil); err != nil {
		t.Fatal(err)
	}
	api := &Server{Store: st, Staging: true, HDHR: &hdhr.Client{}}
	run := newFinishStatus()
	api.finishRun = &run
	api.stepScan(ctx)
	api.stepGuide(ctx)
	api.stepSignal(ctx)
	if hit {
		t.Fatal("staging setup contacted the tuner")
	}
	got := map[string]string{}
	for _, step := range api.finishRun.Steps {
		got[step.ID] = step.Detail
	}
	if got["scan"] != "A test server does not scan the antenna." || got["guide"] != "A test server does not pull the guide." || got["signal"] != "No signal reading." {
		t.Fatalf("%v", got)
	}
}

type closeFunc struct {
	io.ReadCloser
	fn func()
}

func (c closeFunc) Close() error {
	err := c.ReadCloser.Close()
	c.fn()
	return err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSetupScanAbortsOnlyWhileTheTunerIsScanning(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		code    int
		cancel  bool
		wait    time.Duration
		aborts  int32
		detail  string
		syncing bool
	}{
		{name: "still scanning", status: `{"Scan":1,"Found":3}`, wait: 40 * time.Millisecond, aborts: 1, detail: "No channels yet. Check the antenna cable, then scan again in Settings.", syncing: true},
		{name: "finished", status: `{"Scan":0,"Found":4}`, wait: 40 * time.Millisecond, aborts: 0, detail: "No channels yet. Check the antenna cable, then scan again in Settings.", syncing: true},
		{name: "progress error", code: http.StatusInternalServerError, wait: 40 * time.Millisecond, aborts: 1, detail: "No channels yet. Check the antenna cable, then scan again in Settings.", syncing: true},
		{name: "cancelled", status: `{"Scan":1,"Found":1}`, cancel: true, wait: time.Second, aborts: 1, detail: "The scan stopped."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var starts, aborts, discovers atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/lineup.post" && r.Method == http.MethodPost && r.URL.Query().Get("scan") == "start":
					starts.Add(1)
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/lineup.post" && r.Method == http.MethodPost && r.URL.Query().Get("scan") == "abort":
					aborts.Add(1)
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/lineup_status.json":
					if tc.code != 0 {
						http.Error(w, "no status", tc.code)
						return
					}
					_, _ = w.Write([]byte(tc.status))
				default:
					discovers.Add(1)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			client := srv.Client()
			if tc.cancel {
				base := client.Transport
				if base == nil {
					base = http.DefaultTransport
				}
				client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					res, err := base.RoundTrip(req)
					if err != nil || res == nil || res.Body == nil || req.URL.Path != "/lineup_status.json" {
						return res, err
					}
					res.Body = closeFunc{ReadCloser: res.Body, fn: cancel}
					return res, nil
				})
			}
			st := testStore(t)
			if err := st.UpsertDevice(ctx, hdhr.Device{
				DeviceID: "SCAN", FriendlyName: "Fake", BaseURL: srv.URL, TunerCount: 2,
			}, nil); err != nil {
				t.Fatal(err)
			}
			api := &Server{
				Store:    st,
				HDHR:     &hdhr.Client{HTTP: client},
				ScanWait: tc.wait,
			}
			run := newFinishStatus()
			api.finishRun = &run
			started := time.Now()
			api.stepScan(ctx)
			if starts.Load() != 1 || aborts.Load() != tc.aborts {
				t.Fatalf("start %d abort %d, want abort %d", starts.Load(), aborts.Load(), tc.aborts)
			}
			if tc.syncing != (discovers.Load() > 0) {
				t.Fatalf("lineup sync requests %d", discovers.Load())
			}
			if tc.cancel && time.Since(started) > 500*time.Millisecond {
				t.Fatalf("cancel waited %s", time.Since(started))
			}
			var detail string
			for _, step := range api.finishRun.Steps {
				if step.ID == "scan" {
					detail = step.Detail
				}
			}
			if detail != tc.detail {
				t.Fatalf("detail %q", detail)
			}
		})
	}
}

func TestFormPostDoesNotStartSetupOrDiscovery(t *testing.T) {
	api := &Server{Store: testStore(t)}
	h := api.Handler()
	for _, path := range []string{"/api/v1/setup/finish", "/api/v1/sources/discover"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("ip=127.0.0.1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if api.finishOn {
		t.Fatal("a form post started setup")
	}
}

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
	if got := readyLine(1, 0, 1, "Software"); got != "Ready: 1 channel, 1 tuner, Software" {
		t.Fatalf("one ready %q", got)
	}
	if got := readyLine(3, 3, 2, "Apple GPU"); got != "Ready: 3 channels, full guide, 2 tuners, Apple GPU" {
		t.Fatalf("full ready %q", got)
	}
	for _, tc := range []struct {
		great, ok, weak, lost int
		want                  string
	}{
		{0, 0, 0, 0, "check"},
		{3, 1, 1, 0, "done"},
		{1, 0, 2, 1, "check"},
		{0, 0, 0, 4, "check"},
	} {
		if got := signalState(tc.great, tc.ok, tc.weak, tc.lost); got != tc.want {
			t.Fatalf("signalState(%d,%d,%d,%d) = %q, want %q", tc.great, tc.ok, tc.weak, tc.lost, got, tc.want)
		}
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
		// The recordings folder reads the host's disk, so its state depends on the machine.
		if step.State != "done" && step.ID != "folder" {
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
