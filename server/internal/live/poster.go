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
	"slices"
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

// posterLimit is how long one poster may take, waits for a still slot
// included.
var posterLimit = 20 * time.Second

// ErrNoPoster is a recording where no still tried has a picture in it.
var ErrNoPoster = errors.New("no picture for a poster")

// PosterOptions are a poster's size and where its picture may come from.
type PosterOptions struct {
	// Width is the size wanted, as FrameWidth reads it: 480, 1280, or 1920.
	Width int
	// Pregame keeps to the minutes around the airing's start, before a
	// game's score is on screen. Lead is that start, in seconds into the
	// file; a file that starts more than pregameAfter past it has no poster.
	Pregame bool
	Lead    float64
}

// PosterPath is where recording id's 480 poster is kept. The name has a
// version: posters before it were the picture 2 s in, often a flat card,
// and are made again.
func PosterPath(dir string, id int64) string {
	return posterPath(dir, id, PosterOptions{})
}

func posterPath(dir string, id int64, opts PosterOptions) string {
	name := strconv.FormatInt(id, 10) + "-2"
	if opts.Pregame {
		name += "-pregame"
	}
	if w := FrameWidth(opts.Width); w != FrameWidths[0] {
		name += "-" + strconv.Itoa(w)
	}
	return filepath.Join(dir, "posters", name+".jpg")
}

// removePosters removes every poster kept for recording id.
func removePosters(dir string, id int64) {
	paths, _ := filepath.Glob(filepath.Join(dir, "posters", strconv.FormatInt(id, 10)+"-*.jpg"))
	for _, path := range append(paths, oldPosterPath(dir, id)) {
		_ = os.Remove(path)
	}
}

// oldPosterPath is where a poster was kept before PosterPath.
func oldPosterPath(dir string, id int64) string {
	return filepath.Join(dir, "posters", strconv.FormatInt(id, 10)+".jpg")
}

// pregameAfter is how far past the airing's start a pregame poster may be:
// a broadcast game starts a few minutes after its listing, and until then
// the score on screen, if any, is nothing to nothing.
const pregameAfter = 120.0

// PregameTimes are the seconds tried for a poster of a game that keeps its
// score hidden: around lead, the airing's start in the file, never more than
// pregameAfter past it, and out of breaks. None when the file starts later.
func PregameTimes(length, lead float64, breaks []store.Marker) []float64 {
	var times []float64
	for i, off := range []float64{30, -60, 90, -150, 5} {
		at := math.Round(lead + off)
		if at < 1 && lead < 0 {
			// A file that starts after the listing: its first seconds,
			// spread out, up to the cut-off.
			at = math.Min(1+float64(i)*10, math.Floor(lead+pregameAfter))
		}
		for _, b := range breaks {
			if at >= b.Start && at < b.End {
				at = math.Round(b.End) + 2
			}
		}
		if at < 1 || at > lead+pregameAfter || (length > 0 && at > length-1) || slices.Contains(times, at) {
			continue
		}
		times = append(times, at)
	}
	return times
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

// posterMaker makes one poster per recording at a time and remembers
// recordings with no picture for one.
type posterMaker struct {
	mu       sync.Mutex
	when     map[string]time.Time
	inflight map[string]chan struct{}
}

func (m *posterMaker) recent(id string, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.when[id]
	return ok && now.Sub(at) < posterRetry
}

func (m *posterMaker) set(id string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.when == nil {
		m.when = map[string]time.Time{}
	}
	m.when[id] = now
}

// claim makes the caller the one making id's poster and returns its release,
// or returns the channel that closes when the one making it is done.
func (m *posterMaker) claim(id string) (release func(), wait <-chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if done, busy := m.inflight[id]; busy {
		return nil, done
	}
	if m.inflight == nil {
		m.inflight = map[string]chan struct{}{}
	}
	done := make(chan struct{})
	m.inflight[id] = done
	return func() {
		m.mu.Lock()
		delete(m.inflight, id)
		m.mu.Unlock()
		close(done)
	}, nil
}

// RecordingPoster returns a JPEG for recording id's card: the first of a few
// stills through it that is a scene, or the one with the most picture. A
// finished recording keeps it until it is removed. One still recording
// (growing) gets the still itself, looked for again as the file grows, and
// keeps no poster. When every still tried is flat there is no poster, and
// the recording is looked at again after posterRetry. measure gives the
// length and breaks, only when a poster is made. opts pick a wider poster,
// the same picture at that size, or a pregame one (PregameTimes).
//
// A list asks for many posters at once and does not ask again, so a poster
// waits its turn for a still slot for up to posterLimit, where a chapter
// still gives up sooner. Requests for one recording share one making.
func (h *Hub) RecordingPoster(ctx context.Context, id int64, src string, growing bool, opts PosterOptions, measure func() (float64, []store.Marker)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, posterLimit)
	defer cancel()
	dest := posterPath(h.Dir, id, opts)
	// Every size of one kind is the same picture, so they share the memo of
	// a recording with none.
	kind := posterPath(h.Dir, id, PosterOptions{Pregame: opts.Pregame})
	for {
		if !growing {
			if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
				return dest, nil
			}
		}
		if h.posters.recent(kind, time.Now()) {
			return "", ErrNoPoster
		}
		release, wait := h.posters.claim(dest)
		if release != nil {
			defer release()
			break
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return "", ErrStillBusy
		}
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
	times := PosterTimes(length, breaks)
	if opts.Pregame {
		times = PregameTimes(length, opts.Lead, breaks)
	}
	best, bestAt, bestScore := "", 0.0, -1.0
	for _, at := range times {
		still, err := h.still(ctx, id, src, at, 480)
		if errors.Is(err, ErrStillBusy) {
			// Out of time: a picture already found will do.
			if bestScore >= flatPicture {
				break
			}
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
			best, bestAt, bestScore = still, at, score
		}
		if score >= plainPicture {
			break
		}
	}
	if bestScore < flatPicture {
		h.posters.set(kind, time.Now())
		return "", ErrNoPoster
	}
	if width := FrameWidth(opts.Width); width != FrameWidths[0] {
		wide, err := h.still(ctx, id, src, bestAt, width)
		if err != nil {
			return "", err
		}
		best = wide
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
