package guide

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// unmarshalXMLTV is xml.Unmarshal that also reads the 8-bit encodings older
// European grabbers declare. encoding/xml alone takes UTF-8, and refused the
// whole guide of a file that said encoding="ISO-8859-1".
func unmarshalXMLTV(data []byte, v any) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	return dec.Decode(v)
}

func charsetReader(label string, in io.Reader) (io.Reader, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	var table map[byte]rune
	switch label {
	case "utf-8", "utf8":
		return in, nil
	// A file that says Latin-1 is nearly always Windows-1252 (as browsers read
	// it): the curly quotes and the euro sign are in 0x80-0x9F.
	case "iso-8859-1", "iso8859-1", "iso_8859-1", "latin1", "latin-1", "l1", "us-ascii", "ascii", "windows-1252", "cp1252":
		table = cp1252
	case "iso-8859-15", "iso8859-15", "iso_8859-15", "latin-9", "latin9":
		table = latin9
	default:
		return nil, fmt.Errorf("the guide is in %s; save it as UTF-8", label)
	}
	raw, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(raw)+len(raw)/8)
	for _, b := range raw {
		r := rune(b)
		if mapped, ok := table[b]; ok {
			r = mapped
		}
		out = utf8.AppendRune(out, r)
	}
	return bytes.NewReader(out), nil
}

var cp1252 = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ',
	0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘', 0x92: '’', 0x93: '“',
	0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›',
	0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

var latin9 = map[byte]rune{
	0xA4: '€', 0xA6: 'Š', 0xA8: 'š', 0xB4: 'Ž', 0xB8: 'ž', 0xBC: 'Œ', 0xBD: 'œ', 0xBE: 'Ÿ',
}

// first is the first non-empty of a repeated element. A guide lists a title
// once per language, the main language first.
func first(values []string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
