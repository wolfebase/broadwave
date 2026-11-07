package live

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"broadwave/internal/disk"
)

// A played recording that needs a new picture is encoded into work/file/<id>.
// The encode runs only while players ask for it, and the copies are removed by
// age and total size, so a moment of playback does not cost a recording's
// length of disk for as long as the recording is kept.
var (
	// fileKeepFor is how long a copy nobody asks for is kept. It outlasts a
	// viewer pausing overnight.
	fileKeepFor = 24 * time.Hour
	// fileKeepBytes is the most the copies may use together. A copy runs about
	// 4.4 GB an hour.
	fileKeepBytes = uint64(10 << 30)
	// fileLimitIdle protects a paused viewer from the size limit.
	fileLimitIdle = time.Hour
	// fileSweepEvery is the wait between passes.
	fileSweepEvery = 10 * time.Minute
)

// fileIdleAfter is how long after the last request the encode stops. A player
// with a full buffer asks for nothing for a while, and the segments it asks for
// next are already written, so a stop it never sees is harmless.
const fileIdleAfter = 30 * time.Second

// fileEndMargin is how close to the end of a file a seek can land and find no
// picture to encode.
const fileEndMargin = 5

// FileRequested notes a player asking for a recording's playlist or a segment.
// It keeps the encode running, and carries on one that stopped.
func (h *Hub) FileRequested(id int64) {
	h.playMu.Lock()
	h.noteFileRequestLocked(id)
	job := h.fileJobs[id]
	start := job != nil && h.fileEnc[id] == nil && !job.failed && !job.resuming && !h.fileClosed
	if start {
		job.resuming = true
	}
	h.playMu.Unlock()
	if start {
		go h.resumeFile(id, job)
	}
}

// FileEncoding reports an ffmpeg writing this recording's copy now.
func (h *Hub) FileEncoding(id int64) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	return h.fileEnc[id] != nil
}

func (h *Hub) noteFileRequestLocked(id int64) {
	if h.fileSeen == nil {
		h.fileSeen = map[int64]time.Time{}
	}
	h.fileSeen[id] = time.Now()
}

func (h *Hub) resumeFile(id int64, job *fileJob) {
	defer h.lockFileStart(id)()
	h.playMu.Lock()
	job.resuming = false
	current := h.fileJobs[id] == job && h.fileEnc[id] == nil && !job.failed && !h.fileClosed
	h.playMu.Unlock()
	if current {
		h.carryOn(id, filepath.Join(h.Dir, "file", strconv.FormatInt(id, 10)), job)
	}
}

func (h *Hub) keepFileJob(id int64, job *fileJob) {
	h.playMu.Lock()
	if h.fileJobs == nil {
		h.fileJobs = map[int64]*fileJob{}
	}
	h.fileJobs[id] = job
	h.playMu.Unlock()
}

// forgetFileJob drops a finished job. A newer play's job stays.
func (h *Hub) forgetFileJob(id int64, job *fileJob) {
	h.playMu.Lock()
	if h.fileJobs[id] == job {
		delete(h.fileJobs, id)
	}
	h.playMu.Unlock()
}

func (h *Hub) markFileJobFailed(job *fileJob) {
	h.playMu.Lock()
	job.failed = true
	h.playMu.Unlock()
}

// forgetFileCopy drops what memory holds about a recording's copy.
func (h *Hub) forgetFileCopy(id int64) {
	dir := filepath.Join(h.Dir, "file", strconv.FormatInt(id, 10))
	h.playMu.Lock()
	delete(h.fileJobs, id)
	delete(h.fileSeen, id)
	delete(h.fileList, dir)
	h.playMu.Unlock()
}

// watchFileRequests stops the encode once nobody has asked for it for idle.
func (h *Hub) watchFileRequests(id int64, cmd *exec.Cmd, done <-chan struct{}, idle time.Duration) {
	every := min(idle/4, 5*time.Second)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		if !h.fileEncodeIdle(id, cmd, idle) {
			continue
		}
		unlock := h.lockFileStart(id)
		if h.fileEncodeIdle(id, cmd, idle) {
			slog.Debug(fmt.Sprintf("recording %d: nobody is playing it, stopping its encode", id))
			h.stopFileEncode(id)
		}
		unlock()
	}
}

func (h *Hub) fileEncodeIdle(id int64, cmd *exec.Cmd, idle time.Duration) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	cur := h.fileEnc[id]
	return cur != nil && cur.cmd == cmd && time.Since(h.fileSeen[id]) >= idle
}

// RunFileSweep removes old playback copies now and every so often, until ctx
// ends. The first pass cleans up after an earlier run.
func (h *Hub) RunFileSweep(ctx context.Context) {
	h.SweepFileCopies(time.Now())
	tick := time.NewTicker(fileSweepEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.SweepFileCopies(time.Now())
		}
	}
}

type fileCopy struct {
	id   int64
	dir  string
	size uint64
	last time.Time
}

// SweepFileCopies removes copies nobody has asked for in fileKeepFor, then the
// oldest ones while the copies are over their limit. A copy being written, or
// asked for within fileLimitIdle, stays. Only folders named for a recording
// are touched.
func (h *Hub) SweepFileCopies(now time.Time) {
	root := filepath.Join(h.Dir, "file")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var copies []fileCopy
	var total uint64
	for _, entry := range entries {
		id, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil || !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		c := fileCopy{id: id, dir: dir, size: copyBytes(dir), last: info.ModTime()}
		h.playMu.Lock()
		if seen := h.fileSeen[id]; seen.After(c.last) {
			c.last = seen
		}
		h.playMu.Unlock()
		copies = append(copies, c)
		total += c.size
	}
	slices.SortFunc(copies, func(a, b fileCopy) int { return a.last.Compare(b.last) })
	limit := fileCopiesLimit(root, total)
	removed, freed := 0, uint64(0)
	for _, c := range copies {
		age := now.Sub(c.last)
		if age <= fileKeepFor && (total <= limit || age <= fileLimitIdle) {
			continue
		}
		if h.removeFileCopy(c) {
			removed++
			freed += c.size
			total -= c.size
		}
	}
	if removed > 0 {
		slog.Info(fmt.Sprintf("Removed %d playback copies, %.1f GB", removed, float64(freed)/(1<<30)))
	}
}

// fileCopiesLimit is fileKeepBytes, or half of what the copies could have on a
// disk too small for that.
func fileCopiesLimit(root string, total uint64) uint64 {
	limit := fileKeepBytes
	if space, err := disk.Stat(root); err == nil && space.Total > 0 {
		limit = min(limit, (space.Free+total)/2)
	}
	return limit
}

func (h *Hub) removeFileCopy(c fileCopy) bool {
	defer h.lockFileStart(c.id)()
	h.playMu.Lock()
	busy := h.fileEnc[c.id] != nil || h.fileSeen[c.id].After(c.last)
	h.playMu.Unlock()
	if busy {
		return false
	}
	if err := os.RemoveAll(c.dir); err != nil {
		slog.Warn(fmt.Sprintf("recording %d: remove playback copy: %v", c.id, err))
		return false
	}
	h.forgetFileCopy(c.id)
	return true
}

func copyBytes(dir string) uint64 {
	var total uint64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += uint64(info.Size())
		}
		return nil
	})
	return total
}
