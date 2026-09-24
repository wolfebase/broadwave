package live

// DeviceTuners is one HDHomeRun and the tuners it reported.
type DeviceTuners struct {
	Host   string
	Tuners []Tuner
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

// PickTuner chooses the first free tuner, walking devices in the order given.
// The first device is the one the channel belongs to. Later devices are failover.
func PickTuner(devices []DeviceTuners, used, reserved map[int]bool) (host string, tuner int, ok bool) {
	for i, device := range devices {
		if device.Host == "" {
			continue
		}
		own := used
		if i > 0 {
			own = nil
		}
		if n, free := firstFree(device.Tuners, own, reserved); free {
			return device.Host, n, true
		}
	}
	return "", 0, false
}
