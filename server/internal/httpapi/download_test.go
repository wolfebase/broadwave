package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestDownloadRecording(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(filepath.Join(root, "shows"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x47, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	nested := filepath.Join(root, "shows", "Evening News.ts")
	weird := filepath.Join(root, "say\"hi\nnews.ts")
	dotted := filepath.Join(root, "my..show.ts")
	for _, path := range []string{nested, weird, dotted} {
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	secret := []byte("SECRET-BYTES-NOT-A-RECORDING")
	outside := filepath.Join(dir, "secret.ts")
	if err := os.WriteFile(outside, secret, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.ts")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	shelf := filepath.Join(dir, "shelf")
	if err := os.MkdirAll(shelf, 0o755); err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(shelf, "Movie.mp4")
	if err := os.WriteFile(movie, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(dir, "stray.mp4")
	if err := os.WriteFile(stray, secret, 0o644); err != nil {
		t.Fatal(err)
	}

	st := testStore(t)
	ctx := context.Background()
	if _, err := st.AddSource(ctx, "folder", "Shelf", shelf, ""); err != nil {
		t.Fatal(err)
	}
	idOf := func(path string) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: "Show", Status: "complete", Path: path, StartedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	nestedID := idOf(nested)
	weirdID := idOf(weird)
	dottedID := idOf(dotted)
	goneID := idOf(filepath.Join(root, "missing.ts"))
	// Stored as written, so Clean has to resolve the ".." itself.
	escapeID := idOf(root + "/../secret.ts")
	outsideID := idOf(outside)
	linkID := idOf(link)
	movieID := idOf(movie)
	strayID := idOf(stray)

	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	fileURL := func(id int64) string {
		return "/api/v1/recordings/" + strconv.FormatInt(id, 10) + "/file"
	}
	tests := []struct {
		name      string
		path      string
		rangeHdr  string
		want      int
		filename  string
		body      []byte
		notSecret bool
		kind      string
	}{
		{name: "unknown id", path: "/api/v1/recordings/999999/file", want: http.StatusNotFound},
		{name: "file gone", path: fileURL(goneID), want: http.StatusNotFound},
		{name: "base name", path: fileURL(nestedID), want: http.StatusOK, filename: "Evening News.ts", body: payload},
		{name: "quote and newline", path: fileURL(weirdID), want: http.StatusOK, filename: "say\"hi\nnews.ts", body: payload},
		{name: "dots in the name", path: fileURL(dottedID), want: http.StatusOK, filename: "my..show.ts", body: payload},
		{name: "range", path: fileURL(nestedID), rangeHdr: "bytes=4-7", want: http.StatusPartialContent, filename: "Evening News.ts", body: payload[4:8]},
		{name: "dotdot", path: fileURL(escapeID), want: http.StatusNotFound, notSecret: true},
		{name: "outside", path: fileURL(outsideID), want: http.StatusNotFound, notSecret: true},
		{name: "symlink", path: fileURL(linkID), want: http.StatusNotFound, notSecret: true},
		{name: "library folder", path: fileURL(movieID), want: http.StatusOK, filename: "Movie.mp4", body: payload, kind: "video/mp4"},
		{name: "media outside every folder", path: fileURL(strayID), want: http.StatusNotFound, notSecret: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.rangeHdr != "" {
				req.Header.Set("Range", tc.rangeHdr)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body.Bytes())
			}
			if tc.notSecret && bytes.Contains(rec.Body.Bytes(), secret) {
				t.Fatalf("body leaked the file outside the recordings folder: %s", rec.Body.Bytes())
			}
			if tc.body != nil && !bytes.Equal(rec.Body.Bytes(), tc.body) {
				t.Fatalf("body %x", rec.Body.Bytes())
			}
			if tc.rangeHdr != "" {
				wantRange := fmt.Sprintf("bytes 4-7/%d", len(payload))
				if got := rec.Header().Get("Content-Range"); got != wantRange {
					t.Fatalf("Content-Range %q, want %q", got, wantRange)
				}
			}
			if tc.filename == "" {
				return
			}
			disp := rec.Header().Get("Content-Disposition")
			if strings.ContainsAny(disp, "\r\n") {
				t.Fatalf("disposition breaks the header: %q", disp)
			}
			if strings.Contains(disp, dir) || strings.Contains(disp, "shows/") || strings.Contains(disp, `shows\`) {
				t.Fatalf("disposition has a directory: %q", disp)
			}
			kind, params, err := mime.ParseMediaType(disp)
			if err != nil || kind != "attachment" {
				t.Fatalf("disposition %q: %v", disp, err)
			}
			gotName := params["filename"]
			if gotName != tc.filename || gotName != filepath.Base(gotName) || strings.ContainsAny(gotName, `/\`) {
				t.Fatalf("filename %q, want %q", gotName, tc.filename)
			}
			wantKind := tc.kind
			if wantKind == "" {
				wantKind = "video/mp2t"
			}
			if rec.Header().Get("Content-Type") != wantKind {
				t.Fatalf("content type %s, want %s", rec.Header().Get("Content-Type"), wantKind)
			}
		})
	}

	bare := (&Server{Store: st}).Handler()
	rec := httptest.NewRecorder()
	bare.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fileURL(nestedID), nil))
	if rec.Code != http.StatusNotFound || bytes.Contains(rec.Body.Bytes(), payload) {
		t.Fatalf("no recordings folder: %d %s", rec.Code, rec.Body.Bytes())
	}
}
