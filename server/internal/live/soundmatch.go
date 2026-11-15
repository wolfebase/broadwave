package live

import (
	"encoding/binary"
	"math"
)

// A station's two language tracks share the game's crowd and effects, so
// they sound alike only where nobody speaks. The station's other broadcast
// carries the same commentary as one of them, seconds apart. Loudness every
// 10 ms, with its slow drift taken out, follows the words: the track whose
// envelope lines up with the twin's main, at some lag, is the same language.

const (
	// soundRate is what a sound check decodes to, mono; soundStep is the
	// samples in one envelope step (10 ms).
	soundRate = 8000
	soundStep = soundRate / 100
	// soundDrift is the steps a moving mean spans, taken out of the envelope
	// so a track mixed louder does not count as different words.
	soundDrift = 100
	// soundLoud is the RMS (of 32768) a step needs to count as sound, and
	// soundLoudShare the share of steps that must: a muted or dead track
	// says nothing.
	soundLoud      = 64
	soundLoudShare = 0.3
	// soundShortest is the least twin sound, in steps, worth comparing.
	soundShortest = 5 * 100
)

// The verdict's bars. A twin's main must match one track by soundMatchMin,
// beat the other by soundMargin, and the two tracks must not be one sound.
const (
	soundMatchMin = 0.4
	soundMargin   = 0.15
	soundSame     = 0.85
	// soundSameLag is the steps either way the two tracks of one stream are
	// compared over: they start decoding on their own I-frames.
	soundSameLag = 300
)

// soundEnvelope is mono s16le pcm at soundRate as loudness in dB every
// 10 ms, less its moving mean over a second. ok is false when too little of
// it is sound.
func soundEnvelope(pcm []byte) ([]float64, bool) {
	n := len(pcm) / 2 / soundStep
	if n == 0 {
		return nil, false
	}
	env := make([]float64, n)
	loud := 0
	for i := range n {
		var sum float64
		for j := range soundStep {
			off := (i*soundStep + j) * 2
			s := float64(int16(binary.LittleEndian.Uint16(pcm[off:])))
			sum += s * s
		}
		ms := sum / soundStep
		if math.Sqrt(ms) >= soundLoud {
			loud++
		}
		env[i] = 10 * math.Log10(ms+1)
	}
	if float64(loud) < soundLoudShare*float64(n) {
		return nil, false
	}
	prefix := make([]float64, n+1)
	for i, v := range env {
		prefix[i+1] = prefix[i] + v
	}
	out := make([]float64, n)
	half := soundDrift / 2
	for i := range n {
		lo, hi := max(0, i-half), min(n, i+half+1)
		out[i] = env[i] - (prefix[hi]-prefix[lo])/float64(hi-lo)
	}
	return out, true
}

// soundMatch slides short along long and returns the best correlation at any
// offset where short lies wholly inside long; 0 when it cannot fit.
func soundMatch(long, short []float64) float64 {
	n := len(short)
	if n == 0 || n > len(long) {
		return 0
	}
	var mean float64
	for _, v := range short {
		mean += v
	}
	mean /= float64(n)
	centered := make([]float64, n)
	var norm float64
	for i, v := range short {
		centered[i] = v - mean
		norm += centered[i] * centered[i]
	}
	if norm == 0 {
		return 0
	}
	norm = math.Sqrt(norm)
	sum := make([]float64, len(long)+1)
	sq := make([]float64, len(long)+1)
	for i, v := range long {
		sum[i+1] = sum[i] + v
		sq[i+1] = sq[i] + v*v
	}
	best := 0.0
	for k := 0; k+n <= len(long); k++ {
		s := sum[k+n] - sum[k]
		varSum := sq[k+n] - sq[k] - s*s/float64(n)
		if varSum <= 1e-9 {
			continue
		}
		var dot float64
		for i, c := range centered {
			dot += long[k+i] * c
		}
		if r := dot / (math.Sqrt(varSum) * norm); r > best {
			best = r
		}
	}
	return best
}

// soundVerdict is one check's answer: whether the track labelled second
// language carries the twin's main language, and the scores behind it. ok
// is false when the sound does not decide it, with why.
type soundVerdict struct {
	swapped bool
	ok      bool
	why     string
	// main and lang are how well the twin's main matched each track; same
	// is how alike the two tracks are.
	main, lang, same float64
}

// judgeSound compares the twin's main with the channel's main and
// second-language tracks, all envelopes from soundEnvelope.
func judgeSound(twin, main, lang []float64) soundVerdict {
	if len(twin) < soundShortest {
		return soundVerdict{why: "the other broadcast's sound was too short"}
	}
	if len(main) < len(twin) || len(lang) < len(twin) {
		return soundVerdict{why: "the channel's sound was too short"}
	}
	v := soundVerdict{main: soundMatch(main, twin), lang: soundMatch(lang, twin)}
	if len(lang) > 2*soundSameLag+soundShortest {
		v.same = soundMatch(main, lang[soundSameLag:len(lang)-soundSameLag])
	}
	best, other := v.main, v.lang
	if v.lang > v.main {
		best, other = v.lang, v.main
	}
	switch {
	case v.same >= soundSame:
		v.why = "the two tracks sound the same"
	case best < soundMatchMin:
		v.why = "neither track matched the other broadcast"
	case best-other < soundMargin:
		v.why = "both tracks matched the other broadcast about as well"
	default:
		v.ok = true
		v.swapped = v.lang > v.main
	}
	return v
}
