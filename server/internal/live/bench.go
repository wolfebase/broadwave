package live

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var speedPattern = regexp.MustCompile(`speed=\s*([0-9]+(?:\.[0-9]+)?)x`)

// EncoderName is the word setup shows for this encoder.
func EncoderName(encoder string) string {
	switch {
	case strings.Contains(encoder, "nvenc"):
		return "NVIDIA GPU"
	case strings.Contains(encoder, "videotoolbox"):
		return "Apple GPU"
	case strings.Contains(encoder, "qsv"):
		return "Intel GPU"
	case strings.Contains(encoder, "vaapi"):
		return vaapiName()
	default:
		return "Software"
	}
}

func vaapiName() string {
	switch GPUVendor() {
	case "0x8086":
		return "Intel GPU"
	case "0x1002", "0x1022":
		return "AMD GPU"
	default:
		return "GPU"
	}
}

// FormatEncoderLine is the setup sentence for the 1080p60 encode.
func FormatEncoderLine(encoder string, speed float64, ok bool) string {
	name := EncoderName(encoder)
	if !ok || speed <= 0 {
		if name == "Software" {
			return "Software encoder."
		}
		return name + " found."
	}
	rate := fmt.Sprintf("%.1fx", speed)
	if speed >= 10 {
		rate = fmt.Sprintf("%.0fx", speed)
	}
	if name == "Software" {
		return fmt.Sprintf("Software encoder: 1080p60 at %s real time.", rate)
	}
	return fmt.Sprintf("%s found: 1080p60 at %s real time.", name, rate)
}

// ParseSpeed reads the last ffmpeg speed= figure.
func ParseSpeed(log string) float64 {
	matches := speedPattern.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(matches[len(matches)-1][1], 64)
	if err != nil {
		return 0
	}
	return v
}

// BenchEncoder encodes a 1080p60 test picture and reports how fast it ran.
// On a GPU it is three seconds timed with startup, which the GPU rows of
// Budget were set on. libx264 is timed from the first frame out over five
// seconds of picture: with startup in it, the same six cores read 1.6x on a
// cold start and 4.2x a second later, and the budget flipped between one
// picture and four. The graph is a test picture, not a broadcast, and it
// does not deinterlace.
func BenchEncoder(ctx context.Context, ffmpeg, encoder string) (float64, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if encoder == "" {
		encoder = "libx264"
	}
	if encoder == "libx264" {
		return steadyBench(ctx, ffmpeg, softwareBenchArgs)
	}
	cmd := benchCommand(ctx, ffmpeg, benchArgs(encoder))
	out, err := cmd.CombinedOutput()
	speed := ParseSpeed(string(out))
	if err != nil {
		return speed, err
	}
	if speed <= 0 {
		return 0, fmt.Errorf("encoder did not report a speed")
	}
	return speed, nil
}

func benchCommand(ctx context.Context, ffmpeg string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	// CommandContext kills the direct child only. A shell that started the
	// encode keeps the pipes open, and Wait then sits until that child exits.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
}

// BenchLive runs the graph a 1080i channel takes on the CPU (field-rate
// deinterlace, then libx264 with the live settings) on four seconds of a test
// picture and reports the speed once frames flow. BenchEncoder is the wrong
// figure for choosing the CPU: it runs frame threads, which live encodes do
// not (3x faster on a 24-thread host), and its figure includes startup, so on
// that host it read 2.3x to 3.9x within minutes while this graph held 3.5x to
// 3.9x. A broadcast runs about 20% slower than the test picture.
func BenchLive(ctx context.Context, ffmpeg string) (float64, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	return steadyBench(ctx, ffmpeg, liveBenchArgs)
}

// steadyBench runs a bench that writes -progress to stdout and reports its
// speed from the first frame out.
func steadyBench(ctx context.Context, ffmpeg string, args func(fine bool) []string) (float64, error) {
	speed, stderr, err := runSteadyBench(ctx, ffmpeg, args(true))
	// -stats_period is ffmpeg 4.4. Older builds report every half second.
	if err != nil && ctx.Err() == nil && strings.Contains(stderr, "stats_period") {
		speed, stderr, err = runSteadyBench(ctx, ffmpeg, args(false))
	}
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("%w: %s", err, tailLines(stderr, 3))
	}
	return speed, nil
}

func runSteadyBench(ctx context.Context, ffmpeg string, args []string) (float64, string, error) {
	cmd := benchCommand(ctx, ffmpeg, args)
	progress := &progressWriter{now: time.Now}
	var stderr bytes.Buffer
	cmd.Stdout = progress
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, stderr.String(), err
	}
	if speed := steadySpeed(progress.samples); speed > 0 {
		return speed, "", nil
	}
	if progress.cumulative > 0 {
		return progress.cumulative, "", nil
	}
	return 0, stderr.String(), fmt.Errorf("encoder did not report a speed")
}

func progressArgs(fine bool) []string {
	args := []string{"-hide_banner", "-nostdin", "-nostats", "-progress", "pipe:1"}
	if fine {
		args = append(args, "-stats_period", "0.1")
	}
	return args
}

// softwareBenchArgs is five seconds so that the frames libx264 holds in its
// frame threads when the first one comes out are a small part of the run.
func softwareBenchArgs(fine bool) []string {
	return append(progressArgs(fine),
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=60:duration=5",
		"-c:v", "libx264", "-preset", "veryfast", "-f", "null", "-")
}

func liveBenchArgs(fine bool) []string {
	graph, codec := liveBenchGraph()
	args := append(progressArgs(fine),
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=60000/1001:duration=4",
		"-filter_complex", graph,
		"-map", "[v]",
	)
	args = append(args, codec...)
	return append(args, "-f", "null", "-")
}

// liveBenchGraph is the video filter and encoder renditionArgs builds for a
// 1080 transcode of an HD MPEG-2 broadcast on libx264. The lavfi source is
// progressive, so tinterlace makes the interlaced frames the live path is fed.
func liveBenchGraph() (string, []string) {
	g := Graph{VideoCodec: "MPEG2", Profile: renditionProfile("1080"), Encoder: "libx264"}
	width, height, rate := outputSize(g, true)
	fps, _ := pictureRate(g, true)
	vf := "tinterlace=mode=interleave_top,setfield=tff,setparams=colorspace=bt709:color_primaries=bt709:color_trc=bt709," + videoFilter(g, "", true, true, width, height, fps)
	graph, ok := weaveFieldKey(vf, "0:v")
	if !ok {
		graph = vf
	}
	return graph, videoCodec("libx264", rate, sourceKeyint)
}

type progressSample struct {
	at  time.Time
	out time.Duration
}

// progressWriter stamps each out_time_us line from ffmpeg -progress as it
// arrives and keeps the last cumulative speed. It is the command's Stdout
// rather than a pipe read before Wait, so WaitDelay still ends a bench whose
// output something else holds open.
type progressWriter struct {
	now        func() time.Time
	partial    []byte
	samples    []progressSample
	cumulative float64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.line(string(w.partial[:i]))
		w.partial = w.partial[i+1:]
	}
}

func (w *progressWriter) line(text string) {
	key, value, ok := strings.Cut(strings.TrimSpace(text), "=")
	if !ok {
		return
	}
	value = strings.TrimSpace(value)
	switch key {
	case "out_time_us":
		if us, err := strconv.ParseInt(value, 10, 64); err == nil {
			w.samples = append(w.samples, progressSample{at: w.now(), out: time.Duration(us) * time.Microsecond})
		}
	case "speed":
		if v, err := strconv.ParseFloat(strings.TrimSuffix(value, "x"), 64); err == nil {
			w.cumulative = v
		}
	}
}

// steadySpeed is encoded time over wall time from the first frame out to the
// last, which leaves out process startup. Frames already inside the encoder at
// the first sample still count, so it reads a few percent high; the 2.5x bar
// in PreferSoftware was set on this figure.
func steadySpeed(samples []progressSample) float64 {
	first := -1
	for i, s := range samples {
		if s.out > 0 {
			first = i
			break
		}
	}
	if first < 0 {
		return 0
	}
	last := samples[len(samples)-1]
	wall := last.at.Sub(samples[first].at)
	if wall < 200*time.Millisecond {
		return 0
	}
	return (last.out - samples[first].out).Seconds() / wall.Seconds()
}

func tailLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

func benchArgs(encoder string) []string {
	// Three seconds. See BenchEncoder.
	src := "testsrc2=size=1920x1080:rate=60:duration=3"
	input := []string{"-hide_banner", "-nostdin", "-f", "lavfi", "-i", src}
	switch {
	case vaapiFamily(encoder):
		return append([]string{
			"-hide_banner", "-nostdin",
			"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
			"-f", "lavfi", "-i", src,
			"-vf", "format=nv12,hwupload", "-c:v", encoder,
		}, append(vaapiPower(encoder), "-f", "null", "-")...)
	case strings.Contains(encoder, "videotoolbox"), encoder == "h264_nvenc", encoder == "h264_qsv", encoder == "hevc_qsv":
		return append(input, "-pix_fmt", "yuv420p", "-c:v", encoder, "-f", "null", "-")
	default:
		return append(input, "-c:v", "libx264", "-preset", "veryfast", "-f", "null", "-")
	}
}
