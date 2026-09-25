package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
)

func TestDeviceHealthRoute(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	tuner := &fake.Server{}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)

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
	if _, err := (hdhr.Control{Addr: "127.0.0.1:" + control}).Set("/tuner0/vchannel", "4.1"); err != nil {
		t.Fatal(err)
	}

	api := &Server{Store: st, HDHR: client}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/devices/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Devices []hdhr.DeviceHealth `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Devices) != 1 {
		t.Fatalf("%s", rec.Body.String())
	}
	got := body.Devices[0]
	if got.DeviceID != "FAKEHDHR" || got.Model != "HDHR4-2US" || got.FirmwareVersion != "20260101" || got.Error != "" {
		t.Fatalf("%+v", got)
	}
	if len(got.Tuners) != 2 || !got.Tuners[0].Locked || got.Tuners[1].Locked {
		t.Fatalf("%+v", got.Tuners)
	}
}

func TestDeviceHealthSkipsAPlaylist(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "SRC", FriendlyName: "Playlist", BaseURL: "source", TunerCount: 0,
	}, nil); err != nil {
		t.Fatal(err)
	}
	api := &Server{Store: st}
	rec := get(t, api.Handler(), "/api/v1/devices/health")
	var body struct {
		Devices []hdhr.DeviceHealth `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Devices) != 0 {
		t.Fatalf("a playlist is not a tuner: %s", rec.Body.String())
	}
}

func TestDeviceHealthEmpty(t *testing.T) {
	api := &Server{Store: testStore(t)}
	rec := get(t, api.Handler(), "/api/v1/devices/health")
	var body struct {
		Devices []hdhr.DeviceHealth `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Devices == nil || len(body.Devices) != 0 {
		t.Fatalf("%s", rec.Body.String())
	}
}
