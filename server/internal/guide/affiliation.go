package guide

import (
	"regexp"
	"strings"
)

// Affiliation reports ABC, CBS, FOX, or NBC.
// A guide name wins when it is the network, "FOX 4", or ends in "(NBC)".
// Anything else is a call sign: DT/HD suffixes come off, then the table.
// A name that merely contains those letters is not a network.
func Affiliation(guideNames []string, callSigns ...string) string {
	for _, name := range guideNames {
		if net := brandedNetwork(name); net != "" {
			return net
		}
	}
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, guideNames...), callSigns...) {
		call := normCall(name)
		if len(call) < 3 || seen[call] {
			continue
		}
		seen[call] = true
		if net := calls[call]; net != "" {
			return net
		}
	}
	return ""
}

// Calls is a copy of the call-sign table the API exposes.
func Calls() map[string]string {
	out := make(map[string]string, len(calls))
	for call, net := range calls {
		out[call] = net
	}
	return out
}

func brandedNetwork(name string) string {
	s := strings.ToUpper(strings.TrimSpace(name))
	s = strings.Join(strings.Fields(s), " ")
	switch s {
	case "ABC", "CBS", "FOX", "NBC":
		return s
	case "AMERICAN BROADCASTING COMPANY":
		return "ABC"
	case "FOX BROADCASTING", "FOX BROADCASTING COMPANY":
		return "FOX"
	case "NATIONAL BROADCASTING COMPANY":
		return "NBC"
	}
	if m := leadingNetwork.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := parenNetwork.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

var leadingNetwork = regexp.MustCompile(`^(ABC|CBS|FOX|NBC)[\s-]+\d`)
var parenNetwork = regexp.MustCompile(`\((ABC|CBS|FOX|NBC)\)$`)
