package realtime

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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

	mu      sync.Mutex
	clients map[*client]struct{}
	now     func() time.Time
}

type client struct {
	send  chan []byte
	rooms map[string]bool
}

func NewBus() *Bus {
	return &Bus{Rooms: NewRooms(), clients: map[*client]struct{}{}, now: time.Now}
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

func (b *Bus) publishRoom(st RoomState) {
	msg := frame("sync.state", st)
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.clients {
		if c.rooms[st.Room] {
			select {
			case c.send <- msg:
			default:
			}
		}
	}
}

// Clients reports how many sockets are connected.
func (b *Bus) Clients() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients)
}

// ServeHTTP upgrades to the event socket.
func (b *Bus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	c := &client{send: make(chan []byte, 64), rooms: map[string]bool{}}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, c)
		b.mu.Unlock()
		for room := range c.rooms {
			b.Rooms.Leave(room)
			if st, ok := b.Rooms.State(room); ok {
				b.publishRoom(st)
			}
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
	c.send <- frame("hello", map[string]any{"serverTime": unixMS(b.now())})
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
		}
		if json.Unmarshal(m.Data, &req) != nil || !validRoom(req.Room) || !b.setMember(c, req.Room, true) {
			return
		}
		st := b.Rooms.Join(req.Room, req.ChannelID)
		b.publishRoom(st)
	case "sync.leave":
		var req struct {
			Room string `json:"room"`
		}
		if json.Unmarshal(m.Data, &req) != nil || !b.setMember(c, req.Room, false) {
			return
		}
		b.Rooms.Leave(req.Room)
		if st, ok := b.Rooms.State(req.Room); ok {
			b.publishRoom(st)
		}
	case "sync.command":
		var req struct {
			Room string `json:"room"`
			Command
		}
		if json.Unmarshal(m.Data, &req) != nil || !b.isMember(c, req.Room) {
			return
		}
		st, err := b.Rooms.Apply(req.Room, req.Command)
		if err != nil {
			b.reply(c, "error", map[string]string{"code": "sync", "message": err.Error()})
			return
		}
		b.publishRoom(st)
	default:
		log.Printf("realtime: unknown message %q", m.Type)
	}
}

// setMember changes membership and reports whether anything changed.
func (b *Bus) setMember(c *client, room string, in bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.rooms[room] == in {
		return false
	}
	if in {
		c.rooms[room] = true
	} else {
		delete(c.rooms, room)
	}
	return true
}

func (b *Bus) isMember(c *client, room string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return c.rooms[room]
}

func validRoom(room string) bool {
	if len(room) > 64 {
		return false
	}
	return strings.HasPrefix(room, "channel:") || strings.HasPrefix(room, "group:") || strings.HasPrefix(room, "multiview:")
}
