package dvbsi

import (
	"os"
	"strings"
)

// language is the server locale as an ISO 639-2 code, eng when unset.
func language() string {
	for _, key := range []string{"LC_ALL", "LANG"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "" || v == "c" || v == "posix" {
			continue
		}
		if i := strings.IndexAny(v, "._@"); i >= 0 {
			v = v[:i]
		}
		if len(v) == 3 {
			return v
		}
		if len(v) >= 2 {
			if code, ok := twoLetter[v[:2]]; ok {
				return code
			}
		}
	}
	return "eng"
}

func langCode(b []byte) string {
	return strings.ToLower(strings.Trim(string(b), "\x00 "))
}

// sameLanguage matches a code in either ISO 639-2 form: stations send both.
func sameLanguage(a, b string) bool {
	if a == b {
		return true
	}
	if t, ok := terminology[a]; ok && t == b {
		return true
	}
	t, ok := terminology[b]
	return ok && t == a
}

var twoLetter = map[string]string{
	"en": "eng", "de": "ger", "fr": "fre", "nl": "dut", "it": "ita", "es": "spa",
	"pt": "por", "sv": "swe", "no": "nor", "nb": "nor", "nn": "nno", "da": "dan",
	"fi": "fin", "pl": "pol", "cs": "cze", "sk": "slo", "hu": "hun", "ro": "rum",
	"el": "gre", "tr": "tur", "ru": "rus", "uk": "ukr", "bg": "bul", "hr": "hrv",
	"sr": "srp", "sl": "slv", "et": "est", "lv": "lav", "lt": "lit", "is": "ice",
	"ga": "gle", "cy": "wel", "gd": "gla", "ca": "cat", "eu": "baq", "he": "heb",
	"ar": "ara", "th": "tha", "zh": "chi", "ja": "jpn", "ko": "kor", "mk": "mac",
	"sq": "alb", "ms": "may", "vi": "vie", "hi": "hin",
}

// terminology maps an ISO 639-2/B code to its /T twin where they differ.
var terminology = map[string]string{
	"ger": "deu", "fre": "fra", "dut": "nld", "cze": "ces", "slo": "slk",
	"rum": "ron", "gre": "ell", "ice": "isl", "wel": "cym", "baq": "eus",
	"chi": "zho", "mac": "mkd", "alb": "sqi", "may": "msa", "per": "fas",
	"arm": "hye", "geo": "kat", "bur": "mya", "tib": "bod",
}
