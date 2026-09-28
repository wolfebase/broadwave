package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/realtime"
)

func TestWaitServableReturnsWhenTheViewerLeaves(t *testing.T) {
	h := &live.Hub{Dir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	waitServable(ctx, h, 1, "h264-720-aac", 12*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled wait took %s", elapsed)
	}
}

// watchInFlight starts a watch whose picture never becomes servable, so the
// request holds its counted viewer until cancel.
func watchInFlight(t *testing.T, api *Server) (hub *live.Hub, id int64, key string, cancel context.CancelFunc, done chan struct{}) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	// exec replaces the shell so Shutdown's kill reaches the sleeper, not a child
	// that would keep the test's stderr open.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "HLS1", FriendlyName: "HLS", ModelNumber: "HDHR4-2US",
		BaseURL: "http://127.0.0.1:9", TunerCount: 0,
	}, []hdhr.Channel{{
		GuideNumber: "4.1", GuideName: "KBWV", VideoCodec: "H264", AudioCodec: "AAC",
		StreamURL: "http://127.0.0.1/live.m3u8",
	}}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("channels: %v %v", channels, err)
	}
	id = channels[0].ID
	hub = &live.Hub{
		Store: st, Dir: filepath.Join(dir, "hub"), FFmpeg: script,
		Encoder: "libx264", RenditionIdle: time.Hour,
	}
	t.Cleanup(hub.Shutdown)
	api.Store, api.Hub = st, hub

	reqCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch", strings.NewReader(fmt.Sprintf(`{"channelId":%d}`, id)))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	done = make(chan struct{})
	go func() {
		defer close(done)
		api.watch(rec, req)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(filepath.Join(hub.Dir, "live", strconv.FormatInt(id, 10)))
		if len(entries) > 0 {
			key = entries[0].Name()
			if sess, ok := hub.Session(id, key); ok && sess.Viewers > 0 {
				break
			}
		}
		select {
		case <-done:
			t.Fatalf("watch ended before a viewer was counted: %d %s", rec.Code, rec.Body.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	sess, ok := hub.Session(id, key)
	if !ok || sess.Viewers == 0 {
		t.Fatalf("watch did not take a viewer (key %q)", key)
	}
	return hub, id, key, cancel, done
}

func TestWatchDropsTheViewerWhenTheClientLeaves(t *testing.T) {
	api := &Server{}
	hub, id, key, cancel, done := watchInFlight(t, api)

	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled watch did not return")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancel took %s", elapsed)
	}
	if sess, ok := hub.Session(id, key); ok && sess.Viewers != 0 {
		t.Fatalf("viewer still held: %d", sess.Viewers)
	}
}

func TestStopFromAnotherServerProcessKeepsTheViewer(t *testing.T) {
	api := &Server{Bus: realtime.NewBus()}
	hub, id, key, cancel, done := watchInFlight(t, api)
	defer func() {
		cancel()
		<-done
	}()
	stop := func(boot string) {
		body := fmt.Sprintf(`{"rendition":%q,"boot":%q}`, key, boot)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/watch/"+strconv.FormatInt(id, 10)+"/stop", strings.NewReader(body))
		req.SetPathValue("id", strconv.FormatInt(id, 10))
		rec := httptest.NewRecorder()
		api.release(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("stop: %d %s", rec.Code, rec.Body.String())
		}
	}
	viewers := func() int {
		sess, _ := hub.Session(id, key)
		return sess.Viewers
	}
	stop("0123456789abcdef")
	if n := viewers(); n != 1 {
		t.Fatalf("a stop from an earlier process took a viewer: %d left", n)
	}
	stop(api.Bus.Boot)
	if n := viewers(); n != 0 {
		t.Fatalf("a stop from this process kept the viewer: %d left", n)
	}
}
