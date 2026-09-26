package live

import "strings"

// DeviceTuners is one HDHomeRun and the tuners it reported.
// Base is the device origin when two devices share a hostname, as the test
// fleet does. Empty Base means Host is the identity PickTuner returns.
type DeviceTuners struct {
	Host   string
	Base   string
	Tuners []Tuner
}

// Need is what one channel asks of the pool.
// ATSC3 is a capability, not a guide number.
type Need struct {
	ATSC3 bool
}

// HoldBack marks the last free tuners so a recording that is about to start still has one.
func HoldBack(tuners []Tuner, hold int) map[int]bool {
	if hold < 1 {
		return nil
	}
	var free []int
	for _, t := range tuners {
		if t.Target == "" && t.Guide == "" {
			free = append(free, t.Index)
		}
	}
	out := map[int]bool{}
	for i := len(free) - 1; i >= 0 && hold > 0; i-- {
		out[free[i]] = true
		hold--
	}
	return out
}

// atsc3Tuners is how many tuners on this model can lock ATSC 3.0.
// A 4K keeps the first two. Any other model keeps none.
func atsc3Tuners(model string) int {
	if strings.Contains(strings.ToUpper(model), "4K") {
		return 2
	}
	return 0
}

// markATSC3 sets the capability on the first n tuners.
func markATSC3(tuners []Tuner, n int) {
	for i := range tuners {
		tuners[i].ATSC3 = i < n
	}
}

// LineupATSC3 reports an ATSC 3.0 row from the codecs the lineup stored.
// A guide number is not a capability: 100.1 can still be ATSC 1.0.
func LineupATSC3(video, audio string) bool {
	v := strings.ToUpper(strings.TrimSpace(video))
	if v != "HEVC" && v != "H265" {
		return false
	}
	a := strings.ToUpper(strings.ReplaceAll(audio, "-", ""))
	return strings.Contains(a, "AC4")
}

// NeedFor is the pool need for a channel. explicit is the channel's own flag.
func NeedFor(video, audio string, explicit bool) Need {
	return Need{ATSC3: explicit || LineupATSC3(video, audio)}
}

// PickTuner chooses a free tuner, walking devices in the order given.
// The first device is the channel's own. Later devices are failover.
// A 1.0 channel takes a tuner that cannot do 3.0 when one is free on this
// device or a later one, and uses a 3.0 tuner only when none are.
// A 3.0 channel never takes a tuner that cannot do 3.0.
func PickTuner(devices []DeviceTuners, used, reserved map[int]bool, need Need) (host string, tuner int, ok bool) {
	if need.ATSC3 {
		return pickTuner(devices, used, reserved, func(t Tuner) bool { return t.ATSC3 })
	}
	if host, tuner, ok = pickTuner(devices, used, reserved, func(t Tuner) bool { return !t.ATSC3 }); ok {
		return host, tuner, true
	}
	return pickTuner(devices, used, reserved, func(Tuner) bool { return true })
}

func pickTuner(devices []DeviceTuners, used, reserved map[int]bool, allow func(Tuner) bool) (host string, tuner int, ok bool) {
	for i, device := range devices {
		if device.Host == "" {
			continue
		}
		own := used
		held := reserved
		if i > 0 {
			// used and reserved are tuner indexes on the first device.
			// The same index on the next device is a different tuner.
			own = nil
			held = nil
		}
		var eligible []Tuner
		for _, t := range device.Tuners {
			if allow(t) {
				eligible = append(eligible, t)
			}
		}
		if n, free := firstFree(eligible, own, held); free {
			name := device.Base
			if name == "" {
				name = device.Host
			}
			return name, n, true
		}
	}
	return "", 0, false
}
