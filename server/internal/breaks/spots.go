package breaks

import (
	"math/bits"
	"sort"
)

// Spot is a commercial's picture prints, one per second, from a break the
// scan was sure of. Ads repeat across channels and days; a spot seen again
// marks a break the cues alone may miss.
type Spot struct {
	Prints []uint64
	// Start is where the spot sits in the recording it came from. It is -1
	// for a spot from another recording.
	Start int
}

// A second matches when its print is within matchBits of the spot's, and a
// spot matches when matchShare of its seconds do. On two real recordings no
// looser setting (14 bits, half the seconds) matched outside a break.
const (
	matchBits  = 10
	matchShare = 0.7
)

// textured is a print with picture in it. A flat or black frame hashes to
// nearly all zeros or ones and would match any other.
func textured(p uint64) bool {
	n := bits.OnesCount64(p)
	return n > 4 && n < 60
}

// usable is a spot with enough distinct, textured seconds to tell it apart
// from a still card or a fade.
func usable(prints []uint64) bool {
	distinct := map[uint64]bool{}
	for _, p := range prints {
		if textured(p) {
			distinct[p] = true
		}
	}
	return len(prints) >= 8 && len(distinct) >= len(prints)/2
}

// spotsOf cuts the sure breaks into spots: spot-length gaps between the
// scan's boundaries, a second in from each end.
func spotsOf(c Cues, found []Break) []Spot {
	cuts := boundaries(c)
	var out []Spot
	for _, b := range found {
		if b.Confidence < AutoSkip {
			continue
		}
		var in []float64
		for _, cut := range cuts {
			if cut.At >= b.Start-0.5 && cut.At <= b.End+0.5 {
				in = append(in, cut.At)
			}
		}
		for i := 0; i+1 < len(in); {
			j := i + 1
			for j < len(in) && !spotLength(in[j]-in[i]) {
				j++
			}
			if j == len(in) {
				i++
				continue
			}
			from, to := int(in[i])+1, int(in[j])-1
			if to <= len(c.Prints) && usable(c.Prints[from:to]) {
				out = append(out, Spot{Prints: append([]uint64(nil), c.Prints[from:to]...), Start: from})
			}
			i = j
		}
	}
	return out
}

// matchSpots finds where each spot plays in prints. A spot from this
// recording does not count where it came from. It returns the matched spans
// and, per spot, whether it matched at all.
func matchSpots(prints []uint64, spots []Spot) ([]span, []bool) {
	var out []span
	seen := make([]bool, len(spots))
	for i, s := range spots {
		n := len(s.Prints)
		if n == 0 {
			continue
		}
		misses := n - int(float64(n)*matchShare+0.999)
		for at := 0; at+n <= len(prints); at++ {
			if s.Start >= 0 && at > s.Start-n && at < s.Start+n {
				continue
			}
			if !matchesAt(s.Prints, prints[at:at+n], misses) {
				continue
			}
			out = append(out, span{float64(at), float64(at + n)})
			seen[i] = true
			at += n - 1
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, seen
}

func matchesAt(spot, prints []uint64, misses int) bool {
	for k, p := range spot {
		if bits.OnesCount64(p^prints[k]) > matchBits {
			misses--
			if misses < 0 {
				return false
			}
		}
	}
	return true
}

// knownShare is the share of the seconds of b that a known spot covers.
// Spans can overlap (one ad matched twice); a second counts once.
func knownShare(known []span, b Break) float64 {
	if b.End <= b.Start {
		return 0
	}
	var in []span
	for _, s := range known {
		if from, to := max(s.Start, b.Start), min(s.End, b.End); to > from {
			in = append(in, span{from, to})
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Start < in[j].Start })
	covered, reach := 0.0, b.Start
	for _, s := range in {
		if s.End > reach {
			covered += s.End - max(s.Start, reach)
			reach = s.End
		}
	}
	return min(1, covered/(b.End-b.Start))
}
