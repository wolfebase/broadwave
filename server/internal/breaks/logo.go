package breaks

import "math/bits"

// bitmap is one bit per pixel of a scan frame.
type bitmap []uint64

func newBitmap() bitmap { return make(bitmap, (frameW*frameH+63)/64) }

func (b bitmap) set(i int)      { b[i/64] |= 1 << (i % 64) }
func (b bitmap) has(i int) bool { return b[i/64]&(1<<(i%64)) != 0 }

// edgeStep is how sharp a change between neighbours must be to count as an
// edge. A logo is drawn with hard edges; video noise and soft focus are not.
const edgeStep = 48

func edgeMap(frame []byte) bitmap {
	out := newBitmap()
	for y := 1; y < frameH-1; y++ {
		for x := 1; x < frameW-1; x++ {
			i := y*frameW + x
			gx := int(frame[i+1]) - int(frame[i-1])
			gy := int(frame[i+frameW]) - int(frame[i-frameW])
			if abs(gx)+abs(gy) > edgeStep {
				out.set(i)
			}
		}
	}
	return out
}

// findLogo finds the pixels whose edges stay put through most of the
// recording, the station logo, and says per second whether it is on screen.
// It starts from every edge that is there more often than not and narrows to
// the edges that are there whenever the logo is.
func findLogo(edges []bitmap) []bool {
	if len(edges) < 300 {
		return nil
	}
	on := make([]bool, len(edges))
	for i := range on {
		on[i] = true
	}
	// An edge that never goes away, a pillarbox line or a border, is there in
	// the breaks too.
	always := map[int]bool{}
	for _, p := range steadyPixels(edges, on, 0.97) {
		always[p] = true
	}
	var mask []int
	// Breaks keep the logo off a quarter of the time or more, so the first
	// guess asks less of a pixel than the later ones.
	share := 0.6
	for round := 0; round < 4; round++ {
		mask = mask[:0]
		for _, p := range steadyPixels(edges, on, share) {
			if !always[p] {
				mask = append(mask, p)
			}
		}
		share = 0.85
		if len(mask) < minLogoPixels {
			return nil
		}
		for i, frame := range edges {
			on[i] = logoScore(frame, mask) >= 0.5
		}
	}
	shown := 0
	for _, v := range on {
		if v {
			shown++
		}
	}
	// A logo that is always there tells breaks nothing; one that is rarely
	// there is something else.
	if part := float64(shown) / float64(len(on)); part < 0.4 || part > 0.97 {
		return nil
	}
	return smooth(on, 5)
}

// minLogoPixels is the smallest logo worth following. Fewer steady edge
// pixels than this is a graphic, a border, or noise.
const minLogoPixels = 12

// steadyPixels are the pixels that are an edge in at least share of the
// frames where the logo is on.
func steadyPixels(edges []bitmap, on []bool, share float64) []int {
	counts := make([]int, frameW*frameH)
	frames := 0
	for i, frame := range edges {
		if !on[i] {
			continue
		}
		frames++
		for word, bitsSet := range frame {
			for bitsSet != 0 {
				b := bits.TrailingZeros64(bitsSet)
				counts[word*64+b]++
				bitsSet &= bitsSet - 1
			}
		}
	}
	if frames == 0 {
		return nil
	}
	var out []int
	for p, n := range counts {
		if float64(n) >= share*float64(frames) {
			out = append(out, p)
		}
	}
	return out
}

func logoScore(frame bitmap, mask []int) float64 {
	hit := 0
	for _, p := range mask {
		if frame.has(p) {
			hit++
		}
	}
	return float64(hit) / float64(len(mask))
}

// smooth takes the majority over a window, so one busy frame does not drop
// the logo and one dark frame does not end it.
func smooth(in []bool, width int) []bool {
	out := make([]bool, len(in))
	for i := range in {
		n, on := 0, 0
		for j := max(0, i-width/2); j <= min(len(in)-1, i+width/2); j++ {
			n++
			if in[j] {
				on++
			}
		}
		out[i] = on*2 > n
	}
	return out
}

// hashFrame is a difference hash: the frame shrunk to 9×8 cells, one bit per
// cell for whether it is darker than its right neighbour.
func hashFrame(frame []byte) uint64 {
	var cells [8][9]int
	for cy := 0; cy < 8; cy++ {
		y0, y1 := cy*frameH/8, (cy+1)*frameH/8
		for cx := 0; cx < 9; cx++ {
			x0, x1 := cx*frameW/9, (cx+1)*frameW/9
			sum := 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sum += int(frame[y*frameW+x])
				}
			}
			cells[cy][cx] = sum / ((y1 - y0) * (x1 - x0))
		}
	}
	var h uint64
	for cy := 0; cy < 8; cy++ {
		for cx := 0; cx < 8; cx++ {
			h <<= 1
			if cells[cy][cx] < cells[cy][cx+1] {
				h |= 1
			}
		}
	}
	return h
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
