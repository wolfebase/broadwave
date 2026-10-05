package httpapi

import (
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

func TestStampRecordingPlaylistUsesTheRecordingStart(t *testing.T) {
	start := time.Date(2026, 10, 5, 1, 26, 54, 0, time.UTC)
	src := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-10-05T01:41:57.000Z\n#EXTINF:2.000,\nseg00000.ts\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-10-05T01:41:59.000Z\n#EXTINF:2.000,\nseg00001.ts\n"
	got := string(stampRecordingPlaylist([]byte(src), start))
	first := "2026-10-05T01:26:54.000Z"
	second := "2026-10-05T01:26:56.000Z"
	if !strings.Contains(got, "#EXT-X-PROGRAM-DATE-TIME:"+first+"\n#EXTINF:2.000,\nseg00000.ts") {
		t.Fatalf("first date: %s", got)
	}
	if !strings.Contains(got, "#EXT-X-PROGRAM-DATE-TIME:"+second+"\n#EXTINF:2.000,\nseg00001.ts") {
		t.Fatalf("second date: %s", got)
	}
	if strings.Contains(got, "01:41:57") {
		t.Fatalf("play-press time stayed: %s", got)
	}
}

func TestARecordingPlayedLaterKeepsItsBroadcastDates(t *testing.T) {
	st := testStore(t)
	start := time.Date(2026, 10, 5, 1, 26, 54, 0, time.UTC)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "News", Status: "complete", Path: "news.ts", StartedAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	playDir := filepath.Join(dir, "file", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(playDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-10-05T01:41:57.123Z\n#EXTINF:2.002,\nseg00000.ts\n"
	if err := os.WriteFile(filepath.Join(playDir, "index.m3u8"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Dir: dir}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/file/"+strconv.FormatInt(id, 10)+"/index.m3u8", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	want := "#EXT-X-PROGRAM-DATE-TIME:2026-10-05T01:26:54.000Z"
	if !strings.Contains(body, want) {
		t.Fatalf("first date: %s", body)
	}
	if i := strings.Index(body, "#EXT-X-PROGRAM-DATE-TIME:"); i < 0 {
		t.Fatal(body)
	} else if got := body[i+len("#EXT-X-PROGRAM-DATE-TIME:") : i+len("#EXT-X-PROGRAM-DATE-TIME:")+24]; got != "2026-10-05T01:26:54.000Z" {
		t.Fatalf("first PROGRAM-DATE-TIME %q, want the recording start", got)
	}
	if rec.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
}
