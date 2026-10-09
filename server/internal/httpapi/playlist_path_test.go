package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPlaylistPathIsNotOpened(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.m3u")
	const marker = "marker-not-a-channel"
	playlist := "#EXTM3U\n#EXTINF:-1,News\nhttp://example/" + marker + ".ts\n"
	if err := os.WriteFile(path, []byte(playlist), 0o644); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	api := &Server{Store: st}
	h := api.Handler()
	post := func(target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	quoted := strconv.Quote(path)
	rec := post("/api/v1/sources", `{"kind":"m3u","name":"Home","url":`+quoted+`}`)
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), marker) {
		t.Fatalf("m3u path: %d %s", rec.Code, rec.Body.String())
	}
	rec = post("/api/v1/sources/free", `{"name":"Free","playlist":`+quoted+`}`)
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), marker) {
		t.Fatalf("free path: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := st.AddSource(context.Background(), "m3u", "Old", path, ""); err != nil {
		t.Fatal(err)
	}
	n, err := api.RefreshSources(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("refresh read %d playlists", n)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels from a file path: %+v", channels)
	}
}
