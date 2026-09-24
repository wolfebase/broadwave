package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestXtreamImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("password") != "s3cret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			_, _ = w.Write([]byte(`[{"category_id":"1","category_name":"News"}]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[{"num":4,"name":"Local News","stream_id":100,"stream_icon":"http://example/a.png","epg_channel_id":"news","category_id":"1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	body, guide, err := Xtream(t.Context(), srv.URL, "ada", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `tvg-id="news"`) || !strings.Contains(text, `group-title="News"`) || !strings.Contains(text, "/live/ada/s3cret/100.ts") {
		t.Fatal(text)
	}
	if !strings.Contains(guide, "password=s3cret") {
		t.Fatal(guide)
	}
	got := ParseM3U(strings.NewReader(text))
	if len(got) != 1 || got[0].Name != "Local News" || got[0].Number != "4" || got[0].Group != "News" {
		t.Fatalf("%+v", got)
	}
	if _, _, err := Xtream(t.Context(), srv.URL, "ada", "wrong"); err == nil {
		t.Fatal("a rejected login should fail")
	}
}
