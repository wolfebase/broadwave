package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadedPlaylistStopsAtTheCap(t *testing.T) {
	prev := maxPlaylistUpload
	maxPlaylistUpload = 400
	t.Cleanup(func() { maxPlaylistUpload = prev })

	st := testStore(t)
	h := (&Server{Store: st}).Handler()
	playlist := "#EXTM3U\n#EXTINF:-1,News\nhttp://example/news.ts\n" + strings.Repeat("# padding\n", 200)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "home.m3u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(playlist)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("name", "Home"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if int64(body.Len()) <= maxPlaylistUpload {
		t.Fatalf("fixture is %d bytes, under the cap", body.Len())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("accepted a playlist past the cap: %s", rec.Body.String())
	}
	sources, err := st.Sources(context.Background())
	if err != nil || len(sources) != 0 {
		t.Fatalf("stored %+v %v", sources, err)
	}
}
