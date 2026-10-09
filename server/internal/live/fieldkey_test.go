package live

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFieldKeyGraphKeepsOneFlag(t *testing.T) {
	line := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "720", Audio: "aac2"}, "libx264", ""), " ")
	for _, want := range []string{
		"-filter_complex",
		"-map [v]",
		"[0:v:0]",
		"bwdif=mode=send_field",
		`select='not(eq(key\,1)*eq(mod(n\,2)\,1))'`,
		"minterpolate=fps=60000/1001:mi_mode=dup:scd=none",
		"overlay=eof_action=pass:shortest=1",
		"-force_key_frames source",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, ",fps=60000/1001") {
		t.Fatalf("a plain fps filter clones the closing key: %s", line)
	}
	prog := strings.Join(RenditionArgs(3, Source{VideoCodec: "MPEG2"}, Rendition{Video: "1080", Audio: "aac2"}, "libx264", ""), " ")
	if !strings.Contains(prog, "[0:p:3:v:0]") || !strings.Contains(prog, "-map [v]") {
		t.Fatalf("program input: %s", prog)
	}
	progressive := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2", Progressive: true}, Rendition{Video: "720", Audio: "aac2"}, "libx264", ""), " ")
	if strings.Contains(progressive, "filter_complex") || strings.Contains(progressive, "bwdif") {
		t.Fatalf("progressive: %s", progressive)
	}
	saver := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "540", Audio: "aac2"}, "libx264", ""), " ")
	if strings.Contains(saver, "filter_complex") || strings.Contains(saver, "send_field") {
		t.Fatalf("saver: %s", saver)
	}
	gpu := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive"), " ")
	if strings.Contains(gpu, "filter_complex") || strings.Contains(gpu, "minterpolate") || !strings.Contains(gpu, "deinterlace_vaapi") {
		t.Fatalf("vaapi stays on the gpu graph: %s", gpu)
	}
	tb := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "720", Audio: "none"}, "h264_videotoolbox", ""), " ")
	if !strings.Contains(tb, "minterpolate=fps=60000/1001") || !strings.Contains(tb, "-a53cc 0") {
		t.Fatalf("videotoolbox: %s", tb)
	}
}

// TestFieldDoubledEncodeHasOneKeyframePerGroup proves a field-rate transcode
// of an interlaced broadcast writes one IDR per source group. bwdif used to
// hand the encoder two key flags a field apart.
func TestFieldDoubledEncodeHasOneKeyframePerGroup(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "in.ts")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	gen := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=30000/1001:duration=2",
		"-vf", "tinterlace=mode=interleave_top,setfield=tff",
		"-c:v", "mpeg2video", "-b:v", "800k", "-g", "15", "-bf", "2",
		"-sc_threshold", "1000000000",
		"-flags", "+ildct+ilme", "-r", "30000/1001", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	prog := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", src,
		"-filter_complex", "[0:p:1:v:0]null[v]", "-map", "[v]", "-frames:v", "1", "-f", "null", "-")
	if out, err := prog.CombinedOutput(); err != nil {
		t.Fatalf("program filter input: %v %s", err, out)
	}

	outPath := filepath.Join(dir, "out.mp4")
	args := RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "720", Audio: "none"}, "libx264", "")
	for i, a := range args {
		if a == "pipe:0" {
			args[i] = src
		}
		if a == "pipe:1" {
			args[i] = outPath
		}
	}
	enc := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := enc.CombinedOutput(); err != nil {
		t.Fatalf("encode: %v %s", err, out)
	}
	probe := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=key_frame,pts_time", "-of", "csv=p=0", outPath)
	raw, err := probe.Output()
	if err != nil {
		t.Fatal(err)
	}
	var times []float64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		key, pts, ok := strings.Cut(line, ",")
		if !ok || key != "1" {
			continue
		}
		ts, err := strconv.ParseFloat(strings.TrimSuffix(pts, ","), 64)
		if err != nil {
			t.Fatalf("pts %q: %v", line, err)
		}
		times = append(times, ts)
	}
	if len(times) < 3 {
		t.Fatalf("keyframes %v", times)
	}
	for i := 1; i < len(times); i++ {
		gap := times[i] - times[i-1]
		if gap < 0.03 {
			t.Fatalf("keyframes %v are a field apart", times)
		}
		// The source group is half a second. One field of lead is the
		// minterpolate shift; a fixed interval would walk away from it.
		if gap < 0.45 || gap > 0.55 {
			t.Fatalf("gap %.4f in %v", gap, times)
		}
	}
}
