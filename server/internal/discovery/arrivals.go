package discovery

import (
	"strings"
	"sync"
	"time"
)

// hereGrace is how long a reconnecting app stays quiet after the first scan.
// Phones and Apple TVs announce again after a restart; a tuner does not.
const hereGrace = 90 * time.Second

// Arrivals remembers the house and reports a device that shows up later.
// The first non-empty look is the baseline and raises nothing.
type Arrivals struct {
	mu         sync.Mutex
	seen       map[string]bool
	seeded     bool
	graceUntil time.Time
}

// Observe returns one line per device that was not in a previous look.
// A device is reported once. An empty look does not become the baseline,
// so a scan that has not heard the network yet does not banner the whole house.
func (a *Arrivals) Observe(places []Place, now time.Time) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seen == nil {
		a.seen = map[string]bool{}
	}
	if !a.seeded {
		// A saved tuner or a connected app is not a scan. Wait until the
		// network answers, or the next look banners the whole house.
		if !hasNetwork(places) {
			return nil
		}
		for _, place := range places {
			a.remember(place)
		}
		a.seeded = true
		a.graceUntil = now.Add(hereGrace)
		return nil
	}
	var notes []string
	for _, place := range places {
		if place.ID == "" || a.known(place) {
			continue
		}
		a.remember(place)
		// The first scan often misses a speaker or a server. A tuner does not wait.
		if place.Action != "add" && now.Before(a.graceUntil) {
			continue
		}
		if msg, ok := notice(place); ok {
			notes = append(notes, msg)
		}
	}
	return notes
}

func hasNetwork(places []Place) bool {
	for _, place := range places {
		switch place.Action {
		case "add", "use", "found":
			return true
		}
	}
	return false
}

func (a *Arrivals) remember(place Place) {
	if place.ID != "" {
		a.seen[place.ID] = true
	}
	if place.Group == "tuner" && place.Addr != "" {
		a.seen["addr|"+place.Kind+"|"+place.Addr] = true
	}
}

func (a *Arrivals) known(place Place) bool {
	if place.ID != "" && a.seen[place.ID] {
		return true
	}
	return place.Group == "tuner" && place.Addr != "" && a.seen["addr|"+place.Kind+"|"+place.Addr]
}

func notice(place Place) (string, bool) {
	name := cleanLabel(place.Name)
	if name == "" {
		return "", false
	}
	switch place.Action {
	case "add":
		if place.Group != "tuner" {
			return "", false
		}
		return "New tuner found: " + name + ". Add it?", true
	case "use":
		if place.Group != "server" {
			return "", false
		}
		label := kindLabel(place.Kind)
		if strings.EqualFold(name, label) {
			return "New " + label + " server found. Use it as a tuner?", true
		}
		return "New " + label + " server found: " + name + ". Use it as a tuner?", true
	case "found", "here":
		if place.Group != "screen" || place.Kind == "web" {
			return "", false
		}
		return "New " + screenNoun(place.Kind) + " found: " + name + ".", true
	default:
		return "", false
	}
}

func screenNoun(kind string) string {
	switch kind {
	case "appletv":
		return "Apple TV"
	case "iphone":
		return "iPhone"
	case "ipad":
		return "iPad"
	case "chromecast":
		return "Chromecast"
	case "airplay":
		return "AirPlay device"
	case "firetv":
		return "Fire TV"
	case "androidtv":
		return "Android TV"
	case "tv":
		return "TV"
	default:
		return kindLabel(kind)
	}
}
