package psip

import (
	"os"
	"strings"
)

// decodeText reads one multiple_string_structure (A/65). When a station sends
// more than one language, the server locale wins, then eng, then the first
// string that has characters.
func decodeText(b []byte) string {
	return decodeTextLang(b, deviceLanguage())
}

func decodeTextLang(b []byte, lang string) string {
	if len(b) < 1 {
		return ""
	}
	n := int(b[0])
	pos := 1
	var items []langString
	for i := 0; i < n && pos+4 <= len(b); i++ {
		code := strings.ToLower(strings.Trim(string(b[pos:pos+3]), "\x00 "))
		pos += 3
		segs := int(b[pos])
		pos++
		var text string
		for s := 0; s < segs && pos+3 <= len(b); s++ {
			comp := b[pos]
			mode := b[pos+1]
			nbytes := int(b[pos+2])
			pos += 3
			if pos+nbytes > len(b) {
				break
			}
			chunk := b[pos : pos+nbytes]
			pos += nbytes
			part, ok := decodeSegment(comp, mode, chunk)
			if ok {
				text += part
			}
		}
		items = append(items, langString{lang: code, text: text})
	}
	return pickLang(items, lang)
}

type langString struct {
	lang string
	text string
}

func decodeSegment(comp, mode byte, chunk []byte) (string, bool) {
	switch comp {
	case 0:
		return decodePlain(mode, chunk)
	case 1:
		if mode != 0x00 && mode != 0xFF {
			return "", false
		}
		return decodeHuffman(titleTrees, chunk), true
	case 2:
		if mode != 0x00 && mode != 0xFF {
			return "", false
		}
		return decodeHuffman(descTrees, chunk), true
	default:
		return "", false
	}
}

// decodePlain handles the modes in A/65 Table 6.41 that this server can read.
// Mode 0x00 is Unicode U+0000-U+00FF (ISO-8859-1). The other assigned modes
// are the same idea for another 256-character block. Mode 0x3F is UTF-16,
// most significant bit first. Mode 0xFF means the mode byte is not used; the
// bytes are Latin-1.
func decodePlain(mode byte, chunk []byte) (string, bool) {
	switch mode {
	case 0x3F:
		return utf16be(chunk), true
	case 0xFF:
		return latin1(chunk), true
	default:
		if unicodeBlock(mode) {
			return blockText(mode, chunk), true
		}
		return "", false
	}
}

func unicodeBlock(mode byte) bool {
	switch mode {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
		0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27,
		0x30, 0x31, 0x32, 0x33:
		return true
	default:
		return false
	}
}

func blockText(mode byte, chunk []byte) string {
	var out strings.Builder
	for _, c := range chunk {
		r := rune(mode)<<8 | rune(c)
		if r == 0 {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func pickLang(items []langString, want string) string {
	want = bibliographic(want)
	if want == "" {
		want = "eng"
	}
	var first, eng string
	for _, it := range items {
		if it.text == "" {
			continue
		}
		if first == "" {
			first = it.text
		}
		got := bibliographic(it.lang)
		if got == want {
			return it.text
		}
		if got == "eng" && eng == "" {
			eng = it.text
		}
	}
	if want != "eng" && eng != "" {
		return eng
	}
	return first
}

func deviceLanguage() string {
	for _, key := range []string{"LC_ALL", "LANG"} {
		v := os.Getenv(key)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		if code := bibliographic(v); code != "" {
			return code
		}
	}
	return "eng"
}

func bibliographic(loc string) string {
	loc = strings.ToLower(strings.TrimSpace(loc))
	if i := strings.IndexAny(loc, "._@"); i >= 0 {
		loc = loc[:i]
	}
	if loc == "" {
		return ""
	}
	if len(loc) == 3 {
		return loc
	}
	if len(loc) >= 2 {
		if code, ok := bibCode[loc[:2]]; ok {
			return code
		}
	}
	return ""
}

// bibCode maps a POSIX language to the ISO 639-2/B code A/65 stores.
var bibCode = map[string]string{
	"en": "eng",
	"es": "spa",
	"fr": "fre",
	"de": "ger",
	"pt": "por",
	"it": "ita",
	"nl": "dut",
	"zh": "chi",
	"ja": "jpn",
	"ko": "kor",
	"sv": "swe",
	"no": "nor",
	"da": "dan",
	"fi": "fin",
	"pl": "pol",
	"ru": "rus",
	"ar": "ara",
	"he": "heb",
	"el": "gre",
	"tr": "tur",
	"vi": "vie",
	"th": "tha",
	"hi": "hin",
	"cs": "cze",
	"hu": "hun",
	"ro": "rum",
	"uk": "ukr",
}
