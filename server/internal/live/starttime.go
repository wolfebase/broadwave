package live

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

// stamp is the first time something happened. Any goroutine may mark it.
type stamp struct{ ns atomic.Int64 }

func (s *stamp) mark() { s.ns.CompareAndSwap(0, time.Now().UnixNano()) }

func (s *stamp) at() time.Time {
	if n := s.ns.Load(); n != 0 {
		return time.Unix(0, n)
	}
	return time.Time{}
}

// markWriter marks its stamp on the first bytes written through it.
type markWriter struct {
	io.WriteCloser
	s *stamp
}

func (w markWriter) Write(b []byte) (int, error) {
	if len(b) > 0 {
		w.s.mark()
	}
	return w.WriteCloser.Write(b)
}

// StartTimes names the steps of a picture that started after since, each
// measured from since: the tune when it began after since (tuner status read,
// lock, first byte), then the encode (first input, first output, first part,
// first segment). It is empty for a picture that was already running.
func (h *Hub) StartTimes(channelID int64, key string, since time.Time) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil {
		return ""
	}
	r := f.renditions[key]
	if r == nil || r.began.Before(since) {
		return ""
	}
	var steps []string
	step := func(name string, t time.Time) {
		if t.IsZero() {
			steps = append(steps, name+" -")
			return
		}
		steps = append(steps, fmt.Sprintf("%s %.2f", name, t.Sub(since).Seconds()))
	}
	if m := muxOf(h, f); m != nil && !m.tuneBegan.Before(since) {
		step("status", m.tuneStatus)
		step("lock", m.tuneLocked)
		step("first byte", m.firstByte.at())
	}
	step("encode", r.began)
	step("input", r.fed.at())
	if r.input != nil {
		step("output", r.input.firstRead.at())
	}
	if r.gate != nil {
		step("part", r.gate.firstPart.at())
		step("segment", r.gate.firstSegment.at())
	}
	return fmt.Sprintf("live: %s %s started: %s s", f.channel.GuideNumber, key, strings.Join(steps, ", "))
}

// noteTune records a tune's steps. The caller holds h.mu.
func noteTune(m *mux, began, status, locked time.Time) {
	if m != nil {
		m.tuneBegan, m.tuneStatus, m.tuneLocked = began, status, locked
	}
}
