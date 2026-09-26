package live

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/store"
)

type safeBuf struct {
	mu sync.Mutex
	b  []byte
}

func (w *safeBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.b = append(w.b, p...)
	w.mu.Unlock()
	return len(p), nil
}

func (w *safeBuf) Close() error { return nil }

func (w *safeBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.b)
}

type nopWriter struct{ closed bool }

func (w *nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *nopWriter) Close() error                { w.closed = true; return nil }

type fakeBody struct{}

func (fakeBody) Read([]byte) (int, error) { select {} }
func (fakeBody) Close() error             { return nil }

func testHub(t *testing.T) (*Hub, *mux) {
	t.Helper()
	h := &Hub{Dir: t.TempDir(), RenditionIdle: time.Hour, channels: map[int64]*feed{}, muxes: map[int]*mux{}, reserved: map[int]bool{}}
	m := &mux{freq: 575000000, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}, body: fakeBody{}}
	h.muxes[m.freq] = m
	return h, m
}

func addTestFeed(h *Hub, m *mux, id int64, guide string) *feed {
	return h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: id, GuideNumber: guide}, FieldOrder: "progressive"})
}

func addTestRendition(h *Hub, f *feed, key string, viewers int, seen time.Time) *rendition {
	spec, _ := ParseRenditionKey(key)
	w := &nopWriter{}
	r := &rendition{spec: spec, dir: h.Dir + "/" + key, stdin: w, viewers: viewers, seen: seen}
	r.sub = h.attachPipeLocked(muxOf(h, f), w)
	f.renditions[key] = r
	return r
}

func TestStoredOrderDoesNotWaitToStart(t *testing.T) {
	h, m := testHub(t)
	started := time.Now()
	f := h.addFeedLocked(m, store.SourceChannel{
		Channel:    store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"},
		FieldOrder: "tt",
	})
	if waited := time.Since(started); waited > 150*time.Millisecond {
		t.Fatalf("stored scan held the picture for %s", waited)
	}
	if !f.source.Lace || f.source.Progressive {
		t.Fatalf("graph %+v", f.source)
	}
	if len(m.snapshot()) != 0 {
		t.Fatal("the deferred scan must not take a pipe")
	}
}

func TestDeferredScanLearnsTheMainAudio(t *testing.T) {
	h, m := testHub(t)
	m.tuner = 0
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	started := time.Now()
	f := h.addFeedLocked(m, store.SourceChannel{
		Channel:     store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"},
		FieldOrder:  "tt",
		ProgramNum:  1,
		FrequencyHz: m.freq,
	})
	if waited := time.Since(started); waited > 150*time.Millisecond {
		t.Fatalf("stored scan held the picture for %s", waited)
	}
	go writeUntil(ctx, pw, audioTS(1, []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}))
	deadline := time.Now().Add(3 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		h.mu.Lock()
		if len(f.tracks) > 0 {
			pid = f.tracks[0].PID
		}
		h.mu.Unlock()
		if pid != 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid != 0x101 {
		t.Fatalf("main pid %d", pid)
	}
}

func TestRenditionReadsBytesFromBeforeItAttached(t *testing.T) {
	h, m := testHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	if _, err := pw.Write([]byte("HELLO")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		m.pipeMu.Lock()
		n := len(m.lead)
		m.pipeMu.Unlock()
		if n >= len("HELLO") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	w := &safeBuf{}
	sub := h.attachPipe(m, w, true)
	defer sub.stop()
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(w.String(), "HELLO") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(w.String(), "HELLO") {
		t.Fatalf("rendition missed the opening bytes, saw %q", w.String())
	}
	late := &safeBuf{}
	sub2 := h.attachPipe(m, late, true)
	defer sub2.stop()
	time.Sleep(30 * time.Millisecond)
	if late.String() != "" {
		t.Fatalf("a second subscriber replayed %q", late.String())
	}
}

func TestLeadClosesWhenItIsStale(t *testing.T) {
	m := &mux{}
	m.leadSince = time.Now().Add(-3 * time.Second)
	m.lead = []byte("old")
	m.rememberLead([]byte("new"))
	if !m.leadDone || m.lead != nil {
		t.Fatalf("stale lead done=%v bytes=%q", m.leadDone, m.lead)
	}
	m.rememberLead([]byte("later"))
	if m.lead != nil {
		t.Fatalf("closed lead kept %q", m.lead)
	}
}

func TestRecordingWarnsBeforeTheTileStops(t *testing.T) {
	h, m1 := testHub(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h.Store = st
	m1.tuner = 0
	m1.host = "127.0.0.1:1"
	addTestFeed(h, m1, 1, "4.1")
	m2 := &mux{freq: 575000001, tuner: 1, host: "127.0.0.1:1", feeds: map[string]*feed{}, cancel: func() {}, body: fakeBody{}}
	h.muxes[m2.freq] = m2
	addTestFeed(h, m2, 5, "9.1")
	m3 := &mux{freq: 533000000, tuner: 2, host: "127.0.0.1:1", feeds: map[string]*feed{}, cancel: func() {}, body: fakeBody{}}
	h.muxes[m3.freq] = m3
	recorded := addTestFeed(h, m3, 3, "5.1")
	recorded.recording = &recording{id: 9}

	at := time.Date(2026, 9, 25, 15, 0, 0, 0, time.Local)
	labels, feeds := h.viewerFeedsToPreemptLocked(599000000, 8)
	if _, ok := h.channels[5]; !ok {
		t.Fatal("the warning ran after the tile was already gone")
	}
	if len(labels) != 1 || labels[0] != "9.1" || len(feeds) != 1 || feeds[0].channel.ID != 5 {
		t.Fatalf("labels %v feeds %d", labels, len(feeds))
	}
	warning := StopWarning(labels, at, "Jeopardy", at)
	if warning != "9.1 stops at 3:00 PM. Jeopardy is recording." {
		t.Fatal(warning)
	}
	if err := h.Store.AddEvent(context.Background(), "recording", warning); err != nil {
		t.Fatal(err)
	}
	for _, f := range feeds {
		h.stopFeedLocked(f)
	}
	if _, ok := h.channels[5]; ok {
		t.Fatal("9.1 should stop after the warning")
	}
	if _, ok := h.channels[1]; !ok {
		t.Fatal("4.1 stays on")
	}
	if _, ok := h.channels[3]; !ok || recorded.recording == nil {
		t.Fatal("the recording keeps its tuner")
	}
	events, err := st.Events(context.Background(), 5)
	if err != nil || len(events) != 1 || events[0].Message != warning {
		t.Fatalf("%+v %v", events, err)
	}
}

func TestSecondDropLeavesTheReplacementTune(t *testing.T) {
	h, oldMux := testHub(t)
	old := addTestFeed(h, oldMux, 1, "4.1")
	cancelled := false
	freq := oldMux.freq
	replacement := &mux{
		freq: freq, tuner: 0, host: "127.0.0.1:1",
		feeds:  map[string]*feed{},
		cancel: func() { cancelled = true },
		body:   fakeBody{},
	}
	h.muxes[freq] = replacement
	recorded := addTestFeed(h, replacement, 1, "4.1")
	recorded.recording = &recording{id: 4}

	h.stopFeedLocked(old)

	if h.muxes[freq] != replacement {
		t.Fatal("the replacement tune was released")
	}
	if replacement.feeds["4.1"] != recorded || recorded.recording == nil {
		t.Fatal("the recording feed was removed from the mux")
	}
	if h.channels[1] != recorded {
		t.Fatal("the channel no longer points at the recording")
	}
	if cancelled {
		t.Fatal("the replacement mux was cancelled")
	}
}

func TestPlaylistStreamCountIsPerSource(t *testing.T) {
	h, _ := testHub(t)
	h.channels[1] = &feed{channel: store.SourceChannel{Channel: store.Channel{DeviceID: "src-1"}}}
	h.channels[2] = &feed{channel: store.SourceChannel{Channel: store.Channel{DeviceID: "src-1"}}}
	h.channels[3] = &feed{channel: store.SourceChannel{Channel: store.Channel{DeviceID: "src-2"}}}
	if h.streamsForDeviceLocked("src-1") != 2 || h.streamsForDeviceLocked("src-2") != 1 {
		t.Fatalf("src-1 %d src-2 %d", h.streamsForDeviceLocked("src-1"), h.streamsForDeviceLocked("src-2"))
	}
	full := store.SourceChannel{Channel: store.Channel{DeviceID: "src-1"}, StreamURL: "http://example/a.ts", StreamLimit: 1}
	if !streamBusy(full, 1) {
		t.Fatal("a playlist at its limit is busy")
	}
	if streamBusy(full, 0) {
		t.Fatal("a playlist with room is not busy")
	}
	if StreamLimitMessage(2) != "All 2 streams from this playlist are in use. Stop one or raise the limit." {
		t.Fatal(StreamLimitMessage(2))
	}
}

func TestReleaseAbandonedFreesQuietViewersButKeepsRecordings(t *testing.T) {
	h, m := testHub(t)
	quiet := addTestFeed(h, m, 1, "4.1")
	addTestRendition(h, quiet, "copy.aac2", 1, time.Now().Add(-time.Minute))
	recorded := addTestFeed(h, m, 2, "4.2")
	addTestRendition(h, recorded, "copy.aac2", 1, time.Now().Add(-time.Minute))
	recorded.recording = &recording{id: 9}

	h.ReleaseAbandoned(30 * time.Second)
	if _, ok := h.channels[1]; ok {
		t.Fatal("a viewer that stopped requesting video should release the channel")
	}
	if _, ok := h.channels[2]; !ok {
		t.Fatal("a recording keeps the channel tuned")
	}
	if len(recorded.renditions) != 0 {
		t.Fatal("the abandoned rendition on the recorded channel should stop")
	}
}

func TestOneViewerLeavingDoesNotStopAnotherRendition(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "9.1")
	phone := addTestRendition(h, f, "720.aac2.broadcast", 1, time.Now())
	tv := addTestRendition(h, f, "copy.copy", 1, time.Now())

	h.Release(1, "720.aac2.broadcast")
	h.idleStop(1, "720.aac2.broadcast")

	if _, ok := f.renditions["720.aac2.broadcast"]; ok {
		t.Fatal("the idle rendition should stop")
	}
	if f.renditions["copy.copy"] != tv || tv.viewers != 1 {
		t.Fatal("the Apple TV rendition must keep running untouched")
	}
	if got := len(m.snapshot()); got != 1 {
		t.Fatalf("mux should feed only the remaining rendition, got %d pipes", got)
	}
	_ = phone
}

func TestLastViewerReleasesTheTune(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "5.1")
	addTestRendition(h, f, "copy.aac2", 1, time.Now())
	h.Release(1, "")
	h.idleStop(1, "copy.aac2")
	if len(h.channels) != 0 || len(h.muxes) != 0 {
		t.Fatalf("with nobody watching or recording, the tune should be released: %d channels, %d muxes", len(h.channels), len(h.muxes))
	}
}

func TestStoppedSubchannelLeavesMuxFanout(t *testing.T) {
	h, m := testHub(t)
	a := addTestFeed(h, m, 1, "14.1")
	addTestRendition(h, a, "copy.aac2", 1, time.Now())
	b := addTestFeed(h, m, 2, "14.2")
	keep := addTestRendition(h, b, "copy.aac2", 1, time.Now())

	h.stopFeedLocked(a)
	pipes := m.snapshot()
	if len(pipes) != 1 || pipes[0] != keep.sub {
		t.Fatalf("mux should fan out only to the subchannel still watched, got %d pipes", len(pipes))
	}
	if _, ok := h.muxes[m.freq]; !ok {
		t.Fatal("the frequency stays tuned while 14.2 is watched")
	}
}

type endBody struct{}

func (endBody) Read([]byte) (int, error) { return 0, io.EOF }
func (endBody) Close() error             { return nil }

func TestTunerReadEndingReleasesTheMux(t *testing.T) {
	h, m := testHub(t)
	m.body = endBody{}
	addTestFeed(h, m, 1, "4.1")
	h.readLoop(context.Background(), m)
	if _, ok := h.muxes[m.freq]; ok {
		t.Fatal("a tuner that stopped should be released")
	}
	if _, ok := h.channels[1]; ok {
		t.Fatal("the channel should leave with the tuner")
	}
}

func TestRecordingThatCannotStartIsMarkedFailed(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := &Hub{Dir: dir, Store: st, RenditionIdle: time.Hour, channels: map[int64]*feed{}, muxes: map[int]*mux{}, reserved: map[int]bool{}}
	m := &mux{freq: 1, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}, body: fakeBody{}}
	h.muxes[m.freq] = m
	f := addTestFeed(h, m, 1, "4.1")
	ends := time.Now().Add(time.Hour)
	id, err := st.CreateRecording(context.Background(), store.Recording{
		ChannelID: 1, GuideNumber: "4.1", Title: "Jeopardy!", Path: dir + "/j.ts",
		Status: "recording", StartedAt: time.Now(), EndsAt: &ends,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.abortRecordingLocked(context.Background(), f, id)
	got, err := st.Recording(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Error != "Could not start the recording." {
		t.Fatalf("status %s error %q", got.Status, got.Error)
	}
	if _, ok := h.channels[1]; ok {
		t.Fatal("a recording that did not start should release the tuner")
	}
}

func TestFieldOrderFromProbe(t *testing.T) {
	raw := []byte(`{"programs":[{"program_num":3,"streams":[{"codec_type":"video","field_order":"tt"}]},{"program_num":4,"streams":[{"codec_type":"audio"},{"codec_type":"video","field_order":"progressive"}]}]}`)
	if got := fieldOrderFrom(raw, 4); got != "progressive" {
		t.Fatalf("program 4: %q", got)
	}
	if got := fieldOrderFrom(raw, 3); got != "tt" {
		t.Fatalf("program 3: %q", got)
	}
	if got := fieldOrderFrom(raw, 7); got != "" {
		t.Fatalf("missing program should be unknown: %q", got)
	}
}
