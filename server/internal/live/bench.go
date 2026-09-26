package live

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

// BenchEncoder encodes three seconds of 1080p60 and reports how fast it ran.
// One second is mostly process startup, so a fast GPU looks too slow and the
// budget drops a picture it can hold. The graph is a test picture, not a
// broadcast, and it does not deinterlace.
func BenchEncoder(ctx context.Context, ffmpeg, encoder string) (float64, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if encoder == "" {
		encoder = "libx264"
	}
	cmd := exec.CommandContext(ctx, ffmpeg, benchArgs(encoder)...)
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

func benchArgs(encoder string) []string {
	// Three seconds. See BenchEncoder.
	src := "testsrc2=size=1920x1080:rate=60:duration=3"
	input := []string{"-hide_banner", "-nostdin", "-f", "lavfi", "-i", src}
	switch {
	case vaapiFamily(encoder):
		return []string{
			"-hide_banner", "-nostdin",
			"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
			"-f", "lavfi", "-i", src,
			"-vf", "format=nv12,hwupload", "-c:v", encoder, "-f", "null", "-",
		}
	case strings.Contains(encoder, "videotoolbox"), encoder == "h264_nvenc", encoder == "h264_qsv", encoder == "hevc_qsv":
		return append(input, "-pix_fmt", "yuv420p", "-c:v", encoder, "-f", "null", "-")
	default:
		return append(input, "-c:v", "libx264", "-preset", "veryfast", "-f", "null", "-")
	}
}
