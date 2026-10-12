package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// A finished file whose playlist is already built plays from that playlist.
// The stamp is what PlayFile recomputes for an omitted picture mode, an empty
// codec, encoder libx264, and a file too short to scan. A mismatch deletes
// the playlist and starts ffmpeg, which this server does not have.
const warmFileStamp = "broadcast||libx264||transparent|interlaced"

func TestWarmFileCachePlaysWithoutEncoding(t *testing.T) {
	doc := loadOpenAPI(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "recordings", "harbor.ts")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	started := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 0, GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete",
		StartedAt: started, Path: file,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, 60); err != nil {
		t.Fatal(err)
	}
	channel, err := st.CreateVirtual(t.Context(), "900", "Harbor Watch", []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "file", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "graph.txt"), []byte(warmFileStamp), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	api := (&Server{
		Store: st, Version: "dev", Staging: true,
		Hub: &live.Hub{Dir: dir, Encoder: "libx264"},
	}).Handler()
	playlist := fmt.Sprintf("/media/file/%d/index.m3u8", id)

	played := successBody(t, doc, http.MethodPost, "/recordings/{id}/play", sendJSON(t, api, http.MethodPost, fmt.Sprintf("/api/v1/recordings/%d/play", id), "{}"))
	if played["playlist"] != playlist || played["growing"] != false || played["position"] != float64(0) {
		t.Fatalf("play %#v", played)
	}
	if rec := objectField(t, played, "recording"); rec["id"] != float64(id) || rec["title"] != "Harbor Watch" || rec["durationSec"] != float64(60) {
		t.Fatalf("recording %#v", rec)
	}
	if markers := numberList(t, played["markers"]); len(markers) != 0 {
		t.Fatalf("markers %#v", played["markers"])
	}

	virt := successBody(t, doc, http.MethodPost, "/virtuals/{id}/play", sendJSON(t, api, http.MethodPost, fmt.Sprintf("/api/v1/virtuals/%d/play", channel.ID), `{"index":0}`))
	if virt["usesTuner"] != false || virt["playlist"] != playlist || virt["index"] != float64(0) || virt["count"] != float64(1) {
		t.Fatalf("virtual %#v", virt)
	}
	if virt["number"] != "900" || virt["name"] != "Harbor Watch" {
		t.Fatalf("channel %#v", virt)
	}
	if rec := objectField(t, virt, "recording"); rec["id"] != float64(id) {
		t.Fatalf("virtual recording %#v", rec)
	}
	if markers := numberList(t, virt["markers"]); len(markers) != 0 {
		t.Fatalf("virtual markers %#v", virt["markers"])
	}
	if _, err := os.Stat(filepath.Join(cache, "index.m3u8")); err != nil {
		t.Fatal(err)
	}
}

func objectField(t *testing.T, body map[string]any, field string) map[string]any {
	t.Helper()
	obj, ok := body[field].(map[string]any)
	if !ok {
		t.Fatalf("%s is %T", field, body[field])
	}
	return obj
}
