package hdhr

import (
	"strconv"
	"strings"
)

// SplitGuide splits an ATSC virtual channel such as "14.10" into major and minor.
// A number with no dot has minor 0.
func SplitGuide(number string) (major, minor int) {
	number = strings.TrimSpace(number)
	majorPart, minorPart, hasMinor := strings.Cut(number, ".")
	major, _ = strconv.Atoi(majorPart)
	if hasMinor {
		minor, _ = strconv.Atoi(minorPart)
	}
	return major, minor
}

// CompareGuide orders virtual channels numerically so 14.2 sorts before 14.10.
func CompareGuide(a, b string) int {
	am, an := SplitGuide(a)
	bm, bn := SplitGuide(b)
	if am != bm {
		return am - bm
	}
	return an - bn
}
