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
	hub, id, key, cancel, done, _ = watchStarting(t, api)
	return hub, id, key, cancel, done
}

// watchStarting is watchInFlight with the response the watch writes.
func watchStarting(t *testing.T, api *Server) (hub *live.Hub, id int64, key string, cancel context.CancelFunc, done chan struct{}, rec *httptest.ResponseRecorder) {
	t.Helper()
	return watchStartingWith(t, api, "")
}

// watchStartingWith is watchStarting with a ticket on the watch request.
func watchStartingWith(t *testing.T, api *Server, ticket string) (hub *live.Hub, id int64, key string, cancel context.CancelFunc, done chan struct{}, rec *httptest.ResponseRecorder) {
	t.Helper()
	hub, id = watchHub(t, api)
	reqCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch", strings.NewReader(fmt.Sprintf(`{"channelId":%d,"ticket":%q}`, id, ticket)))
	req = req.WithContext(reqCtx)
	rec = httptest.NewRecorder()
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
	return hub, id, key, cancel, done, rec
}

// watchHub is a hub with one channel whose picture never becomes servable.
func watchHub(t *testing.T, api *Server) (hub *live.Hub, id int64) {
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
	return hub, id
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

// A picture that never writes a segment used to answer 200 with an empty
// playlist after 12 s, and the player sat on black for its own timeout.
// The watch now says it did not start.
func TestAWatchThatNeverStartsSaysSo(t *testing.T) {
	prev := servableWait
	servableWait = 800 * time.Millisecond
	t.Cleanup(func() { servableWait = prev })
	api := &Server{}
	hub, id, key, cancel, done, rec := watchStarting(t, api)
	defer cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watch did not give up")
	}
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"start_failed"`) {
		t.Fatalf("watch answered %d %s", rec.Code, rec.Body.String())
	}
	if sess, ok := hub.Session(id, key); ok && sess.Viewers != 0 {
		t.Fatalf("the failed watch kept its viewer: %d", sess.Viewers)
	}
}

func stopByTicket(t *testing.T, api *Server, id int64, ticket string) {
	t.Helper()
	body := fmt.Sprintf(`{"ticket":%q}`, ticket)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch/"+strconv.FormatInt(id, 10)+"/stop", strings.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	api.release(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body.String())
	}
}

// A page that leaves before it reads the watch answer does not know the
// rendition, and the server may already count it. Its stop names the request
// instead, and that viewer goes at once, not after the abandoned-viewer sweep.
func TestStopByTicketReleasesTheViewerTheWatchCounted(t *testing.T) {
	api := &Server{}
	hub, id, key, cancel, done, _ := watchStartingWith(t, api, "page-1")
	stopByTicket(t, api, id, "page-1")
	if sess, _ := hub.Session(id, key); sess.Viewers != 0 {
		t.Fatalf("the stop by ticket kept the viewer: %d", sess.Viewers)
	}
	// The request then ends as cancelled. That must not take a second viewer.
	spec, ok := live.ParseRenditionKey(key)
	if !ok {
		t.Fatalf("key %q", key)
	}
	other, err := hub.Watch(context.Background(), id, spec, false)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	if sess, _ := hub.Session(id, other.Rendition); sess.Viewers != 1 {
		t.Fatalf("the cancelled watch released another viewer: %d left", sess.Viewers)
	}
	// A second stop by the same ticket takes nothing either.
	stopByTicket(t, api, id, "page-1")
	if sess, _ := hub.Session(id, other.Rendition); sess.Viewers != 1 {
		t.Fatalf("a repeated stop took another viewer: %d left", sess.Viewers)
	}
}

// The stop can arrive before the watch counts its viewer.
func TestStopByTicketBeforeTheWatchCountsReleasesItThen(t *testing.T) {
	api := &Server{}
	hub, id := watchHub(t, api)
	stopByTicket(t, api, id, "page-2")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch", strings.NewReader(fmt.Sprintf(`{"channelId":%d,"ticket":"page-2"}`, id)))
	rec := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		api.watch(rec, req)
	}()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("a watch stopped before it counted kept waiting for its picture")
	}
	entries, _ := os.ReadDir(filepath.Join(hub.Dir, "live", strconv.FormatInt(id, 10)))
	for _, e := range entries {
		if sess, ok := hub.Session(id, e.Name()); ok && sess.Viewers != 0 {
			t.Fatalf("%s kept %d viewer(s)", e.Name(), sess.Viewers)
		}
	}
}

// A stop by ticket on another channel takes nothing.
func TestStopByTicketNamesItsChannel(t *testing.T) {
	api := &Server{}
	hub, id, key, cancel, done, _ := watchStartingWith(t, api, "page-3")
	defer func() {
		cancel()
		<-done
	}()
	stopByTicket(t, api, id+1, "page-3")
	if sess, _ := hub.Session(id, key); sess.Viewers != 1 {
		t.Fatalf("a stop for another channel took the viewer: %d left", sess.Viewers)
	}
	stopByTicket(t, api, id, "page-3")
	if sess, _ := hub.Session(id, key); sess.Viewers != 0 {
		t.Fatalf("the stop kept the viewer: %d", sess.Viewers)
	}
}
