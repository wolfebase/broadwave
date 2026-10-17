package httpapi

import (
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

func TestMarkerSpanMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	schema := requestSchema(t, doc, http.MethodPost, "/recordings/{id}/markers")
	if !strings.Contains(fmt.Sprint(schema["description"]), "end is not greater than start") {
		t.Fatalf("description %#v", schema["description"])
	}
	st := testStore(t)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete",
		StartedAt: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	path := fmt.Sprintf("/api/v1/recordings/%d/markers", id)
	kept := successBody(t, doc, http.MethodPost, "/recordings/{id}/markers", sendJSON(t, api, http.MethodPost, path, `{"start":-1,"end":2}`))
	if kept["start"] != float64(-1) || kept["end"] != float64(2) || kept["confidence"] != float64(1) {
		t.Fatalf("marker %#v", kept)
	}
	bad := assertErrorBody(t, doc, http.MethodPost, "/recordings/{id}/markers", "400", sendJSON(t, api, http.MethodPost, path, `{"start":4,"end":4}`))
	if bad["code"] != "bad_request" || bad["message"] != "start and end required" {
		t.Fatalf("equal %#v", bad)
	}
}

func TestFrameWidthMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	width := querySchema(t, doc, http.MethodGet, "/channels/{id}/frame", "w")
	if width["type"] != "string" || width["enum"] != nil {
		t.Fatalf("w %#v", width)
	}
	dir := t.TempDir()
	frames := filepath.Join(dir, "frames")
	if err := os.MkdirAll(frames, 0o755); err != nil {
		t.Fatal(err)
	}
	small := []byte{0xff, 0xd8, 0xff, 0xd9}
	wide := []byte{0xff, 0xd8, 0xff, 0xd9, 0x01}
	if err := os.WriteFile(filepath.Join(frames, "7.jpg"), small, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frames, "7-1280.jpg"), wide, 0o644); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: testStore(t), Version: "dev", Staging: true, Hub: &live.Hub{Dir: dir}}).Handler()
	get := func(query string) string {
		t.Helper()
		rec := sendJSON(t, api, http.MethodGet, "/api/v1/channels/7/frame"+query, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "image/jpeg") {
			t.Fatalf("%s type %s", query, rec.Header().Get("Content-Type"))
		}
		return rec.Body.String()
	}
	for _, query := range []string{"", "?w=480", "?w=640"} {
		if get(query) != string(small) {
			t.Fatalf("%s was not the 480 frame", query)
		}
	}
	if get("?w=1280") != string(wide) {
		t.Fatal("1280 was not the wide frame")
	}
	// Past 1280 with no full-screen still, the wide one.
	if get("?w=1600") != string(wide) || get("?w=1920") != string(wide) {
		t.Fatal("1600 or 1920 was not the wide frame")
	}
}

func TestEmptyPassOrderLeavesRank(t *testing.T) {
	doc := loadOpenAPI(t)
	schema := requestSchema(t, doc, http.MethodPut, "/passes/order")
	if _, ok := schema["required"]; ok {
		t.Fatalf("required %#v", schema["required"])
	}
	if err := validateSchema(doc, schema, map[string]any{}, "$", 0); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	post := func(title string) map[string]any {
		t.Helper()
		return successBody(t, doc, http.MethodPost, "/passes", sendJSON(t, api, http.MethodPost, "/api/v1/passes", fmt.Sprintf(`{"title":%q}`, title)))
	}
	watch := passNamed(t, post("Harbor Watch"), "Harbor Watch")
	north := passNamed(t, post("North Pier"), "North Pier")
	raw := fmt.Sprintf(`{"ids":[%v,%v]}`, north["id"], watch["id"])
	ranked := successBody(t, doc, http.MethodPut, "/passes/order", sendJSON(t, api, http.MethodPut, "/api/v1/passes/order", raw))
	if passNamed(t, ranked, "North Pier")["priority"] != float64(2) {
		t.Fatalf("rank %#v", ranked["passes"])
	}
	again := successBody(t, doc, http.MethodPut, "/passes/order", sendJSON(t, api, http.MethodPut, "/api/v1/passes/order", "{}"))
	if passNamed(t, again, "North Pier")["priority"] != float64(2) || passNamed(t, again, "Harbor Watch")["priority"] != float64(1) {
		t.Fatalf("empty ids %#v", again["passes"])
	}
	blank := assertErrorBody(t, doc, http.MethodPost, "/passes", "400", sendJSON(t, api, http.MethodPost, "/api/v1/passes", `{"title":" "}`))
	if blank["message"] != "title required" {
		t.Fatalf("blank %#v", blank)
	}
	title, _ := asMap(asMapMust(t, doc.schemas["NewPass"])["properties"])
	name, _ := asMap(title["title"])
	if !strings.Contains(fmt.Sprint(name["description"]), "only spaces") {
		t.Fatalf("title %#v", name["description"])
	}
	if err := validateSchema(doc, asMapMust(t, doc.schemas["NewPass"]), map[string]any{"title": " "}, "$", 0); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownFreeSourceIsRefused(t *testing.T) {
	doc := loadOpenAPI(t)
	kind := requestSchema(t, doc, http.MethodPost, "/sources")
	props, _ := asMap(kind["properties"])
	word, _ := asMap(props["kind"])
	if !strings.Contains(fmt.Sprint(word["description"]), "without regard to case") {
		t.Fatalf("kind %#v", word["description"])
	}
	api := (&Server{Store: testStore(t), Version: "dev", Staging: true}).Handler()
	bad := assertErrorBody(t, doc, http.MethodPost, "/sources/free", "400", sendJSON(t, api, http.MethodPost, "/api/v1/sources/free", "{}"))
	if bad["code"] != "bad_request" || bad["message"] != "that is not a free-channel server" {
		t.Fatalf("free %#v", bad)
	}
}

func TestTrackAliasesMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	prefs := asMapMust(t, doc.schemas["Prefs"])
	props, _ := asMap(prefs["properties"])
	track, _ := asMap(props["track"])
	got, err := stringList(track["enum"])
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"main", "language", "lang", "sap", "described", "vi"} {
		if !listHas(got, word) {
			t.Fatalf("enum %v", got)
		}
	}
	if err := validateSchema(doc, prefs, map[string]any{"track": "sap"}, "$", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(track["description"]), "without regard to case") {
		t.Fatalf("track %#v", track["description"])
	}
}

func TestPairingWordsMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	name := requestField(t, doc, http.MethodPost, "/pair", "name")
	if !strings.Contains(fmt.Sprint(name["description"]), "only spaces") {
		t.Fatalf("name %#v", name["description"])
	}
	kind := requestField(t, doc, http.MethodPost, "/pair", "kind")
	if !strings.Contains(fmt.Sprint(kind["description"]), "stored as other") {
		t.Fatalf("kind %#v", kind["description"])
	}
	api := (&Server{Store: testStore(t), Version: "dev", Staging: true}).Handler()
	blank := assertErrorBody(t, doc, http.MethodPost, "/pair", "400", sendJSON(t, api, http.MethodPost, "/api/v1/pair", `{"name":" "}`))
	if blank["message"] != "a name is required" {
		t.Fatalf("blank %#v", blank)
	}
	started := documentedBody(t, doc, http.MethodPost, "/pair", "201", sendJSON(t, api, http.MethodPost, "/api/v1/pair", `{"name":"Harbor Watch","kind":"Phone","scopes":["Watch"]}`))
	code, _ := started["code"].(string)
	if code == "" {
		t.Fatalf("ticket %#v", started)
	}
	approved := documentedBody(t, doc, http.MethodPost, "/pair/approve", "200", sendJSON(t, api, http.MethodPost, "/api/v1/pair/approve", `{"code":"`+code+`","scopes":[" RECORD "]}`))
	device := objectField(t, approved, "device")
	if device["kind"] != "other" || fmt.Sprint(device["scopes"]) != "[record]" {
		t.Fatalf("device %#v", device)
	}
	shown := documentedBody(t, doc, http.MethodPost, "/pair/code", "201", sendJSON(t, api, http.MethodPost, "/api/v1/pair/code", `{"name":" ","kind":"Phone"}`))
	claimCode, _ := shown["code"].(string)
	refused := assertErrorBody(t, doc, http.MethodPost, "/pair/claim", "400", sendJSON(t, api, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+claimCode+`","name":" "}`))
	if refused["message"] != "a name is required" {
		t.Fatalf("claim %#v", refused)
	}
	claimed := documentedBody(t, doc, http.MethodPost, "/pair/claim", "201", sendJSON(t, api, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+claimCode+`","name":"Harbor Lights"}`))
	if objectField(t, claimed, "device")["kind"] != "other" {
		t.Fatalf("claim kind %#v", claimed["device"])
	}
}

func requestField(t *testing.T, doc *apiDoc, method, path, name string) map[string]any {
	t.Helper()
	props, _ := asMap(requestSchema(t, doc, method, path)["properties"])
	field, ok := asMap(props[name])
	if !ok {
		t.Fatalf("no %s on %s %s", name, method, path)
	}
	return field
}

func asMapMust(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := asMap(value)
	if !ok {
		t.Fatalf("%T", value)
	}
	return obj
}
