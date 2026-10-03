package live

import (
	"context"
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
	if !strings.Contains(text, "scale=480:-2") || !strings.Contains(text, "scale=1280:-2") {
		t.Fatal("both sizes are written")
	}
	if strings.Count(text, "-f image2") != 4 {
		t.Fatal("a .part path needs an image format or ffmpeg will not write it")
	}
	if !strings.Contains(text, "/work/frames/4.jpg.part") || !strings.Contains(text, "/work/frames/5-1280.jpg.part") {
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
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	sample := filepath.Join(dir, "link.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001",
		"-t", "8", "-c:v", "mpeg2video", "-b:v", "3M", "-g", "15", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	body, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), frameGrabLimit)
	defer cancel()
	pr, pw := io.Pipe()
	go func() {
		// Real time: 8 s of stream over 8 s, the way a link delivers it.
		step := len(body) / 80 / 188 * 188
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
	cmd := exec.CommandContext(ctx, ffmpeg, FrameArgs([]FrameJob{{ChannelID: 801}}, dir)...)
	cmd.Stdin = pr
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grab did not finish inside %s: %v %s", frameGrabLimit, err, out)
	}
	publishFrame(dir, 801)
	if info, err := os.Stat(filepath.Join(dir, "801.jpg")); err != nil || info.Size() == 0 {
		t.Fatalf("no preview: %v", err)
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
