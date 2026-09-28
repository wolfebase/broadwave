package live

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/ring"
)

// ringFloor is the least free space a ring leaves on its disk, plus the
// recordings reserve when the recordings are on the same disk.
const ringFloor = 4 << 30

// clearRings removes rings a previous process left behind. A ring lives and
// dies with its tune.
func clearRings(dir string) {
	if dir != "" {
		_ = os.RemoveAll(filepath.Join(dir, "ring"))
	}
}

func (h *Hub) openRingLocked(m *mux) {
	if h.Buffer <= 0 || h.Dir == "" {
		return
	}
	h.ringSeq++
	dir := filepath.Join(h.Dir, "ring", fmt.Sprintf("%d-%d", m.freq, h.ringSeq))
	m.ring = ring.Open(dir, ring.Options{Window: h.Buffer, Room: h.ringRoom(dir)})
}

// ringRoom is how many more bytes a ring may hold: at most half of the free
// space it would have without itself, and never the last ringFloor.
// Recordings usually have their own mount, and the ring must not fill the
// disk that holds the catalog and the other apps either.
func (h *Hub) ringRoom(dir string) func(held int64) int64 {
	return func(held int64) int64 {
		space, err := disk.Stat(dir)
		if err != nil {
			return 1 << 62
		}
		floor := uint64(ringFloor)
		if h.Store != nil && disk.SameDevice(dir, filepath.Join(h.Dir, "recordings")) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			values, err := h.Store.Settings(ctx)
			cancel()
			if err == nil {
				floor += disk.WatermarkBytes(values["watermarkGB"])
			}
		}
		free := int64(space.Free)
		return min(free-int64(floor), (free+held)/2-held)
	}
}

// ringStart is the byte where a recording asked to start at from begins, and
// the time of that byte. A show that began before the tune starts at the tune.
func ringStart(m *mux, from, now time.Time) (int64, time.Time, bool) {
	if m == nil || m.ring == nil || m.input != "" || from.IsZero() || !from.Before(now) {
		return 0, time.Time{}, false
	}
	pos, at, ok := m.ring.Position(from)
	if !ok || !at.Before(now) {
		return 0, time.Time{}, false
	}
	return pos, at, true
}

// backfill writes the ring from pos into a recording that started after its
// show did, then hands the recording to the live fan-out at the byte the copy
// reached, so nothing is missed or written twice. Files are read outside
// every lock; under pipeMu only bytes still in memory are copied, and if the
// ring moved them to disk meanwhile the copy runs again.
func (h *Hub) backfill(m *mux, rec *recording, pos int64) {
	var sub *pipeSub
	for sub == nil {
		n, err := m.ring.Copy(rec.stdin, pos, m.ring.Received())
		pos += n
		switch {
		case errors.Is(err, ring.ErrGone):
			next, _, ok := m.ring.Position(time.Time{})
			if !ok || next <= pos {
				// The ring closed, started over, or cannot read its file.
				slog.Warn(fmt.Sprintf("mux %d: recording %d joined live with a gap: %v", m.freq, rec.id, err))
				sub = h.attachPipe(m, rec.stdin, false)
				continue
			}
			slog.Warn(fmt.Sprintf("mux %d: recording %d skipped %d MB the buffer had already dropped", m.freq, rec.id, (next-pos)>>20))
			pos = next
			continue
		case err != nil:
			// The recording closed its input.
			h.endBackfill(m, rec, nil)
			return
		}
		sub = h.attachPipeHead(m, rec.stdin, func() ([]byte, bool) { return m.ring.PendingFrom(pos) })
	}
	h.endBackfill(m, rec, sub)
}

// endBackfill gives the recording its live subscriber and its stop timer,
// which counts from the end the recording asked for. A recording that ended
// meanwhile lets go of sub.
func (h *Hub) endBackfill(m *mux, rec *recording, sub *pipeSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec.backfilling = false
	if rec.finished {
		m.detach(sub)
		return
	}
	rec.sub = sub
	id := rec.id
	rec.timer = time.AfterFunc(time.Until(rec.ends), func() { h.StopRecord(id) })
}
