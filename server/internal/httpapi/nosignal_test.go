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
	}

	tuner.Light("5.1")
	res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, id))
	if res.Code != http.StatusOK {
		t.Fatalf("lit channel: %d %s", res.Code, res.Body.String())
	}
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
// answers the stream with 807 No Video Data. A watch and a recording of it
// say it is not coming in, not that a stream is down.
func TestADeviceWithNoVideoDataSaysItIsNotComingIn(t *testing.T) {
	ctx := context.Background()
	sample := filepath.Join(t.TempDir(), "sample.ts")
	contractSample(t, sample)
	tuner := &fake.Server{TS: sample, NoVideoData: true}
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
		res := postJSON(t, h, path, fmt.Sprintf(`{"channelId":%d}`, id))
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(res.Body.Bytes(), &problem)
		if res.Code != http.StatusServiceUnavailable || problem.Code != "no_signal" {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		if !hub.Idle() || !tunersFree(t, h) {
			t.Fatalf("%s kept the tuner", path)
		}
	}
}
