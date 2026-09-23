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
