package hdhr

import (
	"strconv"
	"strings"
)

// Lock is one /tunerN/status reading. Quality is SNR (snq). Symbol is seq.
type Lock struct {
	Strength int
	Quality  int
	Symbol   int
	Locked   bool
}

// ParseStatus reads ss, snq, and seq from a tuner status line.
func ParseStatus(status string) Lock {
	var lock Lock
	for _, field := range strings.Fields(status) {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "lock":
			lock.Locked = val != "" && val != "none"
		case "ss":
			lock.Strength, _ = strconv.Atoi(val)
		case "snq":
			lock.Quality, _ = strconv.Atoi(val)
		case "seq":
			lock.Symbol, _ = strconv.Atoi(val)
		}
	}
	return lock
}

// Verdict is Great, OK, Weak, or Lost. The tip is empty when the signal is fine.
func (l Lock) Verdict() (verdict, tip string) {
	if !l.Locked {
		return "Lost", "This channel isn't coming in."
	}
	if l.Quality < 50 || l.Symbol < 100 {
		return "Weak", "Move the antenna a little, or check the cable."
	}
	if l.Quality < 80 {
		return "OK", ""
	}
	return "Great", ""
}
