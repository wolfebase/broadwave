package live

import (
	"testing"
	"time"

	"ota-viewer/internal/store"
)

func TestReleaseAbandonedDropsQuietViewers(t *testing.T) {
	h := &Hub{channels: map[int64]*feed{}}
	h.channels[1] = &feed{
		channel: store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}},
		viewers: 1,
		seen:    time.Now().Add(-time.Minute),
	}
	h.channels[2] = &feed{
		channel:   store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "5.1"}},
		viewers:   1,
		seen:      time.Now().Add(-time.Minute),
		recording: &recording{id: 9},
	}
	h.ReleaseAbandoned(30 * time.Second)
	if _, ok := h.channels[1]; ok {
		t.Fatal("a viewer that stopped requesting video should release the tuner")
	}
	if _, ok := h.channels[2]; !ok {
		t.Fatal("a recording keeps the tuner")
	}
}

type nopWriter struct{ closed bool }

func (w *nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *nopWriter) Close() error                { w.closed = true; return nil }

func TestStoppedFeedLeavesMuxFanout(t *testing.T) {
	h := &Hub{channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	m := &mux{freq: 575000000, feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	a := h.attachPipeLocked(m, &nopWriter{})
	b := h.attachPipeLocked(m, &nopWriter{})
	fa := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "14.1"}, FrequencyHz: m.freq}, sub: a}
	fb := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "14.2"}, FrequencyHz: m.freq}, sub: b}
	m.feeds["14.1"], m.feeds["14.2"] = fa, fb
	h.channels[1], h.channels[2] = fa, fb

	h.stopFeedLocked(fa)
	if got := len(m.snapshot()); got != 1 {
		t.Fatalf("mux should fan out to 1 pipe after one subchannel stops, got %d", got)
	}
	if m.snapshot()[0] != b {
		t.Fatal("the remaining pipe should belong to the subchannel still watched")
	}
}
