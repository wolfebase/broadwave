package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// An HDHomeRun emulator has no status.json and no control port. Diagnostics
// lists it with its tuner count and what this server has open, and does not
// call it a tuner that did not answer.
func TestDiagnosticsShowsAnEmulatorHonestly(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	emu := httptest.NewServer(http.NotFoundHandler())
	defer emu.Close()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "EMU", FriendlyName: "Threadfin", ModelNumber: "HDTC-2US", FirmwareVersion: "20190621", BaseURL: emu.URL, TunerCount: 4},
		[]hdhr.Channel{{GuideNumber: "1", GuideName: "News", StreamURL: emu.URL + "/auto/v1"}}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	defer hub.Shutdown()
	api := &Server{Store: st, Hub: hub}
	h := api.Handler()

	// Alone, the emulator is not a tuner error.
	var tuners struct {
		Tuners []live.Tuner `json:"tuners"`
	}
	rec := get(t, h, "/api/v1/tuners")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tuners) != nil || len(tuners.Tuners) != 0 {
		t.Fatalf("tuners %d %s", rec.Code, rec.Body.String())
	}
	var diag struct {
		TunerError string `json:"tunerError"`
		Emulators  []struct {
			DeviceID   string `json:"deviceId"`
			Name       string `json:"name"`
			TunerCount int    `json:"tunerCount"`
			Streams    int    `json:"streams"`
		} `json:"emulators"`
	}
	rec = get(t, h, "/api/v1/diagnostics")
	if err := json.Unmarshal(rec.Body.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag.TunerError != "" {
		t.Fatalf("tuner error %q", diag.TunerError)
	}
	if len(diag.Emulators) != 1 || diag.Emulators[0].DeviceID != "EMU" || diag.Emulators[0].Name != "Threadfin" || diag.Emulators[0].TunerCount != 4 || diag.Emulators[0].Streams != 0 {
		t.Fatalf("emulators %+v", diag.Emulators)
	}

	// Beside a real tuner, both show: the tuner's status, the emulator's count.
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
	if err := st.UpsertDevice(ctx, dev, nil); err != nil {
		t.Fatal(err)
	}
	api.HDHR = client
	rec = get(t, h, "/api/v1/devices/health")
	var health struct {
		Devices []hdhr.DeviceHealth `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if len(health.Devices) != 2 {
		t.Fatalf("health %s", rec.Body.String())
	}
	for _, d := range health.Devices {
		switch d.DeviceID {
		case "EMU":
			if !d.NoStatus || d.Error != "" || d.TunerCount != 4 || d.Streams != 0 || len(d.Tuners) != 0 || d.Model != "HDTC-2US" {
				t.Fatalf("emulator health %+v", d)
			}
		case dev.DeviceID:
			if d.NoStatus || d.Error != "" || len(d.Tuners) != dev.TunerCount {
				t.Fatalf("tuner health %+v", d)
			}
		default:
			t.Fatalf("device %+v", d)
		}
	}
	rec = get(t, h, "/api/v1/tuners")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tuners) != nil || len(tuners.Tuners) != dev.TunerCount {
		t.Fatalf("tuners beside an emulator %d %s", rec.Code, rec.Body.String())
	}
}
