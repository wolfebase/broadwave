package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/live"
)

// TestSourceResponsesMatchOpenAPI starts the API and checks the three add
// results, the free-channel add, and the source list against the spec.
// POST /sources is not one wrapper: a playlist or link is the source, a folder
// is source plus a count, and a long playlist is a pick that saves nothing.
func TestSourceResponsesMatchOpenAPI(t *testing.T) {
	doc := loadOpenAPI(t)
	addSchema, ok, err := responseSchema(doc, "post", "/sources", "200")
	if err != nil || !ok {
		t.Fatalf("POST /sources schema: ok=%v %v", ok, err)
	}
	branches, _ := addSchema["oneOf"].([]any)
	if len(branches) != 3 {
		t.Fatalf("POST /sources 200 must be oneOf the source, the folder result, and the playlist pick, got %#v", addSchema["oneOf"])
	}
	sourceSchema, ok := asMap(doc.schemas["Source"])
	if !ok || sourceSchema["additionalProperties"] != false {
		t.Fatal("Source must refuse fields the spec does not name")
	}
	for _, key := range []string{"groups", "start"} {
		props, _ := asMap(sourceSchema["properties"])
		if _, exists := props[key]; !exists {
			t.Errorf("Source is missing %s", key)
		}
	}
	kinds := sourceKindEnum(t, sourceSchema)
	for _, kind := range []string{"free", "channels", "threadfin", "xteve", "ersatztv", "dispatcharr"} {
		if !listHas(kinds, kind) {
			t.Errorf("Source.kind does not allow %s", kind)
		}
	}

	feeds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/short.m3u":
			fmt.Fprint(w, shortPlaylist())
		case "/long.m3u":
			fmt.Fprint(w, longPlaylist())
		case "/free.m3u":
			fmt.Fprint(w, "#EXTM3U\n#EXTINF:-1,Desk\nhttp://example.invalid/desk.m3u8\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(feeds.Close)

	st := testStore(t)
	api := &Server{
		Store:   st,
		Hub:     &live.Hub{Store: st, Dir: t.TempDir()},
		Version: "dev",
		Staging: true,
	}
	h := api.Handler()

	linkBody := decodePosted(t, postJSON(t, h, "/api/v1/sources", `{"kind":"link","name":"Desk","url":"http://example.invalid/live.ts"}`))
	if err := validateSchema(doc, addSchema, linkBody, "$", 0); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := validateSchema(doc, sourceSchema, linkBody, "$", 0); err != nil {
		t.Fatalf("link is not a Source: %v", err)
	}

	folderBody := decodePosted(t, postJSON(t, h, "/api/v1/sources", `{"kind":"folder","name":"Shelf","url":"`+t.TempDir()+`"}`))
	if err := validateSchema(doc, addSchema, folderBody, "$", 0); err != nil {
		t.Fatalf("folder: %v", err)
	}
	wrapped, _ := folderBody.(map[string]any)
	if _, isSource := wrapped["id"]; isSource {
		t.Fatal("folder add came back as a bare source")
	}
	if wrapped["added"] == nil || wrapped["source"] == nil {
		t.Fatalf("folder add %#v", wrapped)
	}

	pickBody := decodePosted(t, postJSON(t, h, "/api/v1/sources", `{"kind":"m3u","name":"Long","url":"`+feeds.URL+`/long.m3u"}`))
	if err := validateSchema(doc, addSchema, pickBody, "$", 0); err != nil {
		t.Fatalf("pick: %v", err)
	}
	picked, _ := pickBody.(map[string]any)
	if picked["pick"] != true {
		t.Fatalf("long playlist was saved: %#v", picked)
	}

	addedBody := decodePosted(t, postJSON(t, h, "/api/v1/sources", `{"kind":"m3u","name":"News","url":"`+feeds.URL+`/short.m3u","groups":"News","start":100}`))
	if err := validateSchema(doc, addSchema, addedBody, "$", 0); err != nil {
		t.Fatalf("m3u: %v", err)
	}

	freeRec := postJSON(t, h, "/api/v1/sources/free", `{"name":"Fast","playlist":"`+feeds.URL+`/free.m3u"}`)
	freeBody := decodePosted(t, freeRec)
	freeSchema, ok, err := responseSchema(doc, "post", "/sources/free", "200")
	if err != nil || !ok {
		t.Fatalf("free schema: ok=%v %v", ok, err)
	}
	if err := validateSchema(doc, freeSchema, freeBody, "$", 0); err != nil {
		t.Fatalf("free: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var list any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	listSchema, ok, err := responseSchema(doc, "get", "/sources", "200")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := validateSchema(doc, listSchema, list, "$", 0); err != nil {
		t.Fatal(err)
	}
	rows, _ := list.(map[string]any)["sources"].([]any)
	var news map[string]any
	sawFree := false
	for _, row := range rows {
		item, _ := row.(map[string]any)
		if item["name"] == "News" {
			news = item
		}
		if item["kind"] == "free" {
			sawFree = true
		}
	}
	if news == nil || news["groups"] != "News" || news["start"] != float64(100) {
		t.Fatalf("saved filter %#v", news)
	}
	if !sawFree {
		t.Fatal("free source missing from the list")
	}
}

func sourceKindEnum(t *testing.T, schema map[string]any) []string {
	t.Helper()
	props, _ := asMap(schema["properties"])
	kind, _ := asMap(props["kind"])
	list, err := stringList(kind["enum"])
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func shortPlaylist() string {
	return "#EXTM3U\n#EXTINF:-1 group-title=\"News\",Desk\nhttp://example.invalid/desk.m3u8\n"
}

func longPlaylist() string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := 1; i <= 301; i++ {
		fmt.Fprintf(&b, "#EXTINF:-1 group-title=\"News\",Desk %d\nhttp://example.invalid/%d.m3u8\n", i, i)
	}
	return b.String()
}

func decodePosted(t *testing.T, rec *httptest.ResponseRecorder) any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%v %s", err, rec.Body.Bytes())
	}
	return body
}
