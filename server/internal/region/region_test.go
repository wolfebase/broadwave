package region

import (
	"testing"
	"time"
)

func TestAMistypedZoneIsNotValid(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	z := localZone("America/Denvr", time.UTC, "", now)
	if z.Name != "America/Denvr" || z.Valid || z.Offset != 0 {
		t.Fatalf("mistyped zone: %+v", z)
	}
	denver, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}
	z = localZone(" America/Denver ", denver, "", now)
	if z.Name != "America/Denver" || !z.Valid || z.Offset != -6*60 {
		t.Fatalf("Denver: %+v", z)
	}
}

func TestTheHostClockNamesTheZone(t *testing.T) {
	now := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	if z := localZone("", chicago, "/usr/share/zoneinfo/America/Chicago", now); z.Name != "America/Chicago" || !z.Valid || z.Offset != -360 {
		t.Fatalf("linked zone: %+v", z)
	}
	if z := localZone("", chicago, "", now); z.Name != "CST" {
		t.Fatalf("unlinked zone: %+v", z)
	}
	if z := localZone("", time.UTC, "/usr/share/zoneinfo/Etc/UTC", now); z.Set() || !z.Valid {
		t.Fatalf("bare UTC must read as unset: %+v", z)
	}
}

func TestCountry(t *testing.T) {
	cases := map[string]string{
		"America/Toronto": "CA", "America/Vancouver": "CA", "America/St_Johns": "CA", "Canada/Eastern": "CA",
		"America/Halifax": "CA", "America/Regina": "CA",
		"America/New_York": "US", "America/Indiana/Indianapolis": "US", "Pacific/Honolulu": "US",
		"America/Anchorage": "US", "US/Central": "US",
		"Europe/London": "", "CST": "", "": "",
	}
	for zone, want := range cases {
		if got := ZoneCountry(zone); got != want {
			t.Errorf("%s: %q, want %q", zone, got, want)
		}
	}
	if countryOf("", "America/Winnipeg") != "CA" || countryOf("", "Europe/Paris") != "US" {
		t.Fatal("zone country")
	}
	if countryOf("ca", "America/Chicago") != "CA" || countryOf("US", "America/Toronto") != "US" {
		t.Fatal("BROADWAVE_COUNTRY wins")
	}
}
