package live

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"broadwave/internal/fetchguard"
)

// urlProtocols limits a remote ffmpeg input to http and https.
// A local path and a pipe are left alone. file:/path has a scheme and no
// double slash, and it is limited too.
func urlProtocols(args []string, input string) []string {
	if inputHasScheme(input) {
		return append(args, "-protocol_whitelist", fetchguard.FFmpegProtocols)
	}
	return args
}

func inputHasScheme(input string) bool {
	for _, part := range strings.Split(input, "|") {
		part = strings.TrimPrefix(part, "concat:")
		// pipe:0 is the tuner. It is not a URL.
		if part == "pipe:0" || strings.HasPrefix(part, "pipe:") {
			continue
		}
		if strings.Contains(part, "://") {
			return true
		}
		if i := strings.Index(part, ":"); i > 1 && !strings.Contains(part[:i], "/") {
			return true
		}
	}
	return false
}

func DetectEncoder(ffmpeg string) string {
	// VAAPI is probed before QSV. jellyfin-ffmpeg's QSV check succeeds on
	// UHD 770, and the measured picture path is VAAPI.
	for _, name := range []string{"h264_nvenc", "h264_videotoolbox"} {
		cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=size=160x120:rate=30:duration=0.2",
			"-pix_fmt", "yuv420p", "-c:v", name, "-f", "null", "-")
		if cmd.Run() == nil {
			return name
		}
	}
	vaapi := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=30:duration=0.2",
		"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-f", "null", "-")
	if vaapi.Run() == nil {
		return "h264_vaapi"
	}
	qsv := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=30:duration=0.2",
		"-pix_fmt", "yuv420p", "-c:v", "h264_qsv", "-f", "null", "-")
	if qsv.Run() == nil {
		return "h264_qsv"
	}
	return "libx264"
}

// MissingAC4 reports an ffmpeg that lists its decoders without AC-4, the
// sound of ATSC 3.0. jellyfin-ffmpeg has one; a stock build such as
// Homebrew's does not. An ffmpeg that does not answer is not judged.
func MissingAC4(ffmpeg string) bool {
	if ffmpeg == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-decoders")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return err == nil && listsDecoder(string(out), "aac") && !listsDecoder(string(out), "ac4")
}

// listsDecoder finds name in `ffmpeg -decoders` output, whose lines are
// " A....D ac4                  AC-4".
func listsDecoder(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == name {
			return true
		}
	}
	return false
}

// ProbeHEVC reports whether this machine can encode HEVC with the same hardware family.
func ProbeHEVC(ffmpeg, encoder string) bool {
	if ffmpeg == "" {
		return false
	}
	name := OutputEncoder(encoder, "hevc")
	if name == "libx265" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error"}
	if vaapiFamily(name) {
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc=size=160x120:rate=30:duration=0.2", "-pix_fmt", "yuv420p")
	if vaapiFamily(name) {
		args = append(args, "-vf", "format=nv12,hwupload")
	}
	args = append(args, "-c:v", name, "-f", "null", "-")
	return exec.CommandContext(ctx, ffmpeg, args...).Run() == nil
}

// ProbeLowPower returns the VAAPI encoders of this family that run on the
// GPU's fixed-function encoder (Intel VDEnc). Drivers without it refuse
// low_power, and older Intel chips have it for H.264 but not HEVC. The test
// picture is 320x240: a UHD 770 refuses low-power HEVC at 160x120.
func ProbeLowPower(ffmpeg, encoder string) []string {
	if !vaapiFamily(encoder) || ffmpeg == "" {
		return nil
	}
	var found []string
	for _, name := range []string{"h264_vaapi", "hevc_vaapi"} {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
			"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
			"-f", "lavfi", "-i", "testsrc=size=320x240:rate=30:duration=0.2",
			"-vf", "format=nv12,hwupload", "-c:v", name, "-low_power", "1", "-bf", "0", "-f", "null", "-")
		if cmd.Run() == nil {
			found = append(found, name)
		}
		cancel()
	}
	return found
}

// ProbeDeint reports which VAAPI deinterlacers this machine can run.
// Broadcast is motion adaptive. Smooth prefers motion compensated when it exists.
func ProbeDeint(ffmpeg, encoder string) (broadcast, smooth string) {
	if encoder != "h264_vaapi" || ffmpeg == "" {
		return "", ""
	}
	if deintWorks(ffmpeg, "motion_adaptive") {
		broadcast = "motion_adaptive"
		smooth = "motion_adaptive"
	}
	if deintWorks(ffmpeg, "motion_compensated") {
		smooth = "motion_compensated"
	}
	return broadcast, smooth
}

func deintWorks(ffmpeg, mode string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	vf := "format=nv12,hwupload,deinterlace_vaapi=mode=" + mode + ":rate=field,scale_vaapi=w=320:h=240"
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=30:duration=0.2",
		"-vf", vf, "-c:v", "h264_vaapi", "-f", "null", "-")
	return cmd.Run() == nil
}

func copyArgs(program int, input, userAgent, referrer, path string) []string {
	if input == "" {
		input = "pipe:0"
	}
	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+genpts+discardcorrupt",
	}
	if strings.Contains(input, "://") {
		args = urlProtocols(args, input)
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
	}
	args = append(args, headerArgs(userAgent, referrer)...)
	args = append(args, "-i", input)
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d:v:0", program), "-map", fmt.Sprintf("0:p:%d:a:0", program))
	} else {
		args = append(args, "-map", "0:v:0", "-map", "0:a:0")
	}
	args = append(args, "-c", "copy", "-f", "mpegts", path)
	return args
}

func headerArgs(userAgent, referrer string) []string {
	userAgent = fetchguard.OneLine(userAgent)
	referrer = fetchguard.OneLine(referrer)
	var b strings.Builder
	if userAgent != "" {
		b.WriteString("User-Agent: ")
		b.WriteString(userAgent)
		b.WriteString("\r\n")
	}
	if referrer != "" {
		b.WriteString("Referer: ")
		b.WriteString(referrer)
		b.WriteString("\r\n")
	}
	if b.Len() == 0 {
		return nil
	}
	return []string{"-headers", b.String()}
}
