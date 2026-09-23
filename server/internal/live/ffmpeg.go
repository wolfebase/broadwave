package live

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func DetectEncoder(ffmpeg string) string {
	for _, name := range []string{"h264_nvenc", "h264_qsv", "h264_videotoolbox"} {
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
	return "libx264"
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

// ProbeBlend reports whether a light 30p-to-60p blend holds realtime at 1080p.
// A failed probe leaves Smooth on the broadcast picture instead of dropping frames.
func ProbeBlend(ffmpeg string) bool {
	if ffmpeg == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=30:duration=1",
		"-vf", "minterpolate=fps=60:mi_mode=blend", "-f", "null", "-")
	if cmd.Run() != nil {
		return false
	}
	return time.Since(start) <= 2500*time.Millisecond
}

func copyArgs(program int, path string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+genpts+discardcorrupt",
		"-i", "pipe:0",
	}
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d:v:0", program), "-map", fmt.Sprintf("0:p:%d:a:0", program))
	} else {
		args = append(args, "-map", "0:v:0", "-map", "0:a:0")
	}
	args = append(args, "-c", "copy", "-f", "mpegts", path)
	return args
}
