package breaks

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Frame size and rate of the picture the scan reads: small enough to keep a
// whole recording's edges in memory, big enough to see a station logo.
const (
	frameW = 160
	frameH = 90
)

type span struct{ Start, End float64 }

// Cues are what one ffmpeg pass over a recording finds.
type Cues struct {
	Blacks   []span
	Silences []span
	Cuts     []float64
	// Logo is, per second, whether the station logo is on screen. It is nil
	// when the recording shows no logo steady enough to follow.
	Logo []bool
	// Prints are a 64-bit picture hash per second, for repeated spots.
	Prints []uint64
	// Known are the stretches where a spot seen in a sure break plays.
	Known  []span
	Length float64
}

var (
	blackLine   = regexp.MustCompile(`black_start:\s*([0-9.]+) black_end:\s*([0-9.]+)`)
	silenceFrom = regexp.MustCompile(`silence_start: (-?[0-9.]+)`)
	silenceTo   = regexp.MustCompile(`silence_end: ([0-9.]+)`)
	cutLine     = regexp.MustCompile(`lavfi\.scd\.time: ([0-9.]+)`)
)

// logTail is what a failed scan says about itself.
type logTail struct {
	last    string
	noSound bool
}

// readLog fills the black, silence, and scene cut cues from ffmpeg's log.
func (c *Cues) readLog(r io.Reader) logTail {
	var tail logTail
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	open := -1.0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "a:0") && strings.Contains(line, "matches no streams") {
			tail.noSound = true
		}
		if m := blackLine.FindStringSubmatch(line); m != nil {
			start, _ := strconv.ParseFloat(m[1], 64)
			end, _ := strconv.ParseFloat(m[2], 64)
			if end > start {
				c.Blacks = append(c.Blacks, span{start, end})
			}
			continue
		}
		if m := silenceFrom.FindStringSubmatch(line); m != nil {
			open, _ = strconv.ParseFloat(m[1], 64)
			open = max(open, 0)
			continue
		}
		if m := silenceTo.FindStringSubmatch(line); m != nil && open >= 0 {
			end, _ := strconv.ParseFloat(m[1], 64)
			if end > open {
				c.Silences = append(c.Silences, span{open, end})
			}
			open = -1
			continue
		}
		if m := cutLine.FindStringSubmatch(line); m != nil {
			at, _ := strconv.ParseFloat(m[1], 64)
			c.Cuts = append(c.Cuts, at)
			continue
		}
		if strings.TrimSpace(line) != "" {
			tail.last = line
		}
	}
	_, _ = io.Copy(io.Discard, r)
	return tail
}

// scanFrames is what a scan keeps of its frames: edges for the logo and a
// hash per second. The edges cost 1.8 kB a second, about 26 MB for four hours.
type scanFrames struct {
	edges  []bitmap
	prints []uint64
}

// readFrames reads one gray frame per second.
func readFrames(r io.Reader) (scanFrames, error) {
	var out scanFrames
	frame := make([]byte, frameW*frameH)
	for {
		if _, err := io.ReadFull(r, frame); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return out, nil
			}
			return out, err
		}
		out.edges = append(out.edges, edgeMap(frame))
		out.prints = append(out.prints, hashFrame(frame))
	}
}

func (c *Cues) addFrames(f scanFrames) {
	c.Logo = findLogo(f.edges)
	c.Prints = f.prints
	c.Length = max(c.Length, float64(len(f.edges)))
}
