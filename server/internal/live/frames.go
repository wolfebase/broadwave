package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

const (
	// FrameEvery is how often a tuned mux is sampled. A grab is a short
	// keyframe read, not a second tune.
	FrameEvery = 60 * time.Second
	// FrameStaleAfter marks a preview that no longer matches the broadcast.
	FrameStaleAfter = 10 * time.Minute
	// A grab waits for the next keyframe, which can be a couple of seconds
	// away. -skip_frame nokey does not decode the frames in between, so the
	// wait is not a busy core. Anything longer is stopped.
	frameGrabLimit = 6 * time.Second
	// Every program on a mux waits for its own keyframe, so the grab ends on
	// the slowest one: five programs on one 1.0 mux took 3.8-4.8 s, one of
	// them 4 s after the rest, and five and six were killed at 8 and 9 s.
	frameGrabPerProgram = 1500 * time.Millisecond
	frameGrabMax        = 16 * time.Second
	// frameRetries is how many times a grab tries again, frameRetry apart,
	// for the channels it missed before the next FrameEvery. A guide scan
	// holds a frequency for 40 s and its tune is gone by the next minute.
	frameRetries = 3
)

// frameFirst is the wait from a tune to its first grab; frameRetry is
// the wait before a grab tries again for the channels it missed.
var frameFirst, frameRetry = 2 * time.Second, 4 * time.Second

// grabLimit is how long a grab for these jobs may run.
func grabLimit(jobs []FrameJob) time.Duration {
	limit := frameGrabLimit
	if len(jobs) > 1 {
		limit += time.Duration(len(jobs)-1) * frameGrabPerProgram
	}
	return min(limit, frameGrabMax)
}

// FrameJob is one program on an already-tuned mux.
type FrameJob struct {
	ChannelID int64
	Program   int
}

// FrameDue reports whether a mux should be sampled again.
func FrameDue(last, now time.Time) bool {
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= FrameEvery
}

// FrameStale reports whether a saved preview is too old to trust.
func FrameStale(modified, now time.Time) bool {
	if modified.IsZero() {
		return true
	}
	return now.Sub(modified) > FrameStaleAfter
}

// FrameWidths are the sizes each grab writes. The first is the preview the
// guide lists; 1920 is a full screen, as the Apple TV Top Shelf asks for.
var FrameWidths = []int{480, 1280, 1920}

// FrameWidth is the size kept for a request of w: the largest one not
// wider than w, and 480 for anything smaller or unknown.
func FrameWidth(w int) int {
	out := FrameWidths[0]
	for _, size := range FrameWidths {
		if w >= size {
			out = size
		}
	}
	return out
}

// FramePath is the JPEG of one size for a channel (FrameWidth of width).
func FramePath(dir string, channelID int64, width int) string {
	return filepath.Join(dir, "frames", frameName(channelID, FrameWidth(width)))
}

func frameName(channelID int64, width int) string {
	if width == FrameWidths[0] {
		return fmt.Sprintf("%d.jpg", channelID)
	}
	return fmt.Sprintf("%d-%d.jpg", channelID, width)
}

// FrameArgs builds an ffmpeg command that reads the mux already on stdin and
// writes one keyframe per program. It never opens a tuner URL.
func FrameArgs(jobs []FrameJob, dir string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-skip_frame", "nokey"}
	// A link or an ATSC 3.0 tune carries one program at a few Mb/s in real
	// time. Its PMT already names the codecs, and the probe outlasts
	// frameGrabLimit: on a 3.0 tune it waits on the AC-4 tracks until ~8 MB
	// (13 s) are read. A tuned 1.0 mux keeps the probe, or a program whose
	// PMT comes later is not there to map.
	if singleProgram(jobs) {
		args = append(args, "-nofind_stream_info")
	}
	args = append(args, "-i", "pipe:0")
	for _, job := range jobs {
		if job.ChannelID <= 0 {
			continue
		}
		mapSpec := "0:v:0"
		if job.Program > 0 {
			mapSpec = fmt.Sprintf("0:p:%d:v:0", job.Program)
		}
		// .part is not an image extension, so the format has to be named or
		// ffmpeg refuses the file and the still never lands.
		for _, width := range FrameWidths {
			part := filepath.Join(dir, frameName(job.ChannelID, width)+".part")
			args = append(args, "-map", mapSpec, "-vf", fmt.Sprintf("scale=%d:-2", width), "-frames:v", "1", "-f", "image2", part)
		}
	}
	return args
}

func singleProgram(jobs []FrameJob) bool {
	for _, job := range jobs {
		if job.Program > 0 {
			return false
		}
	}
	return true
}

func (h *Hub) startFrames(ctx context.Context, m *mux) {
	if h == nil || m == nil || h.FFmpeg == "" {
		return
	}
	m.frames.Do(func() {
		h.grabbers.Add(1)
		go func() {
			defer h.grabbers.Done()
			h.watchFrames(ctx, m)
		}()
	})
}

func (h *Hub) watchFrames(ctx context.Context, m *mux) {
	timer := time.NewTimer(frameFirst)
	defer timer.Stop()
	var last time.Time
	// missing are the channels the last grab did not get, tried again soon.
	var missing []int64
	tries := 0
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			next := FrameEvery
			if FrameDue(last, now) || len(missing) > 0 {
				left, err := h.grabFramesOf(ctx, m, missing)
				switch {
				case err == nil:
					last, missing, tries = now, nil, 0
				case tries < frameRetries && ctx.Err() == nil:
					tries++
					missing = left
					next = frameRetry
				default:
					slog.Warn(fmt.Sprintf("frames freq %d: %v", m.freq, err))
					last, missing, tries = now, nil, 0
				}
			}
			timer.Reset(next)
		}
	}
}

func (h *Hub) frameJobs(m *mux) []FrameJob {
	h.mu.Lock()
	device, freq, tuner := m.device, m.freq, m.tuner
	var feeds []FrameJob
	if tuner < 0 {
		for _, f := range m.feeds {
			feeds = append(feeds, FrameJob{ChannelID: f.channel.ID})
		}
	}
	h.mu.Unlock()
	if tuner < 0 {
		return feeds
	}
	if h.Store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	targets, err := h.Store.FrameTargets(ctx, device, freq)
	if err != nil || len(targets) == 0 {
		return nil
	}
	jobs := make([]FrameJob, 0, len(targets))
	for _, t := range targets {
		jobs = append(jobs, FrameJob{ChannelID: t.ID, Program: t.ProgramNum})
	}
	return jobs
}

func (h *Hub) grabFrames(ctx context.Context, m *mux) error {
	_, err := h.grabFramesOf(ctx, m, nil)
	return err
}

// grabFramesOf grabs the mux's channels, only those in only when it is not
// empty, and reports the ones it did not get.
func (h *Hub) grabFramesOf(ctx context.Context, m *mux, only []int64) ([]int64, error) {
	jobs := h.frameJobs(m)
	if len(only) > 0 {
		jobs = slices.DeleteFunc(jobs, func(j FrameJob) bool { return !slices.Contains(only, j.ChannelID) })
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	dir := filepath.Join(h.Dir, "frames")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	grabCtx, cancel := context.WithTimeout(ctx, grabLimit(jobs))
	defer cancel()
	pr, pw := io.Pipe()
	sub := h.attachPipeLocked(m, pw)
	defer m.detach(sub)
	// Closing the pipe when the grab ends unblocks the copy below if the
	// mux has gone quiet.
	go func() {
		<-grabCtx.Done()
		_ = pw.Close()
	}()
	cmd := exec.CommandContext(grabCtx, h.FFmpeg, FrameArgs(jobs, dir)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	// The copy into ffmpeg is ours, not exec's: an ffmpeg that has written
	// every still and exited is done, without a wait for the next read.
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err == nil {
		go func() {
			_, _ = io.Copy(stdin, pr)
			_ = stdin.Close()
		}()
		err = cmd.Wait()
	}
	_ = pr.Close()
	var missing []int64
	for _, job := range jobs {
		if job.ChannelID > 0 && !publishFrame(dir, job.ChannelID) {
			missing = append(missing, job.ChannelID)
		}
	}
	// ffmpeg can be stopped after the last still is written; that grab worked.
	if len(missing) == 0 {
		return nil, nil
	}
	if err == nil {
		err = errors.New("no keyframe")
	}
	return missing, fmt.Errorf("channels %v: %w", missing, err)
}

// publishFrame reports whether every size landed.
func publishFrame(dir string, channelID int64) bool {
	ok := true
	for _, width := range FrameWidths {
		name := frameName(channelID, width)
		part := filepath.Join(dir, name+".part")
		info, err := os.Stat(part)
		if err != nil || info.Size() == 0 {
			_ = os.Remove(part)
			ok = false
			continue
		}
		_ = os.Rename(part, filepath.Join(dir, name))
	}
	return ok
}
