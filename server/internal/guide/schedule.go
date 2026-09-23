package guide

import "time"

// SiliconDust asks that the free XMLTV feed be refreshed at a random
// interval between 20 and 28 hours, not on a fixed clock.
const (
	PullMin         = 20 * time.Hour
	PullMax         = 28 * time.Hour
	ManualGap       = time.Hour
	RetryAfterError = 30 * time.Minute
)

// NextPull is PullMin plus jitter after a successful refresh.
// Jitter is clamped to the 8 hour window so the result stays inside 20-28 h.
func NextPull(success time.Time, jitter time.Duration) time.Time {
	if jitter < 0 {
		jitter = 0
	}
	span := PullMax - PullMin
	if jitter > span {
		jitter = span
	}
	return success.Add(PullMin + jitter)
}

// ManualAllowed is false until ManualGap has passed since the last manual refresh.
// retryAt is when another manual refresh is allowed.
func ManualAllowed(lastManual, now time.Time) (bool, time.Time) {
	if lastManual.IsZero() {
		return true, time.Time{}
	}
	retryAt := lastManual.Add(ManualGap)
	if !now.Before(retryAt) {
		return true, time.Time{}
	}
	return false, retryAt
}

// Delay is how long to wait before the next automatic pull.
// A zero or past next means the pull is due.
func Delay(next, now time.Time) time.Duration {
	if next.IsZero() || !next.After(now) {
		return 0
	}
	return next.Sub(now)
}
