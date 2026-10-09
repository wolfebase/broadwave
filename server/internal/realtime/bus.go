package realtime

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Message is every frame on the socket, in both directions.
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Bus fans server events out to connected clients and hosts sync rooms.
type Bus struct {
	Rooms *Rooms
	// MediaStart is the earliest program time (Unix ms) a channel can play.
	// The hub sets it. A miss leaves the room on the latency target.
	MediaStart func(channelID int64) (float64, bool)
	// LongHold reports a channel whose playlists hold Apple players 12 s
	// back from their live edge. The hub sets it.
	LongHold func(channelID int64) bool
	// Boot names this server process in every hello. A client that sees a
	// new one knows the server restarted: every watch and room it had is gone.
	Boot string

	mu      sync.Mutex
	clients map[*client]struct{}
	now     func() time.Time
}

// Presence is one app that announced itself on the event socket.
type Presence struct {
	Name string
	Kind string
	Addr string
}

type client struct {
	send  chan []byte
	rooms map[string]bool
	here  Presence
}

func NewBus() *Bus {
	var id [8]byte
	_, _ = rand.Read(id[:])
	return &Bus{Rooms: NewRooms(), Boot: hex.EncodeToString(id[:]), clients: map[*client]struct{}{}, now: time.Now}
}

// SetClock pins the time on hello, clock, and sync messages. Tests use it.
func (b *Bus) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	b.now = now
	if b.Rooms != nil {
		b.Rooms.now = now
	}
}

func frame(kind string, v any) []byte {
	data, _ := json.Marshal(v)
	b, _ := json.Marshal(Message{Type: kind, Data: data})
	return b
}

// Publish sends an event to every client. Slow clients drop events rather than
// block the server; they resync from the REST API on reconnect.
func (b *Bus) Publish(kind string, v any) {
	msg := frame(kind, v)
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.clients {
		select {
		case c.send <- msg:
		default:
		}
	}
}

// LongGroupsFound floors each room on the channel that has an Apple screen.
// A station's long groups are known only once a tune has seen one, which can
// be after its screens joined.
func (b *Bus) LongGroupsFound(channelID int64) {
	if b == nil || b.Rooms == nil {
		return
	}
	for _, room := range b.Rooms.OnChannel(channelID) {
		b.mu.Lock()
		apple := b.appleInLocked(room)
		b.mu.Unlock()
		if !apple {
			continue
		}
		if _, floored := b.Rooms.Floor(room, b.appleLatencyFor(room)); floored {
			b.roomChanged(room)
		}
	}
}

// Settle eases this channel's follow rooms onto their latency target once the
// playlist covers it, tells the members, and puts each room back to 1x when it
// arrives. A fresh tune is left alone.
func (b *Bus) Settle(channelID int64, earliest float64) {
	if b == nil || b.Rooms == nil {
		return
	}
	browsers := b.roomsWithBrowsers()
	b.mu.Lock()
	eased := b.Rooms.Settle(channelID, earliest, func(room string) bool { return browsers[room] })
	for _, e := range eased {
		b.publishRoomLocked(e.State)
	}
	b.mu.Unlock()
	for _, e := range eased {
		if e.Until <= 0 {
			continue
		}
		room, version := e.State.Room, e.State.Version
		time.AfterFunc(e.Until, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if st, ok := b.Rooms.EndEase(room, version); ok {
				b.publishRoomLocked(st)
			}
		})
	}
}

// roomsWithBrowsers lists rooms with a member that is not a known Apple app.
// A screen that has not said what it is counts as a browser.
func (b *Bus) roomsWithBrowsers() map[string]bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]bool{}
	for c := range b.clients {
		switch c.here.Kind {
		case "iphone", "ipad", "appletv":
			continue
		}
		for room, in := range c.rooms {
			if in {
				out[room] = true
			}
		}
	}
	return out
}

// publishRoomLocked sends a room's state with b.mu held. A state read from
// Rooms under the same hold cannot be overtaken by an older one: a pause and
// a join at the same moment otherwise delivered the join's state last, and
// every screen played on in a paused room.
func (b *Bus) publishRoomLocked(st RoomState) {
	if groupRoom(st.Room) {
		st.People = b.peopleLocked(st.Room)
	}
	msg := frame("sync.state", st)
	for c := range b.clients {
		if c.rooms[st.Room] {
			select {
			case c.send <- msg:
			default:
			}
		}
	}
}

// peopleLocked names the screens in a room. A screen that has not said what
// it is still counts, with no name.
func (b *Bus) peopleLocked(room string) []Person {
	out := []Person{}
	for c := range b.clients {
		if c.rooms[room] {
			out = append(out, Person{Name: c.here.Name, Kind: c.here.Kind})
		}
	}
	slices.SortFunc(out, func(a, b Person) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.Kind, b.Kind))
	})
	return out
}

// Groups lists every group room with who is in it.
func (b *Bus) Groups() []RoomState {
	if b == nil || b.Rooms == nil {
		return nil
	}
	list := b.Rooms.Groups()
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range list {
		list[i].People = b.peopleLocked(list[i].Room)
	}
	return list
}

// roomChanged tells a room's members its new state after someone joined or
// left, and tells everyone when the list of groups changed.
func (b *Bus) roomChanged(room string) {
	b.mu.Lock()
	if st, ok := b.Rooms.State(room); ok {
		b.publishRoomLocked(st)
	}
	b.mu.Unlock()
	if groupRoom(room) {
		b.Publish("groups.changed", struct{}{})
	}
}

// Screens returns clients that have announced a name and a kind.
func (b *Bus) Screens() []Presence {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Presence
	for c := range b.clients {
		if c.here.Kind == "" {
			continue
		}
		out = append(out, c.here)
	}
	return out
}

// Clients reports how many sockets are connected.
func (b *Bus) Clients() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients)
}

// maxClients is how many event sockets one server keeps. Past this the
// next socket is closed, so one browser cannot pin the process.
var maxClients = 128

// ServeHTTP upgrades to the event socket.
func (b *Bus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The HTTP layer refuses other sites; a proxy may rewrite Host, so the
	// library's Origin-equals-Host check would refuse the web app behind one.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	c := &client{send: make(chan []byte, 64), rooms: map[string]bool{}, here: Presence{Addr: hostOnly(r.RemoteAddr)}}
	b.mu.Lock()
	if len(b.clients) >= maxClients {
		b.mu.Unlock()
		return
	}
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, c)
		b.mu.Unlock()
		for room := range c.rooms {
			b.Rooms.Leave(room)
			b.roomChanged(room)
		}
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-c.send:
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					return
				}
			}
		}
	}()
	c.send <- frame("hello", map[string]any{"serverTime": unixMS(b.now()), "boot": b.Boot})
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m Message
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		b.handle(c, m)
	}
}

func (b *Bus) reply(c *client, kind string, v any) {
	select {
	case c.send <- frame(kind, v):
	default:
	}
}

func (b *Bus) handle(c *client, m Message) {
	switch m.Type {
	case "here":
		var req struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		if json.Unmarshal(m.Data, &req) != nil {
			return
		}
		kind := strings.ToLower(strings.TrimSpace(req.Kind))
		switch kind {
		case "iphone", "ipad", "appletv", "web":
		default:
			return
		}
		name := cleanLabel(req.Name)
		if name == "" {
			name = kind
		}
		b.mu.Lock()
		c.here.Name = name
		c.here.Kind = kind
		rooms := slices.Collect(maps.Keys(c.rooms))
		b.mu.Unlock()
		for _, room := range rooms {
			floored := false
			if appleKind(kind) {
				_, floored = b.Rooms.Floor(room, b.appleLatencyFor(room))
			}
			if floored || groupRoom(room) {
				b.roomChanged(room)
			}
		}
	case "clock":
		var req struct {
			T0 float64 `json:"t0"`
		}
		_ = json.Unmarshal(m.Data, &req)
		b.reply(c, "clock", map[string]float64{"t0": req.T0, "t1": unixMS(b.now())})
	case "sync.join":
		var req struct {
			Room      string `json:"room"`
			ChannelID int64  `json:"channelId"`
			// Latency is the screen's default, used only when this join starts the room.
			Latency string `json:"latency"`
		}
		if json.Unmarshal(m.Data, &req) != nil || !validRoom(req.Room) || !b.setMember(c, req.Room, true) {
			return
		}
		var earliest float64
		b.mu.Lock()
		kind := c.here.Kind
		b.mu.Unlock()
		// AVPlayer holds back from the live edge and cannot play a fresh
		// tune's first frame, so a room an Apple screen starts begins on the
		// latency target. The screen then waits once on its first picture
		// instead of playing a few seconds and freezing when Settle moves the
		// room back.
		if b.MediaStart != nil && !appleKind(kind) {
			if v, ok := b.MediaStart(req.ChannelID); ok {
				earliest = v
			}
		}
		b.Rooms.JoinAt(req.Room, req.ChannelID, earliest, req.Latency)
		if appleKind(kind) {
			b.Rooms.Floor(req.Room, b.appleLatencyFor(req.Room))
		}
		b.roomChanged(req.Room)
	case "sync.report":
		// A screen's playback health, logged so a stutter on a real TV shows
		// up in the server log without Xcode attached to it.
		var req map[string]any
		if json.Unmarshal(m.Data, &req) != nil || len(req) > 32 {
			return
		}
		b.mu.Lock()
		name, kind := c.here.Name, c.here.Kind
		b.mu.Unlock()
		attrs := []any{"screen", name, "kind", kind}
		for k, v := range req {
			if len(k) > 24 {
				continue
			}
			switch v.(type) {
			case float64, bool:
				attrs = append(attrs, k, v)
			case string:
				attrs = append(attrs, k, cleanLabel(v.(string)))
			}
		}
		slog.Info("sync report", attrs...)
	case "sync.leave":
		var req struct {
			Room string `json:"room"`
		}
		if json.Unmarshal(m.Data, &req) != nil || !b.setMember(c, req.Room, false) {
			return
		}
		b.Rooms.Leave(req.Room)
		b.roomChanged(req.Room)
	case "sync.command":
		var req struct {
			Room string `json:"room"`
			Command
		}
		if json.Unmarshal(m.Data, &req) != nil || !b.isMember(c, req.Room) {
			return
		}
		var floor string
		if req.Action == "latency" {
			floor = b.appleLatencyFor(req.Room)
		}
		b.mu.Lock()
		if floor != "" && latencyMS(req.Latency) < latencyMS(floor) && b.appleInLocked(req.Room) {
			req.Latency = floor
		}
		st, err := b.Rooms.Apply(req.Room, req.Command)
		if err == nil {
			b.publishRoomLocked(st)
		}
		b.mu.Unlock()
		if err != nil {
			b.reply(c, "error", map[string]string{"code": "sync", "message": err.Error()})
		}
	default:
		slog.Info(fmt.Sprintf("realtime: unknown message %q", m.Type))
	}
}

// setMember changes membership and reports whether anything changed.
// A screen joins a handful of rooms (a quad joins five). The caps stop one
// socket from growing memory without end or flooding every screen with
// groups.changed.
const (
	maxClientRooms = 32
	maxGroups      = 64
)

func (b *Bus) setMember(c *client, room string, in bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.rooms[room] == in {
		return false
	}
	if in && len(c.rooms) >= maxClientRooms {
		return false
	}
	if in && groupRoom(room) && b.groupCountLocked(room) >= maxGroups {
		return false
	}
	if in {
		c.rooms[room] = true
	} else {
		delete(c.rooms, room)
	}
	return true
}

// groupCountLocked counts the group rooms screens are in, or 0 when room is
// already one of them.
func (b *Bus) groupCountLocked(room string) int {
	groups := map[string]bool{}
	for c := range b.clients {
		for r := range c.rooms {
			if r == room {
				return 0
			}
			if groupRoom(r) {
				groups[r] = true
			}
		}
	}
	return len(groups)
}

func (b *Bus) isMember(c *client, room string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return c.rooms[room]
}

func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
		n++
		if n >= 64 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func validRoom(room string) bool {
	if len(room) > 64 {
		return false
	}
	if code, ok := strings.CutPrefix(room, "group:"); ok {
		return validCode(code)
	}
	return strings.HasPrefix(room, "channel:") || strings.HasPrefix(room, "multiview:")
}

// validCode keeps group codes to what a person can read out and type.
func validCode(code string) bool {
	if code == "" || len(code) > 32 {
		return false
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func groupRoom(room string) bool {
	return strings.HasPrefix(room, "group:")
}

// appleLatency is the closest to live a room with an Apple screen plays.
const appleLatency = "balanced"

// appleLatencyFor is appleLatency for a room's channel. A channel with long
// groups of pictures holds AVPlayer 12 s back from a live edge that is
// itself a group old, so at balanced 1.02x does nothing and the screen
// falls behind the room.
func (b *Bus) appleLatencyFor(room string) string {
	if b.LongHold == nil {
		return appleLatency
	}
	if st, ok := b.Rooms.State(room); ok && b.LongHold(st.ChannelID) {
		return "stable"
	}
	return appleLatency
}

func (b *Bus) appleInLocked(room string) bool {
	for c := range b.clients {
		if c.rooms[room] && appleKind(c.here.Kind) {
			return true
		}
	}
	return false
}

func appleKind(kind string) bool {
	return kind == "iphone" || kind == "ipad" || kind == "appletv"
}
