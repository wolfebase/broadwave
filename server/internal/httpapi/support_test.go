package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/logbuf"
	"broadwave/internal/store"
)

func TestSupportBundleLeavesOutSecrets(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	const (
		password  = "ops3-fixture-password"
		auth      = "ops3-device-auth-token"
		tmdbKey   = "ops3-tmdb-key-value"
		sportsKey = "ops3-sportsdb-key-value"
	)
	playlist := "http://ops3user:" + password + "@playlist.example/pl.m3u?password=" + password
	guide := "http://playlist.example/xmltv.php?username=ops3user&password=" + password
	guidePage := "http://ops3user:" + password + "@guides.example/xmltv?DeviceAuth=" + auth
	item, err := st.AddSource(ctx, "xtream", "IPTV", playlist, guide)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{
		"layout":      "tv",
		"sdPassword":  password,
		"tmdbKey":     tmdbKey,
		"sportsdbKey": sportsKey,
		"guideUrl":    guidePage,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "DUO", FriendlyName: "HDHomeRun", TunerCount: 2,
		BaseURL: "http://tuner.example", LineupURL: "http://tuner.example/lineup.json?DeviceAuth=" + auth,
	}, nil); err != nil {
		t.Fatal(err)
	}
	list, err := st.Sources(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	if list[0].URL != store.MaskURL(playlist) || strings.Contains(list[0].URL, password) {
		t.Fatalf("list url %s", list[0].URL)
	}
	if list[0].XMLTV != store.MaskURL(guide) || strings.Contains(list[0].XMLTV, password) {
		t.Fatalf("list guide %s", list[0].XMLTV)
	}
	if got := st.FetchURL(ctx, item.ID, list[0].URL); got != playlist {
		t.Fatal("playlist did not round-trip")
	}
	if got := st.FetchURL(ctx, item.ID, list[0].XMLTV); got != guide {
		t.Fatal("guide did not round-trip")
	}

	logbuf.Install(os.Stderr)
	log.Printf("source %s DeviceAuth=%s", playlist, auth)

	api := &Server{Store: st, Version: "dev"}
	rec := get(t, api.Handler(), "/api/v1/support")
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("content type %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "broadwave-support.zip") {
		t.Fatal(rec.Header().Get("Content-Disposition"))
	}
	body := rec.Body.Bytes()
	if bytes.Contains(body, []byte(password)) || bytes.Contains(body, []byte(auth)) || bytes.Contains(body, []byte(tmdbKey)) || bytes.Contains(body, []byte(sportsKey)) || bytes.Contains(body, []byte("DeviceAuth")) {
		t.Fatal("bundle bytes contain a secret")
	}
	if text := logbuf.Text(); strings.Contains(text, password) || strings.Contains(text, auth) || strings.Contains(text, "DeviceAuth") {
		t.Fatal("log ring kept a secret")
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	var names []string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[f.Name] = string(data)
		names = append(names, f.Name)
		if bytes.Contains(data, []byte(password)) || bytes.Contains(data, []byte(auth)) || bytes.Contains(data, []byte(tmdbKey)) || bytes.Contains(data, []byte(sportsKey)) || bytes.Contains(data, []byte("DeviceAuth")) {
			t.Fatalf("%s contains a secret", f.Name)
		}
	}
	for _, name := range []string{"versions.json", "doctor.json", "config.json", "logs.txt"} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("missing %s in %v", name, names)
		}
	}
	var versions struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(entries["versions.json"]), &versions); err != nil || versions.Version != "dev" {
		t.Fatalf("versions %s %v", entries["versions.json"], err)
	}
	var report struct {
		Notes []any `json:"notes"`
	}
	if err := json.Unmarshal([]byte(entries["doctor.json"]), &report); err != nil || report.Notes == nil {
		t.Fatalf("doctor %s %v", entries["doctor.json"], err)
	}
	var cfg struct {
		Settings map[string]string `json:"settings"`
		Sources  []store.Source    `json:"sources"`
		Devices  []store.Device    `json:"devices"`
	}
	if err := json.Unmarshal([]byte(entries["config.json"]), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Settings["layout"] != "tv" || cfg.Settings["sdPasswordSet"] != "1" || cfg.Settings["tmdbKeySet"] != "1" || cfg.Settings["sportsdbKeySet"] != "1" {
		t.Fatalf("settings %+v", cfg.Settings)
	}
	if _, ok := cfg.Settings["sdPassword"]; ok {
		t.Fatal("settings included sdPassword")
	}
	if _, ok := cfg.Settings["tmdbKey"]; ok {
		t.Fatal("settings included tmdbKey")
	}
	if _, ok := cfg.Settings["sportsdbKey"]; ok {
		t.Fatal("settings included sportsdbKey")
	}
	if !strings.Contains(cfg.Settings["guideUrl"], "guides.example") || strings.Contains(cfg.Settings["guideUrl"], password) || strings.Contains(cfg.Settings["guideUrl"], auth) {
		t.Fatalf("guide url %s", cfg.Settings["guideUrl"])
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].URL != list[0].URL || cfg.Sources[0].XMLTV != list[0].XMLTV {
		t.Fatalf("exported source %+v list %+v", cfg.Sources, list[0])
	}
	if len(cfg.Devices) != 1 || !strings.Contains(cfg.Devices[0].LineupURL, "tuner.example") || strings.Contains(cfg.Devices[0].LineupURL, auth) {
		t.Fatalf("device %+v", cfg.Devices)
	}
	if !strings.Contains(entries["logs.txt"], "playlist.example") {
		t.Fatalf("logs %s", entries["logs.txt"])
	}
	writeSupportSample(t, body)
}

func writeSupportSample(t *testing.T, body []byte) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", ".evidence", "ops3")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broadwave-support.zip"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}
