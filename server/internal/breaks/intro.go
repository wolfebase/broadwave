package breaks

import (
	"math/bits"
	"sort"
)

// Episode is what the intro search knows of one recording of a show.
type Episode struct {
	Sound Sound
	// Breaks are its commercial breaks. A stretch two episodes share inside
	// one is an ad, not the show's.
	Breaks []Break
	// ShowStart and ShowEnd are where the show starts and ends in the file,
	// after the early padding and before the late. ShowEnd is 0 when unknown.
	ShowStart, ShowEnd float64
}

// Ends is where a recording's intro and end titles are, in seconds. A zero
// field was not found.
type Ends struct {
	IntroStart, IntroEnd float64
	// Credits is where the end titles start, or where the show ends when its
	// end titles were not found.
	Credits float64
}

// An intro or a closing theme plays for at least introMin and at most
// introMax seconds. A listing's start is good to showSlack seconds.
const (
	introMin  = 8
	introMax  = 180
	showSlack = 10
)

// FindEnds finds the stretch of sound near its start that a recording shares
// with other episodes of its show, its intro, and the one near its end, its
// end titles. A stretch more of the others share wins; then the earlier
// intro and the later end titles.
func FindEnds(self Episode, others []Episode) Ends {
	var ends Ends
	head := func(e Episode) ([]uint32, float64) { return e.Sound.Head, 0 }
	tail := func(e Episode) ([]uint32, float64) { return e.Sound.Tail, e.Sound.TailFrom }
	if s, ok := pickShared(self, others, head, true); ok {
		ends.IntroStart, ends.IntroEnd = s.Start, s.End
	}
	if s, ok := pickShared(self, others, tail, false); ok {
		ends.Credits = s.Start
	} else if self.ShowEnd > 0 {
		ends.Credits = self.ShowEnd
	}
	// The two ends of a short recording can overlap; the intro wins.
	if ends.Credits > 0 && ends.Credits < ends.IntroEnd {
		ends.Credits = 0
	}
	return ends
}

func pickShared(self Episode, others []Episode, part func(Episode) ([]uint32, float64), first bool) (span, bool) {
	prints, from := part(self)
	// Each pair's stretches, in this recording's seconds.
	var pairs [][]span
	for _, o := range others {
		theirs, theirFrom := part(o)
		if len(prints) == 0 || len(theirs) == 0 {
			continue
		}
		var found []span
		for _, r := range sharedRuns(prints, theirs) {
			mine := span{from + at(r.a), from + at(r.a+r.n-1)}
			their := span{theirFrom + at(r.b), theirFrom + at(r.b+r.n-1)}
			if inBreak(mine, self.Breaks) || inBreak(their, o.Breaks) {
				continue
			}
			if self.ShowEnd > 0 && !first && mine.Start > self.ShowEnd-introMin {
				continue // the next show, in the late padding
			}
			if self.ShowEnd > 0 && first && mine.Start < self.ShowStart-showSlack {
				continue // the show before, in the early padding
			}
			found = append(found, mine)
		}
		pairs = append(pairs, found)
	}
	need := max(1, (len(pairs)+1)/2)
	var best span
	bestSupport := 0
	for _, found := range pairs {
		for _, c := range found {
			var starts, endings []float64
			for _, other := range pairs {
				for _, d := range other {
					if same(c, d) {
						starts, endings = append(starts, d.Start), append(endings, d.End)
						break
					}
				}
			}
			if len(starts) < need {
				continue
			}
			s := span{median(starts), median(endings)}
			better := len(starts) > bestSupport
			if len(starts) == bestSupport {
				better = (first && s.Start < best.Start) || (!first && s.Start > best.Start)
			}
			if better {
				best, bestSupport = s, len(starts)
			}
		}
	}
	return best, bestSupport > 0
}

// same is two stretches that overlap by half of the shorter one.
func same(a, b span) bool {
	o := min(a.End, b.End) - max(a.Start, b.Start)
	return o > 0 && o >= 0.5*min(a.End-a.Start, b.End-b.Start)
}

func median(v []float64) float64 {
	sort.Float64s(v)
	return v[len(v)/2]
}

// inBreak is a stretch that overlaps a break by more than a second.
func inBreak(s span, breaks []Break) bool {
	for _, b := range breaks {
		if min(s.End, b.End)-max(s.Start, b.Start) > 1 {
			return true
		}
	}
	return false
}

// run is n hops that play at a in one recording and at b in the other.
type run struct{ a, b, n int }

// Two prints match within soundBits of 32. Random sound matches about one
// hop in forty; the same broadcast about nine in ten.
const (
	soundBits = 10
	// A run ends after this many hops in a row that do not match, and keeps
	// at least half its hops matched.
	soundGap = 12
	// Offsets tried: the ones most votes point to.
	soundOffsets = 12
	// A print shared by more hops than this is a steady tone, not a vote.
	soundBucket = 64
)

// sharedRuns finds the stretches of at least introMin seconds two
// recordings share. Each 16-bit half of a print votes for the offsets where
// the other recording has the same half; the best offsets are then checked
// hop by hop.
func sharedRuns(a, b []uint32) []run {
	index := map[uint32][]int{}
	for j, p := range b {
		if p == quietPrint {
			continue
		}
		index[p>>16|1<<16] = append(index[p>>16|1<<16], j)
		index[p&0xffff] = append(index[p&0xffff], j)
	}
	votes := make([]int, len(a)+len(b))
	for i, p := range a {
		if p == quietPrint {
			continue
		}
		for _, key := range []uint32{p>>16 | 1<<16, p & 0xffff} {
			hits := index[key]
			if len(hits) > soundBucket {
				continue
			}
			for _, j := range hits {
				votes[i-j+len(b)]++
			}
		}
	}
	offsets := make([]int, 0, len(votes))
	for d, v := range votes {
		if v >= 4 {
			offsets = append(offsets, d)
		}
	}
	sort.Slice(offsets, func(i, j int) bool { return votes[offsets[i]] > votes[offsets[j]] })
	if len(offsets) > soundOffsets {
		offsets = offsets[:soundOffsets]
	}
	tried := map[int]bool{}
	var runs []run
	minHops, maxHops := hops(introMin), hops(introMax)
	for _, d := range offsets {
		// A frame that starts half a hop off can vote for the next offset.
		for _, near := range []int{d - 1, d, d + 1} {
			if tried[near] || near < 0 || near >= len(votes) {
				continue
			}
			tried[near] = true
			for _, r := range runsAt(a, b, near-len(b), minHops) {
				if r.n <= maxHops {
					runs = append(runs, r)
				}
			}
		}
	}
	return longestFirst(runs)
}

func hops(seconds float64) int { return int(seconds / HopSeconds) }

// at is the middle of a print's frame, in seconds.
func at(hop int) float64 { return float64(hop)*HopSeconds + float64(soundFrame)/2/soundRate }

// runsAt checks a against b moved by shift (hop i of a against i-shift of b).
func runsAt(a, b []uint32, shift, minHops int) []run {
	from, to := max(0, shift), min(len(a), len(b)+shift)
	if to <= from {
		return nil
	}
	// Per hop: 1 matched, 0 not, -1 quiet in both, a pause both share.
	hit := make([]int8, to-from)
	for i := from; i < to; i++ {
		p, q := a[i], b[i-shift]
		switch {
		case p == quietPrint && q == quietPrint:
			hit[i-from] = -1
		case p != quietPrint && q != quietPrint && bits.OnesCount32(p^q) <= soundBits:
			hit[i-from] = 1
		}
	}
	var out []run
	start, last := -1, -1
	flush := func() {
		if start >= 0 {
			start, last = trim(hit, start, last)
			matched := 0
			for _, h := range hit[start : last+1] {
				matched += max(0, int(h))
			}
			if last-start+1 >= minHops && 2*matched >= last-start+1 {
				out = append(out, run{a: from + start, b: from + start - shift, n: last - start + 1})
			}
		}
		start, last = -1, -1
	}
	for i, h := range hit {
		switch {
		case h == 1:
			if start < 0 {
				start = i
			}
			last = i
		case h == 0 && start >= 0 && i-last > soundGap:
			flush()
		}
	}
	flush()
	return out
}

// trim moves a run's ends in past chance matches: an end must start a
// stretch of soundGap hops that mostly match.
func trim(hit []int8, start, last int) (int, int) {
	dense := func(from, step int) bool {
		n, k := 0, 0
		for i := from; i >= start && i <= last && k < soundGap; i += step {
			if hit[i] >= 0 {
				k++
				n += int(hit[i])
			}
		}
		return 2*n > k
	}
	for start < last && (hit[start] != 1 || !dense(start, 1)) {
		start++
	}
	for last > start && (hit[last] != 1 || !dense(last, -1)) {
		last--
	}
	return start, last
}

// longestFirst keeps the longest of runs that cover the same hops of a.
func longestFirst(runs []run) []run {
	sort.Slice(runs, func(i, j int) bool { return runs[i].n > runs[j].n })
	var out []run
	for _, r := range runs {
		clash := false
		for _, k := range out {
			if r.a < k.a+k.n && k.a < r.a+r.n {
				clash = true
				break
			}
		}
		if !clash {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].a < out[j].a })
	return out
}
