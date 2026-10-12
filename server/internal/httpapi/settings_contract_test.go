package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

// TestSettingsRoundTripMatchesTheSpec checks the settings document the
// handlers return. Server-owned guide fields are ignored on write, a value
// outside an enum is refused, and a bad value already stored is returned as
// the default the spec allows.
func TestSettingsRoundTripMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	settingsSchema, ok := asMap(doc.schemas["Settings"])
	if !ok {
		t.Fatal("Settings schema missing")
	}
	props, _ := asMap(settingsSchema["properties"])
	for _, key := range []string{
		"needsSetup", "lastGuidePull", "nextGuidePull", "lastManualGuidePull", "lastGuideError", "lastGuideErrorAt",
	} {
		prop, declared := asMap(props[key])
		if !declared {
			t.Fatalf("Settings.%s is not in the spec", key)
		}
		desc, _ := prop["description"].(string)
		if !strings.Contains(desc, "ignored on write") {
			t.Fatalf("Settings.%s description %q", key, desc)
		}
	}

	dir := t.TempDir()
	var st *store.Store
	open := func() {
		t.Helper()
		var err error
		st, err = store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	t.Cleanup(func() { _ = st.Close() })

	ctx := t.Context()
	if err := st.PutSettings(ctx, map[string]string{"setupComplete": "1"}); err != nil {
		t.Fatal(err)
	}
	failedAt := time.Date(2026, 9, 24, 15, 4, 5, 0, time.UTC)
	if err := st.SetGuideError(ctx, "listings did not load", failedAt); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	body := settingsBody(t, doc, api, http.MethodGet, "/api/v1/settings", "/settings")
	if body["needsSetup"] != "0" || body["lastGuideError"] != "listings did not load" || body["lastGuideErrorAt"] != failedAt.Format(time.RFC3339) {
		t.Fatalf("settings %+v", body)
	}

	saved := putSettingsJSON(t, doc, api, `{
		"needsSetup":"1",
		"layout":"tv",
		"lastGuideError":"changed",
		"lastGuideErrorAt":"2020-01-01T00:00:00Z",
		"lastGuidePull":"2020-01-01T00:00:00Z",
		"nextGuidePull":"2020-01-01T00:00:00Z",
		"lastManualGuidePull":"2020-01-01T00:00:00Z"
	}`)
	if saved["layout"] != "tv" || saved["needsSetup"] != "0" || saved["lastGuideError"] != "listings did not load" || saved["lastGuideErrorAt"] != failedAt.Format(time.RFC3339) {
		t.Fatalf("after write %+v", saved)
	}
	stored, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["needsSetup"]; ok {
		t.Fatalf("needsSetup was stored: %+v", stored)
	}
	for _, key := range []string{"lastGuidePull", "nextGuidePull", "lastManualGuidePull"} {
		if stored[key] != "" {
			t.Fatalf("%s was stored as %q", key, stored[key])
		}
	}
	if stored["lastGuideError"] != "listings did not load" || stored["lastGuideErrorAt"] != failedAt.Format(time.RFC3339) || stored["layout"] != "tv" {
		t.Fatalf("stored %+v", stored)
	}

	for _, item := range []struct{ body, fragment string }{
		{`{"nope":"1"}`, "unknown setting"},
		{`{"autoplay":"yes"}`, "autoplay"},
		{`{"autoplay":""}`, "autoplay"},
		{`{"profile":"cinema"}`, "profile"},
		{`{"audio":"mono"}`, "audio"},
		{`{"hdhrEmulate":"yes"}`, "hdhrEmulate"},
		{`{"setupComplete":"yes"}`, "setupComplete"},
	} {
		rec := putRaw(t, api, item.body)
		assertErrorBody(t, doc, http.MethodPut, "/settings", "400", rec)
		if !strings.Contains(rec.Body.String(), item.fragment) {
			t.Fatalf("%s: %s", item.body, rec.Body.String())
		}
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	plantSettings(t, dir, map[string]string{
		"autoplay":    "yes",
		"profile":     "cinema",
		"audio":       "mono",
		"hdhrEmulate": "yes",
	})
	open()
	api = (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	playSchema, ok, err := responseSchema(doc, http.MethodGet, "/playback", "200")
	if err != nil || !ok {
		t.Fatalf("playback schema: ok=%v %v", ok, err)
	}
	play := getObject(t, api, "/api/v1/playback")
	if err := validateSchema(doc, playSchema, play, "$", 0); err != nil {
		t.Fatal(err)
	}
	if play["autoplay"] != "1" || play["pictureMode"] != "broadcast" || play["layout"] != "tv" {
		t.Fatalf("playback %+v", play)
	}
	again := settingsBody(t, doc, api, http.MethodGet, "/api/v1/settings", "/settings")
	if again["profile"] != "transparent" || again["audio"] != "stereo" || again["hdhrEmulate"] != "0" || again["autoplay"] != "1" {
		t.Fatalf("coerced %+v", again)
	}
	raw, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if raw["autoplay"] != "yes" || raw["profile"] != "cinema" || raw["audio"] != "mono" || raw["hdhrEmulate"] != "yes" {
		t.Fatalf("read changed the stored row %+v", raw)
	}

	freshDir := t.TempDir()
	fresh, err := store.Open(freshDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
	plantSettings(t, freshDir, map[string]string{"setupComplete": "yes"})
	fresh, err = store.Open(freshDir)
	if err != nil {
		t.Fatal(err)
	}
	freshAPI := (&Server{Store: fresh, Version: "dev", Staging: true}).Handler()
	wizard := settingsBody(t, doc, freshAPI, http.MethodGet, "/api/v1/settings", "/settings")
	if wizard["setupComplete"] != "0" || wizard["needsSetup"] != "1" {
		t.Fatalf("fresh %+v", wizard)
	}
	kept, err := fresh.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kept["setupComplete"] != "yes" {
		t.Fatalf("fresh stored %+v", kept)
	}
}

func TestClearedGuideFailureTimeIsOmitted(t *testing.T) {
	doc := loadOpenAPI(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := t.Context()
	if err := st.PutSettings(ctx, map[string]string{"setupComplete": "1"}); err != nil {
		t.Fatal(err)
	}
	last := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)
	next := last.Add(24 * time.Hour)
	if err := st.SetGuideSchedule(ctx, last, next); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	body := settingsBody(t, doc, api, http.MethodGet, "/api/v1/settings", "/settings")
	if _, ok := body["lastGuideErrorAt"]; ok {
		t.Fatalf("cleared failure time was sent: %+v", body["lastGuideErrorAt"])
	}
	if body["lastGuideError"] != "" || body["lastGuidePull"] != last.Format(time.RFC3339) || body["nextGuidePull"] != next.Format(time.RFC3339) {
		t.Fatalf("schedule %+v", body)
	}
	stored, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored["lastGuideErrorAt"] != "" || stored["lastGuideError"] != "" {
		t.Fatalf("stored failure was rewritten: %+v", stored)
	}

	failedAt := time.Date(2026, 10, 8, 16, 4, 5, 0, time.UTC)
	if err := st.SetGuideError(ctx, "listings did not load", failedAt); err != nil {
		t.Fatal(err)
	}
	failed := settingsBody(t, doc, api, http.MethodGet, "/api/v1/settings", "/settings")
	if failed["lastGuideError"] != "listings did not load" || failed["lastGuideErrorAt"] != failedAt.Format(time.RFC3339) {
		t.Fatalf("failure %+v", failed)
	}
	saved := putSettingsJSON(t, doc, api, `{"lastGuideErrorAt":"","lastGuideError":"changed","layout":"tv"}`)
	if saved["lastGuideErrorAt"] != failedAt.Format(time.RFC3339) || saved["lastGuideError"] != "listings did not load" || saved["layout"] != "tv" {
		t.Fatalf("write %+v", saved)
	}
}

func settingsBody(t *testing.T, doc *apiDoc, h http.Handler, method, url, specPath string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, url, rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	schema, ok, err := responseSchema(doc, method, specPath, "200")
	if err != nil || !ok {
		t.Fatalf("%s %s schema: ok=%v %v", method, specPath, ok, err)
	}
	if err := validateSchema(doc, schema, body, "$", 0); err != nil {
		t.Fatal(err)
	}
	return body
}

func putSettingsJSON(t *testing.T, doc *apiDoc, h http.Handler, body string) map[string]any {
	t.Helper()
	rec := putRaw(t, h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	var saved map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	schema, ok, err := responseSchema(doc, http.MethodPut, "/settings", "200")
	if err != nil || !ok {
		t.Fatalf("put schema: ok=%v %v", ok, err)
	}
	if err := validateSchema(doc, schema, saved, "$", 0); err != nil {
		t.Fatal(err)
	}
	return saved
}

func putRaw(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getObject(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// plantSettings writes rows the settings cleaner would refuse, the way an
// older catalog can still hold them.
func plantSettings(t *testing.T, dir string, values map[string]string) {
	t.Helper()
	path := filepath.Join(dir, "broadwave.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for key, value := range values {
		if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			t.Fatal(err)
		}
	}
}
