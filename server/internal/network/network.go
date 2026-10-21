// Package network names the broadcast network of a station from its guide
// names, its name on the air, and its call sign.
package network

import (
	"regexp"
	"strings"
)

// Of reports the broadcast network of a station: ABC, CBS, FOX, NBC,
// PBS, CW, MyNetworkTV, ION, Univision, Telemundo, UniMás, and in Canada CBC,
// Radio-Canada, CTV, CTV 2, Global, Citytv, TVO, Télé-Québec, TVA, and Noovo.
// A guide name wins when it is the network ("PBS", "UNIVSN"), a branded
// number ("FOX 5", "NBC4", "CW50"), or ends in the network ("KBWV-NBC",
// "KBWV (NBC)"). Anything else is a call sign: DT/HD suffixes come off, then
// the call-sign rules and the table. A name that merely contains those
// letters is not a network.
func Of(guideNames []string, callSigns ...string) string {
	for _, name := range guideNames {
		if net := brandedNetwork(name); net != "" {
			return net
		}
	}
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, guideNames...), callSigns...) {
		call := NormCall(name)
		if len(call) < 3 || seen[call] {
			continue
		}
		seen[call] = true
		if net := calls[call]; net != "" {
			return net
		}
		if net := callRule(call); net != "" {
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

// network is one broadcast network and the names guides and PSIP short names
// (seven letters at most) use for it.
type network struct {
	name    string
	country string
	aliases []string
}

// networks lists the main network first in each country: setup stars these.
var networks = []network{
	{"ABC", "US", []string{"ABC", "AMERICAN BROADCASTING COMPANY"}},
	{"CBS", "US", []string{"CBS"}},
	{"FOX", "US", []string{"FOX", "FOX BROADCASTING", "FOX BROADCASTING COMPANY"}},
	{"NBC", "US", []string{"NBC", "NATIONAL BROADCASTING COMPANY"}},
	{"PBS", "US", []string{"PBS", "PUBLIC BROADCASTING SERVICE"}},
	{"CW", "US", []string{"CW", "THE CW", "CWTV", "THE CW NETWORK"}},
	{"MyNetworkTV", "US", []string{"MYNETWORKTV", "MY NETWORK TV", "MYNET", "MYNETTV", "MNTV", "MYTV"}},
	{"ION", "US", []string{"ION", "ION TV", "IONTV", "ION TELEVISION"}},
	{"Univision", "US", []string{"UNIVISION", "UNIVSN", "UNIVISN"}},
	{"Telemundo", "US", []string{"TELEMUNDO", "TELEMUN", "TELMUND", "TLMD"}},
	{"UniMás", "US", []string{"UNIMAS", "UNI MAS"}},
	{"CBC", "CA", []string{"CBC", "CBC TV", "CBC TELEVISION"}},
	{"Radio-Canada", "CA", []string{"RADIO-CANADA", "RADIO CANADA", "ICI RADIO-CANADA", "ICI RADIO CANADA", "ICI TELE", "SRC", "RADCAN"}},
	{"CTV", "CA", []string{"CTV"}},
	{"CTV 2", "CA", []string{"CTV 2", "CTV2", "CTV TWO"}},
	{"Global", "CA", []string{"GLOBAL", "GLOBAL TV", "GLOBALTV"}},
	{"Citytv", "CA", []string{"CITYTV", "CITY TV", "CITY"}},
	{"TVO", "CA", []string{"TVO", "TVONTARIO"}},
	{"Télé-Québec", "CA", []string{"TELE-QUEBEC", "TELE QUEBEC", "TELEQC", "TELEQUE", "TLQ"}},
	{"TVA", "CA", []string{"TVA"}},
	{"Noovo", "CA", []string{"NOOVO"}},
}

// Main are the networks setup stars and names in a country: the big
// four in the US, the national English networks in Canada (and the French
// ones, since a Quebec lineup carries those instead).
func Main(country string) []string {
	if country == "CA" {
		return []string{"CBC", "CTV", "Global", "Citytv", "Radio-Canada", "TVA", "Noovo"}
	}
	return []string{"ABC", "CBS", "FOX", "NBC"}
}

// IsMain is a network setup stars, in either country.
func IsMain(name string) bool {
	for _, country := range []string{"US", "CA"} {
		for _, net := range Main(country) {
			if net == name {
				return true
			}
		}
	}
	return false
}

var byAlias = map[string]string{}

// branded matches a network alias followed by a channel number ("FOX 5",
// "NBC4", "CW50", "MY20"), a call sign followed by the network ("KBWV-NBC",
// "CBWV CTV"), or a network in parentheses at the end ("KBWV (NBC)").
var branded, trailing, paren *regexp.Regexp

func init() {
	var alts []string
	for _, net := range networks {
		for _, alias := range net.aliases {
			byAlias[alias] = net.name
			alts = append(alts, regexp.QuoteMeta(alias))
		}
	}
	// Longer aliases first so "CTV 2" wins over "CTV".
	for i := 1; i < len(alts); i++ {
		for j := i; j > 0 && len(alts[j]) > len(alts[j-1]); j-- {
			alts[j], alts[j-1] = alts[j-1], alts[j]
		}
	}
	alt := strings.Join(alts, "|")
	branded = regexp.MustCompile(`^(` + alt + `)[\s-]?(\d{1,2})(\.\d{1,2})?$`)
	trailing = regexp.MustCompile(`^[CKWX][A-Z]{2,4}(-(DT|TV|HD|LD|CD))?[\s-]+(` + alt + `)$`)
	paren = regexp.MustCompile(`\((` + alt + `)\)$`)
	// "MY20" and "MY 9" are MyNetworkTV stations by their channel number.
	myNumber = regexp.MustCompile(`^MY[\s-]?\d{1,2}$`)
}

var myNumber *regexp.Regexp

func brandedNetwork(name string) string {
	s := cleanName(name)
	if s == "" {
		return ""
	}
	if net := byAlias[s]; net != "" {
		return net
	}
	if m := branded.FindStringSubmatch(s); m != nil {
		// "CTV2" is CTV 2, not CTV on channel 2.
		if net := byAlias[m[1]+m[2]]; net != "" && m[3] == "" {
			return net
		}
		return byAlias[m[1]]
	}
	if myNumber.MatchString(s) {
		return "MyNetworkTV"
	}
	if m := trailing.FindStringSubmatch(s); m != nil {
		return byAlias[m[3]]
	}
	if m := paren.FindStringSubmatch(s); m != nil {
		return byAlias[m[1]]
	}
	return ""
}

// cleanName is a guide name in capitals with accents, runs of spaces, and a
// trailing HD or DT taken off ("Télé-Québec HD" reads as "TELE-QUEBEC").
func cleanName(name string) string {
	s := strings.ToUpper(strings.TrimSpace(foldAccents(name)))
	s = strings.Join(strings.Fields(s), " ")
	for _, suf := range []string{" HD", "-HD", " DT", "-DT"} {
		s = strings.TrimSuffix(s, suf)
	}
	return s
}

func foldAccents(s string) string {
	return accents.Replace(s)
}

var accents = strings.NewReplacer(
	"é", "e", "É", "E", "è", "e", "È", "E", "ê", "e", "Ê", "E", "á", "a", "Á", "A",
	"à", "a", "À", "A", "â", "a", "Â", "A", "í", "i", "Í", "I", "ó", "o", "Ó", "O",
	"ô", "o", "Ô", "O", "ú", "u", "Ú", "U", "ç", "c", "Ç", "C", "ñ", "n", "Ñ", "N",
)

// callRule names the network from how Canada's public broadcaster assigns
// call signs: CBC Television stations are CB?T (CBWT), Radio-Canada's are
// CB??FT (CBWFT) plus the older CBFT, CBVT, and CBST.
func callRule(call string) string {
	if !strings.HasPrefix(call, "CB") {
		return ""
	}
	switch call {
	case "CBFT", "CBVT", "CBST":
		return "Radio-Canada"
	}
	if len(call) == 4 && call[3] == 'T' && isLetter(call[2]) {
		return "CBC"
	}
	if len(call) == 5 && strings.HasSuffix(call, "FT") && isLetter(call[2]) {
		return "Radio-Canada"
	}
	return ""
}

func isLetter(b byte) bool { return b >= 'A' && b <= 'Z' }

// NormCall is a call sign without dashes, spaces, and a DT, DT1, HD, TV, or
// LD suffix. DT2 and higher are subchannels (KBWV-DT2) and stay.
func NormCall(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	for _, suf := range []string{"DT1", "HD", "DT", "TV", "LD"} {
		if strings.HasSuffix(s, suf) && len(s)-len(suf) >= 3 {
			s = strings.TrimSuffix(s, suf)
		}
	}
	return s
}
