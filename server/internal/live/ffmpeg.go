package live

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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
