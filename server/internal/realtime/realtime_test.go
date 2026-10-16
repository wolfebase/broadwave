package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestAFreshMultiviewTileStartsAStepBack(t *testing.T) {
	start := time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	first := unixMS(start.Add(-3500 * time.Millisecond))
	one := r.Join("channel:4", 4, first)
	tile := r.Join("multiview:7:4", 4, first)
	if one.AnchorMedia != first || tile.AnchorMedia != first {
		t.Fatalf("both start on the first frame: %+v %+v", one, tile)
	}
	if got := tile.AnchorServer - one.AnchorServer; got != float64(stallStep/time.Millisecond) {
		t.Fatalf("a tile waits one stall step longer on its first frame, got %v ms", got)
	}
}

// A quad kept for Back left its rooms, and they went. Its encodes kept
// running, so the rooms it joins again need no wait on the first frame.
func TestARoomOnAnEncodeThatRanAWhileWaitsOnlyForTheRest(t *testing.T) {
	start := time.Date(2026, 10, 4, 4, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	now := unixMS(start)
	old := unixMS(start.Add(-8 * time.Second))
	tile := r.Join("multiview:7:4", 4, old)
	if tile.AnchorMedia != old || tile.AnchorServer != now {
		t.Fatalf("an 8 s old first frame plays at once: %+v", tile)
	}
	mid := unixMS(start.Add(-6 * time.Second))
	part := r.Join("multiview:7:5", 5, mid)
	if got := part.AnchorServer - now; got != 1500 {
		t.Fatalf("a 6 s old first frame waits the 1.5 s it lacks, got %v ms", got)
	}
	one := r.Join("channel:6", 6, mid)
	if one.AnchorMedia != mid || one.AnchorServer != now {
		t.Fatalf("a one-screen room on a 6 s old first frame plays at once: %+v", one)
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
	if st.AnchorMedia != unixMS(now.Add(-16*time.Second)) {
		t.Fatalf("seeking past the live edge stops where Live goes: %+v", st)
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

func TestEventSocketStopsAtTheClientCap(t *testing.T) {
	prev := maxClients
	maxClients = 2
	t.Cleanup(func() { maxClients = prev })
	bus := NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	var held []*websocket.Conn
	for range 2 {
		c := dial()
		held = append(held, c)
		if _, _, err := c.Read(ctx); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, c := range held {
			c.CloseNow()
		}
	}()
	extra := dial()
	defer extra.CloseNow()
	if _, _, err := extra.Read(ctx); err == nil {
		t.Fatal("a socket past the cap stayed open")
	}
	deadline := time.Now().Add(time.Second)
	for bus.Clients() > 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if bus.Clients() != 2 {
		t.Fatalf("clients %d", bus.Clients())
	}
}

func TestGroupRoomNamesWhoIsIn(t *testing.T) {
	bus := NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	send := func(c *websocket.Conn, kind string, v any) {
		data, _ := json.Marshal(v)
		raw, _ := json.Marshal(Message{Type: kind, Data: data})
		if err := c.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
	}
	people := func(c *websocket.Conn, want int) RoomState {
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for %d people: %v", want, err)
			}
			var m Message
			_ = json.Unmarshal(raw, &m)
			var st RoomState
			if m.Type == "sync.state" && json.Unmarshal(m.Data, &st) == nil && len(st.People) == want {
				return st
			}
		}
	}
	tv, phone, other := dial(), dial(), dial()
	defer tv.CloseNow()
	defer phone.CloseNow()
	defer other.CloseNow()

	send(tv, "here", map[string]string{"name": "Den TV", "kind": "appletv"})
	send(tv, "sync.join", map[string]any{"room": "group:den", "channelId": 9})
	people(tv, 1)
	send(phone, "sync.join", map[string]any{"room": "group:den", "channelId": 9})
	send(phone, "here", map[string]string{"name": "Sam's iPhone", "kind": "iphone"})
	st := people(tv, 2)
	for st.People[1].Name != "Sam's iPhone" {
		st = people(tv, 2)
	}
	if st.People[0] != (Person{Name: "Den TV", Kind: "appletv"}) || st.People[1].Kind != "iphone" {
		t.Fatalf("people: %+v", st.People)
	}

	groups := bus.Groups()
	if len(groups) != 1 || groups[0].Room != "group:den" || groups[0].ChannelID != 9 || len(groups[0].People) != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	send(other, "sync.join", map[string]any{"room": "group:no spaces", "channelId": 9})
	send(other, "sync.join", map[string]any{"room": "channel:9", "channelId": 9})
	for {
		_, raw, err := other.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m Message
		_ = json.Unmarshal(raw, &m)
		if m.Type == "sync.state" {
			var cs RoomState
			_ = json.Unmarshal(m.Data, &cs)
			if cs.Room != "channel:9" || cs.People != nil {
				t.Fatalf("only a valid code joins, and channel rooms name nobody: %+v", cs)
			}
			break
		}
	}
	if len(bus.Groups()) != 1 {
		t.Fatalf("a bad code made a group: %+v", bus.Groups())
	}

	// Everything published so far reaches other before its clock reply.
	send(other, "clock", map[string]float64{"t0": 1})
	for {
		_, raw, err := other.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"type":"clock"`) {
			break
		}
	}
	phone.Close(websocket.StatusNormalClosure, "")
	st = people(tv, 1)
	if st.People[0].Name != "Den TV" || st.Members != 1 {
		t.Fatalf("after leaving: %+v", st)
	}
	for {
		_, raw, err := other.Read(ctx)
		if err != nil {
			t.Fatal("everyone hears that the groups changed")
		}
		if strings.Contains(string(raw), `"groups.changed"`) {
			break
		}
	}
}

func TestOneSocketCannotJoinRoomsWithoutEnd(t *testing.T) {
	bus := NewBus()
	a := &client{rooms: map[string]bool{}}
	b := &client{rooms: map[string]bool{}}
	bus.clients[a] = struct{}{}
	bus.clients[b] = struct{}{}
	for i := range maxClientRooms {
		if !bus.setMember(a, fmt.Sprintf("channel:%d", i), true) {
			t.Fatalf("room %d refused", i)
		}
	}
	if bus.setMember(a, "channel:999", true) {
		t.Fatal("a socket joined more than the cap")
	}
	if !bus.setMember(a, "channel:0", false) || !bus.setMember(a, "channel:999", true) {
		t.Fatal("leaving a room frees its place")
	}

	// Group rooms are counted across the house, and joining one that exists
	// always works.
	for i := range maxGroups {
		c := &client{rooms: map[string]bool{fmt.Sprintf("group:g%d", i): true}}
		bus.clients[c] = struct{}{}
	}
	if bus.setMember(b, "group:new", true) {
		t.Fatal("a group past the cap was made")
	}
	if !bus.setMember(b, "group:g3", true) {
		t.Fatal("joining an existing group was refused")
	}
	if !bus.setMember(b, "channel:4", true) {
		t.Fatal("the group cap refused a channel room")
	}
}

func TestGroupCodes(t *testing.T) {
	for room, want := range map[string]bool{
		"group:den":                        true,
		"group:K7-QX_2":                    true,
		"group:":                           false,
		"group:has space":                  false,
		"group:é":                          false,
		"group:" + strings.Repeat("a", 33): false,
		"channel:4":                        true,
		"multiview:1:4":                    true,
		"lobby":                            false,
	} {
		if validRoom(room) != want {
			t.Errorf("validRoom(%q) = %v", room, !want)
		}
	}
}

func TestJoinAtStartsANewRoomOnTheScreensLatency(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	st := r.JoinAt("channel:4", 4, 0, "lowest")
	if st.Latency != "lowest" || st.LatencyMS != 6000 || st.AnchorMedia != liveAnchor(start, "lowest") {
		t.Fatalf("a new room should start at the joiner's latency, got %+v", st)
	}
	if st = r.JoinAt("channel:4", 4, 0, "stable"); st.Latency != "lowest" {
		t.Fatalf("a second screen must not move the room, got %+v", st)
	}
	if st = r.JoinAt("channel:5", 5, 0, "fastest"); st.Latency != "balanced" {
		t.Fatalf("an unknown latency starts balanced, got %+v", st)
	}
}

func TestFloorOnlyMovesARoomCloserToLive(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	r.JoinAt("channel:4", 4, 0, "lowest")
	st, ok := r.Floor("channel:4", "balanced")
	if !ok || st.Latency != "balanced" || st.AnchorMedia != liveAnchor(start, "balanced") {
		t.Fatalf("a lowest room should move back to balanced, got %+v %v", st, ok)
	}
	r.JoinAt("channel:5", 5, 0, "stable")
	if _, ok := r.Floor("channel:5", "balanced"); ok {
		t.Fatal("a stable room is already behind balanced")
	}
	if _, ok := r.Floor("channel:9", "balanced"); ok {
		t.Fatal("no room, nothing to floor")
	}
	// An Apple multiview tile on a long-group channel sat at balanced, 1.5 s
	// inside what AVPlayer reaches, labelled stable.
	r.JoinAt("multiview:ab12:9", 9, 0, "balanced")
	if st, ok := r.Floor("multiview:ab12:9", "stable"); !ok || st.AnchorMedia != liveAnchor(start, "stable") || st.Rate != 1 {
		t.Fatalf("a playing tile room should move back to stable, got %+v %v", st, ok)
	}
	// A group room the viewers rewound is already further back and stays.
	r.JoinAt("group:ch9", 9, 0, "balanced")
	back := liveAnchor(start, "balanced") - 60000
	if _, err := r.Apply("group:ch9", Command{Action: "seek", MediaTime: back}); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Floor("group:ch9", "stable"); st.Target(unixMS(start)) != back {
		t.Fatalf("a rewound room kept its place, got target %v want %v", st.Target(unixMS(start)), back)
	}
	// Once floored, a seek toward live stops at the floor, not at lowest.
	if st, _ := r.Apply("group:ch9", Command{Action: "seek", MediaTime: unixMS(start)}); st.AnchorMedia != liveAnchor(start, "stable") {
		t.Fatalf("a floored room's seek should stop at stable, got %+v", st)
	}
	// A room paused closer to live than an Apple screen reaches holds a frame
	// it cannot show, so it moves back too, still paused.
	r.JoinAt("group:ch4", 4, 0, "lowest")
	if _, err := r.Apply("group:ch4", Command{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Floor("group:ch4", "balanced"); st.Rate != 0 || st.Target(unixMS(start)) != liveAnchor(start, "balanced") {
		t.Fatalf("a paused room near live should move back, still paused, got %+v", st)
	}
}

func TestAnAppleScreenKeepsItsRoomAtBalanced(t *testing.T) {
	bus := NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(here string) *websocket.Conn {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		send(t, ctx, conn, "here", here)
		return conn
	}
	latencyIs := func(conn *websocket.Conn, want string) {
		t.Helper()
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for %s: %v", want, err)
			}
			var m Message
			_ = json.Unmarshal(msg, &m)
			var st RoomState
			if m.Type == "sync.state" && json.Unmarshal(m.Data, &st) == nil && st.Latency == want {
				return
			}
		}
	}
	web := dial(`{"name":"Chrome","kind":"web"}`)
	send(t, ctx, web, "sync.join", `{"room":"channel:4","channelId":4,"latency":"lowest"}`)
	latencyIs(web, "lowest")
	tv := dial(`{"name":"Den","kind":"appletv"}`)
	send(t, ctx, tv, "sync.join", `{"room":"channel:4","channelId":4,"latency":"lowest"}`)
	latencyIs(tv, "balanced")
	latencyIs(web, "balanced")
	send(t, ctx, web, "sync.command", `{"room":"channel:4","action":"latency","latency":"lowest"}`)
	latencyIs(web, "balanced")
	if st, _ := bus.Rooms.State("channel:4"); st.Latency != "balanced" {
		t.Fatalf("lowest is out of an Apple screen's reach, got %+v", st)
	}
	send(t, ctx, web, "sync.command", `{"room":"channel:4","action":"latency","latency":"stable"}`)
	latencyIs(tv, "stable")
}

// A station with 2.4 s groups gets a 4 s target and AVPlayer holds back
// 12 s. At balanced an Apple TV sat at 1x and fell 3 ms a second behind.
func TestAnAppleScreenHoldsALongGroupRoomAtStable(t *testing.T) {
	bus := NewBus()
	bus.LongHold = func(channelID int64) bool { return channelID == 9 }
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	send(t, ctx, conn, "here", `{"name":"Den","kind":"appletv"}`)
	send(t, ctx, conn, "sync.join", `{"room":"channel:9","channelId":9,"latency":"balanced"}`)
	send(t, ctx, conn, "sync.join", `{"room":"channel:4","channelId":4,"latency":"balanced"}`)
	// Each multiview tile on Apple joins its own room with its channel.
	send(t, ctx, conn, "sync.join", `{"room":"multiview:ab12:9","channelId":9,"latency":"balanced"}`)
	send(t, ctx, conn, "sync.join", `{"room":"multiview:ab12:4","channelId":4,"latency":"balanced"}`)
	send(t, ctx, conn, "sync.command", `{"room":"channel:9","action":"latency","latency":"balanced"}`)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		long, _ := bus.Rooms.State("channel:9")
		one, _ := bus.Rooms.State("channel:4")
		longTile, _ := bus.Rooms.State("multiview:ab12:9")
		tile, _ := bus.Rooms.State("multiview:ab12:4")
		if long.Version >= 3 && one.Latency != "" && longTile.Latency != "" && tile.Latency != "" {
			if long.Latency != "stable" || one.Latency != "balanced" {
				t.Fatalf("long groups at %s, one-second groups at %s", long.Latency, one.Latency)
			}
			if longTile.Latency != "stable" || tile.Latency != "balanced" {
				t.Fatalf("tiles: long groups at %s, one-second groups at %s", longTile.Latency, tile.Latency)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the rooms never settled")
}

// A station's long groups are known only once a tune has seen one: after the
// screens joined on a first tune, or on a tile whose second join the bus
// ignored because the screen was still a member.
func TestLongGroupsFoundLaterFloorAnAppleRoom(t *testing.T) {
	bus := NewBus()
	var long atomic.Bool
	bus.LongHold = func(channelID int64) bool { return channelID == 9 && long.Load() }
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func() *websocket.Conn {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	tv, web := dial(), dial()
	defer tv.CloseNow()
	defer web.CloseNow()
	send(t, ctx, tv, "here", `{"name":"Den","kind":"appletv"}`)
	send(t, ctx, web, "here", `{"name":"Laptop","kind":"web"}`)
	send(t, ctx, tv, "sync.join", `{"room":"multiview:ab12:9","channelId":9,"latency":"balanced"}`)
	send(t, ctx, web, "sync.join", `{"room":"channel:9","channelId":9,"latency":"balanced"}`)
	settled := func(rooms ...string) bool {
		for _, r := range rooms {
			if st, ok := bus.Rooms.State(r); !ok || st.Latency == "" {
				return false
			}
		}
		return true
	}
	deadline := time.Now().Add(2 * time.Second)
	for !settled("multiview:ab12:9", "channel:9") {
		if time.Now().After(deadline) {
			t.Fatal("the rooms never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	long.Store(true)
	bus.LongGroupsFound(9)
	tile, _ := bus.Rooms.State("multiview:ab12:9")
	browser, _ := bus.Rooms.State("channel:9")
	if tile.Latency != "stable" || browser.Latency != "balanced" {
		t.Fatalf("the Apple tile at %s, the browser room at %s", tile.Latency, browser.Latency)
	}
	// Not only the label: the tile room plays where AVPlayer reaches.
	if now := unixMS(time.Now()); tile.Target(now) > now-19900 {
		t.Fatalf("the tile room plays %.0f ms behind live", now-tile.Target(now))
	}
}

func send(t *testing.T, ctx context.Context, conn *websocket.Conn, kind, data string) {
	t.Helper()
	raw, _ := json.Marshal(Message{Type: kind, Data: json.RawMessage(data)})
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
}

func TestTwoTabsOfOneBrowserAreOneScreen(t *testing.T) {
	b := NewBus()
	one := &client{send: make(chan []byte, 4), rooms: map[string]bool{}, here: Presence{ID: "chrome-1", Name: "Chrome on Mac", Kind: "web"}}
	two := &client{send: make(chan []byte, 4), rooms: map[string]bool{"channel:7": true}, here: Presence{ID: "chrome-1", Name: "Chrome on Mac", Kind: "web"}}
	quad := &client{send: make(chan []byte, 4), rooms: map[string]bool{"channel:7": true, "channel:8": true}, here: Presence{ID: "tv-1", Name: "Den", Kind: "appletv"}}
	for _, c := range []*client{one, two, quad} {
		b.clients[c] = struct{}{}
	}
	screens := b.Screens()
	if len(screens) != 2 {
		t.Fatalf("screens: %+v", screens)
	}
	for _, s := range screens {
		if s.ID == "chrome-1" && s.ChannelID != 7 || s.ID == "tv-1" && s.ChannelID != 0 {
			t.Fatalf("watching: %+v", screens)
		}
	}
	if n := b.SendTo("chrome-1", "screen.watch", map[string]int{"channelId": 7}); n != 2 || len(one.send) != 1 || len(two.send) != 1 || len(quad.send) != 0 {
		t.Fatalf("sent to %d", n)
	}
	if b.SendTo("", "screen.watch", nil) != 0 {
		t.Fatal("an empty id reached a screen")
	}
	for _, bad := range []string{"has space", "semi;colon", strings.Repeat("a", 65)} {
		if cleanID(bad) != "" {
			t.Fatalf("kept %q", bad)
		}
	}
}

func TestAScreenWithSyncOffSaysWhatItWatches(t *testing.T) {
	b := NewBus()
	tv := &client{send: make(chan []byte, 8), rooms: map[string]bool{}}
	b.clients[tv] = struct{}{}
	say := func(body string) {
		b.handle(tv, Message{Type: "here", Data: json.RawMessage(body)})
	}
	watchingNow := func() int64 {
		screens := b.Screens()
		if len(screens) != 1 {
			t.Fatalf("screens: %+v", screens)
		}
		return screens[0].ChannelID
	}
	say(`{"id":"tv-1","name":"Den","kind":"appletv"}`)
	if got := watchingNow(); got != 0 {
		t.Fatalf("watching %d before it said", got)
	}
	say(`{"id":"tv-1","name":"Den","kind":"appletv","channelId":7}`)
	if got := watchingNow(); got != 7 {
		t.Fatalf("watching %d, want 7", got)
	}
	// A room it joins wins over what it said.
	b.mu.Lock()
	tv.rooms["channel:9"] = true
	b.mu.Unlock()
	if got := watchingNow(); got != 9 {
		t.Fatalf("watching %d, want the room's 9", got)
	}
	b.mu.Lock()
	delete(tv.rooms, "channel:9")
	b.mu.Unlock()
	say(`{"id":"tv-1","name":"Den","kind":"appletv","channelId":-3}`)
	if got := watchingNow(); got != 0 {
		t.Fatalf("watching %d after it stopped", got)
	}
}

// J0.114: AVPlayer starts a low-latency playlist about 8 s behind live, ahead
// of a room at 16 s. The first Apple screen brings a young room to its frame
// less measureMargin, and Settle and the Apple floor then leave it there.
func TestAheadMeasuresAYoungRoomAtTheScreen(t *testing.T) {
	start := time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	r.Join("channel:33", 33, unixMS(start.Add(-60*time.Second)))
	*now = start.Add(6 * time.Second)
	nowMS := unixMS(*now)
	before, _ := r.State("channel:33")
	if math.Abs(before.Target(nowMS)-(nowMS-16000)) > 1 {
		t.Fatalf("a deep buffer starts at the 16 s target: %v", nowMS-before.Target(nowMS))
	}
	onScreen := nowMS - 8200
	st, err := r.Apply("channel:33", Command{Action: "ahead", MediaTime: onScreen})
	if err != nil || st.Version != before.Version+1 || math.Abs(st.Target(nowMS)-(onScreen-2500)) > 1 || st.Rate != 1 {
		t.Fatalf("the room should sit 2.5 s behind the screen: %+v %v", st, err)
	}
	// Settle and the floor leave a measured room where it is.
	*now = start.Add(40 * time.Second)
	if eased := r.Settle(33, unixMS(start.Add(-60*time.Second)), nil); len(eased) != 0 {
		t.Fatalf("Settle moved a measured room: %+v", eased)
	}
	if _, floored := r.Floor("channel:33", "stable"); floored {
		t.Fatal("the Apple floor moved a measured room")
	}
	// Never closer to live than lowest, and never back.
	r.Join("channel:34", 34, unixMS(start.Add(-60*time.Second)))
	*now = start.Add(42 * time.Second)
	nowMS = unixMS(*now)
	st, _ = r.Apply("channel:34", Command{Action: "ahead", MediaTime: nowMS})
	if st.Target(nowMS) > nowMS-6000+1 {
		t.Fatalf("measured past lowest: %v", nowMS-st.Target(nowMS))
	}
	same, _ := r.State("channel:34")
	if back, _ := r.Apply("channel:34", Command{Action: "ahead", MediaTime: same.Target(nowMS) - 5000}); back.Version != same.Version {
		t.Fatalf("ahead moved a room back: %+v", back)
	}
	// A viewer's Live delay choice wins and clears the mark.
	st, _ = r.Apply("channel:33", Command{Action: "latency", Latency: "stable"})
	if math.Abs(st.Target(nowMS)-(nowMS-20000)) > 1 {
		t.Fatalf("latency should override the measure: %v", nowMS-st.Target(nowMS))
	}
	if eased := r.Settle(33, unixMS(start.Add(-60*time.Second)), nil); eased != nil && len(eased) != 0 {
		t.Fatalf("settle on target should be quiet: %+v", eased)
	}
}

func TestAheadOnlyForYoungFollowRoomsAndSoloTiles(t *testing.T) {
	start := time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	for _, room := range []string{"channel:9", "group:den", "multiview:abc"} {
		r.Join(room, 9, unixMS(start.Add(-60*time.Second)))
	}
	*now = start.Add(5 * time.Second)
	for _, room := range []string{"group:den", "multiview:abc"} {
		before, _ := r.State(room)
		if st, _ := r.Apply(room, Command{Action: "ahead", MediaTime: unixMS(*now) - 8000}); st.Version != before.Version {
			t.Fatalf("%s took ahead: %+v", room, st)
		}
	}
	// A multiview tile's own room, played by one screen, comes to that tile.
	r.Join("multiview:abc:9", 9, unixMS(start.Add(-60*time.Second)))
	tile, _ := r.State("multiview:abc:9")
	if st, _ := r.Apply("multiview:abc:9", Command{Action: "ahead", MediaTime: unixMS(*now) - 8000}); st.Version == tile.Version {
		t.Fatalf("a solo tile room should take ahead: %+v", st)
	}
	r.Join("multiview:abc:10", 10, unixMS(start.Add(-60*time.Second)))
	r.Join("multiview:abc:10", 10, unixMS(start.Add(-60*time.Second)))
	shared, _ := r.State("multiview:abc:10")
	if st, _ := r.Apply("multiview:abc:10", Command{Action: "ahead", MediaTime: unixMS(*now) - 8000}); st.Version != shared.Version {
		t.Fatalf("a tile room two screens play took ahead: %+v", st)
	}
	*now = start.Add(youngRoom + time.Second)
	old, _ := r.State("channel:9")
	if st, _ := r.Apply("channel:9", Command{Action: "ahead", MediaTime: unixMS(*now) - 8000}); st.Version != old.Version {
		t.Fatalf("an old room was pulled forward: %+v", st)
	}
	// A room that ends and is made again is young again.
	r.Leave("channel:9")
	r.Join("channel:9", 9, unixMS(now.Add(-60*time.Second)))
	*now = now.Add(5 * time.Second)
	fresh, _ := r.State("channel:9")
	if st, _ := r.Apply("channel:9", Command{Action: "ahead", MediaTime: unixMS(*now) - 8000}); st.Version == fresh.Version {
		t.Fatalf("a remade room should be young: %+v", st)
	}
}

// A screen whose live edge sits further back than a measured room takes the
// room back to itself, so nobody is stranded behind. An unmeasured room keeps
// today's behavior (its 16 s target already covers every Apple screen).
func TestBehindStepsAMeasuredRoomBackToTheScreen(t *testing.T) {
	start := time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)
	r, now := fixedRooms(start)
	r.Join("channel:5", 5, unixMS(start.Add(-60*time.Second)))
	*now = start.Add(5 * time.Second)
	nowMS := unixMS(*now)
	if st, _ := r.Apply("channel:5", Command{Action: "behind", MediaTime: nowMS - 20000}); st.Version != 1 {
		t.Fatalf("behind moved an unmeasured room: %+v", st)
	}
	r.Apply("channel:5", Command{Action: "ahead", MediaTime: nowMS - 8000})
	*now = start.Add(60 * time.Second)
	nowMS = unixMS(*now)
	onScreen := nowMS - 12500
	st, _ := r.Apply("channel:5", Command{Action: "behind", MediaTime: onScreen})
	if math.Abs(st.Target(nowMS)-(onScreen-2500)) > 1 {
		t.Fatalf("the room should step back to 2.5 s behind the stranded screen: %v", nowMS-st.Target(nowMS))
	}
	// A screen only a little behind leaves it to the trims.
	same, _ := r.State("channel:5")
	if st, _ := r.Apply("channel:5", Command{Action: "behind", MediaTime: same.Target(nowMS) - 200}); st.Version != same.Version {
		t.Fatalf("a small gap moved the room: %+v", st)
	}
}

// Sync screens: lining up is a choice. A new channel room starts unsynced;
// "syncall" marks it once so every screen on it lines up, and a remade room
// starts fresh.
func TestSyncAllMarksTheRoomOnce(t *testing.T) {
	start := time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC)
	r, _ := fixedRooms(start)
	st := r.Join("channel:4", 4, 0)
	if st.Synced {
		t.Fatalf("a new room starts unsynced: %+v", st)
	}
	st, err := r.Apply("channel:4", Command{Action: "syncall"})
	if err != nil || !st.Synced || st.Version != 2 {
		t.Fatalf("syncall should mark the room: %+v %v", st, err)
	}
	if again, _ := r.Apply("channel:4", Command{Action: "syncall"}); again.Version != st.Version {
		t.Fatalf("a second syncall changed the room: %+v", again)
	}
	r.Leave("channel:4")
	if fresh := r.Join("channel:4", 4, 0); fresh.Synced {
		t.Fatalf("a remade room should start unsynced: %+v", fresh)
	}
}

func TestHelloCarriesDataVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bus := NewBus()
	bus.Versions = func() any { return map[string]int{"guide": 7} }
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
	if !strings.Contains(string(raw), `"versions":{"guide":7}`) {
		t.Fatalf("hello %s", raw)
	}
}

// A client that stops reading is closed once its queue fills, so it
// reconnects and catches up, rather than silently missing events.
func TestSlowClientIsDroppedNotSkipped(t *testing.T) {
	c := &client{send: make(chan []byte, 2), behind: make(chan struct{})}
	if !c.offer([]byte("a")) || !c.offer([]byte("b")) {
		t.Fatal("room in the queue refused")
	}
	if c.offer([]byte("c")) {
		t.Fatal("a full queue accepted")
	}
	select {
	case <-c.behind:
	default:
		t.Fatal("a client that fell behind was kept")
	}
	c.offer([]byte("d")) // a second overflow must not close twice
}
