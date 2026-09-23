package guide

import "strconv"

// ArtLayout chooses how a picture can be shown.
// A full-bleed hero needs a landscape image at least 1280 px wide,
// and the slot must stay within 1.25× that width. Anything else is composed:
// the picture stays near its real size over a blurred copy.
func ArtLayout(width, height, slot int) string {
	if width >= 1280 && height > 0 && width > height && slot > 0 && slot*4 <= width*5 {
		return "bleed"
	}
	return "composed"
}

// DisplayEdge is the longest side to draw. Zero means the picture's own size,
// which is what we use until a probe has measured it.
func DisplayEdge(native, slot int) int {
	if native <= 0 || slot <= 0 {
		return 0
	}
	limit := native + native/4
	if slot < limit {
		return slot
	}
	return limit
}

// PickIcon prefers the largest landscape icon, then the largest icon, then the first usable URL.
func PickIcon(icons []iconEl) (src string, width, height int) {
	bestScore := -1
	for _, icon := range icons {
		url := cleanIcon(icon.Src)
		if url == "" {
			continue
		}
		w, _ := strconv.Atoi(icon.Width)
		h, _ := strconv.Atoi(icon.Height)
		score := 0
		if w > 0 {
			score = w
		}
		if w > 0 && h > 0 && w > h {
			score += 1_000_000
		}
		if src == "" || score > bestScore {
			src, width, height = url, w, h
			bestScore = score
		}
	}
	return src, width, height
}
