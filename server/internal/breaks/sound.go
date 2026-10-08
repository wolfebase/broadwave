package breaks

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/cmplx"
	"strconv"
	"time"
)

// Sound prints are 32-bit fingerprints of the sound, one per hop, after
// Haitsma and Kalker: the sign of how the energy in neighboring bands changes
// from one frame to the next. They survive a different encode of the same
// broadcast and a frame that starts a little off.
const (
	soundRate  = 8000
	soundFrame = 2048 // 256 ms
	soundHop   = 512  // 64 ms
	soundBands = 33
	// HopSeconds is the time between two sound prints.
	HopSeconds = float64(soundHop) / soundRate
)

// quietPrint marks a frame with too little sound to print; it matches nothing.
const quietPrint = 0

// Sound is the start and the end of a recording, printed.
type Sound struct {
	Head []uint32
	// Tail starts TailFrom seconds into the recording.
	Tail     []uint32
	TailFrom float64
}

// How much of each end is printed. An intro comes after a cold open of up to
// about ten minutes and the recording's early start; end titles come in the
// last minutes of the show, before the late padding.
const (
	headSeconds = 20 * 60
	tailSeconds = 10 * 60
)

// soundLimit stops one end's decode that hangs.
var soundLimit = 10 * time.Minute

// ListenEnds prints the first and the last minutes of a recording's sound.
// length is the recording's length in seconds; with none, only the start is
// printed. A recording with no sound has no prints.
func ListenEnds(ctx context.Context, ffmpeg, path string, length float64) (Sound, error) {
	var s Sound
	head, err := listen(ctx, ffmpeg, path, 0, headSeconds)
	if err != nil {
		return s, err
	}
	s.Head = head
	if length > headSeconds {
		s.TailFrom = max(headSeconds, length-tailSeconds)
		tail, err := listen(ctx, ffmpeg, path, s.TailFrom, length-s.TailFrom)
		if err != nil {
			return s, err
		}
		s.Tail = tail
	}
	return s, nil
}

func listenArgs(path string, from, seconds float64) []string {
	args := []string{"-hide_banner", "-nostats", "-loglevel", "error", "-threads", "1"}
	if from > 0 {
		args = append(args, "-ss", strconv.FormatFloat(from, 'f', 3, 64))
	}
	return append(args, "-t", strconv.FormatFloat(seconds, 'f', 3, 64), "-i", path,
		"-map", "0:a:0", "-ac", "1", "-ar", strconv.Itoa(soundRate), "-f", "s16le", "pipe:1")
}

func listen(ctx context.Context, ffmpeg, path string, from, seconds float64) ([]uint32, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	ctx, cancel := context.WithTimeout(ctx, soundLimit)
	defer cancel()
	cmd := lowPriority(ctx, ffmpeg, listenArgs(path, from, seconds))
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var log logTail
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	logged := make(chan struct{})
	go func() {
		var c Cues
		log = c.readLog(stderr)
		close(logged)
	}()
	prints, readErr := PrintSound(out)
	_, _ = io.Copy(io.Discard, out)
	<-logged
	if err := cmd.Wait(); err != nil {
		if log.noSound {
			return nil, nil
		}
		return nil, fmt.Errorf("listen failed: %w: %s", err, log.last)
	}
	return prints, readErr
}

// PrintSound prints 8 kHz mono 16-bit little-endian sound.
func PrintSound(r io.Reader) ([]uint32, error) {
	p := newPrinter()
	buf := make([]byte, 2*soundHop)
	hop := make([]float64, soundHop)
	var out []uint32
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return out, nil
			}
			return out, err
		}
		for i := range hop {
			hop[i] = float64(int16(binary.LittleEndian.Uint16(buf[2*i:]))) / 32768
		}
		if print, ok := p.push(hop); ok {
			out = append(out, print)
		}
	}
}

type printer struct {
	window  []float64
	samples []float64
	edges   [soundBands + 1]int
	last    [soundBands]float64
	primed  bool
	filled  int
	scratch []complex128
}

func newPrinter() *printer {
	p := &printer{
		window:  make([]float64, soundFrame),
		samples: make([]float64, soundFrame),
		scratch: make([]complex128, soundFrame),
	}
	for i := range p.window {
		p.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(soundFrame-1))
	}
	// Bands spaced evenly in pitch from 300 Hz to 2 kHz, where most of the
	// energy of speech and music sits and a broadcast codec keeps it.
	lo, hi := 300.0, 2000.0
	for b := range p.edges {
		f := lo * math.Pow(hi/lo, float64(b)/soundBands)
		p.edges[b] = int(math.Round(f * soundFrame / soundRate))
	}
	return p
}

// push adds a hop of samples and prints the frame that now ends there.
func (p *printer) push(hop []float64) (uint32, bool) {
	copy(p.samples, p.samples[len(hop):])
	copy(p.samples[soundFrame-len(hop):], hop)
	if p.filled < soundFrame {
		p.filled += len(hop)
		if p.filled < soundFrame {
			return 0, false
		}
	}
	for i, v := range p.samples {
		p.scratch[i] = complex(v*p.window[i], 0)
	}
	fft(p.scratch)
	var energy [soundBands]float64
	total := 0.0
	for b := 0; b < soundBands; b++ {
		for k := p.edges[b]; k < max(p.edges[b+1], p.edges[b]+1); k++ {
			e := cmplx.Abs(p.scratch[k])
			energy[b] += e * e
		}
		total += energy[b]
	}
	prev, primed := p.last, p.primed
	p.last, p.primed = energy, true
	// About -55 dB: silence, or a hiss no other recording shares.
	if !primed || total < 1 {
		return quietPrint, true
	}
	var print uint32
	for m := 0; m < soundBands-1; m++ {
		if (energy[m]-energy[m+1])-(prev[m]-prev[m+1]) > 0 {
			print |= 1 << m
		}
	}
	if print == quietPrint {
		print = 1
	}
	return print, true
}

// fft transforms x in place; its length is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				a, b := x[start+k], x[start+k+size/2]*w
				x[start+k], x[start+k+size/2] = a+b, a-b
				w *= step
			}
		}
	}
}
