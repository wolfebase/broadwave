package dvbsi

// genres names a content_descriptor nibble pair (EN 300 468 table 29) in the
// words the guide sorts on: Movie, News, Sports, and Children land in their
// own rows. A pair the table leaves reserved takes its first nibble's name.
var genres = map[byte][]string{
	0x10: {"Movie"},
	0x11: {"Movie", "Thriller"},
	0x12: {"Movie", "Adventure"},
	0x13: {"Movie", "Science fiction"},
	0x14: {"Comedy"},
	0x15: {"Soap"},
	0x16: {"Romance"},
	0x17: {"Drama"},
	0x18: {"Movie"},

	0x20: {"News"},
	0x21: {"News", "Weather"},
	0x22: {"News magazine"},
	0x23: {"Documentary"},
	0x24: {"Talk"},

	0x30: {"Entertainment"},
	0x31: {"Game show"},
	0x32: {"Variety"},
	0x33: {"Talk"},

	0x40: {"Sports"},
	0x41: {"Sports", "Special event"},
	0x42: {"Sports", "Magazine"},
	0x43: {"Sports", "Soccer"},
	0x44: {"Sports", "Tennis"},
	0x45: {"Sports"},
	0x46: {"Sports", "Athletics"},
	0x47: {"Sports", "Motorsports"},
	0x48: {"Sports", "Water sports"},
	0x49: {"Sports", "Winter sports"},
	0x4A: {"Sports", "Equestrian"},
	0x4B: {"Sports", "Martial arts"},

	0x50: {"Children"},
	0x51: {"Children"},
	0x52: {"Children"},
	0x53: {"Children"},
	0x54: {"Children", "Educational"},
	0x55: {"Children", "Animation"},

	0x60: {"Music"},
	0x61: {"Music"},
	0x62: {"Music"},
	0x63: {"Music"},
	0x64: {"Music"},
	0x65: {"Music"},
	0x66: {"Dance"},

	0x70: {"Arts"},
	0x71: {"Arts"},
	0x72: {"Arts"},
	0x73: {"Religious"},
	0x74: {"Arts"},
	0x75: {"Arts"},
	0x76: {"Arts"},
	0x77: {"Arts"},
	0x78: {"Arts"},
	0x79: {"Arts"},
	0x7A: {"Arts"},
	0x7B: {"Arts"},

	0x80: {"Public affairs"},
	0x81: {"Documentary"},
	0x82: {"Public affairs"},
	0x83: {"Biography"},

	0x90: {"Education"},
	0x91: {"Nature"},
	0x92: {"Science"},
	0x93: {"Health"},
	0x94: {"Travel"},
	0x95: {"Education"},
	0x96: {"Education"},
	0x97: {"Education"},

	0xA0: {"Lifestyle"},
	0xA1: {"Travel"},
	0xA2: {"Lifestyle"},
	0xA3: {"Health"},
	0xA4: {"Health"},
	0xA5: {"Cooking"},
	0xA6: {"Shopping"},
	0xA7: {"Gardening"},
}

// liveContent is "live broadcast" among the special characteristics (0xB3).
const liveContent = 0xB3

// genreNames is the guide's words for a content_descriptor body, in order,
// once each, and whether a pair marks the event live.
func genreNames(body []byte) ([]string, bool) {
	var out []string
	live := false
	seen := map[string]bool{}
	for i := 0; i+1 < len(body); i += 2 {
		pair := body[i]
		if pair == liveContent {
			live = true
			continue
		}
		names, ok := genres[pair]
		if !ok {
			names = genres[pair&0xF0]
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out, live
}
