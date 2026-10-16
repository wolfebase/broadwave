package live

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// procTail is how many of a process's last lines are kept for its exit.
const procTail = 24

// procLogEvery and procLogBurst bound what one process writes to the log at
// warning level: a weak signal makes a decoder complain many times a second.
const (
	procLogEvery = 10 * time.Second
	procLogBurst = 8
)

// procLog is one ffmpeg's stderr. Each line goes to the server log behind the
// tag naming what the process is for: decoder chatter at debug, warnings and
// failures at warn, rate-limited per process. The last lines are kept, so a
// death can be told apart (sound the muxer could not describe, a GPU that
// would not open) and logged with its cause. The log handler masks
// passwords and tuner credentials.
type procLog struct {
	tag     string
	mu      sync.Mutex
	partial []byte
	tail    []string
	window  time.Time
	shown   int
	dropped int
}

// logCommand sends cmd's stderr through a procLog tagged with tag.
func logCommand(cmd *exec.Cmd, tag string) *procLog {
	l := &procLog{tag: tag}
	if cmd != nil {
		cmd.Stderr = l
	}
	return l
}

func (l *procLog) Write(p []byte) (int, error) {
	if l == nil {
		return len(p), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexAny(l.partial, "\r\n")
		if i < 0 {
			break
		}
		line := strings.TrimSpace(string(l.partial[:i]))
		l.partial = l.partial[i+1:]
		if line != "" {
			l.lineLocked(line)
		}
	}
	if len(l.partial) > 4096 {
		l.lineLocked(strings.TrimSpace(string(l.partial)))
		l.partial = nil
	}
	if len(l.partial) == 0 {
		l.partial = nil
	}
	return len(p), nil
}

func (l *procLog) lineLocked(line string) {
	l.tail = append(l.tail, line)
	if len(l.tail) > procTail {
		l.tail = append(l.tail[:0], l.tail[len(l.tail)-procTail:]...)
	}
	level := ffmpegLevel(line)
	if level < slog.LevelWarn {
		slog.Debug(l.tag + ": " + line)
		return
	}
	now := time.Now()
	if now.Sub(l.window) >= procLogEvery {
		if l.dropped > 0 {
			slog.Warn(fmt.Sprintf("%s: %d more ffmpeg lines", l.tag, l.dropped))
		}
		l.window, l.shown, l.dropped = now, 0, 0
	}
	if l.shown >= procLogBurst {
		l.dropped++
		return
	}
	l.shown++
	slog.Log(context.Background(), level, l.tag+": "+line)
}

// Tail is the process's last lines, oldest first.
func (l *procLog) Tail() []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.tail...)
}

// lastWords is the tail cut to the lines that say something, for one log line.
func (l *procLog) lastWords() string {
	var keep []string
	for _, line := range l.Tail() {
		if ffmpegLevel(line) >= slog.LevelWarn {
			keep = append(keep, line)
		}
	}
	if len(keep) > 4 {
		keep = keep[len(keep)-4:]
	}
	return strings.Join(keep, " | ")
}

// routineFFmpeg is what a decoder says about a weak or spliced broadcast.
// It is worth keeping for a debug log, not for every operator.
var routineFFmpeg = []string{
	"mmco:", "Last message repeated", "number of reference frames", "x265 [info]",
	"non monotonically increasing dts", "Packet corrupt", "PES packet size mismatch",
	"decode_slice_header error", "concealing", "error while decoding MB", "co located POCs unavailable",
	"Invalid NAL unit size", "missing picture in access unit", "reference picture missing",
	"Error splitting the input into NAL units", "Broken pipe", "Error writing trailer", "Error closing file",
	"Invalid frame dimensions 0x0", "Skipping invalid undecodable NALU", "deprecated pixel format",
	"Timestamps are unset", "Queue input is backward in time", "Starting new cluster", "set_mempolicy", "codec frame size is not set",
}

// failingFFmpeg is what ffmpeg says when a start or an encode is failing.
var failingFFmpeg = []string{
	"Could not", "could not", "Invalid argument", "Error opening", "Error while opening", "Error initializing",
	"Failed", "failed", "No such", "not set", "Conversion failed", "Impossible to", "Cannot ", "cannot ",
	"Task finished with error", "Error sending frames", "Device creation", "Connection refused",
	"unspecified sample rate", "Unsupported", "not supported", "No device", "out of memory",
}

// ffmpegLevel is the log level for one line of ffmpeg's stderr.
func ffmpegLevel(line string) slog.Level {
	for _, s := range routineFFmpeg {
		if strings.Contains(line, s) {
			return slog.LevelDebug
		}
	}
	for _, s := range failingFFmpeg {
		if strings.Contains(line, s) {
			return slog.LevelWarn
		}
	}
	return slog.LevelDebug
}

// exitCause is why an encode died, from its last lines.
type exitCause int

const (
	causeUnknown exitCause = iota
	// causeParams is sound the muxer could not describe: a copied AC-3 track
	// whose rate the probe did not see. The GPU is fine; a new encode with
	// that sound converted starts.
	causeParams
	// causeGPU is a hardware encoder or decoder that would not open or run.
	causeGPU
)

func (c exitCause) String() string {
	switch c {
	case causeParams:
		return "sound parameters"
	case causeGPU:
		return "GPU"
	}
	return "unknown"
}

var paramsFFmpeg = []string{
	"sample rate not set", "unspecified sample rate", "frame size is not set", "channel layout not set",
}

var gpuFFmpeg = []string{
	"vaapi", "VAAPI", "hwaccel", "hw_device", "hwupload", "No VA display", "libva",
	"nvenc", "NVENC", "CUDA", "cuda", "qsv", "QSV", "MFX", "videotoolbox", "VideoToolbox", "VTCompression",
	"Failed to create surface", "Failed to upload", "Device creation failed", "Failed to initialise",
}

// causeOf reads an exit from the process's last lines. Sound parameters win
// when the muxer could not write its header: that error is followed by the
// picture graph's own. A missing frame size alone is a warning every AC-3
// copy starts with.
func causeOf(tail []string) exitCause {
	header, sound := false, false
	for _, line := range tail {
		header = header || strings.Contains(line, "Could not write header")
		for _, s := range paramsFFmpeg {
			sound = sound || strings.Contains(line, s)
		}
	}
	if header && sound {
		return causeParams
	}
	for _, line := range tail {
		if ffmpegLevel(line) < slog.LevelWarn {
			continue
		}
		for _, s := range gpuFFmpeg {
			if strings.Contains(line, s) {
				return causeGPU
			}
		}
	}
	return causeUnknown
}
