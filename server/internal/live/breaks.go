package live

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var blackLine = regexp.MustCompile(`black_start:([0-9.]+) black_end:([0-9.]+)`)

// Break is a span that looks like a commercial break.
type Break struct {
	Start float64
	End   float64
}

// ParseBreaks reads ffmpeg blackdetect log lines and returns the commercial
// breaks in them. A black stretch on its own is a fade or a scene change; a
// station puts a few black frames between spots, so a break is a run of
// spot-length gaps between black stretches.
func ParseBreaks(log string) []Break {
	var blacks []Break
	for _, match := range blackLine.FindAllStringSubmatch(log, -1) {
		start, err1 := strconv.ParseFloat(match[1], 64)
		end, err2 := strconv.ParseFloat(match[2], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		blacks = append(blacks, Break{Start: start, End: end})
	}
	return spotRuns(blacks)
}

// minSpots is how many spots in a row make a break. Skipping a show's own
// scenes is worse than missing a break, and a break runs several spots.
const minSpots = 3

// spotRuns groups black stretches into breaks: at least minSpots gaps in a
// row that each last as long as a spot.
func spotRuns(blacks []Break) []Break {
	var out []Break
	first, spots := 0, 0
	flush := func(last int) {
		if spots >= minSpots {
			out = append(out, Break{Start: blacks[first].Start, End: blacks[last].End})
		}
	}
	for i := 1; i < len(blacks); i++ {
		gap := (blacks[i].Start+blacks[i].End)/2 - (blacks[i-1].Start+blacks[i-1].End)/2
		if spotLength(gap) {
			if spots == 0 {
				first = i - 1
			}
			spots++
			continue
		}
		flush(i - 1)
		spots = 0
	}
	flush(len(blacks) - 1)
	return out
}

// spotLength is a gap that lasts as long as a commercial: 10 s to about two
// minutes, within half a second of a whole number of 5 s.
func spotLength(seconds float64) bool {
	if seconds < 9.5 || seconds > 125.5 {
		return false
	}
	return math.Abs(seconds-5*math.Round(seconds/5)) <= 0.5
}

// DetectBreaks scans a recording for black stretches. It does not modify the file.
func DetectBreaks(ffmpeg, path string) ([]Break, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", path, "-vf", "blackdetect=d=0.1:pic_th=0.90:pix_th=0.10", "-an", "-f", "null", "-")
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

// comskipINI asks for an EDL only. Without it comskip writes a .txt and no
// .edl, so its breaks were never read.
const comskipINI = "output_edl=1\noutput_default=0\n"

// comskipLimit stops a comskip that hangs; an hour of 1080i takes minutes.
var comskipLimit = 2 * time.Hour

// comskipBreaks runs comskip with its output in a temporary folder, so the
// recordings folder gets no logo, log, or .txt files. It runs at low priority
// on two threads: it starts when a recording ends, maybe while others play.
func comskipBreaks(path string) ([]Break, bool) {
	exe, err := exec.LookPath("comskip")
	if err != nil {
		return nil, false
	}
	dir, err := os.MkdirTemp("", "comskip")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(dir)
	ini := filepath.Join(dir, "comskip.ini")
	if err := os.WriteFile(ini, []byte(comskipINI), 0o600); err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), comskipLimit)
	defer cancel()
	args := []string{"--quiet", "--threads=2", "--ini=" + ini, "--output=" + dir, path}
	cmd := exec.CommandContext(ctx, exe, args...)
	if nice, err := exec.LookPath("nice"); err == nil {
		cmd = exec.CommandContext(ctx, nice, append([]string{"-n", "10", exe}, args...)...)
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn(fmt.Sprintf("breaks: comskip on %s: %v %s", filepath.Base(path), err, lastLine(out)))
		return nil, false
	}
	edl := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".edl")
	body, err := os.ReadFile(edl)
	if err != nil {
		return nil, false
	}
	parsed := parseEDL(string(body))
	return parsed, len(parsed) > 0
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
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
