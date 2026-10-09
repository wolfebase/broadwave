package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSpeedUsesTheLastFigure(t *testing.T) {
	log := "frame=30 speed=5.5x\nframe=60 fps=660 speed=11.2x\n"
	if got := ParseSpeed(log); got != 11.2 {
		t.Fatalf("speed %v", got)
	}
	if ParseSpeed("no speed here") != 0 {
		t.Fatal("missing speed should be 0")
	}
}

func TestFormatEncoderLine(t *testing.T) {
	if got := FormatEncoderLine("h264_videotoolbox", 11, true); got != "Apple GPU found: 1080p60 at 11x real time." {
		t.Fatalf("apple: %q", got)
	}
	if got := FormatEncoderLine("libx264", 2.4, true); got != "Software encoder: 1080p60 at 2.4x real time." {
		t.Fatalf("software: %q", got)
	}
	if got := FormatEncoderLine("h264_nvenc", 0, false); got != "NVIDIA GPU found." {
		t.Fatalf("failed: %q", got)
	}
	if got := FormatEncoderLine("libx264", 0, false); got != "Software encoder." {
		t.Fatalf("software failed: %q", got)
	}
	if got := EncoderName("h264_qsv"); got != "Intel GPU" {
		t.Fatalf("qsv: %q", got)
	}
}

func TestBenchEncoderParsesAScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho 'speed=4.0x' >&2\necho 'frame=60 speed=11x' >&2\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	speed, err := BenchEncoder(context.Background(), path, "h264_videotoolbox")
	if err != nil {
		t.Fatal(err)
	}
	if speed != 11 {
		t.Fatalf("speed %v", speed)
	}
}

func TestSteadySpeedLeavesOutStartup(t *testing.T) {
	t0 := time.Unix(0, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	samples := []progressSample{
		{at(100), 0}, {at(900), 0},
		{at(1000), 300 * time.Millisecond},
		{at(1500), 2 * time.Second},
		{at(2000), 3700 * time.Millisecond},
	}
	// Cumulative is 3.7 s in 2 s (1.85x); from the first frame out it is 3.4x.
	if got := steadySpeed(samples); got < 3.39 || got > 3.41 {
		t.Fatalf("speed %v", got)
	}
	if steadySpeed(samples[:3]) != 0 {
		t.Fatal("one frame out is not a speed")
	}
	if steadySpeed(nil) != 0 {
		t.Fatal("no samples")
	}
}

func TestProgressWriterStampsEachFrameTime(t *testing.T) {
	in := "frame=0\nout_time_us=N/A\nout_time_us=0\nspeed=N/A\nout_time_us=500000\nspeed=1.2x\nprogress=continue\nout_time_us=1500000\nspeed= 2.5x\nprogress=end\n"
	var calls int
	w := &progressWriter{now: func() time.Time {
		calls++
		return time.Unix(0, 0).Add(time.Duration(calls) * 250 * time.Millisecond)
	}}
	// ffmpeg's writes do not end on line breaks.
	for len(in) > 0 {
		n := min(7, len(in))
		if _, err := w.Write([]byte(in[:n])); err != nil {
			t.Fatal(err)
		}
		in = in[n:]
	}
	if len(w.samples) != 3 || w.samples[2].out != 1500*time.Millisecond || w.cumulative != 2.5 {
		t.Fatalf("%+v %v", w.samples, w.cumulative)
	}
	// 1 s of picture from the first frame out in 250 ms of wall time.
	if got := steadySpeed(w.samples); got != 4 {
		t.Fatalf("speed %v", got)
	}
}

func TestLiveBenchMatchesTheLiveRendition(t *testing.T) {
	live := renditionArgs(0, Source{VideoCodec: "MPEG2", HD: true}, Rendition{Video: "1080", Audio: "aac2"}, "libx264", "", "pipe:0")
	bench := liveBenchArgs(true)
	graph := func(args []string) string {
		for i, a := range args {
			if a == "-filter_complex" {
				return args[i+1]
			}
		}
		return ""
	}
	const laced = "tinterlace=mode=interleave_top,setfield=tff,"
	liveBody := strings.TrimPrefix(graph(live), "[0:v:0]")
	benchBody := strings.TrimPrefix(graph(bench), "[0:v]"+laced)
	if liveBody == "" || liveBody != benchBody || !strings.Contains(benchBody, "bwdif=mode=send_field") {
		t.Fatalf("bench filter %q\nlive filter %q", graph(bench), graph(live))
	}
	_, codec := liveBenchGraph()
	if !strings.Contains(strings.Join(live, " "), strings.Join(codec, " ")) || !strings.Contains(strings.Join(bench, " "), strings.Join(codec, " ")) {
		t.Fatalf("encoder %v\nlive %v\nbench %v", codec, live, bench)
	}
}

func TestBenchLiveRunsTheLiveGraph(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\necho out_time_us=0\nsleep 0.1\necho out_time_us=100000\nsleep 0.3\necho out_time_us=1000000\necho speed=2.0x\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	speed, err := BenchLive(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if speed <= 0 || speed > 3.1 {
		t.Fatalf("speed %v", speed)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"sliced-threads=1", "-b:v 14M", "-progress pipe:1 -stats_period 0.1"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in %s", want, args)
		}
	}
}

func TestBenchLiveReportsAFailedEncode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho 'Encoder libx264 not found.' >&2\necho 'Error opening output files: Encoder not found' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BenchLive(context.Background(), path); err == nil || !strings.Contains(err.Error(), "libx264 not found") {
		t.Fatalf("err %v", err)
	}
}

func TestBenchLiveRetriesAnOldFFmpeg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *stats_period*) echo \"Unrecognized option 'stats_period'.\" >&2; echo 'Error splitting the argument list: Option not found' >&2; exit 1;; esac\necho out_time_us=100000\nsleep 0.3\necho out_time_us=1000000\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	speed, err := BenchLive(context.Background(), path)
	if err != nil || speed <= 0 || speed > 3.1 {
		t.Fatalf("speed %v err %v", speed, err)
	}
}

func TestBenchLiveGivesUpAtTheDeadline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	// The wrapper's child holds stdout past the kill of the wrapper alone.
	script := "#!/bin/sh\necho out_time_us=0\nsleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := BenchLive(ctx, path)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("deadline took %s", elapsed)
	}
}
