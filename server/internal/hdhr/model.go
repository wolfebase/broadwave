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
