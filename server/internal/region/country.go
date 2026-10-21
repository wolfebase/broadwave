package region

import (
	"os"
	"strings"
)

// Country is US or CA: BROADWAVE_COUNTRY when set, else the country of the
// server's time zone. Anything that can't tell reads as US.
func Country() string {
	return CountryOf(Local().Name)
}

// CountryOf is Country for a zone name.
func CountryOf(zone string) string {
	return countryOf(os.Getenv("BROADWAVE_COUNTRY"), zone)
}

func countryOf(override, zone string) string {
	switch strings.ToUpper(strings.TrimSpace(override)) {
	case "CA", "CANADA":
		return "CA"
	case "US", "USA":
		return "US"
	}
	if ZoneCountry(zone) == "CA" {
		return "CA"
	}
	return "US"
}

// ZoneCountry is CA for a Canadian zone name, US for a US one, and empty for
// anything else (an abbreviation, a zone outside North America).
func ZoneCountry(zone string) string {
	zone = strings.TrimSpace(zone)
	if strings.HasPrefix(zone, "Canada/") {
		return "CA"
	}
	if strings.HasPrefix(zone, "US/") {
		return "US"
	}
	city, ok := strings.CutPrefix(zone, "America/")
	if !ok {
		if zone == "Pacific/Honolulu" {
			return "US"
		}
		return ""
	}
	if canadian[city] {
		return "CA"
	}
	if american[city] || strings.HasPrefix(city, "Indiana/") || strings.HasPrefix(city, "Kentucky/") || strings.HasPrefix(city, "North_Dakota/") {
		return "US"
	}
	return ""
}

var canadian = set(`Toronto Montreal Vancouver Edmonton Winnipeg Regina Halifax St_Johns Moncton
	Glace_Bay Goose_Bay Whitehorse Dawson Dawson_Creek Fort_Nelson Creston Yellowknife Inuvik
	Cambridge_Bay Rankin_Inlet Resolute Iqaluit Pangnirtung Swift_Current Thunder_Bay Nipigon
	Rainy_River Atikokan Coral_Harbour Blanc-Sablon`)

var american = set(`New_York Chicago Denver Phoenix Los_Angeles Anchorage Juneau Sitka Nome Yakutat
	Metlakatla Adak Atka Boise Detroit Menominee Indianapolis Louisville Fort_Wayne Knox_IN Shiprock`)

func set(list string) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Fields(list) {
		out[name] = true
	}
	return out
}
