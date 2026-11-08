package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

var (
	// fillRetryAfter is how long a copy's fills rest after one died on the
	// processor before the next request tries again.
	fillRetryAfter = 30 * time.Second
	// fillFirstWithin is how long a fill encode has to write its first
	// segment. One that hasn't is stopped, and the next request starts afresh.
	fillFirstWithin = 10 * time.Second
)

// fillAhead is how far past what a fill encode has written a request can be
// and still wait for it, in segments, instead of starting a new one there.
const fillAhead = 4

// fillRunsMax is how many fill encodes one recording runs at once: two
// screens going back to different places each keep theirs.
const fillRunsMax = 2

var (
	// ErrFillGone is a fill segment nothing can make.
	ErrFillGone = errors.New("fill segment cannot be made")
	// ErrFillLate is one not written yet. Asking again may find it.
	ErrFillLate = errors.New("fill segment is not ready")
)

// fillSource is what a recording's fill encode reads and how. It is kept in
// the copy's folder too, so a restart doesn't lose it. failed is when a fill
// last died on the processor.
type fillSource struct {
	Path   string  `json:"path"`
	Graph  Graph   `json:"graph"`
	Offset float64 `json:"offset"`
	failed time.Time
}

type fillRun struct {
	first int
	list  string
	cmd   *exec.Cmd
	done  chan struct{}
	// seen, stopped, and hung (stopped for a late first segment) are under playMu.
	seen    time.Time
	stopped bool
	hung    bool
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

// setFillSource notes how to fill the copy in dir. The same copy keeps what
// it learned, a move to the processor included.
func (h *Hub) setFillSource(id int64, dir string, src fillSource) {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if !fillable(src.Offset) {
		delete(h.fillSrc, id)
		return
	}
	if old, ok := h.fillSrc[id]; ok && old.Path == src.Path && old.Offset == src.Offset {
		return
	}
	if h.fillSrc == nil {
		h.fillSrc = map[int64]fillSource{}
	}
	h.fillSrc[id] = src
	writeFillSource(dir, src)
}

func writeFillSource(dir string, src fillSource) {
	body, err := json.Marshal(src)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "fill.json"), body, 0o644)
}

// fillSourceFor is the recording's fill source, read from its copy's folder
// when this run hasn't played it.
func (h *Hub) fillSourceFor(id int64, dir string) (fillSource, bool) {
	h.playMu.Lock()
	src, ok := h.fillSrc[id]
	h.playMu.Unlock()
	if ok {
		return src, true
	}
	if src, ok = readFillSource(dir); !ok {
		if src, ok = h.fillSourceFromCopy(id, dir); !ok {
			return fillSource{}, false
		}
		writeFillSource(dir, src)
	}
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if cur, ok := h.fillSrc[id]; ok {
		return cur, true
	}
	if h.fillSrc == nil {
		h.fillSrc = map[int64]fillSource{}
	}
	h.fillSrc[id] = src
	return src, true
}

func readFillSource(dir string) (fillSource, bool) {
	body, err := os.ReadFile(filepath.Join(dir, "fill.json"))
	if err != nil {
		return fillSource{}, false
	}
	var src fillSource
	if json.Unmarshal(body, &src) != nil || src.Path == "" || !fillable(src.Offset) || math.Abs(src.Offset-FileOffset(dir)) > 0.0005 {
		return fillSource{}, false
	}
	return src, true
}

// fillSourceFromCopy rebuilds what a copy made before fill.json needs: the
// recording's file from the catalog, and the picture its graph.txt names,
// which has to come out the same.
func (h *Hub) fillSourceFromCopy(id int64, dir string) (fillSource, bool) {
	offset := FileOffset(dir)
	body, err := os.ReadFile(filepath.Join(dir, "graph.txt"))
	if h.Store == nil || !fillable(offset) || err != nil {
		return fillSource{}, false
	}
	stamp := strings.TrimSpace(string(body))
	parts := strings.Split(stamp, "|")
	if len(parts) < 6 {
		return fillSource{}, false
	}
	rec, err := h.Store.Recording(context.Background(), id)
	if err != nil || rec.Path == "" {
		return fillSource{}, false
	}
	path, err := filepath.Abs(rec.Path)
	if err != nil {
		return fillSource{}, false
	}
	order := ""
	if parts[5] == "progressive" || parts[5] == "film" {
		order = parts[5]
	}
	g := h.fileGraphFor(path, parts[1], parts[0], order)
	g.Live = false
	g.Encoder, g.Deint, g.Profile = parts[2], parts[3], parts[4]
	if graphStamp(g) != stamp {
		return fillSource{}, false
	}
	return fillSource{Path: path, Graph: g, Offset: offset}, true
}

// FillSegment is the path of fill segment n of a recording's copy, encoding
// it if it isn't written. ErrFillGone means nothing can make it, ErrFillLate
// that it didn't come in time, and a cancelled ctx ends the wait.
func (h *Hub) FillSegment(ctx context.Context, id int64, n int) (string, error) {
	dir := filepath.Join(h.Dir, "file", strconv.FormatInt(id, 10))
	path := filepath.Join(dir, fillName(n))
	h.playMu.Lock()
	if h.fillSeen == nil {
		h.fillSeen = map[int64]time.Time{}
	}
	h.fillSeen[id] = time.Now()
	h.playMu.Unlock()
	if n < 0 {
		return "", ErrFillGone
	}
	if segmentReady(dir, fillName(n)) {
		return path, nil
	}
	deadline := time.NewTimer(fillWait)
	defer deadline.Stop()
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	// A run that exits without the segment is tried again: the next one may
	// be on the processor.
	for tries := 0; tries < 3; tries++ {
		run, err := h.ensureFill(id, dir, n)
		if err != nil {
			return "", err
		}
	wait:
		for {
			if segmentReady(dir, fillName(n)) {
				return path, nil
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-deadline.C:
				return "", ErrFillLate
			case <-run.done:
				break wait
			case <-tick.C:
			}
		}
		if segmentReady(dir, fillName(n)) {
			return path, nil
		}
	}
	return "", ErrFillLate
}

func (h *Hub) ensureFill(id int64, dir string, n int) (*fillRun, error) {
	defer h.lockFill(id)()
	src, ok := h.fillSourceFor(id, dir)
	h.playMu.Lock()
	closed := h.fileClosed
	var live []*fillRun
	for _, r := range h.fillRuns[id] {
		if !r.ended() {
			live = append(live, r)
		}
	}
	var found *fillRun
	for _, r := range live {
		if r.covers(dir, n) {
			found = r
			r.seen = time.Now()
			break
		}
	}
	h.playMu.Unlock()
	switch {
	case closed:
		return nil, ErrFillLate
	case !ok || time.Since(src.failed) < fillRetryAfter || float64(2*n) >= src.Offset-0.001:
		return nil, ErrFillGone
	case found != nil:
		return found, nil
	}
	// The run asked for longest ago makes room.
	if len(live) >= fillRunsMax {
		oldest := live[0]
		h.playMu.Lock()
		for _, r := range live[1:] {
			if r.seen.Before(oldest.seen) {
				oldest = r
			}
		}
		h.playMu.Unlock()
		h.stopFillRun(id, oldest)
	}
	run, err := h.startFill(id, dir, src, n)
	if err != nil {
		slog.Warn(fmt.Sprintf("recording %d: fill: %v", id, err))
		return nil, ErrFillGone
	}
	return run, nil
}

func (r *fillRun) ended() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// covers reports a run that will reach segment n soon.
func (r *fillRun) covers(dir string, n int) bool {
	written := segmentCount(filepath.Join(dir, r.list))
	return n >= r.first && n <= r.first+written+fillAhead
}

func (h *Hub) startFill(id int64, dir string, src fillSource, n int) (*fillRun, error) {
	g := src.Graph
	g.Input = src.Path
	g.Carry = 0
	g.Start = float64(2 * n)
	g.Shift = g.Start
	g.Until = src.Offset - g.Start
	g.Name = "fill"
	g.First = n
	// Each run lists what it wrote in its own file.
	g.List = fmt.Sprintf("fill-%05d.m3u8", n)
	_ = os.Remove(filepath.Join(dir, g.List))
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	logCommand(cmd, "playback fill "+filepath.Base(dir))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	run := &fillRun{first: n, list: g.List, cmd: cmd, done: make(chan struct{}), seen: time.Now()}
	h.playMu.Lock()
	if h.fillRuns == nil {
		h.fillRuns = map[int64][]*fillRun{}
	}
	h.fillRuns[id] = append(h.fillRuns[id], run)
	h.playMu.Unlock()
	go func() {
		_ = cmd.Wait()
		wrote := segmentReady(dir, fillName(n))
		_ = os.Remove(filepath.Join(dir, run.list))
		h.playMu.Lock()
		h.dropFillRunLocked(id, run)
		if !wrote && (!run.stopped || run.hung) {
			h.fillFailedLocked(id, dir, g.Encoder, run.hung)
		}
		h.playMu.Unlock()
		close(run.done)
	}()
	// Requests keep a run from going idle, so one that never writes would be
	// waited on for good.
	time.AfterFunc(fillFirstWithin, func() {
		if run.ended() || segmentReady(dir, fillName(n)) {
			return
		}
		slog.Warn(fmt.Sprintf("recording %d: a fill wrote nothing in %v, stopping it", id, fillFirstWithin))
		h.playMu.Lock()
		run.hung = true
		h.playMu.Unlock()
		h.stopFillRun(id, run)
	})
	go h.watchFill(id, run)
	return run, nil
}

// fillFailedLocked notes a fill encode that ended before its first segment.
// One on the GPU moves to the processor, as the copy does. One that died on
// the processor rests the copy's fills for fillRetryAfter; one that hung there
// was stopped, and the next request starts another.
func (h *Hub) fillFailedLocked(id int64, dir, encoder string, hung bool) {
	src, ok := h.fillSrc[id]
	if !ok || src.Graph.Encoder != encoder {
		return
	}
	if gpuEncoder(encoder) {
		slog.Warn(fmt.Sprintf("recording %d: a fill did not start on the GPU. Trying the processor.", id))
		src.Graph = softwareGraph(src.Graph)
		writeFillSource(dir, src)
	} else if !hung {
		slog.Warn(fmt.Sprintf("recording %d: a fill did not start on the processor.", id))
		src.failed = time.Now()
	}
	h.fillSrc[id] = src
}

func (h *Hub) dropFillRunLocked(id int64, run *fillRun) {
	runs := h.fillRuns[id]
	for i, r := range runs {
		if r == run {
			h.fillRuns[id] = append(runs[:i:i], runs[i+1:]...)
			break
		}
	}
	if len(h.fillRuns[id]) == 0 {
		delete(h.fillRuns, id)
	}
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
		quiet := time.Since(run.seen) >= idle
		h.playMu.Unlock()
		if quiet {
			h.stopFillRun(id, run)
		}
	}
}

// stopFill ends the recording's fill encodes and waits for them to exit.
func (h *Hub) stopFill(id int64) {
	h.playMu.Lock()
	runs := append([]*fillRun(nil), h.fillRuns[id]...)
	h.playMu.Unlock()
	for _, run := range runs {
		h.stopFillRun(id, run)
	}
}

func (h *Hub) stopFillRun(id int64, run *fillRun) {
	h.playMu.Lock()
	run.stopped = true
	h.dropFillRunLocked(id, run)
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
