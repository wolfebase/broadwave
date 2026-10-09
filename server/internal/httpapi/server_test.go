package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestGuideAndFavorite(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "DUO", ModelNumber: "HDHR5-2US",
		BaseURL: "http://192.168.1.252", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "14.2", GuideName: "Snapshp", VideoCodec: "H264", AudioCodec: "AC3"},
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, Favorite: true, StreamURL: "http://192.168.1.252:5004/auto/v4.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>Broadwave</title>")},
	}
	h := (&Server{Store: st, Assets: assets}).Handler()

	res := get(t, h, "/api/channels?guide=1")
	var payload struct {
		Channels []store.Channel `json:"channels"`
		Listings string          `json:"listings"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Listings != "empty" || len(payload.Channels) != 2 {
		t.Fatalf("%+v", payload)
	}
	if payload.Channels[0].GuideNumber != "4.1" || !payload.Channels[0].Favorite {
		t.Fatalf("first channel %+v", payload.Channels[0])
	}
	raw, _ := json.Marshal(payload.Channels[0])
	if bytes.Contains(raw, []byte("5004")) {
		t.Fatalf("stream url leaked to the guide api: %s", raw)
	}

	body := bytes.NewBufferString(`{"favorite":false,"customName":"Fox 4"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/channels/"+strconv.FormatInt(payload.Channels[0].ID, 10), body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch %d %s", rec.Code, rec.Body.String())
	}
	var ch store.Channel
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil {
		t.Fatal(err)
	}
	if ch.Favorite || ch.DisplayName != "Fox 4" {
		t.Fatalf("%+v", ch)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"password":"secret"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("password status %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/guide", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("Broadwave")) {
		t.Fatalf("spa %d %s", rec.Code, rec.Body.String())
	}
}

func TestVirtualChannelAndSchedule(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "DUO", ModelNumber: "HDHR5-2US",
		BaseURL: "http://192.168.1.252", TunerCount: 2,
	}, nil); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var channelID int64
	if len(channels) > 0 {
		channelID = channels[0].ID
	}
	start := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 2, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 3, Title: "News", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	recID, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: channelID, GuideNumber: "4.1", Title: "Fox check",
		Path: filepath.Join(t.TempDir(), "missing.ts"), Status: "complete", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateVirtual(ctx, "900", "Fox replays", []int64{recID})
	if err != nil {
		t.Fatal(err)
	}

	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	res := get(t, h, "/api/virtuals")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"number":"900"`)) {
		t.Fatalf("virtuals %s", res.Body.String())
	}
	res = get(t, h, "/api/schedule")
	var sched struct {
		TunerCount int `json:"tunerCount"`
		Items      []struct {
			Skipped bool `json:"skipped"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &sched); err != nil {
		t.Fatal(err)
	}
	if sched.TunerCount != 2 || len(sched.Items) != 3 {
		t.Fatalf("%+v", sched)
	}
	skipped := 0
	for _, item := range sched.Items {
		if item.Skipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("one of three equal-priority shows gives up a tuner: %+v", sched)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/virtuals/"+strconv.FormatInt(created.ID, 10)+"/play", bytes.NewBufferString(`{"index":0}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("play without a player %d %s", rec.Code, rec.Body.String())
	}

	hub := &live.Hub{Store: st, Dir: t.TempDir(), FFmpeg: filepath.Join(t.TempDir(), "missing-ffmpeg"), Encoder: "libx264"}
	h = (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	req = httptest.NewRequest(http.MethodPost, "/api/virtuals/"+strconv.FormatInt(created.ID, 10)+"/play", bytes.NewBufferString(`{"index":0}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("antenna")) {
		t.Fatalf("library playback must fail on the file, not by taking a tuner: %d %s", rec.Code, rec.Body.String())
	}
}

func TestProgressAndNewPassPadding(t *testing.T) {
	st := testStore(t)
	recID, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", Status: "complete", Path: filepath.Join(t.TempDir(), "clip.ts"), StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	req := httptest.NewRequest(http.MethodPut, "/api/recordings/"+strconv.FormatInt(recID, 10)+"/progress", bytes.NewBufferString(`{"position":12.5}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress %d %s", rec.Code, rec.Body.String())
	}
	got, err := st.Progress(context.Background(), recID)
	if err != nil || got != 12.5 {
		t.Fatalf("stored %v %v", got, err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/passes", bytes.NewBufferString(`{"title":"Jeopardy!","channelId":1}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"padBefore":1`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"padAfter":2`)) {
		t.Fatalf("new pass padding %d %s", rec.Code, rec.Body.String())
	}
}

func TestDiskReserveBlocksRecording(t *testing.T) {
	st := testStore(t)
	if err := st.PutSettings(context.Background(), map[string]string{"watermarkGB": "999999"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.ts")
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	recID, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", GuideNumber: "4.1", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	var asked uint64
	hub.MakeRoom = func(_ context.Context, need uint64) { asked = need }
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/recordings", bytes.NewBufferString(`{"channelId":1,"minutes":5,"title":"Nope"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInsufficientStorage || !bytes.Contains(rec.Body.Bytes(), []byte("in reserve")) {
		t.Fatalf("reserve %d %s", rec.Code, rec.Body.String())
	}
	if asked != 999999*1000*1000*1000 {
		t.Fatalf("make room asked for %d bytes before refusing", asked)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/recordings/"+strconv.FormatInt(recID, 10)+"/markers", bytes.NewBufferString(`{"start":1.5,"end":4}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("marker %d %s", rec.Code, rec.Body.String())
	}
	edl, err := os.ReadFile(strings.TrimSuffix(path, ".ts") + ".edl")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(edl, []byte("1.500 4.000 0")) {
		t.Fatalf("edl %s", edl)
	}

	res := get(t, h, "/api/storage")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"watermarkGB":999999`)) {
		t.Fatalf("storage %s", res.Body.String())
	}
}

func TestKeepForeverAndCleanUpSettings(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Finale", GuideNumber: "4.1", Status: "complete", Path: "/x/finale.ts", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	put := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := put("/api/recordings/"+strconv.FormatInt(id, 10)+"/keep", `{"keep":true}`); rec.Code != http.StatusOK {
		t.Fatalf("keep %d %s", rec.Code, rec.Body.String())
	}
	if res := get(t, h, "/api/recordings"); !bytes.Contains(res.Body.Bytes(), []byte(`"keep":true`)) {
		t.Fatalf("list %s", res.Body.String())
	}
	if rec := put("/api/recordings/"+strconv.FormatInt(id, 10)+"/keep", `{"keep":false}`); rec.Code != http.StatusOK {
		t.Fatalf("unkeep %d", rec.Code)
	}
	if res := get(t, h, "/api/recordings"); bytes.Contains(res.Body.Bytes(), []byte(`"keep"`)) {
		t.Fatalf("still kept %s", res.Body.String())
	}
	if rec := put("/api/recordings/999/keep", `{"keep":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing recording %d", rec.Code)
	}

	res := get(t, h, "/api/settings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"deleteWatchedDays":"0"`)) || !bytes.Contains(res.Body.Bytes(), []byte(`"makeRoom":"0"`)) ||
		!bytes.Contains(res.Body.Bytes(), []byte(`"folderLayout":"shows"`)) {
		t.Fatalf("defaults %s", res.Body.String())
	}
	if rec := put("/api/settings", `{"deleteWatchedDays":" 14","makeRoom":"1","folderLayout":"flat"}`); rec.Code != http.StatusOK {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	res = get(t, h, "/api/settings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"deleteWatchedDays":"14"`)) || !bytes.Contains(res.Body.Bytes(), []byte(`"makeRoom":"1"`)) ||
		!bytes.Contains(res.Body.Bytes(), []byte(`"folderLayout":"flat"`)) {
		t.Fatalf("saved %s", res.Body.String())
	}
	for _, body := range []string{`{"deleteWatchedDays":"-1"}`, `{"deleteWatchedDays":"week"}`, `{"deleteWatchedDays":"3651"}`, `{"makeRoom":"yes"}`, `{"folderLayout":"../x"}`} {
		if rec := put("/api/settings", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s saved with %d", body, rec.Code)
		}
	}
}

func TestOmittedRecordingFieldsAreRejected(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, store.Recording{
		Title: "Finale", GuideNumber: "4.1", Status: "complete",
		Path: filepath.Join(t.TempDir(), "finale.ts"), StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeep(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(ctx, id, 40); err != nil {
		t.Fatal(err)
	}
	if err := st.SetWatched(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	put := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/recordings/"+strconv.FormatInt(id, 10)+path, bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/keep", "/progress", "/watched"} {
		rec := put(path, `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if rec := put("/watched", ``); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty watched %d %s", rec.Code, rec.Body.String())
	}
	got, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Keep || got.Position != 40 || got.Watched != 1 {
		t.Fatalf("stored keep %v position %v watched %d", got.Keep, got.Position, got.Watched)
	}
	if rec := put("/keep", `{"keep":false}`); rec.Code != http.StatusOK {
		t.Fatalf("clear keep %d %s", rec.Code, rec.Body.String())
	}
	if rec := put("/progress", `{"position":0}`); rec.Code != http.StatusOK {
		t.Fatalf("rewind %d %s", rec.Code, rec.Body.String())
	}
	if rec := put("/watched", `{"watched":false}`); rec.Code != http.StatusOK {
		t.Fatalf("unwatched %d %s", rec.Code, rec.Body.String())
	}
	got, err = st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Keep || got.Position != 0 || got.Watched != 2 {
		t.Fatalf("explicit values keep %v position %v watched %d", got.Keep, got.Position, got.Watched)
	}
}

func TestDetectMissingFileIs404WithoutPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := testStore(t)
	missing := filepath.Join(dir, "recordings", "gone.ts")
	id, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Path: missing, Status: "complete", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/detect", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", rec.Code, rec.Body.Bytes())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(missing)) || bytes.Contains(rec.Body.Bytes(), []byte(dir)) {
		t.Fatalf("body leaked the path: %s", rec.Body.Bytes())
	}
}

func TestMoveARecording(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	hub := &live.Hub{Store: st, Dir: t.TempDir()}
	if err := os.MkdirAll(hub.Recordings(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hub.Recordings(), "20261005_193000_4.1_KBWV.ts")
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Harbor Watch", GuideNumber: "4.1", Status: "complete", Path: path, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	move := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/recordings/"+strconv.FormatInt(id, 10)+"/move", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := move(`{"name":"../escape"}`); rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("inside the recordings folder")) {
		t.Fatalf("escape %d %s", rec.Code, rec.Body.String())
	}
	if rec := move(`{"name":"Harbor Watch/S01E02"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"file":"Harbor Watch/S01E02.ts"`)) {
		t.Fatalf("move %d %s", rec.Code, rec.Body.String())
	}
	if res := get(t, h, "/api/recordings"); !bytes.Contains(res.Body.Bytes(), []byte(`"file":"Harbor Watch/S01E02.ts"`)) || bytes.Contains(res.Body.Bytes(), []byte(`"missing":true`)) {
		t.Fatalf("list %s", res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(hub.Recordings(), "Harbor Watch", "S01E02.ts")); err != nil {
		t.Fatal(err)
	}
	busy, err := st.CreateRecording(ctx, store.Recording{Title: "News", Status: "recording", Path: filepath.Join(hub.Recordings(), "news.ts"), StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/recordings/"+strconv.FormatInt(busy, 10)+"/move", bytes.NewBufferString(`{"name":"later"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("moved a recording in progress: %d", rec.Code)
	}
}

func TestARecordingOutsideItsFoldersIsNotOpened(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	secret := filepath.Join(t.TempDir(), "secret.ts")
	if err := os.WriteFile(secret, []byte{0x47}, 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Planted", GuideNumber: "4.1", Status: "complete", Path: secret, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVirtual(ctx, "9001", "Planted", []int64{id}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	ids := strconv.FormatInt(id, 10)
	// A poster made earlier would be served without reading the file.
	if err := os.MkdirAll(filepath.Join(dir, "posters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "posters", ids+".jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/recordings/" + ids + "/play"},
		{http.MethodGet, "/media/poster/" + ids},
		{http.MethodGet, "/api/recordings/" + ids + "/file"},
	} {
		req := httptest.NewRequest(c.method, c.path, bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	// A recording made before the recordings folder moved still opens.
	hub.RecordingsDir = filepath.Join(t.TempDir(), "moved")
	earlier := filepath.Join(dir, "recordings", "earlier.ts")
	if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(earlier, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := st.CreateRecording(ctx, store.Recording{Title: "Earlier", GuideNumber: "4.1", Status: "complete", Path: earlier, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if res := get(t, h, "/api/recordings/"+strconv.FormatInt(old, 10)+"/file"); res.Code != http.StatusOK {
		t.Fatalf("earlier recording: %d", res.Code)
	}
	// The HDHomeRun emulator would serve the file as is when there is no ffmpeg.
	hub.FFmpeg = ""
	emu := &emuHandler{store: st, hub: hub}
	rec := httptest.NewRecorder()
	emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil), "9001")
	if rec.Code != http.StatusNotFound || rec.Body.Len() > 20 {
		t.Fatalf("emulator: %d %q", rec.Code, rec.Body.String())
	}
}

func TestUnwritableRecordingsAreRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a mode 0555 directory")
	}
	st := testStore(t)
	dir := t.TempDir()
	recDir := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(recDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(recDir, 0o755) })
	if f, err := os.CreateTemp(recDir, ".probe-*"); err == nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		t.Skip("this user can still write a mode 0555 directory")
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/recordings", bytes.NewBufferString(`{"channelId":1,"minutes":5,"title":"Nope"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInsufficientStorage || !bytes.Contains(rec.Body.Bytes(), []byte("recordings folder")) {
		t.Fatalf("unwritable %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteRecordingRemovesTheFile(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	recDir := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(recDir, "clip.ts")
	if err := os.WriteFile(path, []byte("mpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", GuideNumber: "4.1", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(context.Background(), id, 12); err != nil {
		t.Fatal(err)
	}
	busyPath := filepath.Join(recDir, "busy.ts")
	if err := os.WriteFile(busyPath, []byte("busy"), 0o644); err != nil {
		t.Fatal(err)
	}
	busyID, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Busy", Status: "recording", Path: busyPath, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	res := get(t, h, "/api/recordings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"bytes":4`)) || !bytes.Contains(res.Body.Bytes(), []byte(`"position":12`)) {
		t.Fatalf("list %s", res.Body.String())
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/recordings/"+strconv.FormatInt(busyID, 10), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("busy delete %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(busyPath); err != nil {
		t.Fatal("in-progress file was removed")
	}
	req = httptest.NewRequest(http.MethodDelete, "/api/recordings/"+strconv.FormatInt(id, 10), nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
	res = get(t, h, "/api/recordings")
	if bytes.Contains(res.Body.Bytes(), []byte(`"title":"Clip"`)) {
		t.Fatalf("deleted recording still listed: %s", res.Body.String())
	}
}

func TestRecordingDurationRoundTrip(t *testing.T) {
	st := testStore(t)
	clip := filepath.Join(t.TempDir(), "clip.ts")
	if err := os.WriteFile(clip, make([]byte, 188), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", Status: "complete", Path: clip, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(context.Background(), id, 95); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	res := get(t, h, "/api/recordings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"durationSec":95`)) {
		t.Fatalf("duration %s", res.Body.String())
	}
}

func TestSetupSettings(t *testing.T) {
	st := testStore(t)
	if _, err := st.Identity(context.Background(), "Broadwave"); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/settings")
	var body map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["needsSetup"] != "1" {
		t.Fatalf("fresh needsSetup %q", body["needsSetup"])
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(`{"setupComplete":"1"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["setupComplete"] != "1" || body["needsSetup"] != "0" {
		t.Fatalf("after finish %+v", body)
	}
}

func TestGuideManualRateLimit(t *testing.T) {
	st := testStore(t)
	if err := st.SetManualGuidePull(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/guide/refresh", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("just refreshed")) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("retryAt")) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestGuideDelayFollowsStoredSchedule(t *testing.T) {
	st := testStore(t)
	next := time.Now().Add(5 * time.Hour)
	if err := st.SetGuideSchedule(context.Background(), time.Now(), next); err != nil {
		t.Fatal(err)
	}
	delay := (&Server{Store: st}).GuideDelay(time.Now())
	if delay < 4*time.Hour || delay > 5*time.Hour+time.Minute {
		t.Fatalf("delay %s", delay)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/diagnostics")
	if !bytes.Contains(res.Body.Bytes(), []byte("nextRefresh")) {
		t.Fatalf("diagnostics %s", res.Body.String())
	}
}

func TestScanUsesTheFakeTuner(t *testing.T) {
	var started bool
	tuner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lineup.post" && r.URL.Query().Get("scan") == "start" {
			started = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer tuner.Close()
	st := testStore(t)
	if err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "FAKE", FriendlyName: "Fake", BaseURL: tuner.URL, TunerCount: 1,
	}, nil); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, HDHR: &hdhr.Client{HTTP: tuner.Client()}}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/FAKE/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !started {
		t.Fatalf("%d %s started=%v", rec.Code, rec.Body.String(), started)
	}
}

func TestFinishedScanReadsTheNewLineup(t *testing.T) {
	var done atomic.Bool
	var tuner *httptest.Server
	tuner = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			fmt.Fprintf(w, `{"DeviceID":"FAKE","FriendlyName":"Fake","ModelNumber":"HDHR5-2US","TunerCount":2,"BaseURL":%q,"LineupURL":%q}`, tuner.URL, tuner.URL+"/lineup.json")
		case "/lineup.json":
			if !done.Load() {
				_, _ = w.Write([]byte(`[{"GuideNumber":"4.1","GuideName":"KBWV","URL":"http://x/v4.1"}]`))
				return
			}
			_, _ = w.Write([]byte(`[{"GuideNumber":"4.1","GuideName":"KBWV","URL":"http://x/v4.1"},{"GuideNumber":"9.1","GuideName":"KRVR","URL":"http://x/v9.1"}]`))
		case "/lineup.post":
			w.WriteHeader(http.StatusOK)
		case "/lineup_status.json":
			if !done.Load() {
				_, _ = w.Write([]byte(`{"ScanInProgress":1,"Progress":40,"Found":0}`))
				return
			}
			_, _ = w.Write([]byte(`{"ScanInProgress":0,"Progress":100,"Found":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer tuner.Close()
	st := testStore(t)
	if err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "FAKE", FriendlyName: "Fake", BaseURL: tuner.URL, TunerCount: 2,
	}, nil); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, HDHR: &hdhr.Client{HTTP: tuner.Client()}}).Handler()
	idle := get(t, h, "/api/v1/devices/FAKE/scan")
	if idle.Code != http.StatusOK {
		t.Fatalf("status before a scan %d", idle.Code)
	}
	if channels, _ := st.Channels(context.Background(), false); len(channels) != 0 {
		t.Fatalf("a status read before any scan synced %d channels", len(channels))
	}
	start := httptest.NewRecorder()
	h.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/v1/devices/FAKE/scan", nil))
	if start.Code != http.StatusOK {
		t.Fatalf("start %d %s", start.Code, start.Body.String())
	}
	res := get(t, h, "/api/v1/devices/FAKE/scan")
	if !strings.Contains(res.Body.String(), `"found":0`) || !strings.Contains(res.Body.String(), `"progress":40`) {
		t.Fatalf("running %s", res.Body.String())
	}
	done.Store(true)
	res = get(t, h, "/api/v1/devices/FAKE/scan")
	if !strings.Contains(res.Body.String(), `"scanning":false`) || !strings.Contains(res.Body.String(), `"found":2`) {
		t.Fatalf("finished %s", res.Body.String())
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 2 {
		t.Fatalf("lineup after scan %d %v", len(channels), err)
	}
}

func TestUploadedPlaylist(t *testing.T) {
	st := testStore(t)
	h := (&Server{Store: st}).Handler()
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Local News\nhttp://example/news.ts\n"))
	_ = zw.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("name", "Home")
	part, err := w.CreateFormFile("file", "home.m3u.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(gz.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 || channels[0].GuideName != "Local News" {
		t.Fatalf("%+v %v", channels, err)
	}
}

func TestRefreshKeepsTheChannel(t *testing.T) {
	body := "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Local News\nhttp://example/news.ts\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	st := testStore(t)
	api := &Server{Store: st}
	h := api.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(`{"kind":"m3u","name":"Home","url":"`+srv.URL+`/pl.m3u"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	id := channels[0].ID
	fav := true
	if _, err := st.PatchChannel(context.Background(), id, store.ChannelPatch{Favorite: &fav}); err != nil {
		t.Fatal(err)
	}
	body = "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",News Tonight\nhttp://example/news2.ts\n"
	sources, err := st.Sources(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatal(err, len(sources))
	}
	if err := st.NoteRefresh(context.Background(), sources[0].ID, time.Now().Add(-time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	n, err := api.RefreshSources(context.Background(), time.Now())
	if err != nil || n != 1 {
		t.Fatalf("refreshed %d: %v", n, err)
	}
	channels, err = st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 || channels[0].ID != id || channels[0].GuideName != "News Tonight" || !channels[0].Favorite {
		t.Fatalf("%+v %v", channels, err)
	}
}

func TestSourceGoesOfflineAndComesBack(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			http.Error(w, "gone", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",News\nhttp://example/a.ts\n"))
	}))
	defer srv.Close()
	st := testStore(t)
	api := &Server{Store: st}
	h := api.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(`{"kind":"m3u","name":"News","url":"`+srv.URL+`/pl.m3u"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	sources, err := st.Sources(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatal(err)
	}
	if err := st.NoteRefresh(context.Background(), sources[0].ID, time.Now().Add(-time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	up = false
	if _, err := api.RefreshSources(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := api.RefreshSources(context.Background(), time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	events, err := st.Events(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	offline := 0
	for _, ev := range events {
		if ev.Kind == "source" && ev.Message == "News is offline." {
			offline++
		}
	}
	if offline != 1 {
		t.Fatalf("offline events %d in %+v", offline, events)
	}
	up = true
	if n, err := api.RefreshSources(context.Background(), time.Now().Add(3*time.Hour)); err != nil || n != 1 {
		t.Fatalf("back %d %v", n, err)
	}
	events, _ = st.Events(context.Background(), 10)
	back := false
	for _, ev := range events {
		if ev.Message == "News is back." {
			back = true
		}
	}
	if !back {
		t.Fatalf("no return event in %+v", events)
	}
	sources, _ = st.Sources(context.Background())
	if sources[0].Health != "" || sources[0].LastRefresh == "" {
		t.Fatalf("%+v", sources[0])
	}
}

func TestShellStaysFresh(t *testing.T) {
	html := []byte("<!doctype html><title>Broadwave</title>")
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(html); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: html},
		"index.html.gz":        &fstest.MapFile{Data: append([]byte(nil), buf.Bytes()...)},
		"assets/app.js":        &fstest.MapFile{Data: []byte("console.log(1)\n")},
		"manifest.webmanifest": &fstest.MapFile{Data: []byte(`{"name":"Broadwave"}`)},
		"sw.js":                &fstest.MapFile{Data: []byte("self.addEventListener('fetch',()=>{})\n")},
	}
	h := (&Server{Store: testStore(t), Assets: assets}).Handler()

	index := get(t, h, "/")
	if got := index.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index cache %q", got)
	}
	named := get(t, h, "/index.html")
	if got := named.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index.html cache %q", got)
	}
	guide := get(t, h, "/guide")
	if got := guide.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("guide cache %q", got)
	}
	if !bytes.Contains(guide.Body.Bytes(), []byte("Broadwave")) {
		t.Fatalf("guide body %s", guide.Body.Bytes())
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("gzip index %d %s", rec.Code, rec.Body.Bytes())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("gzip index cache %q", got)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip encoding %q", rec.Header().Get("Content-Encoding"))
	}

	js := get(t, h, "/assets/app.js")
	if got := js.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache %q", got)
	}

	manifest := get(t, h, "/manifest.webmanifest")
	if got := manifest.Header().Get("Content-Type"); !strings.Contains(got, "application/manifest+json") {
		t.Fatalf("manifest type %q", got)
	}
	if got := manifest.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("manifest cache %q", got)
	}

	worker := get(t, h, "/sw.js")
	if got := worker.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("worker cache %q", got)
	}
	if ct := worker.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("worker type %q", ct)
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func TestPatchChannelTwinChoice(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "14.2", GuideName: "Snapshp", VideoCodec: "H264", AudioCodec: "AC3", StreamURL: "http://flex:5004/auto/v14.2"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
	}); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	chs, _ := st.Channels(ctx, false)
	for _, ch := range chs {
		ids[ch.GuideNumber] = ch.ID
	}
	h := (&Server{Store: st}).Handler()
	patch := func(id int64, body string) (int, store.Channel) {
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/channels/%d", id), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var ch store.Channel
		_ = json.Unmarshal(rec.Body.Bytes(), &ch)
		return rec.Code, ch
	}
	code, ch := patch(ids["4.1"], `{"twinChoice":"both"}`)
	if code != http.StatusOK || ch.ID != ids["4.1"] || ch.Hidden || ch.TwinID != ids["104.1"] || ch.TwinChoice != "both" {
		t.Fatalf("%d %+v", code, ch)
	}
	if code, ch = patch(ids["104.1"], `{"twinChoice":"atsc1","favorite":true}`); code != http.StatusOK || !ch.Hidden || !ch.Favorite || ch.Standard != "atsc3" {
		t.Fatalf("%d %+v", code, ch)
	}
	if code, _ = patch(ids["14.2"], `{"twinChoice":"atsc3"}`); code != http.StatusConflict {
		t.Fatalf("a channel with no twin answered %d", code)
	}
	if code, _ = patch(ids["4.1"], `{"twinChoice":"sometimes"}`); code != http.StatusBadRequest {
		t.Fatalf("an unknown choice answered %d", code)
	}
}
