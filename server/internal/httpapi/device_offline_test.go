package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestDeviceOfflineAfterThreeMissedDiscoveryRounds(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	fresh := store.Device{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: now.Add(-time.Minute).Format(time.RFC3339)}
	if deviceOffline(fresh, now) {
		t.Fatal("a tuner seen a minute ago is not offline")
	}
	twoRounds := store.Device{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: now.Add(-10 * time.Minute).Format(time.RFC3339)}
	if deviceOffline(twoRounds, now) {
		t.Fatal("two missed discovery rounds are not yet offline")
	}
	stale := store.Device{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: now.Add(-16 * time.Minute).Format(time.RFC3339)}
	if !deviceOffline(stale, now) {
		t.Fatal("three missed discovery rounds mark a tuner offline")
	}
	playlist := store.Device{Device: hdhr.Device{DeviceID: "SRC", TunerCount: 0}, LastSeen: now.Add(-time.Hour).Format(time.RFC3339)}
	if deviceOffline(playlist, now) {
		t.Fatal("a playlist uses source health, not lastSeen")
	}
}

func TestDevicesListsOfflineFromLastSeen(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "DUO", FriendlyName: "CONNECT", BaseURL: "http://127.0.0.1", TunerCount: 2,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "SRC", FriendlyName: "Playlist", BaseURL: "source", TunerCount: 0,
	}, nil); err != nil {
		t.Fatal(err)
	}
	list, err := st.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]time.Time{}
	for _, d := range list {
		at, err := time.Parse(time.RFC3339, d.LastSeen)
		if err != nil {
			t.Fatal(d.DeviceID, err)
		}
		seen[d.DeviceID] = at
	}

	api := &Server{Store: st, Clock: func() time.Time { return seen["DUO"].Add(time.Minute) }}
	got := devicesByID(t, api)
	if got["DUO"].Offline {
		t.Fatalf("a just-added tuner is offline: %+v", got["DUO"])
	}
	if got["SRC"].Offline {
		t.Fatalf("a playlist is offline from lastSeen: %+v", got["SRC"])
	}

	api.Clock = func() time.Time { return seen["DUO"].Add(16 * time.Minute) }
	got = devicesByID(t, api)
	if !got["DUO"].Offline {
		t.Fatalf("a tuner last seen 16 minutes ago is still online: %+v", got["DUO"])
	}
	if got["SRC"].Offline {
		t.Fatalf("a playlist became offline from lastSeen: %+v", got["SRC"])
	}
}

type deviceRow struct {
	DeviceID string `json:"deviceId"`
	Offline  bool   `json:"offline"`
}

func devicesByID(t *testing.T, api *Server) map[string]deviceRow {
	t.Helper()
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/devices", nil))
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Devices []deviceRow `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]deviceRow{}
	for _, d := range body.Devices {
		out[d.DeviceID] = d
	}
	return out
}
