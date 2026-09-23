package live

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var blackLine = regexp.MustCompile(`black_start:([0-9.]+) black_end:([0-9.]+)`)

// Break is a span that looks like a commercial break: a run of black frames.
type Break struct {
	Start float64
	End   float64
}

// ParseBreaks reads ffmpeg blackdetect log lines.
func ParseBreaks(log string) []Break {
	var out []Break
	for _, match := range blackLine.FindAllStringSubmatch(log, -1) {
		start, err1 := strconv.ParseFloat(match[1], 64)
		end, err2 := strconv.ParseFloat(match[2], 64)
		if err1 != nil || err2 != nil || end-start < 0.4 {
			continue
		}
		out = append(out, Break{Start: start, End: end})
	}
	return out
}

// DetectBreaks scans a recording for black stretches. It does not modify the file.
func DetectBreaks(ffmpeg, path string) ([]Break, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", path, "-vf", "blackdetect=d=0.4:pic_th=0.90:pix_th=0.10", "-an", "-f", "null", "-")
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil && !cmdOk(err) {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, statErr
		}
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	return ParseBreaks(buf.String()), nil
}

func cmdOk(err error) bool {
	return err == nil
}

// IndexBreaks prefers comskip when it is installed and returns black-frame spans otherwise.
func IndexBreaks(ffmpeg, path string) ([]Break, error) {
	if breaks, ok := comskipBreaks(path); ok {
		return breaks, nil
	}
	return DetectBreaks(ffmpeg, path)
}

func comskipBreaks(path string) ([]Break, bool) {
	exe, err := exec.LookPath("comskip")
	if err != nil {
		return nil, false
	}
	cmd := exec.Command(exe, path)
	cmd.Dir = filepath.Dir(path)
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	edl := strings.TrimSuffix(path, filepath.Ext(path)) + ".edl"
	body, err := os.ReadFile(edl)
	if err != nil {
		return nil, false
	}
	parsed := parseEDL(string(body))
	return parsed, len(parsed) > 0
}

func parseEDL(text string) []Break {
	var out []Break
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		start, err1 := strconv.ParseFloat(fields[0], 64)
		end, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		out = append(out, Break{Start: start, End: end})
	}
	return out
}
