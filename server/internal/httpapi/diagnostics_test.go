package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestATuneMeansTheTunerIsAnswering(t *testing.T) {
	old := time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339)
	devices := []store.Device{{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: old}}
	if !tunerWentQuiet(devices, false, time.Now()) {
		t.Fatal("an idle tuner last seen 20 minutes ago should be quiet")
	}
	// Discovery refreshes every five minutes: a tuner seen 7 minutes ago just missed one round.
	between := time.Now().Add(-7 * time.Minute).UTC().Format(time.RFC3339)
	if tunerWentQuiet([]store.Device{{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: between}}, false, time.Now()) {
		t.Fatal("an idle tuner between two refreshes is not quiet")
	}
	retired := []store.Device{{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: old, Retired: true}}
	if tunerWentQuiet(retired, false, time.Now()) {
		t.Fatal("a retired tuner is away on purpose")
	}
	if tunerWentQuiet(devices, true, time.Now()) {
		t.Fatal("a tune in progress is not a tuner that stopped answering")
	}
	fresh := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if tunerWentQuiet([]store.Device{{Device: hdhr.Device{TunerCount: 2}, LastSeen: fresh}}, false, time.Now()) {
		t.Fatal("a recent reply is not quiet")
	}
}

func TestDiagnosticsCountsOnlyListedGuideChannels(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "FAKE", TunerCount: 2}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV"},
		{GuideNumber: "5.1", GuideName: "WTST"},
	})
	if err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 2 {
		t.Fatal(err, chs)
	}
	hidden := true
	if _, err := st.PatchChannel(ctx, chs[1].ID, store.ChannelPatch{Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-10 * time.Minute).UTC()
	var rows []store.Airing
	for _, ch := range chs {
		rows = append(rows, store.Airing{ChannelID: ch.ID, Title: "News", Start: start, End: start.Add(time.Hour)})
	}
	if err := st.ReplaceAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	var body struct {
		Guide struct {
			Channels int `json:"channels"`
			Listed   int `json:"channelsWithListings"`
		} `json:"guide"`
	}
	if err := json.Unmarshal(get(t, h, "/api/v1/diagnostics").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Guide.Channels != 1 || body.Guide.Listed != 1 {
		t.Fatalf("guide %d/%d, want 1/1 (a hidden channel's listings don't count)", body.Guide.Listed, body.Guide.Channels)
	}
}
