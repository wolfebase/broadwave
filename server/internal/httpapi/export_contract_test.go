package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"broadwave/internal/live"
)

// TestExportRoutesMatchTheSpec checks the playlist, guide, poster, and art
// routes that are not under /api. Nothing here tunes. A stream HEAD only
// names the type.
func TestExportRoutesMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	h := (&Server{
		Store:   st,
		Hub:     &live.Hub{Store: st, Dir: t.TempDir(), FFmpeg: "ffmpeg"},
		Version: "dev",
		Staging: true,
	}).Handler()

	hits := []struct {
		name, method, url, spec, status, contentType, body string
		empty                                              bool
	}{
		{"lineup", http.MethodGet, "/export/lineup.m3u", "/export/lineup.m3u", "200", "audio/x-mpegurl; charset=utf-8", "#EXTM3U", false},
		{"guide", http.MethodGet, "/export/guide.xml", "/export/guide.xml", "200", "application/xml; charset=utf-8", "<?xml", false},
		{"poster id", http.MethodGet, "/media/poster/nope", "/media/poster/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"poster missing", http.MethodGet, "/media/poster/4404", "/media/poster/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"art id", http.MethodGet, "/media/art/channel/0", "/media/art/{kind}/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"art kind", http.MethodGet, "/media/art/show/1", "/media/art/{kind}/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"art missing", http.MethodGet, "/media/art/channel/4404", "/media/art/{kind}/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"stream id", http.MethodGet, "/export/stream/nope", "/export/stream/{id}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"mosaic key", http.MethodGet, "/export/mosaic/nope", "/export/mosaic/{key}", "404", "text/plain; charset=utf-8", "404 page not found\n", false},
		{"stream head", http.MethodHead, "/export/stream/4404", "/export/stream/{id}", "200", "video/mp2t", "", true},
	}
	for _, tc := range hits {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			req.Host = "localhost"
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != statusCode(t, tc.status) || rec.Header().Get("Content-Type") != tc.contentType {
				t.Fatalf("%d %q\n%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
			}
			if tc.empty {
				if rec.Body.Len() != 0 || rec.Header().Get("X-HDHomeRun-Error") != "" {
					t.Fatalf("probe body %d header %q", rec.Body.Len(), rec.Header().Get("X-HDHomeRun-Error"))
				}
			} else if !strings.HasPrefix(rec.Body.String(), tc.body) {
				t.Fatalf("body %q", rec.Body.String())
			}
			assertExportSpec(t, doc, tc.spec, tc.status, []string{mediaKey(tc.contentType)})
		})
	}

	specOnly := []struct {
		path, status string
		types        []string
	}{
		{"/export/stream/{id}", "403", []string{"application/json"}},
		{"/export/stream/{id}", "429", []string{"text/plain"}},
		{"/export/stream/{id}", "503", []string{"text/plain"}},
		{"/export/mosaic/{key}", "200", []string{"video/mp2t"}},
		{"/export/mosaic/{key}", "403", []string{"application/json"}},
		{"/export/mosaic/{key}", "429", []string{"text/plain"}},
		{"/export/mosaic/{key}", "503", []string{"text/plain"}},
		{"/media/poster/{id}", "200", []string{"image/jpeg"}},
		{"/media/art/{kind}/{id}", "200", []string{"image/jpeg", "image/png", "image/svg+xml"}},
	}
	for _, tc := range specOnly {
		assertExportSpec(t, doc, tc.path, tc.status, tc.types)
	}

	absent := map[string][]string{
		"/export/lineup.m3u":     {"401", "403", "404", "421", "500", "206", "304", "412", "416"},
		"/export/guide.xml":      {"401", "403", "404", "421", "500", "206", "304", "412", "416"},
		"/export/stream/{id}":    {"401", "421", "500", "206", "304", "412", "416"},
		"/export/mosaic/{key}":   {"401", "421", "500", "206", "304", "412", "416"},
		"/media/poster/{id}":     {"401", "403", "421", "500", "206", "304", "412", "416"},
		"/media/art/{kind}/{id}": {"401", "403", "421", "500", "206", "304", "412", "416"},
	}
	for path, codes := range absent {
		op := exportOperation(t, doc, path)
		responses, ok := asMap(op["responses"])
		if !ok {
			t.Fatalf("%s responses", path)
		}
		for _, code := range codes {
			if _, ok := responses[code]; ok {
				t.Errorf("%s lists %s", path, code)
			}
		}
	}
}

func statusCode(t *testing.T, status string) int {
	t.Helper()
	code, err := strconv.Atoi(status)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func mediaKey(header string) string {
	key, _, _ := strings.Cut(header, ";")
	return strings.TrimSpace(key)
}

func exportOperation(t *testing.T, doc *apiDoc, path string) map[string]any {
	t.Helper()
	item, ok := asMap(doc.paths[path])
	if !ok {
		t.Fatalf("spec missing %s", path)
	}
	servers, ok := item["servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("%s servers %#v", path, item["servers"])
	}
	server, ok := asMap(servers[0])
	if !ok || server["url"] != "http://localhost:8477" {
		t.Fatalf("%s server %#v", path, servers[0])
	}
	op, ok := asMap(item["get"])
	if !ok {
		t.Fatalf("%s has no get", path)
	}
	return op
}

func assertExportSpec(t *testing.T, doc *apiDoc, path, status string, types []string) {
	t.Helper()
	op := exportOperation(t, doc, path)
	responses, ok := asMap(op["responses"])
	if !ok {
		t.Fatalf("%s responses", path)
	}
	raw, ok := responses[status]
	if !ok {
		t.Fatalf("%s missing %s", path, status)
	}
	resp, ok := asMap(raw)
	if !ok {
		t.Fatalf("%s %s %#v", path, status, raw)
	}
	if ref, _ := resp["$ref"].(string); ref != "" {
		name := strings.TrimPrefix(ref, "#/components/responses/")
		if name == ref {
			t.Fatalf("bad ref %s", ref)
		}
		resp, ok = asMap(doc.responses[name])
		if !ok {
			t.Fatalf("missing response %s", name)
		}
	}
	content, ok := asMap(resp["content"])
	if !ok || len(content) != len(types) {
		t.Fatalf("%s %s content %#v, want %v", path, status, resp["content"], types)
	}
	for _, key := range types {
		if _, ok := content[key]; !ok {
			t.Fatalf("%s %s content %#v, missing %s", path, status, resp["content"], key)
		}
	}
}
