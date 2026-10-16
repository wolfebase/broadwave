package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"broadwave/internal/realtime"
	"broadwave/internal/store"
)

func TestDataChanges(t *testing.T) {
	was := store.Versions{Guide: 1, Channels: 1, Recordings: 1, Schedule: 1, Settings: 1}
	if got := dataChanges(was, was); len(got) != 0 {
		t.Fatalf("no change sent %v", got)
	}
	now := was
	now.Guide, now.Recordings = 4, 2
	got := dataChanges(was, now)
	if len(got) != 2 || got[0] != (dataChange{"guide.changed", 4}) || got[1] != (dataChange{"recordings.changed", 2}) {
		t.Fatalf("%v", got)
	}
}

// A write anywhere reaches the socket as one event with the new version.
func TestWatchDataPublishesAChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st := testStore(t)
	bus := realtime.NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if _, _, err := c.Read(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Bus: bus}
	watch, stop := context.WithCancel(ctx)
	defer stop()
	go s.WatchData(watch, 20*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if err := st.AddPass(ctx, "News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Type string
			Data struct{ Version int64 }
		}
		_ = json.Unmarshal(raw, &m)
		if m.Type == "schedule.changed" && m.Data.Version > 1 {
			return
		}
		if strings.HasSuffix(m.Type, ".changed") && m.Type != "schedule.changed" {
			t.Fatalf("unexpected %s", raw)
		}
	}
}
