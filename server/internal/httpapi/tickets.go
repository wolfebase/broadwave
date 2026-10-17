package httpapi

import (
	"sync"
	"time"
)

// A ticket names one watch request. A page that leaves before it reads the
// watch answer does not know the rendition to stop, and the server may already
// have counted that viewer: the answer was written, but the page never read it.
// The page sends the ticket in its stop instead, so that viewer is released at
// once and not after the abandoned-viewer sweep (45 s without a fetch).
//
// Exactly one release happens per counted ticket: the watch handler's own
// (a cancelled or failed start) or the stop's, whichever comes first. A stop
// that arrives before the viewer is counted leaves the ticket cancelled, and the
// watch releases its viewer as soon as it counts it.

// ticketKeep is how long a ticket is remembered. A stop by ticket comes from a
// page leaving within moments of its watch.
const ticketKeep = 2 * time.Minute

// ticketLen bounds a ticket so a client cannot grow the book with long keys.
const ticketLen = 64

type ticketState int

const (
	ticketCounted ticketState = iota
	ticketCancelled
	ticketDone
)

type ticket struct {
	state   ticketState
	channel int64
	key     string
	at      time.Time
}

type ticketBook struct {
	mu   sync.Mutex
	list map[string]*ticket
}

func validTicket(id string) bool {
	return id != "" && len(id) <= ticketLen
}

func (b *ticketBook) pruneLocked(now time.Time) {
	for id, t := range b.list {
		if now.Sub(t.at) > ticketKeep {
			delete(b.list, id)
		}
	}
}

// count records the viewer a watch counted. It reports false when the page's
// stop already came: the caller releases that viewer itself.
func (b *ticketBook) count(id string, channel int64, key string) bool {
	if !validTicket(id) {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.pruneLocked(now)
	if b.list == nil {
		b.list = map[string]*ticket{}
	}
	if t := b.list[id]; t != nil && t.state != ticketCounted {
		t.state = ticketDone
		return false
	}
	b.list[id] = &ticket{state: ticketCounted, channel: channel, key: key, at: now}
	return true
}

// take reports whether the caller is the one to release a counted viewer. The
// watch handler calls it before releasing on a failed or cancelled start.
func (b *ticketBook) take(id string) bool {
	if !validTicket(id) {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.list[id]
	if t == nil {
		return true
	}
	if t.state != ticketCounted {
		return false
	}
	t.state = ticketDone
	return true
}

// stop is a page's stop by ticket for a channel. It returns the rendition to
// release when the watch counted a viewer on that channel that nobody released
// yet; otherwise it remembers the ticket as cancelled for a watch still on its way.
func (b *ticketBook) stop(id string, channel int64) (key string, ok bool) {
	if !validTicket(id) {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.pruneLocked(now)
	if b.list == nil {
		b.list = map[string]*ticket{}
	}
	t := b.list[id]
	if t == nil {
		b.list[id] = &ticket{state: ticketCancelled, channel: channel, at: now}
		return "", false
	}
	if t.state != ticketCounted || t.channel != channel {
		return "", false
	}
	t.state = ticketDone
	return t.key, true
}
