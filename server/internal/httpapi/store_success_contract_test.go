package httpapi

import (
	"net/http"
	"testing"
	"time"

	"broadwave/internal/backup"
	"broadwave/internal/hdhr"
)

// Store-only successes: no tuner is opened. A health row uses a loopback
// address nothing is listening on, so the refusal is local.
func TestStoreSuccessMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	if err := st.UpsertDevice(t.Context(), hdhr.Device{
		DeviceID:        "HWHARBOR",
		FriendlyName:    "Harbor Tuner",
		ModelNumber:     "HDHR-TEST",
		FirmwareVersion: "0.0.0",
		BaseURL:         "http://127.0.0.1:19199",
	}, nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	api := (&Server{
		Store:     st,
		BackupDir: dir,
		Version:   "dev",
		Staging:   true,
	}).Handler()

	reception := documentedBody(t, doc, "get", "/channels/{id}/reception", "200",
		call(t, api, http.MethodGet, "/api/v1/channels/1/reception", "", ""))
	if reception["freeTuner"] != false || reception["tunerAnswers"] != false || reception["signalLost"] != false {
		t.Fatalf("reception %#v", reception)
	}
	for _, id := range []string{"nope", "0"} {
		bad := assertErrorBody(t, doc, "get", "/channels/{id}/reception", "400",
			call(t, api, http.MethodGet, "/api/v1/channels/"+id+"/reception", "", ""))
		if bad["code"] != "bad_request" {
			t.Fatalf("%s code %#v", id, bad["code"])
		}
	}

	signals := documentedBody(t, doc, "get", "/signals", "200", call(t, api, http.MethodGet, "/api/v1/signals", "", ""))
	if signals["running"] != false || len(signals["channels"].([]any)) != 0 {
		t.Fatalf("signals %#v", signals)
	}
	busy := assertErrorBody(t, doc, "post", "/signals/check", "409",
		call(t, api, http.MethodPost, "/api/v1/signals/check", "", ""))
	if busy["code"] != "conflict" {
		t.Fatalf("check %#v", busy)
	}

	tuners := documentedBody(t, doc, "get", "/tuners", "200", call(t, api, http.MethodGet, "/api/v1/tuners", "", ""))
	if len(tuners["tuners"].([]any)) != 0 {
		t.Fatalf("tuners %#v", tuners)
	}

	screens := documentedBody(t, doc, "get", "/screens", "200", call(t, api, http.MethodGet, "/api/v1/screens", "", ""))
	if len(screens["screens"].([]any)) != 0 {
		t.Fatalf("screens %#v", screens)
	}

	for _, key := range []string{"1-2", "nope"} {
		stopped := documentedBody(t, doc, "post", "/mosaic/{key}/stop", "200",
			postJSON(t, api, "/api/v1/mosaic/"+key+"/stop", `{}`))
		if stopped["ok"] != true {
			t.Fatalf("stop %s %#v", key, stopped)
		}
	}

	empty := documentedBody(t, doc, "get", "/backups", "200", call(t, api, http.MethodGet, "/api/v1/backups", "", ""))
	if len(empty["backups"].([]any)) != 0 {
		t.Fatalf("empty backups %#v", empty)
	}
	item, err := backup.Take(t.Context(), st, dir, time.Date(2026, 5, 2, 3, 0, 0, 0, time.UTC), backup.KindDaily)
	if err != nil {
		t.Fatal(err)
	}
	listed := documentedBody(t, doc, "get", "/backups", "200", call(t, api, http.MethodGet, "/api/v1/backups", "", ""))
	copies, ok := listed["backups"].([]any)
	if !ok || len(copies) != 1 {
		t.Fatalf("backups %#v", listed)
	}
	copy, ok := copies[0].(map[string]any)
	if !ok || copy["name"] != item.Name || copy["kind"] != "daily" {
		t.Fatalf("backup %#v", copies[0])
	}
	restored := documentedBody(t, doc, "post", "/backups/{name}/restore", "200",
		call(t, api, http.MethodPost, "/api/v1/backups/"+item.Name+"/restore", "", ""))
	if restored["ok"] != true {
		t.Fatalf("restore %#v", restored)
	}

	health := documentedBody(t, doc, "get", "/devices/health", "200",
		call(t, api, http.MethodGet, "/api/v1/devices/health", "", ""))
	devices, ok := health["devices"].([]any)
	if !ok || len(devices) != 1 {
		t.Fatalf("health %#v", health)
	}
	device, ok := devices[0].(map[string]any)
	if !ok || device["deviceId"] != "HWHARBOR" || device["model"] != "HDHR-TEST" || device["firmwareVersion"] != "0.0.0" {
		t.Fatalf("device %#v", devices[0])
	}
	if device["error"] != "This tuner did not answer. Check that it is on." {
		t.Fatalf("error %#v", device["error"])
	}
	if len(device["tuners"].([]any)) != 0 {
		t.Fatalf("tuners %#v", device["tuners"])
	}
}
