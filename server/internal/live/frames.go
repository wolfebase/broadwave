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
	frameGrabLimit = 4 * time.Second
	// Every program on a mux waits for its own keyframe, so the grab ends on
	// the slowest one: five programs on one 1.0 mux took 3.8-4.8 s.
	frameGrabPerProgram = time.Second
	frameGrabMax        = 10 * time.Second
)

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

// FramePath is the JPEG for a channel. width 1280 is the large copy; anything else is 480.
func FramePath(dir string, channelID int64, width int) string {
	name := fmt.Sprintf("%d.jpg", channelID)
	if width >= 1280 {
		name = fmt.Sprintf("%d-1280.jpg", channelID)
	}
	return filepath.Join(dir, "frames", name)
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
		small := filepath.Join(dir, fmt.Sprintf("%d.jpg.part", job.ChannelID))
		large := filepath.Join(dir, fmt.Sprintf("%d-1280.jpg.part", job.ChannelID))
		// .part is not an image extension, so the format has to be named or
		// ffmpeg refuses the file and the still never lands.
		args = append(args,
			"-map", mapSpec, "-vf", "scale=480:-2", "-frames:v", "1", "-f", "image2", small,
			"-map", mapSpec, "-vf", "scale=1280:-2", "-frames:v", "1", "-f", "image2", large,
		)
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
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			if FrameDue(last, now) {
				if err := h.grabFrames(ctx, m); err != nil {
					slog.Warn(fmt.Sprintf("frames freq %d: %v", m.freq, err))
				} else {
					last = now
				}
			}
			timer.Reset(FrameEvery)
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
	jobs := h.frameJobs(m)
	if len(jobs) == 0 {
		return nil
	}
	dir := filepath.Join(h.Dir, "frames")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	grabCtx, cancel := context.WithTimeout(ctx, grabLimit(jobs))
	defer cancel()
	pr, pw := io.Pipe()
	sub := h.attachPipeLocked(m, pw)
	defer m.detach(sub)
	// Wait does not return until the stdin copy finishes. Closing the pipe
	// when the grab ends unblocks that copy if the mux has gone quiet.
	go func() {
		<-grabCtx.Done()
		_ = pw.Close()
	}()
	cmd := exec.CommandContext(grabCtx, h.FFmpeg, FrameArgs(jobs, dir)...)
	cmd.Stdin = pr
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	var missing []int64
	for _, job := range jobs {
		if job.ChannelID > 0 && !publishFrame(dir, job.ChannelID) {
			missing = append(missing, job.ChannelID)
		}
	}
	// ffmpeg can be stopped after the last still is written; that grab worked.
	if len(missing) == 0 {
		return nil
	}
	if err == nil {
		err = errors.New("no keyframe")
	}
	return fmt.Errorf("channels %v: %w", missing, err)
}

// publishFrame reports whether both sizes landed.
func publishFrame(dir string, channelID int64) bool {
	ok := true
	for _, name := range []string{fmt.Sprintf("%d.jpg", channelID), fmt.Sprintf("%d-1280.jpg", channelID)} {
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
