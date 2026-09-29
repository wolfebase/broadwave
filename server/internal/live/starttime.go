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

// StartRecord is how long one picture took to start, in seconds from the
// watch that asked for it. Tune is zero when the frequency was already tuned.
type StartRecord struct {
	At          time.Time `json:"at"`
	ChannelID   int64     `json:"channelId"`
	GuideNumber string    `json:"guideNumber"`
	Rendition   string    `json:"rendition"`
	Seconds     float64   `json:"seconds"`
	Tune        float64   `json:"tune,omitempty"`
	Keyframe    float64   `json:"keyframe"`
	Encoder     float64   `json:"encoder"`
	Segment     float64   `json:"segment"`
}

// keptStarts is how many StartRecords Diagnostics lists.
const keptStarts = 10

// NoteStart records a picture that started after since and returns the log
// line naming each step from since: the tune when it began after since
// (tuner status read, lock, first byte), then the encode (first input, first
// output, first part, first segment). It is empty for a picture that was
// already running.
func (h *Hub) NoteStart(channelID int64, key string, since time.Time) string {
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
	var locked time.Time
	if m := muxOf(h, f); m != nil && !m.tuneBegan.Before(since) {
		locked = m.tuneLocked
		step("status", m.tuneStatus)
		step("lock", m.tuneLocked)
		step("first byte", m.firstByte.at())
	}
	input := r.fed.at()
	var output, segment time.Time
	step("encode", r.began)
	step("input", input)
	if r.input != nil {
		output = r.input.firstRead.at()
		step("output", output)
	}
	if r.gate != nil {
		segment = r.gate.firstSegment.at()
		step("part", r.gate.firstPart.at())
		step("segment", segment)
	}
	if !r.noted && !input.IsZero() && !output.IsZero() && !segment.IsZero() {
		r.noted = true
		rec := StartRecord{
			At: since, ChannelID: channelID, GuideNumber: f.channel.GuideNumber, Rendition: key,
			Seconds:  segment.Sub(since).Seconds(),
			Keyframe: input.Sub(r.began).Seconds(),
			Encoder:  output.Sub(input).Seconds(),
			Segment:  segment.Sub(output).Seconds(),
		}
		if !locked.IsZero() {
			rec.Tune = locked.Sub(since).Seconds()
		}
		h.starts = append(h.starts, rec)
		if len(h.starts) > keptStarts {
			h.starts = h.starts[len(h.starts)-keptStarts:]
		}
	}
	return fmt.Sprintf("live: %s %s started: %s s", f.channel.GuideNumber, key, strings.Join(steps, ", "))
}

// RecentStarts are the last pictures a watch started, newest first.
func (h *Hub) RecentStarts() []StartRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]StartRecord, 0, len(h.starts))
	for i := len(h.starts) - 1; i >= 0; i-- {
		out = append(out, h.starts[i])
	}
	return out
}

// noteTune records a tune's steps. The caller holds h.mu.
func noteTune(m *mux, began, status, locked time.Time) {
	if m != nil {
		m.tuneBegan, m.tuneStatus, m.tuneLocked = began, status, locked
	}
}
