package disk

import (
	"fmt"
	"strconv"
	"strings"
)

// Space is the filesystem that holds recordings.
type Space struct {
	Free  uint64
	Total uint64
}

// LowError means a new recording would cross the free-space reserve.
type LowError struct {
	Free uint64
	Need uint64
}

func (e *LowError) Error() string {
	return fmt.Sprintf("recordings disk has %s free, and this server keeps %s in reserve", formatBytes(e.Free), formatBytes(e.Need))
}

// WatermarkGB is the reserve in gigabytes. An empty value uses 10. Zero turns the reserve off.
func WatermarkGB(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 10
	}
	if n > 1000000 {
		return 1000000
	}
	return n
}

func WatermarkBytes(raw string) uint64 {
	gb := WatermarkGB(raw)
	if gb == 0 {
		return 0
	}
	return uint64(gb) * 1000 * 1000 * 1000
}

func BelowReserve(free, reserve uint64) bool {
	return reserve > 0 && free < reserve
}

func formatBytes(n uint64) string {
	if n >= 1000*1000*1000*1000 {
		return fmt.Sprintf("%.1f TB", float64(n)/1e12)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}
