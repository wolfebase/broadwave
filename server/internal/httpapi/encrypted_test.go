package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// Watching an encrypted 3.0 channel plays its station's clear 1.0 channel
// under the id the player asked for, and says so.
func TestAnEncryptedChannelPlaysItsClearTwin(t *testing.T) {
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
	var station string
	for _, ch := range channels {
		if ch.GuideNumber == "5.1" {
			station = ch.GuideName
		}
	}
	// The fake has no 3.0 stream, so the watch only plays if it opens 5.1.
	channels = append(channels, hdhr.Channel{GuideNumber: "105.1", GuideName: station, VideoCodec: "HEVC", AudioCodec: "AC4", Protected: true, StreamURL: base + "/auto/v105.1"})
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, HDHR: client, Hub: hub}).Handler()
	enc, clear := guideID(t, st, "105.1"), guideID(t, st, "5.1")

	list, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range list {
		if ch.ID == enc && ch.PlaysAs != clear {
			t.Fatalf("105.1 plays as %d, want 5.1 (%d)", ch.PlaysAs, clear)
		}
	}
	res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, enc))
	if res.Code != http.StatusOK {
		t.Fatalf("watch: %d %s", res.Code, res.Body.String())
	}
	var got live.Session
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ChannelID != enc || got.Stream.Reason != "The 3.0 version is encrypted. Showing the regular broadcast." {
		t.Fatalf("session for %d with reason %q, want %d and the encrypted note", got.ChannelID, got.Stream.Reason, enc)
	}
}
