package live

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	matches, _ := filepath.Glob("/sys/class/drm/renderD*/device/vendor")
	if len(matches) == 0 {
		matches, _ = filepath.Glob("/sys/class/drm/card*/device/vendor")
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(raw)) {
		case "0x8086":
			return "Intel GPU"
		case "0x1002", "0x1022":
			return "AMD GPU"
		}
	}
	return "GPU"
}

// FormatEncoderLine is the setup sentence for a one-second 1080p60 encode.
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

// BenchEncoder encodes one second of 1080p60 and reports how fast it ran.
// The graph is a test picture, not a broadcast, and it does not deinterlace.
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
	input := []string{"-hide_banner", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=60:duration=1"}
	switch {
	case vaapiFamily(encoder):
		return []string{
			"-hide_banner", "-nostdin",
			"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va",
			"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=60:duration=1",
			"-vf", "format=nv12,hwupload", "-c:v", encoder, "-f", "null", "-",
		}
	case strings.Contains(encoder, "videotoolbox"), encoder == "h264_nvenc", encoder == "h264_qsv", encoder == "hevc_qsv":
		return append(input, "-pix_fmt", "yuv420p", "-c:v", encoder, "-f", "null", "-")
	default:
		return append(input, "-c:v", "libx264", "-preset", "veryfast", "-f", "null", "-")
	}
}
