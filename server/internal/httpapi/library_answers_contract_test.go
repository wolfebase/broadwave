package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestRecordingNFOMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "recordings", "harbor.ts")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "elsewhere", "other.ts")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	started := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	if _, err := st.CreateRecording(t.Context(), store.Recording{
		GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete", StartedAt: started, Path: file,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRecording(t.Context(), store.Recording{
		GuideNumber: "12.1", Title: "Harbor Lights", Status: "complete", StartedAt: started, Path: outside,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRecording(t.Context(), store.Recording{
		GuideNumber: "12.1", Title: "North Pier", Status: "recording", StartedAt: started, Path: file,
	}); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true, Hub: &live.Hub{Dir: dir}}).Handler()
	body := successBody(t, doc, http.MethodPost, "/recordings/nfo", sendJSON(t, api, http.MethodPost, "/api/v1/recordings/nfo", ""))
	if body["written"] != float64(1) {
		t.Fatalf("written %#v", body["written"])
	}
	side, err := os.ReadFile(strings.TrimSuffix(file, filepath.Ext(file)) + ".nfo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(side), "Harbor Watch") {
		t.Fatalf("sidecar %s", side)
	}
	if _, err := os.Stat(strings.TrimSuffix(outside, filepath.Ext(outside)) + ".nfo"); !os.IsNotExist(err) {
		t.Fatalf("outside sidecar %v", err)
	}
}

func TestPassOrderMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	post := func(title string) map[string]any {
		t.Helper()
		return successBody(t, doc, http.MethodPost, "/passes", sendJSON(t, api, http.MethodPost, "/api/v1/passes", fmt.Sprintf(`{"title":%q}`, title)))
	}
	watch := passNamed(t, post("Harbor Watch"), "Harbor Watch")
	north := passNamed(t, post("North Pier"), "North Pier")
	raw, err := json.Marshal(map[string]any{"ids": []any{north["id"], watch["id"]}})
	if err != nil {
		t.Fatal(err)
	}
	ordered := successBody(t, doc, http.MethodPut, "/passes/order", sendJSON(t, api, http.MethodPut, "/api/v1/passes/order", string(raw)))
	if passNamed(t, ordered, "North Pier")["priority"] != float64(2) || passNamed(t, ordered, "Harbor Watch")["priority"] != float64(1) {
		t.Fatalf("order %#v", ordered["passes"])
	}
}

func TestTeamFollowMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	body := successBody(t, doc, http.MethodPut, "/teams", sendJSON(t, api, http.MethodPut, "/api/v1/teams", `{"name":"Harbor Watch","abbr":"HBR","league":"other","record":true,"logo":"http://example.com/mark.png"}`))
	teams := numberList(t, body["teams"])
	if len(teams) != 1 {
		t.Fatalf("teams %#v", body["teams"])
	}
	team, ok := teams[0].(map[string]any)
	if !ok {
		t.Fatalf("team %#v", teams[0])
	}
	if team["name"] != "Harbor Watch" || team["abbr"] != "HBR" || team["league"] != "other" || team["record"] != true {
		t.Fatalf("team %#v", team)
	}
	if _, ok := team["logo"]; ok {
		t.Fatalf("logo %#v", team["logo"])
	}
}

func TestBlankTeamNameIsRejected(t *testing.T) {
	doc := loadOpenAPI(t)
	schema, ok := asMap(doc.schemas["TeamFollow"])
	if !ok {
		t.Fatal("TeamFollow schema missing")
	}
	props, _ := asMap(schema["properties"])
	name, _ := asMap(props["name"])
	desc, _ := name["description"].(string)
	if !strings.Contains(desc, "only spaces") {
		t.Fatalf("name description %q", desc)
	}
	api := (&Server{Store: testStore(t), Version: "dev", Staging: true}).Handler()
	blank := sendJSON(t, api, http.MethodPut, "/api/v1/teams", `{"name":"  "}`)
	errBody := assertErrorBody(t, doc, http.MethodPut, "/teams", "400", blank)
	if errBody["code"] != "bad_request" || errBody["message"] != "a team needs a name" {
		t.Fatalf("%#v", errBody)
	}
	body := successBody(t, doc, http.MethodPut, "/teams", sendJSON(t, api, http.MethodPut, "/api/v1/teams", `{"name":" Harbor Watch ","abbr":"HBR"}`))
	teams := numberList(t, body["teams"])
	if len(teams) != 1 {
		t.Fatalf("teams %#v", body["teams"])
	}
	team, ok := teams[0].(map[string]any)
	if !ok || team["name"] != "Harbor Watch" {
		t.Fatalf("team %#v", teams[0])
	}
}

func TestScheduleFixMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	for _, pass := range []store.Pass{
		{Title: "North Pier", Priority: 10},
		{Title: "South Pier", Priority: 10},
		{Title: "Harbor Watch", Priority: 1},
	} {
		if _, err := st.AddSeriesPass(t.Context(), pass); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	hour := now.Add(2 * time.Hour)
	later := now.Add(4 * time.Hour)
	if err := st.ReplaceAirings(t.Context(), []store.Airing{
		{ChannelID: 11, Title: "North Pier", Start: hour, End: hour.Add(time.Hour)},
		{ChannelID: 12, Title: "South Pier", Start: hour, End: hour.Add(time.Hour)},
		{ChannelID: 13, Title: "Harbor Watch", Start: hour, End: hour.Add(time.Hour)},
		{ChannelID: 11, Title: "Harbor Watch", Start: later, End: later.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	before := successBody(t, doc, http.MethodGet, "/schedule", sendJSON(t, api, http.MethodGet, "/api/v1/schedule", ""))
	if before["tunerCount"] != float64(2) {
		t.Fatalf("tuners %#v", before["tunerCount"])
	}
	item := plannedShowing(t, before["items"], "Harbor Watch", true)
	if item["skipped"] != true || item["conflict"] != true {
		t.Fatalf("before %#v", item)
	}
	airing := objectField(t, item, "airing")
	suggestion := objectField(t, item, "suggestion")
	raw, err := json.Marshal(map[string]any{
		"passId":              item["passId"],
		"channelId":           airing["channelId"],
		"start":               airing["start"],
		"suggestionChannelId": suggestion["channelId"],
		"suggestionStart":     suggestion["start"],
	})
	if err != nil {
		t.Fatal(err)
	}
	after := successBody(t, doc, http.MethodPost, "/schedule/fix", sendJSON(t, api, http.MethodPost, "/api/v1/schedule/fix", string(raw)))
	if after["tunerCount"] != float64(2) {
		t.Fatalf("tuners %#v", after["tunerCount"])
	}
	fixed := showingAt(t, after["items"], "Harbor Watch", airing["start"])
	if fixed["skipped"] != true || fixed["conflict"] != false || fixed["reason"] != "Skipped once" {
		t.Fatalf("fixed %#v", fixed)
	}
	if _, ok := fixed["suggestion"]; ok {
		t.Fatalf("suggestion %#v", fixed["suggestion"])
	}
	kept := showingAt(t, after["items"], "Harbor Watch", suggestion["start"])
	if kept["skipped"] != false {
		t.Fatalf("later %#v", kept)
	}
}

func passNamed(t *testing.T, body map[string]any, title string) map[string]any {
	t.Helper()
	for _, raw := range numberList(t, body["passes"]) {
		pass, ok := raw.(map[string]any)
		if ok && pass["title"] == title {
			return pass
		}
	}
	t.Fatalf("no pass %s in %#v", title, body["passes"])
	return nil
}

func plannedShowing(t *testing.T, items any, title string, suggestion bool) map[string]any {
	t.Helper()
	for _, raw := range numberList(t, items) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		airing, ok := item["airing"].(map[string]any)
		if !ok || airing["title"] != title {
			continue
		}
		_, has := item["suggestion"]
		if has == suggestion {
			return item
		}
	}
	t.Fatalf("no %s suggestion=%v in %#v", title, suggestion, items)
	return nil
}

func showingAt(t *testing.T, items any, title string, start any) map[string]any {
	t.Helper()
	for _, raw := range numberList(t, items) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		airing, ok := item["airing"].(map[string]any)
		if ok && airing["title"] == title && airing["start"] == start {
			return item
		}
	}
	t.Fatalf("no %s at %#v in %#v", title, start, items)
	return nil
}
