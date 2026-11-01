package dvbsi

import "testing"

func TestTextTables(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"plain ASCII is the Latin table", []byte("Evening News"), "Evening News"},
		{"6937 acute before its letter", []byte{'C', 'a', 'f', 0xC2, 'e'}, "Café"},
		{"6937 diaeresis and caron", []byte{0xC8, 'U', 'b', 'e', 'r', ' ', 0xCF, 's', 'a'}, "Über ša"},
		{"6937 cedilla, ring, double acute", []byte{'G', 'a', 'r', 0xCB, 'c', 'o', 'n', ' ', 0xCA, 'A', ' ', 0xCD, 'o'}, "Garçon Å ő"},
		{"6937 euro, pound, and letters of their own", []byte{0xA4, '5', ' ', 0xA3, '2', ' ', 0xFB, 0xE9, 0xF9, 0xE8}, "€5 £2 ßØøŁ"},
		{"6937 quotes", []byte{0xAA, 'H', 'i', 0xBA}, "“Hi”"},
		{"6937 mark with no letter after it", []byte{'E', 'n', 'd', 0xC2}, "End"},
		{"8859-5 by its byte", []byte{0x01, 0xB0, 0xD1, 0xD0}, "Аба"},
		{"8859-7 by its byte", []byte{0x03, 0xC1, 0xE8}, "Αθ"},
		{"8859-9 by its byte", []byte{0x05, 'I', 's', 't', 'a', 'n', 'b', 'u', 'l', ' ', 0xFD, 0xF0}, "Istanbul ığ"},
		{"8859-15 by its byte", []byte{0x0B, 0xA4, '9'}, "€9"},
		{"8859-2 by number", []byte{0x10, 0x00, 0x02, 0xB9, 'k', 'o', 'd', 'a'}, "škoda"},
		{"8859-1 by number", []byte{0x10, 0x00, 0x01, 'G', 'r', 0xFC, 0xDF, 'e'}, "Grüße"},
		{"UTF-8", []byte("\x15Smörgåsbord ✓"), "Smörgåsbord ✓"},
		{"UTF-8 broken byte is dropped", []byte("\x15ok\xffay"), "okay"},
		{"UCS-2", []byte{0x11, 0x00, 'H', 0x00, 0xE9, 0x04, 0x10}, "HéА"},
		{"emphasis dropped, line break a space", []byte{0x86, 'B', 'i', 'g', 0x87, 0x8A, 'S', 'h', 'o', 'w'}, "Big Show"},
		{"UCS-2 control codes", []byte{0x11, 0xE0, 0x86, 0x00, 'A', 0xE0, 0x87, 0xE0, 0x8A, 0x00, 'B'}, "A B"},
		{"UTF-8 control codes", []byte("\x15ABC"), "AB C"},
		{"C0 controls and extra spaces", []byte{' ', 'A', 0x01, ' ', ' ', 'B', 0x1F, ' '}, "A B"},
		{"Korean is not read", []byte{0x12, 0xB0, 0xA1}, ""},
		{"Chinese is not read", []byte{0x13, 0xB0, 0xA1}, ""},
		{"an encoding id is not read", []byte{0x1F, 0x01, 'x'}, ""},
		{"a reserved table is not read", []byte{0x0C, 'x'}, ""},
		{"8859-12 does not exist", []byte{0x10, 0x00, 0x0C, 'x'}, ""},
		{"a short 0x10 header", []byte{0x10, 0x00}, ""},
		{"empty", nil, ""},
	}
	for _, tc := range cases {
		if got := Text(tc.in); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Every ISO 8859 part the tables name is read from both selector forms.
func TestEveryISOPartReads(t *testing.T) {
	for part := 1; part <= 15; part++ {
		_, ok := iso8859[part]
		if ok != (part != 12) {
			t.Fatalf("part %d present %v", part, ok)
		}
		if !ok {
			continue
		}
		long := Text([]byte{0x10, 0x00, byte(part), 'a', 0xE9})
		if long == "" || long[0] != 'a' || len([]rune(long)) != 2 && part != 6 && part != 8 {
			t.Errorf("part %d: %q", part, long)
		}
		if part >= 5 && part <= 15 && part != 12 {
			short := Text([]byte{byte(part - 4), 'a', 0xE9})
			if short != long {
				t.Errorf("part %d: one-byte selector %q, three-byte %q", part, short, long)
			}
		}
	}
}
