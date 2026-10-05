package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// A channel that is off the air answers the watch with the player's words
// soon after the tune, not a dead playlist after the whole wait, and gives
// the tuner back. Once it is on again the same watch plays.
func TestWatchingADarkChannelSaysItIsNotComingIn(t *testing.T) {
	ctx := context.Background()
	sample := filepath.Join(t.TempDir(), "sample.ts")
	contractSample(t, sample)
	tuner := &fake.Server{TS: sample}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	st := testStore(t)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	was := darkAfter
	darkAfter = time.Second
	t.Cleanup(func() { darkAfter = was })
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, HDHR: client, Hub: hub}).Handler()
	id := guideID(t, st, "5.1")
	tuner.Dark("5.1")

	// The first watch probes the channel; the second opens its known frequency.
	for _, pass := range []string{"first tune", "known frequency"} {
		began := time.Now()
		res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, id))
		took := time.Since(began)
		var problem struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(res.Body.Bytes(), &problem)
		if res.Code != http.StatusServiceUnavailable || problem.Code != "no_signal" || problem.Message != "This channel isn't coming in. Check the antenna." {
			t.Fatalf("%s: %d %s", pass, res.Code, res.Body.String())
		}
		if took > 9*time.Second {
			t.Fatalf("%s took %v", pass, took)
		}
		if !hub.Idle() || !tunersFree(t, h) {
			t.Fatalf("%s kept the tuner", pass)
		}
		// A player asking why it stopped still hears about the antenna.
		if live, verdict := signalOf(t, h, id); !live || verdict != "Lost" {
			t.Fatalf("%s: signal after release live=%v verdict=%q", pass, live, verdict)
		}
	}

	tuner.Light("5.1")
	res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, id))
	if res.Code != http.StatusOK {
		t.Fatalf("lit channel: %d %s", res.Code, res.Body.String())
	}
	if _, verdict := signalOf(t, h, id); verdict == "Lost" {
		t.Fatal("a channel that came back still reads lost")
	}
}

func signalOf(t *testing.T, h http.Handler, channelID int64) (bool, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/signals", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body struct {
		Channels []struct {
			ChannelID int64  `json:"channelId"`
			Live      bool   `json:"live"`
			Verdict   string `json:"verdict"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("signals: %v %s", err, rec.Body.String())
	}
	for _, row := range body.Channels {
		if row.ChannelID == channelID {
			return row.Live, row.Verdict
		}
	}
	t.Fatalf("signals: no row for %d", channelID)
	return false, ""
}

// tunersFree waits briefly for every tuner to report nothing tuned.
func tunersFree(t *testing.T, h http.Handler) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tuners", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body struct {
			Tuners []struct {
				Target string `json:"target"`
				Guide  string `json:"guide"`
			} `json:"tuners"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		free := len(body.Tuners) > 0
		for _, tuner := range body.Tuners {
			if tuner.Target != "" || tuner.Guide != "" {
				free = false
			}
		}
		if free || time.Now().After(deadline) {
			if !free {
				t.Logf("tuners: %s", rec.Body.String())
			}
			return free
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A FLEX tunes a channel with no signal to its frequency, never locks, and
// answers the stream with 807 No Video Data about 10 s later. A watch and a
// recording of it say it is not coming in, not that a stream is down, and
// without waiting for the device's own answer.
func TestADeviceWithNoVideoDataSaysItIsNotComingIn(t *testing.T) {
	ctx := context.Background()
	sample := filepath.Join(t.TempDir(), "sample.ts")
	contractSample(t, sample)
	tuner := &fake.Server{TS: sample, NoVideoData: true, TuneDelay: 10 * time.Second}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	st := testStore(t)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	// The reserve is checked against this machine's disk.
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, HDHR: client, Hub: hub}).Handler()
	id := guideID(t, st, "5.1")
	tuner.Dark("5.1")

	for _, path := range []string{"/api/v1/watch", "/api/v1/recordings"} {
		began := time.Now()
		res := postJSON(t, h, path, fmt.Sprintf(`{"channelId":%d}`, id))
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(res.Body.Bytes(), &problem)
		if res.Code != http.StatusServiceUnavailable || problem.Code != "no_signal" {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		if took := time.Since(began); took > 9*time.Second {
			t.Fatalf("%s took %v", path, took)
		}
		if !hub.Idle() || !tunersFree(t, h) {
			t.Fatalf("%s kept the tuner", path)
		}
	}
}
