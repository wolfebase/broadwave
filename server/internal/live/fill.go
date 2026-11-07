package live

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// A copy that starts at a resume lists the recording before it as fill
// segments, two seconds each on the recording's clock. A player that goes
// back there asks for one, and a second encode writes that stretch beside the
// copy, from the asked-for segment up to the resume. The copy and its encode
// are not touched, and the list puts a discontinuity between the two clocks.

// fillWait is how long a request for a fill segment waits for its encode.
var fillWait = 12 * time.Second

// fillAhead is how far past what a fill encode has written a request can be
// and still wait for it, in segments, instead of starting a new one there.
const fillAhead = 4

// fillSource is what a recording's fill encode reads and how.
type fillSource struct {
	path   string
	graph  Graph
	offset float64
}

type fillRun struct {
	first int
	cmd   *exec.Cmd
	done  chan struct{}
}

// fillName is fill segment n.
func fillName(n int) string {
	return fmt.Sprintf("fill%05d.ts", n)
}

// fillable reports a copy start that fill segments can cover: on the
// two-second grid, so each one is a whole stretch an encode can cut.
func fillable(offset float64) bool {
	if offset < resumeMin {
		return false
	}
	return math.Abs(offset/2-math.Round(offset/2)) < 0.0005
}

// gridStart is where a copy for a resume at at starts: on the two-second grid
// at or before it, so the stretch before is whole fill segments.
func gridStart(at float64) float64 {
	if at < resumeMin {
		return 0
	}
	return max(float64(int(at/2))*2, resumeMin)
}

func (h *Hub) setFillSource(id int64, src fillSource) {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if !fillable(src.offset) {
		delete(h.fillSrc, id)
		return
	}
	if h.fillSrc == nil {
		h.fillSrc = map[int64]fillSource{}
	}
	h.fillSrc[id] = src
}

// FillSegment is the path of fill segment n of a recording's copy, encoding
// it if it isn't written. Empty means it could not be made in time.
func (h *Hub) FillSegment(id int64, n int) string {
	dir := filepath.Join(h.Dir, "file", strconv.FormatInt(id, 10))
	h.playMu.Lock()
	if h.fillSeen == nil {
		h.fillSeen = map[int64]time.Time{}
	}
	h.fillSeen[id] = time.Now()
	h.playMu.Unlock()
	if n < 0 {
		return ""
	}
	if segmentReady(dir, fillName(n)) {
		return filepath.Join(dir, fillName(n))
	}
	if !h.ensureFill(id, dir, n) {
		return ""
	}
	deadline := time.Now().Add(fillWait)
	for {
		if segmentReady(dir, fillName(n)) {
			return filepath.Join(dir, fillName(n))
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func (h *Hub) ensureFill(id int64, dir string, n int) bool {
	defer h.lockFill(id)()
	h.playMu.Lock()
	src, ok := h.fillSrc[id]
	run := h.fillRuns[id]
	closed := h.fileClosed
	h.playMu.Unlock()
	if !ok || closed || float64(2*n) >= src.offset-0.001 {
		return false
	}
	if run != nil && run.covers(dir, n) {
		return true
	}
	h.stopFill(id)
	if err := h.startFill(id, dir, src, n); err != nil {
		slog.Warn(fmt.Sprintf("recording %d: fill: %v", id, err))
		return false
	}
	return true
}

// covers reports a run still going that will reach segment n soon.
func (r *fillRun) covers(dir string, n int) bool {
	select {
	case <-r.done:
		return false
	default:
	}
	written := segmentCount(filepath.Join(dir, "fill.m3u8"))
	return n >= r.first && n <= r.first+written+fillAhead
}

func (h *Hub) startFill(id int64, dir string, src fillSource, n int) error {
	g := src.graph
	g.Input = src.path
	g.Carry = 0
	g.Start = float64(2 * n)
	g.Shift = g.Start
	g.Until = src.offset - g.Start
	g.Name = "fill"
	g.First = n
	// The list of the run before would say this one has written its segments.
	_ = os.Remove(filepath.Join(dir, "fill.m3u8"))
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	logCommand(cmd, "playback fill "+filepath.Base(dir))
	if err := cmd.Start(); err != nil {
		return err
	}
	run := &fillRun{first: n, cmd: cmd, done: make(chan struct{})}
	h.playMu.Lock()
	if h.fillRuns == nil {
		h.fillRuns = map[int64]*fillRun{}
	}
	h.fillRuns[id] = run
	h.playMu.Unlock()
	go func() {
		_ = cmd.Wait()
		h.playMu.Lock()
		if h.fillRuns[id] == run {
			delete(h.fillRuns, id)
		}
		h.playMu.Unlock()
		close(run.done)
	}()
	go h.watchFill(id, run)
	return nil
}

// watchFill stops a fill encode nobody has asked for lately.
func (h *Hub) watchFill(id int64, run *fillRun) {
	idle := h.FileIdle
	if idle <= 0 {
		idle = fileIdleAfter
	}
	tick := time.NewTicker(min(idle/4, 5*time.Second))
	defer tick.Stop()
	for {
		select {
		case <-run.done:
			return
		case <-tick.C:
		}
		h.playMu.Lock()
		quiet := h.fillRuns[id] == run && time.Since(h.fillSeen[id]) >= idle
		h.playMu.Unlock()
		if quiet {
			h.stopFillRun(id, run)
		}
	}
}

// stopFill ends the recording's fill encode and waits for it to exit.
func (h *Hub) stopFill(id int64) {
	h.playMu.Lock()
	run := h.fillRuns[id]
	h.playMu.Unlock()
	if run != nil {
		h.stopFillRun(id, run)
	}
}

func (h *Hub) stopFillRun(id int64, run *fillRun) {
	h.playMu.Lock()
	if h.fillRuns[id] == run {
		delete(h.fillRuns, id)
	}
	h.playMu.Unlock()
	if run.cmd.Process != nil {
		_ = run.cmd.Process.Kill()
	}
	select {
	case <-run.done:
	case <-time.After(3 * time.Second):
	}
}

func (h *Hub) stopFills() {
	h.playMu.Lock()
	ids := make([]int64, 0, len(h.fillRuns))
	for id := range h.fillRuns {
		ids = append(ids, id)
	}
	h.playMu.Unlock()
	for _, id := range ids {
		h.stopFill(id)
	}
}

func (h *Hub) lockFill(id int64) func() {
	h.playMu.Lock()
	if h.fillLock == nil {
		h.fillLock = map[int64]*sync.Mutex{}
	}
	mu := h.fillLock[id]
	if mu == nil {
		mu = &sync.Mutex{}
		h.fillLock[id] = mu
	}
	h.playMu.Unlock()
	mu.Lock()
	return mu.Unlock
}
