package hdhr

import "strings"

// ModelNote is the one line Sources shows for a tuner family.
// An empty note means there is nothing special to say.
func ModelNote(model string) string {
	m := strings.ToUpper(model)
	switch {
	case strings.Contains(m, "4K") || strings.Contains(m, "FLEX"):
		return "ATSC 3.0 channels stay on the tuner."
	case strings.Contains(m, "PRIME") || strings.Contains(m, "-CC"):
		return "Copy protected channels stay off the guide."
	case strings.Contains(m, "EXTEND") || strings.Contains(m, "HDTC"):
		return "This tuner can convert the picture."
	case strings.Contains(m, "SCRIBE") || strings.Contains(m, "SERVIO"):
		return "Recordings on the device can be added from its library."
	default:
		return ""
	}
}

// ExtendQuery is the transcode profile an EXTEND tuner can apply.
// Other models ignore it, so the server converts the picture instead.
func ExtendQuery(model string) string {
	m := strings.ToUpper(model)
	if strings.Contains(m, "EXTEND") || strings.Contains(m, "HDTC") {
		return "transcode=mobile"
	}
	return ""
}

// BroadcastGuide reports a tuner whose channels carry listings on the air:
// ATSC on an antenna model, DVB on a DVB-T, T2, or C model. A CableCARD
// PRIME does not. An unknown model is assumed to be an antenna tuner.
func BroadcastGuide(model string) bool {
	m := strings.ToUpper(strings.TrimSpace(model))
	if strings.Contains(m, "PRIME") {
		return false
	}
	return !strings.HasSuffix(m, "CC") || !strings.Contains(m, "-")
}

// DVB reports a DVB-T, T2, or C model (HDHR5-4DT, HDHR3-4DC, HDHR3-EU).
func DVB(model string) bool {
	m := strings.ToUpper(strings.TrimSpace(model))
	if !strings.Contains(m, "-") {
		return false
	}
	for _, tail := range []string{"DT", "DC", "EU"} {
		if strings.HasSuffix(m, tail) {
			return true
		}
	}
	return false
}
