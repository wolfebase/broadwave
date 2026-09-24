package live

import (
	"context"
	"io"
	"testing"
	"time"

	"waveguide/internal/store"
)

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
