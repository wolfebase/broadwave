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
	st := r.Join("channel:4", 4, 0)
	if st.Mode != "follow" || st.Members != 1 {
		t.Fatalf("channel rooms follow live: %+v", st)
	}
	want := unixMS(start.Add(-16 * time.Second))
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

func TestFreshRoomStartsOnTheFirstFrame(t *testing.T) {
	start := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	st := r.Join("channel:4", 4, first)
	if st.AnchorMedia != first || st.Members != 1 {
		t.Fatalf("a fresh room starts on the first frame: %+v", st)
	}
	next := r.Join("channel:4", 4, unixMS(start))
	if next.AnchorMedia != first || next.Members != 2 {
		t.Fatalf("the next screen keeps that frame: %+v", next)
	}
	deep := r.Join("channel:9", 9, unixMS(start.Add(-30*time.Second)))
	want := unixMS(start.Add(-16 * time.Second))
	if deep.AnchorMedia != want {
		t.Fatalf("a deep buffer stays at the latency target, got %v want %v", deep.AnchorMedia, want)
	}
}

func TestFollowRoomSettlesOnceTheBufferCoversTheLatency(t *testing.T) {
	start := time.Date(2026, 9, 26, 21, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	st := r.Join("channel:4", 4, first)
	if st.AnchorMedia != first || math.Abs(st.Target(unixMS(start))-(first-1500)) > 1 {
		t.Fatalf("a fresh room starts on the first frame, a moment later: %+v", st)
	}
	if moved := r.Settle(4, first, nil); len(moved) != 0 {
		t.Fatalf("a fresh first frame must stay, got %+v", moved)
	}
	*now = start.Add(20 * time.Second)
	recent := unixMS(now.Add(-3 * time.Second))
	if moved := r.Settle(4, recent, nil); len(moved) != 0 {
		t.Fatalf("a first frame newer than the target must stay, got %+v", moved)
	}
	if moved := r.Settle(4, first, nil); len(moved) != 0 {
		t.Fatalf("a room with one screen must stay at 1x, got %+v", moved)
	}
	r.Join("channel:4", 4, first)
	before, _ := r.State("channel:4")
	onScreen := before.Target(unixMS(*now))
	eased := r.Settle(4, first, nil)
	if len(eased) != 1 {
		t.Fatalf("expected one settle, got %+v", eased)
	}
	moved := eased[0].State
	// The frame on screen does not jump; the room slows instead of freezing players.
	if math.Abs(moved.Target(unixMS(*now))-onScreen) > 1 || moved.Rate != settleRate {
		t.Fatalf("settle must keep the frame and slow down: target %v want %v (%+v)", moved.Target(unixMS(*now)), onScreen, moved)
	}
	held := moved.Version
	if again := r.Settle(4, first, nil); len(again) != 0 {
		t.Fatalf("a second settle moved the room: %+v", again)
	}
	// At the end of the ease the room is on its latency target and back at 1x.
	*now = now.Add(eased[0].Until)
	st, ok := r.EndEase("channel:4", held)
	want := unixMS(now.Add(-16 * time.Second))
	if !ok || st.Rate != 1 || math.Abs(st.Target(unixMS(*now))-want) > 5 {
		t.Fatalf("after the ease target %v want %v (%+v ok=%v)", st.Target(unixMS(*now)), want, st, ok)
	}
	// A screen that cannot reach the easing frame aims at server time less
	// latencyMs, which is where the room lands when the ease ends.
	if math.Abs(unixMS(*now)-moved.LatencyMS-st.Target(unixMS(*now))) > 5 {
		t.Fatalf("latencyMs %v does not point at the end of the ease", moved.LatencyMS)
	}
	if _, ok := r.EndEase("channel:4", held); ok {
		t.Fatal("a stale ease must not change the room again")
	}
	if eased[0].Until < 100*time.Second || eased[0].Until > 500*time.Second {
		t.Fatalf("a ~9.5 s gap at 2.5%% should take a few minutes, got %v", eased[0].Until)
	}
	g := r.Join("group:den", 4, first)
	*now = start.Add(40 * time.Second)
	if moved := r.Settle(4, first, nil); len(moved) != 0 {
		t.Fatalf("a group room must stay: %+v (group %+v)", moved, g)
	}
	if st, _ = r.State("group:den"); st.AnchorMedia != g.AnchorMedia || st.Version != g.Version {
		t.Fatalf("group anchor changed: %+v", st)
	}
}

func TestAppleOnlyRoomJumpsToItsLatency(t *testing.T) {
	start := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	r.Join("channel:4", 4, first)
	apple := func(string) bool { return false }
	if moved := r.Settle(4, first, apple); len(moved) != 0 {
		t.Fatalf("a fresh first frame must stay, got %+v", moved)
	}
	// AVPlayer cannot play near the first frame, so even one screen jumps
	// straight to the latency target instead of waiting out an ease.
	*now = start.Add(20 * time.Second)
	moved := r.Settle(4, first, apple)
	want := unixMS(now.Add(-16 * time.Second))
	if len(moved) != 1 || moved[0].Until != 0 || moved[0].State.Rate != 1 || math.Abs(moved[0].State.Target(unixMS(*now))-want) > 1 {
		t.Fatalf("an Apple room must jump to its target at 1x, got %+v", moved)
	}
	if again := r.Settle(4, first, apple); len(again) != 0 {
		t.Fatalf("a room on its target must stay: %+v", again)
	}
}

func TestMultiviewRoomSharesOneTarget(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	a := r.Join("multiview:games", 0, 0)
	b := r.Join("multiview:games", 0, 0)
	if a.Mode != "group" || b.Members != 2 || a.AnchorMedia != b.AnchorMedia {
		t.Fatalf("tiles share one live target: %+v %+v", a, b)
	}
	st, err := r.Apply("multiview:games", Command{Action: "pause"})
	if err != nil || st.Rate != 0 {
		t.Fatalf("pause moves every tile: %+v %v", st, err)
	}
}

func TestGroupRoomPauseSeekLive(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	r.Join("group:den", 9, 0)
	r.Join("group:den", 9, 0)
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
	if st.AnchorMedia != unixMS(now.Add(-16*time.Second)) || st.Rate != 1 {
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

func TestFreshJoinUsesTheFirstFrame(t *testing.T) {
	start := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	if st := freshJoin(t, start, first, ""); st.AnchorMedia != first {
		t.Fatalf("join should anchor on the first frame, got %v", st.AnchorMedia)
	}
}

// AVPlayer cannot play a fresh tune's first frame. An Apple screen that
// started there froze a few seconds in, when Settle moved the room back.
func TestAppleFreshJoinStartsOnTheTarget(t *testing.T) {
	start := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	st := freshJoin(t, start, first, `{"name":"Living Room","kind":"appletv"}`)
	if want := liveAnchor(start, "balanced"); st.AnchorMedia != want {
		t.Fatalf("an Apple room should start on the latency target %v, got %v", want, st.AnchorMedia)
	}
}

func freshJoin(t *testing.T, start time.Time, first float64, here string) RoomState {
	t.Helper()
	bus := NewBus()
	bus.SetClock(func() time.Time { return start })
	bus.MediaStart = func(channelID int64) (float64, bool) {
		if channelID == 4 {
			return first, true
		}
		return 0, false
	}
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if here != "" {
		raw, _ := json.Marshal(Message{Type: "here", Data: json.RawMessage(here)})
		if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := json.Marshal(map[string]any{"room": "channel:4", "channelId": 4})
	raw, _ := json.Marshal(Message{Type: "sync.join", Data: data})
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m Message
		_ = json.Unmarshal(msg, &m)
		if m.Type != "sync.state" {
			continue
		}
		var st RoomState
		_ = json.Unmarshal(m.Data, &st)
		return st
	}
	t.Fatal("no room state")
	return RoomState{}
}

func TestHereAnnouncesAScreen(t *testing.T) {
	bus := NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	raw, _ := json.Marshal(Message{Type: "here", Data: json.RawMessage(`{"name":"Broadwave Staging TV","kind":"appletv"}`)})
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var screens []Presence
	for time.Now().Before(deadline) {
		screens = bus.Screens()
		if len(screens) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(screens) != 1 || screens[0].Kind != "appletv" || screens[0].Name != "Broadwave Staging TV" {
		t.Fatalf("%+v", screens)
	}
	conn.Close(websocket.StatusNormalClosure, "")
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(bus.Screens()) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("screen stayed after disconnect: %+v", bus.Screens())
}

func TestRoomsWithBrowsersSkipsAppleApps(t *testing.T) {
	b := NewBus()
	tv := &client{rooms: map[string]bool{"channel:1": true}, here: Presence{Kind: "appletv"}}
	phone := &client{rooms: map[string]bool{"channel:1": true, "channel:2": true}, here: Presence{Kind: "iphone"}}
	unknown := &client{rooms: map[string]bool{"channel:3": true}}
	web := &client{rooms: map[string]bool{"channel:2": true, "channel:4": false}, here: Presence{Kind: "web"}}
	for _, c := range []*client{tv, phone, unknown, web} {
		b.clients[c] = struct{}{}
	}
	got := b.roomsWithBrowsers()
	if got["channel:1"] || !got["channel:2"] || !got["channel:3"] || got["channel:4"] {
		t.Fatalf("rooms with browsers: %v", got)
	}
}

func TestAStalledScreenStepsItsRoomBack(t *testing.T) {
	start := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	first := unixMS(start.Add(-4 * time.Second))
	r.Join("channel:24", 24, first)
	before, _ := r.State("channel:24")
	*now = start.Add(10 * time.Second)
	onScreen := before.Target(unixMS(*now))
	st, err := r.Apply("channel:24", Command{Action: "stalled"})
	if err != nil || math.Abs(st.Target(unixMS(*now))-(onScreen-2000)) > 1 || st.Rate != 1 {
		t.Fatalf("a lone screen that stalls steps back 2 s: %+v %v", st, err)
	}
	if again, _ := r.Apply("channel:24", Command{Action: "stalled"}); again.Version != st.Version {
		t.Fatalf("a second stall inside the quiet time moved it again: %+v", again)
	}
	// Never further back than the latency target.
	for i := range 20 {
		*now = start.Add(time.Duration(14+4*i) * time.Second)
		st, _ = r.Apply("channel:24", Command{Action: "stalled"})
	}
	if floor := unixMS(now.Add(-16 * time.Second)); st.Target(unixMS(*now)) < floor-1 {
		t.Fatalf("stepped past the latency target: %v < %v", st.Target(unixMS(*now)), floor)
	}
	// Two screens share the room: the other one would pause for the step.
	r.Join("channel:5", 5, 0)
	r.Join("channel:5", 5, 0)
	shared, _ := r.State("channel:5")
	if st, _ := r.Apply("channel:5", Command{Action: "stalled"}); st.Version != shared.Version {
		t.Fatalf("a shared follow room stepped back: %+v", st)
	}
	// A multiview is one screen with several tiles.
	fresh := unixMS(now.Add(-4 * time.Second))
	r.Join("multiview:m", 24, fresh)
	r.Join("multiview:m", 22, fresh)
	*now = now.Add(5 * time.Second)
	mv, _ := r.State("multiview:m")
	if st, _ := r.Apply("multiview:m", Command{Action: "stalled"}); st.Version == mv.Version {
		t.Fatalf("a multiview did not step back: %+v", st)
	}
}

func TestLatencyChangeCarriesItsMilliseconds(t *testing.T) {
	r, _ := fixedRooms(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC))
	if st := r.Join("channel:4", 4, 0); st.LatencyMS != 16000 {
		t.Fatalf("balanced is 16 s, got %+v", st)
	}
	st, err := r.Apply("channel:4", Command{Action: "latency", Latency: "stable"})
	if err != nil || st.LatencyMS != 20000 {
		t.Fatalf("stable is 20 s, got %+v %v", st, err)
	}
}

func TestHelloNamesTheServerProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	boot := func(bus *Bus) string {
		srv := httptest.NewServer(bus)
		defer srv.Close()
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.CloseNow()
		_, raw, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Type string `json:"type"`
			Data struct {
				Boot string `json:"boot"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Type != "hello" {
			t.Fatalf("first frame %s", raw)
		}
		return m.Data.Boot
	}
	running, restarted := NewBus(), NewBus()
	first, again := boot(running), boot(running)
	if first == "" || first != again {
		t.Fatalf("one process said %q then %q", first, again)
	}
	if next := boot(restarted); next == first {
		t.Fatalf("a restarted server kept boot %q", next)
	}
}
