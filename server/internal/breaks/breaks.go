// Package breaks finds the commercial breaks in a recording.
package breaks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Index finds a recording's breaks with comskip, when it is installed, and
// with its own scan, and scores each break by how well the two agree. It does
// not modify the file.
func Index(ffmpeg, path string) ([]Break, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	cues, err := Scan(ffmpeg, path)
	if err != nil {
		return nil, err
	}
	own := Find(cues)
	skipped, ok := comskipBreaks(path)
	if !ok {
		return own, nil
	}
	return combine(skipped, own), nil
}

// Confidence of comskip's breaks. On real news it was right about 99 % of the
// time (J0.136), so on its own it skips; with the scan agreeing, more so.
const (
	comskipAlone  = 0.75
	bothAgree     = 0.95
	scanOverruled = 0.6
)

// combine keeps comskip's spans, which end on the exact frame, and adds the
// scan's breaks comskip did not find, below the auto-skip line.
func combine(skipped, own []Break) []Break {
	var out []Break
	used := make([]bool, len(own))
	for _, b := range skipped {
		b.Confidence = comskipAlone
		for i, o := range own {
			if overlap(b, o) > 0.5*min(b.End-b.Start, o.End-o.Start) {
				b.Confidence = max(bothAgree, o.Confidence)
				used[i] = true
			}
		}
		out = append(out, b)
	}
	for i, o := range own {
		if used[i] {
			continue
		}
		clash := false
		for _, b := range skipped {
			if overlap(b, o) > 0 {
				clash = true
			}
		}
		if clash {
			continue
		}
		o.Confidence = min(o.Confidence, scanOverruled)
		out = append(out, o)
	}
	// Not merged: joining a sure break with a near one would skip the show
	// between them on the sure one's word.
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func overlap(a, b Break) float64 {
	return max(0, min(a.End, b.End)-max(a.Start, b.Start))
}

// scanLimit stops a scan that hangs. An hour of 1080i takes about a minute.
var scanLimit = time.Hour

// scanArgs reads black stretches, scene cuts, and silences from the log and
// one small gray frame per second from stdout, in one pass.
func scanArgs(path string, sound bool) []string {
	picture := "[0:v:0]blackdetect=d=0.1:pic_th=0.90:pix_th=0.10,scdet=t=10,fps=1,scale=" +
		strconv.Itoa(frameW) + ":" + strconv.Itoa(frameH) + ",format=gray[v]"
	args := []string{"-hide_banner", "-nostats", "-threads", "2", "-filter_threads", "2", "-i", path}
	if !sound {
		return append(args, "-filter_complex", picture, "-map", "[v]", "-f", "rawvideo", "pipe:1")
	}
	return append(args, "-filter_complex", picture+";[0:a:0]silencedetect=n=-50dB:d=0.1[a]",
		"-map", "[v]", "-f", "rawvideo", "pipe:1", "-map", "[a]", "-f", "null", "-")
}

// Scan reads a recording's cues with ffmpeg at low priority: it runs when a
// recording ends, maybe while others play. A recording with no sound track
// is read for its picture alone.
func Scan(ffmpeg, path string) (Cues, error) {
	c, err := scan(ffmpeg, path, true)
	if err != nil && errors.Is(err, errNoSound) {
		return scan(ffmpeg, path, false)
	}
	return c, err
}

var errNoSound = errors.New("no sound track")

func scan(ffmpeg, path string, sound bool) (Cues, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanLimit)
	defer cancel()
	cmd := lowPriority(ctx, ffmpeg, scanArgs(path, sound))
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Cues{}, err
	}
	// The log is read as it comes: a damaged broadcast can log megabytes of
	// decoder errors over a few hours.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Cues{}, err
	}
	if err := cmd.Start(); err != nil {
		return Cues{}, err
	}
	var c Cues
	var tail logTail
	logged := make(chan struct{})
	go func() {
		tail = c.readLog(stderr)
		close(logged)
	}()
	frames, readErr := readFrames(out)
	_, _ = io.Copy(io.Discard, out)
	<-logged
	if err := cmd.Wait(); err != nil {
		if sound && tail.noSound {
			return Cues{}, errNoSound
		}
		return Cues{}, fmt.Errorf("scan failed: %w: %s", err, tail.last)
	}
	if readErr != nil {
		return Cues{}, readErr
	}
	c.addFrames(frames)
	return c, nil
}

// lowPriority runs a command under nice when the system has it.
func lowPriority(ctx context.Context, exe string, args []string) *exec.Cmd {
	if nice, err := exec.LookPath("nice"); err == nil {
		return exec.CommandContext(ctx, nice, append([]string{"-n", "10", exe}, args...)...)
	}
	return exec.CommandContext(ctx, exe, args...)
}

// comskipINI asks for an EDL only. Without it comskip writes a .txt and no
// .edl, so its breaks were never read.
const comskipINI = "output_edl=1\noutput_default=0\n"

// comskipLimit stops a comskip that hangs; an hour of 1080i takes minutes.
var comskipLimit = 2 * time.Hour

// comskipBreaks runs comskip with its output in a temporary folder, so the
// recordings folder gets no logo, log, or .txt files. It runs at low priority
// on two threads.
func comskipBreaks(path string) ([]Break, bool) {
	exe, err := exec.LookPath("comskip")
	if err != nil {
		return nil, false
	}
	dir, err := os.MkdirTemp("", "comskip")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(dir)
	ini := filepath.Join(dir, "comskip.ini")
	if err := os.WriteFile(ini, []byte(comskipINI), 0o600); err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), comskipLimit)
	defer cancel()
	cmd := lowPriority(ctx, exe, []string{"--quiet", "--threads=2", "--ini=" + ini, "--output=" + dir, path})
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn(fmt.Sprintf("breaks: comskip on %s: %v %s", filepath.Base(path), err, lastLine(out)))
		return nil, false
	}
	edl := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".edl")
	body, err := os.ReadFile(edl)
	if err != nil {
		return nil, false
	}
	parsed := parseEDL(string(body))
	return parsed, len(parsed) > 0
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}

func parseEDL(text string) []Break {
	var out []Break
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		start, err1 := strconv.ParseFloat(fields[0], 64)
		end, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		out = append(out, Break{Start: start, End: end})
	}
	return out
}
