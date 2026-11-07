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
)

// A played recording that needs a new picture is encoded into work/file/<id>.
// The encode runs only while players ask for it, and the copies are removed by
// age and total size, so a moment of playback does not cost a recording's
// length of disk for as long as the recording is kept.
var (
	// fileKeepFor is how long a copy nobody asks for is kept.
	fileKeepFor = 24 * time.Hour
	// fileKeepBytes is what the copies aim to stay under together. A copy runs
	// about 4.4 GB an hour, so one game can pass it alone.
	fileKeepBytes = uint64(10 << 30)
	// fileLimitIdle is how long a copy has to be left before the size limit can
	// take it. A paused player asks for nothing, and its next request after the
	// copy goes is a 404 it gives up on, so this outlasts a viewer pausing
	// overnight. The limit can be passed for that long.
	fileLimitIdle = 12 * time.Hour
	// fileRoomFloor is the free space below which a disk comes before a paused
	// viewer. fileRoomIdle is how long a copy has to be left even then: that
	// viewer gets a failed picture, and Try again plays from where they were.
	fileRoomFloor = uint64(4 << 30)
	fileRoomIdle  = 5 * time.Minute
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
	delete(h.fileSoft, id)
	delete(h.fileList, dir)
	delete(h.fillSrc, id)
	delete(h.fillSeen, id)
	h.playMu.Unlock()
}

// watchFileRequests stops the encode once nobody has asked for it for idle. It
// never stops one whose clock it can't trust: that one runs to the end, as
// every encode used to, and the sweep still removes the copy.
func (h *Hub) watchFileRequests(id int64, cmd *exec.Cmd, job *fileJob, done <-chan struct{}, idle time.Duration) {
	every := min(idle/4, 5*time.Second)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		if !h.fileEncodeIdle(id, cmd, idle) || !job.clock.safe(job.follow) {
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
// oldest ones while the copies are over their limit and left for fileLimitIdle,
// then the oldest left for fileRoomIdle while the disk is nearly full. A copy
// being written stays. Only folders named for a recording are touched.
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
		for _, seen := range []time.Time{h.fileSeen[id], h.fillSeen[id]} {
			if seen.After(c.last) {
				c.last = seen
			}
		}
		h.playMu.Unlock()
		copies = append(copies, c)
		total += c.size
	}
	slices.SortFunc(copies, func(a, b fileCopy) int { return a.last.Compare(b.last) })
	limit := fileCopiesLimit(root, total)
	free, known := uint64(0), false
	if space, err := diskStat(root); err == nil && space.Total > 0 {
		free, known = space.Free, true
	}
	removed, freed := 0, uint64(0)
	for _, c := range copies {
		age := now.Sub(c.last)
		old := age > fileKeepFor
		over := total > limit && age > fileLimitIdle
		tight := known && free+freed < fileRoomFloor && age > fileRoomIdle
		if !old && !over && !tight {
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
	if space, err := diskStat(root); err == nil && space.Total > 0 {
		limit = min(limit, (space.Free+total)/2)
	}
	return limit
}

func (h *Hub) removeFileCopy(c fileCopy) bool {
	defer h.lockFileStart(c.id)()
	h.playMu.Lock()
	busy := h.fileEnc[c.id] != nil || h.fillRuns[c.id] != nil || h.fileSeen[c.id].After(c.last) || h.fillSeen[c.id].After(c.last)
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
