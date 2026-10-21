// Package region tells which time zone and country the server runs in, so
// listings, prime time, and free channels fit the household anywhere in the
// US or Canada.
package region

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Zone is the server's time zone as the owner set it.
type Zone struct {
	// Name is the IANA name (America/Denver), or the abbreviation when the
	// host clock is set without a name. Empty when nothing is set.
	Name string
	// Valid is false when TZ names a zone that does not exist (America/Denvr):
	// the clock then runs on UTC without saying so.
	Valid bool
	// Offset is the server clock's offset from UTC right now, in minutes.
	Offset int
}

// Set is a zone the owner chose, valid or not.
func (z Zone) Set() bool { return z.Name != "" }

// Local reads TZ, then the host clock (/etc/localtime).
func Local() Zone {
	return localZone(os.Getenv("TZ"), time.Local, readLink("/etc/localtime"), time.Now())
}

func localZone(tz string, local *time.Location, link string, now time.Time) Zone {
	_, offset := now.In(local).Zone()
	z := Zone{Offset: offset / 60, Valid: true}
	if tz = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tz), ":")); tz != "" {
		z.Name = tz
		if _, err := time.LoadLocation(tz); err != nil {
			z.Valid = false
		}
		return z
	}
	if i := strings.Index(link, "zoneinfo/"); i >= 0 {
		if name := link[i+len("zoneinfo/"):]; name != "" && name != "UTC" && name != "Etc/UTC" {
			z.Name = name
			return z
		}
	}
	// A host with no zone at all (a container without zone files) runs on
	// bare UTC, so it reads as unset.
	if abbr, _ := now.In(local).Zone(); abbr != "UTC" {
		z.Name = abbr
	}
	return z
}

func readLink(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(target)
}
