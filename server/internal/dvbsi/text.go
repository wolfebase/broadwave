package dvbsi

import (
	"strings"
	"unicode/utf8"
)

// Text reads one DVB string (EN 300 468 Annex A). The first byte may name
// the character table; without one the text is the Latin table, ISO/IEC 6937
// with the euro sign. ISO/IEC 8859 parts, UCS-2, and UTF-8 are read. Korean,
// Chinese, and an encoding_type_id are not, and give an empty string.
// Emphasis marks are dropped and a line break becomes a space.
func Text(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var s string
	switch first := b[0]; {
	case first >= 0x20:
		s = latin6937(b)
	case first >= 0x01 && first <= 0x0B:
		s = iso(int(first)+4, b[1:])
	case first == 0x10:
		if len(b) < 3 || b[1] != 0 {
			return ""
		}
		s = iso(int(b[2]), b[3:])
	case first == 0x11 || first == 0x14:
		// 0x14 is the Big5 subset of ISO/IEC 10646, two bytes a character.
		s = ucs2(b[1:])
	case first == 0x15:
		s = strings.ToValidUTF8(string(b[1:]), "")
	default:
		return ""
	}
	return clean(s)
}

func iso(part int, b []byte) string {
	table, ok := iso8859[part]
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, c := range b {
		switch {
		case c < 0xA0:
			out.WriteRune(rune(c))
		default:
			if r := table[c-0xA0]; r != 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

func ucs2(b []byte) string {
	var out strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i])<<8 | rune(b[i+1])
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// latin6937 is EN 300 468 figure A.1: ISO/IEC 6937, where a diacritical
// byte (0xC1-0xCF) comes before the letter it sits on.
func latin6937(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 0xC1 && c <= 0xCF {
			if i+1 >= len(b) {
				break
			}
			base := rune(b[i+1])
			i++
			if r, ok := compose6937(c, base); ok {
				out.WriteRune(r)
			} else if base >= 0x20 && base < 0x7F {
				out.WriteRune(base)
			}
			continue
		}
		switch {
		case c < 0xA0:
			out.WriteRune(rune(c))
		default:
			if r := upper6937[c-0xA0]; r != 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

func compose6937(mark byte, base rune) (rune, bool) {
	pairs := composed6937[mark]
	for len(pairs) > 0 {
		b, n := utf8.DecodeRuneInString(pairs)
		pairs = pairs[n:]
		r, m := utf8.DecodeRuneInString(pairs)
		pairs = pairs[m:]
		if b == base {
			return r, true
		}
	}
	return 0, false
}

// upper6937 is 0xA0-0xFF of figure A.1. EN 300 468 V1.17.1 (2022-10),
// annex A, under the figure: "This table is a superset of ISO/IEC 6937 with
// addition of the Euro symbol (U+20AC) in position 0xA4." The rest follows
// ISO/IEC 6937 (ITU-T T.51): 0xA6 is "#", 0xA8 the currency sign, and
// 0xC1-0xCF the diacritical marks, which T.51 leaves 0xC9 out of; it is read
// as the diaeresis an older edition put there, and 0xCC (underline) is dropped.
var upper6937 = [96]rune{
	0x00A0, 0x00A1, 0x00A2, 0x00A3, 0x20AC, 0x00A5, 0x0023, 0x00A7, 0x00A4, 0x2018, 0x201C, 0x00AB, 0x2190, 0x2191, 0x2192, 0x2193,
	0x00B0, 0x00B1, 0x00B2, 0x00B3, 0x00D7, 0x00B5, 0x00B6, 0x00B7, 0x00F7, 0x2019, 0x201D, 0x00BB, 0x00BC, 0x00BD, 0x00BE, 0x00BF,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0x2015, 0x00B9, 0x00AE, 0x00A9, 0x2122, 0x266A, 0x00AC, 0x00A6, 0, 0, 0, 0, 0x215B, 0x215C, 0x215D, 0x215E,
	0x2126, 0x00C6, 0x0110, 0x00AA, 0x0126, 0, 0x0132, 0x013F, 0x0141, 0x00D8, 0x0152, 0x00BA, 0x00DE, 0x0166, 0x014A, 0x0149,
	0x0138, 0x00E6, 0x0111, 0x00F0, 0x0127, 0x0131, 0x0133, 0x0140, 0x0142, 0x00F8, 0x0153, 0x00DF, 0x00FE, 0x0167, 0x014B, 0x00AD,
}

// clean drops control codes (C0, C1, and their UCS-2 forms at U+E080-U+E09F),
// turns a line break (0x8A) into a space, and collapses the spaces.
func clean(s string) string {
	var out strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == 0x8A || r == 0xE08A || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == 0xA0:
			space = true
			continue
		case r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F) || (r >= 0xE080 && r <= 0xE09F) || r == 0xAD || r == utf8.RuneError:
			continue
		}
		if space && out.Len() > 0 {
			out.WriteByte(' ')
		}
		space = false
		out.WriteRune(r)
	}
	return out.String()
}
