package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"waveguide/internal/hdhr"
	"waveguide/internal/live"
	"waveguide/internal/store"
)

func TestGuideAndFavorite(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "DUO", ModelNumber: "HDHR5-2US",
		BaseURL: "http://192.168.1.252", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "14.2", GuideName: "Snapshp", VideoCodec: "H264", AudioCodec: "AC3"},
		{GuideNumber: "4.1", GuideName: "WDAF-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, Favorite: true, StreamURL: "http://192.168.1.252:5004/auto/v4.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>Waveguide</title>")},
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
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("Waveguide")) {
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

	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()
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
	h = (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()
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
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()
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
	recID, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", GuideNumber: "4.1", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/recordings", bytes.NewBufferString(`{"channelId":1,"minutes":5,"title":"Nope"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInsufficientStorage || !bytes.Contains(rec.Body.Bytes(), []byte("reserve")) {
		t.Fatalf("reserve %d %s", rec.Code, rec.Body.String())
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
	h := (&Server{Store: st, Hub: hub, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()
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
	id, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Clip", Status: "complete", Path: filepath.Join(t.TempDir(), "clip.ts"), StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(context.Background(), id, 95); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Waveguide")}}}).Handler()
	res := get(t, h, "/api/recordings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"durationSec":95`)) {
		t.Fatalf("duration %s", res.Body.String())
	}
}

func TestSetupSettings(t *testing.T) {
	st := testStore(t)
	if _, err := st.Identity(context.Background(), "Waveguide"); err != nil {
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
