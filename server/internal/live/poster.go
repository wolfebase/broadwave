package live

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"broadwave/internal/store"
)

// posterTries are where a poster is looked for, as parts of the recording's
// length. A recording starts a little before its show, often in a break, so
// a picture near the start is often an ad's end card or a station slate.
var posterTries = []float64{0.10, 0.25, 0.40, 0.60}

// posterTriesBlind are the seconds tried when the length is not known.
var posterTriesBlind = []float64{60, 300, 900}

const (
	// flatPicture is the luma entropy, in bits, under which a still is one
	// colour or nearly: a slate, a fade, a logo on a plain card. A scene is
	// above 5.
	flatPicture = 3.5
	// plainPicture is the entropy from which a still is a scene and the
	// search stops.
	plainPicture = 5.0
	// posterRetry is how long a recording with no picture for a poster waits
	// before it is looked at again.
	posterRetry = 10 * time.Minute
)

// ErrNoPoster is a recording where no still tried has a picture in it.
var ErrNoPoster = errors.New("no picture for a poster")

// PosterPath is where recording id's poster is kept. The name has a version:
// posters before it were the picture 2 s in, often a flat card, and are made
// again.
func PosterPath(dir string, id int64) string {
	return filepath.Join(dir, "posters", strconv.FormatInt(id, 10)+"-2.jpg")
}

// oldPosterPath is where a poster was kept before PosterPath.
func oldPosterPath(dir string, id int64) string {
	return filepath.Join(dir, "posters", strconv.FormatInt(id, 10)+".jpg")
}

// PosterTimes are the seconds tried for a poster of a recording length long,
// in order. A time inside a break moves to just after it.
func PosterTimes(length float64, breaks []store.Marker) []float64 {
	if length <= 0 {
		return posterTriesBlind
	}
	times := make([]float64, 0, len(posterTries))
	for _, part := range posterTries {
		at := math.Round(length * part)
		for _, b := range breaks {
			if at >= b.Start && at < b.End && math.Round(b.End)+2 < length {
				at = math.Round(b.End) + 2
			}
		}
		times = append(times, at)
	}
	return times
}

type posterMisses struct {
	mu   sync.Mutex
	when map[int64]time.Time
}

func (m *posterMisses) recent(id int64, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.when[id]
	return ok && now.Sub(at) < posterRetry
}

func (m *posterMisses) set(id int64, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.when == nil {
		m.when = map[int64]time.Time{}
	}
	m.when[id] = now
}

// RecordingPoster returns a JPEG for recording id's card: the first of a few
// stills through it that is a scene, or the one with the most picture. A
// finished recording keeps it until it is removed. One still recording
// (growing) gets the still itself, looked for again as the file grows, and
// keeps no poster. When every still tried is flat there is no poster, and
// the recording is looked at again after posterRetry. measure gives the
// length and breaks, only when a poster is made.
func (h *Hub) RecordingPoster(ctx context.Context, id int64, src string, growing bool, measure func() (float64, []store.Marker)) (string, error) {
	dest := PosterPath(h.Dir, id)
	if !growing {
		if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
			return dest, nil
		}
	}
	if h.posters.recent(id, time.Now()) {
		return "", ErrNoPoster
	}
	length, breaks := measure()
	if growing {
		if length < 10 {
			return "", ErrNoPoster
		}
		// Steps keep the times, and so the stills made, few while it grows.
		step := 10.0
		switch {
		case length >= 600:
			step = 600
		case length >= 60:
			step = 60
		}
		length = math.Floor(length/step) * step
	}
	best, bestScore := "", -1.0
	for _, at := range PosterTimes(length, breaks) {
		still, err := h.RecordingStill(ctx, id, src, at, 480)
		if errors.Is(err, ErrStillBusy) {
			return "", err
		}
		if err != nil {
			continue
		}
		score, err := pictureScore(still)
		if err != nil {
			continue
		}
		if score > bestScore {
			best, bestScore = still, score
		}
		if score >= plainPicture {
			break
		}
	}
	if bestScore < flatPicture {
		h.posters.set(id, time.Now())
		return "", ErrNoPoster
	}
	if growing {
		return best, nil
	}
	data, err := os.ReadFile(best)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	// Two first requests at once each write their own file.
	part, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(part.Name())
	_, err = part.Write(data)
	if closeErr := part.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(part.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(part.Name(), dest)
	}
	if err != nil {
		return "", err
	}
	_ = os.Remove(oldPosterPath(h.Dir, id))
	return dest, nil
}

// pictureScore is how much picture a JPEG has: the entropy of its luma, in
// bits. One flat colour is 0; a busy scene is about 7.
func pictureScore(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		return 0, err
	}
	var hist [256]int
	n := 0
	b := img.Bounds()
	if ycc, ok := img.(*image.YCbCr); ok {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				hist[ycc.Y[ycc.YOffset(x, y)]]++
				n++
			}
		}
	} else {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				hist[color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y]++
				n++
			}
		}
	}
	if n == 0 {
		return 0, nil
	}
	var bits float64
	for _, c := range hist {
		if c > 0 {
			p := float64(c) / float64(n)
			bits -= p * math.Log2(p)
		}
	}
	return bits, nil
}
