package breaks

import (
	"math"
	"sort"
)

// Break is a span that looks like a commercial break, with how sure the scan
// is: 1 is certain, and a player skips on its own only from AutoSkip up.
type Break struct {
	Start      float64
	End        float64
	Confidence float64
}

// AutoSkip is the confidence a break needs before a player skips it without
// asking. Below it a player offers the skip.
const AutoSkip = 0.7

// boundary is a moment that looks like the cut between two spots.
type boundary struct {
	At       float64
	Strength float64
}

// Find reads the cues of one recording and returns the breaks in it.
// Spots are cut apart with black frames, a moment of silence, or both, and
// run a whole number of 5 s; a station logo is on during the show and off
// during the spots. Each cue alone is wrong often; together they are not.
func Find(c Cues) []Break {
	cuts := boundaries(c)
	var spans []Break
	gaps := logoGaps(c.Logo)
	for i := 0; i < len(gaps); i++ {
		b, ok := snapGap(cuts, gaps[i], c.Length)
		// A blip of logo up to 10 s inside a break, a station promo between
		// spots, can leave both halves with an end that has no cut.
		if !ok && i+1 < len(gaps) && gaps[i+1].Start-gaps[i].End <= 10 {
			if b, ok = snapGap(cuts, span{gaps[i].Start, gaps[i+1].End}, c.Length); ok {
				i++
			}
		}
		if ok {
			spans = append(spans, b)
		}
	}
	// With no logo to check them against, silences that hold a cut come too
	// often in a show; only black frames time the spots.
	chainCuts := cuts
	if c.Logo == nil {
		chainCuts = strong(cuts)
	}
	for _, chain := range spotChains(chainCuts) {
		spans = append(spans, logoOffSpots(c.Logo, chain)...)
	}
	var out []Break
	for _, b := range merge(spans) {
		b.Confidence = confidence(c, cuts, b)
		if b.Confidence >= minConfidence {
			out = append(out, b)
		}
	}
	return out
}

// longestQuiet is the longest time from..to with no boundary. A spot never
// runs past maxSpot, so a break always has a cut sooner than that.
func longestQuiet(cuts []boundary, from, to float64) float64 {
	last, longest := from, 0.0
	for _, b := range cuts {
		if b.At <= from || b.At >= to {
			continue
		}
		longest = max(longest, b.At-last)
		last = b.At
	}
	return max(longest, to-last)
}

// snapGap moves the ends of a logo gap to the cuts that likely bound it. A
// logo can come back a while after the break, over a bumper or the show's
// open, so an end reaches further into the gap than out.
func snapGap(cuts []boundary, s span, length float64) (Break, bool) {
	start, okStart := snap(cuts, s.Start, s.Start-outReach, s.Start+inReach)
	end, okEnd := snap(cuts, s.End, s.End-inReach, s.End+outReach)
	// A recording that ends in a break ends on its last second.
	if length > 0 && s.End >= length-1 {
		end, okEnd = length, true
	}
	if !okStart || !okEnd || end-start < 20 || longestQuiet(cuts, start, end) > maxSpot {
		return Break{}, false
	}
	return Break{Start: start, End: end}, true
}

// minConfidence is the least a break needs to be offered at all.
const minConfidence = 0.4

// boundaries are black stretches and silences that hold a scene cut, merged
// when they fall within a second of each other.
func boundaries(c Cues) []boundary {
	var out []boundary
	for _, b := range c.Blacks {
		strength := 0.7
		if overlapsAny(c.Silences, b.Start-0.3, b.End+0.3) {
			strength = 1
		}
		out = append(out, boundary{At: (b.Start + b.End) / 2, Strength: strength})
	}
	for _, s := range c.Silences {
		for _, at := range c.Cuts {
			if at >= s.Start-0.3 && at <= s.End+0.3 {
				out = append(out, boundary{At: at, Strength: 0.5})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	var merged []boundary
	for _, b := range out {
		if n := len(merged); n > 0 && b.At-merged[n-1].At < 1 {
			if b.Strength > merged[n-1].Strength {
				merged[n-1] = b
			}
			continue
		}
		merged = append(merged, b)
	}
	return merged
}

func overlapsAny(spans []span, from, to float64) bool {
	for _, s := range spans {
		if s.Start < to && s.End > from {
			return true
		}
	}
	return false
}

// logoGaps are the stretches of at least 20 s with the logo off.
func logoGaps(logo []bool) []span {
	var out []span
	start := -1
	for i := 0; i <= len(logo); i++ {
		off := i < len(logo) && !logo[i]
		switch {
		case off && start < 0:
			start = i
		case !off && start >= 0:
			if i-start >= 20 {
				out = append(out, span{float64(start), float64(i)})
			}
			start = -1
		}
	}
	return out
}

const (
	outReach = 8
	inReach  = 30
)

// snap moves a time to the strongest boundary between from and to, nearest
// first among equals. A recording's very start counts as one.
func snap(cuts []boundary, at, from, to float64) (float64, bool) {
	best, found := at, false
	bestScore := 0.0
	for _, b := range cuts {
		if b.At < from || b.At > to {
			continue
		}
		score := b.Strength - math.Abs(b.At-at)/(2*inReach)
		if !found || score > bestScore {
			best, bestScore, found = b.At, score, true
		}
	}
	if !found && at < 1 {
		return 0, true
	}
	return best, found
}

// logoOffSpots keeps the spots of a chain that have the logo off, joined
// where they follow each other. A show's own scenes can be timed like spots;
// they keep the logo on.
func logoOffSpots(logo []bool, chain []float64) []Break {
	if logo == nil {
		return []Break{{Start: chain[0], End: chain[len(chain)-1]}}
	}
	var out []Break
	for i := 1; i < len(chain); i++ {
		if offShare(logo, chain[i-1], chain[i]) <= 0.5 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].End == chain[i-1] {
			out[n-1].End = chain[i]
			continue
		}
		out = append(out, Break{Start: chain[i-1], End: chain[i]})
	}
	return out
}

// offShare is the share of the seconds from..to with the logo off.
func offShare(logo []bool, from, to float64) float64 {
	off, total := 0, 0
	for i := int(from); i < int(to) && i < len(logo); i++ {
		total++
		if !logo[i] {
			off++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(off) / float64(total)
}

// spotChains are runs of at least minSpots spot-length gaps between
// boundaries, as the times of their cuts. A chain may step over boundaries
// inside a spot.
func spotChains(cuts []boundary) [][]float64 {
	n := len(cuts)
	// length[i] is the most spots in a chain that starts at cut i; next[i]
	// is where its first spot ends.
	length := make([]int, n)
	next := make([]int, n)
	for i := n - 1; i >= 0; i-- {
		next[i] = -1
		for j := i + 1; j < n && cuts[j].At-cuts[i].At <= maxSpot; j++ {
			if !spotLength(cuts[j].At - cuts[i].At) {
				continue
			}
			if 1+length[j] > length[i] {
				length[i], next[i] = 1+length[j], j
			}
		}
	}
	var out [][]float64
	for i := 0; i < n; i++ {
		if length[i] < minSpots {
			continue
		}
		chain := []float64{cuts[i].At}
		end := i
		for next[end] >= 0 && cuts[next[end]].At-cuts[i].At <= longestBreak {
			end = next[end]
			chain = append(chain, cuts[end].At)
		}
		if len(chain) <= minSpots {
			continue
		}
		out = append(out, chain)
		i = end - 1
	}
	return out
}

// strong are the cuts with black frames.
func strong(cuts []boundary) []boundary {
	var out []boundary
	for _, b := range cuts {
		if b.Strength >= 0.7 {
			out = append(out, b)
		}
	}
	return out
}

// minSpots is how many spots in a row make a break on spot timing alone.
const minSpots = 3

const maxSpot = 125.5

const breakGap = 40

// longestBreak is about the longest break a station runs. Joining past it
// takes in a show that has no logo.
const longestBreak = 240

// spotLength is a gap that lasts as long as a commercial: 10 s to about two
// minutes, within half a second of a whole number of 5 s.
func spotLength(seconds float64) bool {
	if seconds < 9.5 || seconds > maxSpot {
		return false
	}
	return math.Abs(seconds-5*math.Round(seconds/5)) <= 0.5
}

// merge joins spans that overlap or nearly touch. Shows run minutes between
// breaks; under breakGap apart is one break with a spot that was missed.
func merge(spans []Break) []Break {
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	var out []Break
	for _, s := range spans {
		if n := len(out); n > 0 && s.Start <= out[n-1].End+breakGap && (s.Start < out[n-1].End || s.End-out[n-1].Start <= longestBreak) {
			out[n-1].End = max(out[n-1].End, s.End)
			continue
		}
		out = append(out, s)
	}
	return out
}

// confidence weighs a span's evidence: how much of it has the logo off,
// and how many of its boundaries are a spot apart.
func confidence(c Cues, cuts []boundary, b Break) float64 {
	if c.Logo == nil {
		// Black-frame timing alone: three spots are offered, four or more skip.
		return round2(math.Min(0.8, 0.3+0.1*float64(spotsWithin(strong(cuts), b))))
	}
	spots := float64(spotsWithin(cuts, b))
	away := offShare(c.Logo, b.Start, b.End)
	return round2(math.Min(0.99, 0.05+0.55*away+0.4*math.Min(spots/6, 1)))
}

// spotsWithin counts the spot-length gaps of the longest chain inside a span.
func spotsWithin(cuts []boundary, b Break) int {
	var inside []boundary
	for _, cut := range cuts {
		if cut.At >= b.Start-0.5 && cut.At <= b.End+0.5 {
			inside = append(inside, cut)
		}
	}
	best := 0
	length := make([]int, len(inside))
	for i := len(inside) - 1; i >= 0; i-- {
		for j := i + 1; j < len(inside) && inside[j].At-inside[i].At <= maxSpot; j++ {
			if spotLength(inside[j].At-inside[i].At) && 1+length[j] > length[i] {
				length[i] = 1 + length[j]
			}
		}
		best = max(best, length[i])
	}
	return best
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
