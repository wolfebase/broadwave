package realtime

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func fixedRooms(at time.Time) (*Rooms, *time.Time) {
	now := at
	r := NewRooms()
	r.now = func() time.Time { return now }
	return r, &now
}

func TestFollowRoomTracksLiveAndRefusesControls(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	st := r.Join("channel:4", 4)
	if st.Mode != "follow" || st.Members != 1 {
		t.Fatalf("channel rooms follow live: %+v", st)
	}
	want := unixMS(start.Add(-10 * time.Second))
	if got := st.Target(unixMS(start)); got != want {
		t.Fatalf("target should sit 10s behind real time, got %v want %v", got, want)
	}
	*now = start.Add(time.Minute)
	st, _ = r.State("channel:4")
	if got := st.Target(unixMS(*now)); got != want+60000 {
		t.Fatalf("a playing room advances in real time, got %v", got)
	}
	if _, err := r.Apply("channel:4", Command{Action: "pause"}); err != ErrFollowRoom {
		t.Fatalf("follow rooms refuse pause, got %v", err)
	}
}

func TestGroupRoomPauseSeekLive(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	r.Join("group:den", 9)
	r.Join("group:den", 9)
	*now = start.Add(30 * time.Second)
	st, err := r.Apply("group:den", Command{Action: "pause"})
	if err != nil || st.Rate != 0 || st.Members != 2 {
		t.Fatalf("pause: %+v %v", st, err)
	}
	paused := st.Target(unixMS(*now))
	*now = start.Add(90 * time.Second)
	st, _ = r.State("group:den")
	if st.Target(unixMS(*now)) != paused {
		t.Fatal("a paused room holds its frame")
	}
	st, _ = r.Apply("group:den", Command{Action: "play"})
	*now = start.Add(100 * time.Second)
	if got := st.Target(unixMS(*now)); math.Abs(got-(paused+10000)) > 0.001 {
		t.Fatalf("play resumes from the paused frame, got %v want %v", got, paused+10000)
	}
	st, _ = r.Apply("group:den", Command{Action: "seek", MediaTime: unixMS(*now) + 60000})
	if st.AnchorMedia > unixMS(now.Add(-6*time.Second)) {
		t.Fatal("seeking past the live edge is clamped")
	}
	st, _ = r.Apply("group:den", Command{Action: "live"})
	if st.AnchorMedia != unixMS(now.Add(-10*time.Second)) || st.Rate != 1 {
		t.Fatalf("live jumps to the latency target: %+v", st)
	}
	r.Leave("group:den")
	r.Leave("group:den")
	if _, ok := r.State("group:den"); ok {
		t.Fatal("empty rooms are forgotten")
	}
}

func TestSocketClockAndRoomBroadcast(t *testing.T) {
	bus := NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	read := func(c *websocket.Conn, kind string) json.RawMessage {
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for %s: %v", kind, err)
			}
			var m Message
			_ = json.Unmarshal(raw, &m)
			if m.Type == kind {
				return m.Data
			}
		}
	}
	send := func(c *websocket.Conn, kind string, v any) {
		data, _ := json.Marshal(v)
		raw, _ := json.Marshal(Message{Type: kind, Data: data})
		if err := c.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
	}
	tv, phone := dial(), dial()
	defer tv.CloseNow()
	defer phone.CloseNow()
	read(tv, "hello")
	read(phone, "hello")

	send(tv, "clock", map[string]float64{"t0": 123})
	var clock struct{ T0, T1 float64 }
	_ = json.Unmarshal(read(tv, "clock"), &clock)
	if clock.T0 != 123 || clock.T1 <= 0 {
		t.Fatalf("clock echo: %+v", clock)
	}

	send(tv, "sync.join", map[string]any{"room": "group:den", "channelId": 9})
	read(tv, "sync.state")
	send(phone, "sync.join", map[string]any{"room": "group:den", "channelId": 9})
	var st RoomState
	_ = json.Unmarshal(read(phone, "sync.state"), &st)
	if st.Members != 2 {
		t.Fatalf("second member: %+v", st)
	}
	send(phone, "sync.command", map[string]any{"room": "group:den", "action": "pause"})
	for {
		_ = json.Unmarshal(read(tv, "sync.state"), &st)
		if st.Rate == 0 {
			break
		}
	}
	bus.Publish("recording.started", map[string]int{"id": 5})
	if !strings.Contains(string(read(phone, "recording.started")), `"id":5`) {
		t.Fatal("events reach every client")
	}
}
