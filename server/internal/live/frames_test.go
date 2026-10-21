package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFrameSchedule(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	if !FrameDue(time.Time{}, now) {
		t.Fatal("the first sample is due immediately")
	}
	if FrameDue(now.Add(-time.Minute), now.Add(-time.Second)) {
		t.Fatal("a fresh sample is not due again")
	}
	if !FrameDue(now.Add(-FrameEvery), now) {
		t.Fatal("a sample is due after a minute")
	}
	if !FrameStale(time.Time{}, now) {
		t.Fatal("a missing frame is stale")
	}
	if FrameStale(now.Add(-9*time.Minute), now) {
		t.Fatal("nine minutes is still current")
	}
	if !FrameStale(now.Add(-10*time.Minute-time.Second), now) {
		t.Fatal("older than ten minutes is stale")
	}
}

func TestFrameArgsSampleTheOpenMux(t *testing.T) {
	args := FrameArgs([]FrameJob{{ChannelID: 4, Program: 3}, {ChannelID: 5, Program: 4}}, "/work/frames")
	text := strings.Join(args, " ")
	if !strings.Contains(text, "-skip_frame nokey") || !strings.Contains(text, "-i pipe:0") {
		t.Fatalf("args = %s", text)
	}
	if strings.Contains(text, "http") || strings.Contains(text, "tuner") {
		t.Fatal("a preview must not open its own tune")
	}
	if !strings.Contains(text, "0:p:3:v:0") || !strings.Contains(text, "0:p:4:v:0") {
		t.Fatal("each program on the mux needs a frame")
	}
	if !strings.Contains(text, "scale=480:-2") || !strings.Contains(text, "scale=1280:-2") || !strings.Contains(text, "scale=1920:-2") {
		t.Fatal("every size is written")
	}
	if strings.Count(text, "-f image2") != 6 {
		t.Fatal("a .part path needs an image format or ffmpeg will not write it")
	}
	if !strings.Contains(text, "/work/frames/4.jpg.part") || !strings.Contains(text, "/work/frames/5-1280.jpg.part") || !strings.Contains(text, "/work/frames/5-1920.jpg.part") {
		t.Fatalf("paths = %s", text)
	}
	if strings.Contains(text, "-analyzeduration") || strings.Contains(text, "-nofind_stream_info") {
		t.Fatal("a tuned mux keeps the full probe so every program is found")
	}
}

// A 3.0 tune is one program whose AC-4 tracks hold ffmpeg's probe for about
// 13 s of stream, so every grab was killed at frameGrabLimit.
func TestASingleProgramGrabSkipsTheProbe(t *testing.T) {
	text := strings.Join(FrameArgs([]FrameJob{{ChannelID: 73}}, "/work/frames"), " ")
	if !strings.Contains(text, "-nofind_stream_info -i pipe:0") {
		t.Fatalf("args = %s", text)
	}
}

func TestLinkPreviewLandsInsideTheGrabLimit(t *testing.T) {
	ffmpeg := ffmpegOrSkip(t)
	dir := t.TempDir()
	sample := filepath.Join(dir, "link.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001",
		"-t", "8", "-c:v", "mpeg2video", "-b:v", "3M", "-g", "15", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	grabAtRealTime(t, ffmpeg, sample, 8*time.Second, []FrameJob{{ChannelID: 801}}, dir)
}

// Each program waits for its own keyframe after the mux probe. Five
// programs on one 1.0 mux took 3.8-4.8 s, past the 4 s a single program
// gets, so every grab on that mux was killed and logged.
func TestAFullMuxGrabLandsEveryProgram(t *testing.T) {
	ffmpeg := ffmpegOrSkip(t)
	dir := t.TempDir()
	sample := filepath.Join(dir, "mux.ts")
	args := []string{"-hide_banner", "-loglevel", "error"}
	var jobs []FrameJob
	for i := range 5 {
		args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001")
		jobs = append(jobs, FrameJob{ChannelID: int64(900 + i), Program: 3 + i})
	}
	args = append(args, "-t", "10")
	for i := range 5 {
		args = append(args, "-map", fmt.Sprintf("%d:v", i))
	}
	args = append(args, "-c:v", "mpeg2video", "-b:v", "1M")
	for i := range 5 {
		args = append(args,
			fmt.Sprintf("-g:v:%d", i), fmt.Sprint(30+15*i),
			"-program", fmt.Sprintf("program_num=%d:st=%d", 3+i, i))
	}
	args = append(args, "-f", "mpegts", sample)
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	if grabLimit(jobs) <= grabLimit(jobs[:1]) {
		t.Fatal("a mux with more programs gets longer")
	}
	if grabLimit(make([]FrameJob, 40)) > frameGrabMax {
		t.Fatal("no grab runs past the cap")
	}
	grabAtRealTime(t, ffmpeg, sample, 10*time.Second, jobs, dir)
}

func ffmpegOrSkip(t *testing.T) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	return ffmpeg
}

// grabAtRealTime feeds sample over length, the way a tuner delivers it, and
// fails unless every job's stills land inside the grab limit.
func grabAtRealTime(t *testing.T, ffmpeg, sample string, length time.Duration, jobs []FrameJob, dir string) {
	t.Helper()
	body, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	limit := grabLimit(jobs)
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	pr, pw := io.Pipe()
	go func() {
		ticks := int(length / (100 * time.Millisecond))
		step := len(body) / ticks / 188 * 188
		for off := 0; off < len(body); off += step {
			if _, err := pw.Write(body[off:min(off+step, len(body))]); err != nil {
				return
			}
			select {
			case <-ctx.Done():
				_ = pw.Close()
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
		_ = pw.Close()
	}()
	cmd := exec.CommandContext(ctx, ffmpeg, FrameArgs(jobs, dir)...)
	cmd.Stdin = pr
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grab did not finish inside %s: %v %s", limit, err, out)
	}
	for _, job := range jobs {
		if !publishFrame(dir, job.ChannelID) {
			t.Fatalf("no preview for channel %d", job.ChannelID)
		}
	}
}

func TestQuietGrabReturns(t *testing.T) {
	h, m := testHub(t)
	h.FFmpeg = "/bin/sleep"
	addTestFeed(h, m, 4, "4.1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		_ = h.grabFrames(ctx, m)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a grab on a quiet mux should stop")
	}
}

// ffmpeg can be stopped at the limit after it has written every still. That
// grab worked; only a channel with no still is reported.
func TestAGrabStoppedAfterItsStillsLandedWorked(t *testing.T) {
	h, m := testHub(t)
	addTestFeed(h, m, 4, "4.1")
	script := filepath.Join(t.TempDir(), "ffmpeg")
	body := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in *.part) echo jpg > \"$a\";; esac; done\nkill -9 $$\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	h.FFmpeg = script
	// The test mux sends nothing, so the stdin copy only ends with the context.
	// The shell must start and write within it on a loaded machine.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.grabFrames(ctx, m); err != nil {
		t.Fatalf("every still landed: %v", err)
	}
	if _, err := os.Stat(FramePath(h.Dir, 4, 1280)); err != nil {
		t.Fatal(err)
	}
	h.FFmpeg = "/usr/bin/false"
	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := h.grabFrames(ctx, m); err == nil || !strings.Contains(err.Error(), "[4]") {
		t.Fatalf("a grab with no still names its channel: %v", err)
	}
}

// A grab that misses a channel tries again for that one soon, so a guide
// scan's 40 s on a frequency gets its stills even when the first grab ran
// out of time before a slow program's keyframe.
func TestAGrabTriesAgainForTheChannelsItMissed(t *testing.T) {
	first, retry := frameFirst, frameRetry
	frameFirst, frameRetry = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { frameFirst, frameRetry = first, retry })
	h, m := testHub(t)
	addTestFeed(h, m, 4, "4.1")
	addTestFeed(h, m, 5, "4.2")
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := filepath.Join(dir, "ffmpeg")
	// The first run writes only channel 4's stills; later runs write all
	// they are asked for. Each run's arguments go to calls.
	body := "#!/bin/sh\necho \"$@\" >> " + calls + "\n" +
		"for a in \"$@\"; do case \"${a##*/}\" in 5*.part) [ -f " + calls + ".again ] && echo jpg > \"$a\";; *.part) echo jpg > \"$a\";; esac; done\n" +
		"touch " + calls + ".again\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	h.FFmpeg = script
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.watchFrames(ctx, m)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(FramePath(h.Dir, 5, 1920)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(calls)
			entries, _ := os.ReadDir(filepath.Join(h.Dir, "frames"))
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("the missed channel was not grabbed again: %d runs, files %v", strings.Count(string(log), "\n"), names)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	runs := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(runs) < 2 || strings.Contains(runs[1], "/4.jpg.part") || !strings.Contains(runs[1], "/5.jpg.part") {
		t.Fatalf("runs %q", runs)
	}
}

func TestAGrabStoppedOnPurposeIsRoutine(t *testing.T) {
	if !routineKill(fmt.Errorf("channels [4]: %w", errors.New("signal: killed"))) || !routineKill(context.Canceled) {
		t.Fatal("a killed grab is routine")
	}
	if routineKill(errors.New("no keyframe")) {
		t.Fatal("a grab that found no picture is not")
	}
}
