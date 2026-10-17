package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// StillWidths are the sizes a recording still is made in. A request gets
// the smallest that is at least as wide as it asked, so a few files serve
// every card.
var StillWidths = []int{240, 480, 960, 1280, 1920}

// StillWidth is the size made for a request of w; 480 when w is not set.
func StillWidth(w int) int {
	if w <= 0 {
		return 480
	}
	for _, size := range StillWidths {
		if w <= size {
			return size
		}
	}
	return StillWidths[len(StillWidths)-1]
}

const (
	// stillSlots is how many stills are made at once. Each is one short
	// decode, but a chapter strip asks for a dozen together.
	stillSlots = 2
	// stillLimit is how long one still may take, the wait for a slot
	// included. A seek in a long MPEG-TS file reads a little at several
	// places; a decode past the keyframe before at reads up to a group.
	stillLimit = 15 * time.Second
)

var (
	// ErrNoStill is a time with no picture: past the end of the file, or
	// a file ffmpeg cannot read.
	ErrNoStill = errors.New("no picture at that time")
	// ErrStillBusy is a request that waited out its time for a slot.
	ErrStillBusy = errors.New("stills are busy")
)

// stillMaker makes recording stills one key at a time, a few at once.
type stillMaker struct {
	once     sync.Once
	slots    chan struct{}
	mu       sync.Mutex
	inflight map[string]chan struct{}
}

func (m *stillMaker) init() {
	m.once.Do(func() {
		m.slots = make(chan struct{}, stillSlots)
		m.inflight = map[string]chan struct{}{}
	})
}

// StillPath is where the still of recording id at seconds, width wide, is
// kept: tenths of a second are one picture.
func StillPath(dir string, id int64, at float64, width int) string {
	tenths := int64(math.Round(at * 10))
	return filepath.Join(dir, "stills", strconv.FormatInt(id, 10), fmt.Sprintf("%d-%d.jpg", tenths, StillWidth(width)))
}

// RecordingStill returns a JPEG of recording id's file src at seconds from
// its start, made once and kept until the recording is removed. Requests
// for the same picture share one ffmpeg.
func (h *Hub) RecordingStill(ctx context.Context, id int64, src string, at float64, width int) (string, error) {
	if at < 0 || math.IsNaN(at) || math.IsInf(at, 0) {
		return "", ErrNoStill
	}
	dest := StillPath(h.Dir, id, at, width)
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return dest, nil
	}
	m := &h.stills
	m.init()
	ctx, cancel := context.WithTimeout(ctx, stillLimit)
	defer cancel()
	for {
		m.mu.Lock()
		wait, busy := m.inflight[dest]
		if !busy {
			done := make(chan struct{})
			m.inflight[dest] = done
			m.mu.Unlock()
			defer func() {
				m.mu.Lock()
				delete(m.inflight, dest)
				m.mu.Unlock()
				close(done)
			}()
			break
		}
		m.mu.Unlock()
		select {
		case <-wait:
			if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
				return dest, nil
			}
		case <-ctx.Done():
			return "", ErrStillBusy
		}
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		return "", ErrStillBusy
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	part := dest + ".part"
	defer os.Remove(part)
	cmd := exec.CommandContext(ctx, h.FFmpeg, StillArgs(src, at, StillWidth(width), part)...)
	err := cmd.Run()
	if info, statErr := os.Stat(part); statErr == nil && info.Size() > 0 {
		return dest, os.Rename(part, dest)
	}
	if ctx.Err() != nil {
		return "", ErrStillBusy
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoStill, err)
	}
	return "", ErrNoStill
}

// stillBack is how far before the picture the input seek lands. MPEG-TS has
// no index, so the seek lands near its time but not on a keyframe; from 3 s
// back, a broadcast group of pictures has started by the picture, and the
// decode runs on to it.
const stillBack = 3.0

// StillArgs is ffmpeg for one still of src at seconds. An interlaced picture
// is deinterlaced: a field pair would show its comb. JPEG is full range.
func StillArgs(src string, at float64, width int, dest string) []string {
	back := min(at, stillBack)
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-ss", strconv.FormatFloat(at-back, 'f', 3, 64),
		"-i", src,
		"-ss", strconv.FormatFloat(back, 'f', 3, 64),
		"-map", "0:v:0", "-an", "-sn", "-dn",
		"-frames:v", "1",
		"-vf", fmt.Sprintf("yadif=deint=interlaced,scale=%d:-2:out_range=full", width),
		"-pix_fmt", "yuvj420p", "-q:v", "3",
		"-f", "image2", dest,
	}
}
