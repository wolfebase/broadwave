package psip

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
)

func TestAnnexCCodeCounts(t *testing.T) {
	if len(titleCodes) != 913 {
		t.Fatalf("title codes %d", len(titleCodes))
	}
	if len(descCodes) != 832 {
		t.Fatalf("description codes %d", len(descCodes))
	}
}

func TestAnnexCEveryCode(t *testing.T) {
	check := func(name string, codes []huffCode, trees [128]*huffNode) {
		t.Helper()
		for _, e := range codes {
			got, ok := readSymbol(trees[e.prev], &bitReader{b: codeBytes(e.bits, e.n)})
			if !ok || got != e.sym {
				t.Fatalf("%s prev %d sym %d (%d bits) got %d ok %v", name, e.prev, e.sym, e.n, got, ok)
			}
		}
	}
	check("title", titleCodes, titleTrees)
	check("description", descCodes, descTrees)
}

func codeBytes(bits uint16, n uint8) []byte {
	acc := uint32(bits) << (32 - n)
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], acc)
	return buf[:]
}

func TestAnnexCExamples(t *testing.T) {
	// Byte strings are the Table C4 and C6 codes for these words, including
	// the terminate symbol. Sqpa is the Annex C.2.1 escape example: compressed
	// q, compressed ESC, uncompressed p, compressed a, reached from S.
	cases := []struct {
		trees [128]*huffNode
		hex   string
		want  string
	}{
		{titleTrees, "41", "The"},
		{titleTrees, "35ef", "News"},
		{titleTrees, "fd2bf2ff", "Hello"},
		{titleTrees, "168e122f", "Sqpa"},
		{titleTrees, "b95be7a403", "Café"},
		{titleTrees, "b95be7a487", "Café!"},
		{titleTrees, "71007f", "A"},
		{descTrees, "d7f985373f", "The show."},
		{descTrees, "22ff4e007f", "News"},
	}
	for _, tc := range cases {
		raw, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		if got := decodeHuffman(tc.trees, raw); got != tc.want {
			t.Fatalf("%q decoded %q", tc.want, got)
		}
		wrapped := oneString("eng", compressionOf(tc.trees), 0x00, raw)
		if got := decodeTextLang(wrapped, "eng"); got != tc.want {
			t.Fatalf("segment %q decoded %q", tc.want, got)
		}
		ff := oneString("eng", compressionOf(tc.trees), 0xFF, raw)
		if got := decodeTextLang(ff, "eng"); got != tc.want {
			t.Fatalf("mode FF %q decoded %q", tc.want, got)
		}
	}
}

func compressionOf(trees [128]*huffNode) byte {
	if trees[0] == titleTrees[0] {
		return 1
	}
	return 2
}

func TestPlainModes(t *testing.T) {
	latin := oneString("eng", 0, 0x00, []byte{'C', 'a', 'f', 0xE9})
	if got := decodeTextLang(latin, "eng"); got != "Café" {
		t.Fatalf("latin-1 %q", got)
	}
	utf := oneString("eng", 0, 0x3F, []byte{0x00, 'C', 0x00, 'a', 0x00, 'f', 0x00, 0xE9})
	if got := decodeTextLang(utf, "eng"); got != "Café" {
		t.Fatalf("utf-16 %q", got)
	}
	// U+1F600 is the surrogate pair D83D DE00.
	emoji := oneString("eng", 0, 0x3F, []byte{0xD8, 0x3D, 0xDE, 0x00})
	if got := decodeTextLang(emoji, "eng"); got != "😀" {
		t.Fatalf("surrogate %q", got)
	}
	// Mode 0x04 is U+0400–U+04FF. Byte 0x10 is U+0410.
	cyr := oneString("eng", 0, 0x04, []byte{0x10})
	if got := decodeTextLang(cyr, "eng"); got != "А" {
		t.Fatalf("cyrillic %q", got)
	}
	bom := oneString("eng", 0, 0x3F, []byte{0xFE, 0xFF, 0x00, 'A'})
	if got := decodeTextLang(bom, "eng"); got != "A" {
		t.Fatalf("bom %q", got)
	}
}

func TestLanguagePreference(t *testing.T) {
	both := twoStrings("spa", "Hola", "eng", "Hello")
	if got := decodeTextLang(both, "spa"); got != "Hola" {
		t.Fatalf("spa %q", got)
	}
	if got := decodeTextLang(both, "es_MX.UTF-8"); got != "Hola" {
		t.Fatalf("locale spa %q", got)
	}
	if got := decodeTextLang(both, "fre"); got != "Hello" {
		t.Fatalf("fallback %q", got)
	}
	only := oneString("spa", 0, 0x00, []byte("Hola"))
	if got := decodeTextLang(only, "fre"); got != "Hola" {
		t.Fatalf("first %q", got)
	}
	parts := multiSegment("eng", []byte("Hel"), []byte("lo"))
	if got := decodeTextLang(parts, "eng"); got != "Hello" {
		t.Fatalf("segments %q", got)
	}
}

func TestDeviceLanguage(t *testing.T) {
	both := twoStrings("spa", "Hola", "eng", "Hello")
	t.Setenv("LC_ALL", "es_MX.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := decodeText(both); got != "Hola" {
		t.Fatalf("LC_ALL %q", got)
	}
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "fr_FR.UTF-8")
	fre := twoStrings("fre", "Bonjour", "eng", "Hello")
	if got := decodeText(fre); got != "Bonjour" {
		t.Fatalf("LANG %q", got)
	}
}

func TestRawEscapeReturnsToTheHuffmanTree(t *testing.T) {
	// A, then a raw é (order-1 escape), then an uncompressed 27, which
	// returns to A's tree for the s. The é stays; the 27 is not text.
	a, an := titleCode(0, 'A')
	esc, escn := titleCode('A', 27)
	s, sn := titleCode('A', 's')
	if an == 0 || escn == 0 || sn == 0 {
		t.Fatal("missing title codes")
	}
	var w bitBuf
	w.bits(a, an)
	w.bits(esc, escn)
	w.bits(0xE9, 8)
	w.bits(27, 8)
	w.bits(s, sn)
	if term, tn := titleCode('s', 0); tn != 0 {
		w.bits(term, tn)
	} else {
		e2, e2n := titleCode('s', 27)
		w.bits(e2, e2n)
		w.bits(0, 8)
	}
	if got := decodeHuffman(titleTrees, w.b); got != "Aés" {
		t.Fatalf("got %q", got)
	}
}

func titleCode(prev, sym byte) (uint16, uint8) {
	for _, e := range titleCodes {
		if e.prev == prev && e.sym == sym {
			return e.bits, e.n
		}
	}
	return 0, 0
}

type bitBuf struct {
	b []byte
	n int
}

func (w *bitBuf) bits(code uint16, n uint8) {
	for i := int(n) - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		if (code>>i)&1 == 1 {
			w.b[w.n/8] |= 1 << (7 - w.n%8)
		}
		w.n++
	}
}

func TestCompressedEITTitleIsNotEmpty(t *testing.T) {
	raw, _ := hex.DecodeString("35ef")
	title := oneString("eng", 1, 0x00, raw)
	sec := eitSection(title)
	events := parseEIT(sec, 0)
	if len(events) != 1 {
		t.Fatalf("events %d", len(events))
	}
	if events[0].Title != "News" {
		t.Fatalf("title %q", events[0].Title)
	}
	body := ettSection(title)
	text, ok := parseETT(body)
	if !ok || text.Body != "News" {
		t.Fatalf("ett %+v ok %v", text, ok)
	}
}

func TestWDAFTitlesNotBlank(t *testing.T) {
	f, err := os.ReadFile("testdata/wdaf.ts")
	if err != nil {
		t.Fatal(err)
	}
	var titled, blank int
	for _, sec := range assemble(f) {
		if len(sec.Body) < 1 {
			continue
		}
		var fields [][]byte
		switch sec.Body[0] {
		case tableEIT:
			fields = eitTitleFields(sec.Body)
		case tableETT:
			if len(sec.Body) > 17 {
				fields = [][]byte{sec.Body[13 : len(sec.Body)-4]}
			}
		default:
			continue
		}
		for _, field := range fields {
			if !mssHasBytes(field) {
				continue
			}
			if decodeTextLang(field, "eng") == "" {
				blank++
				if blank == 1 {
					t.Errorf("blank text from %d bytes, table 0x%02x", len(field), sec.Body[0])
				}
				continue
			}
			titled++
		}
	}
	if titled == 0 {
		t.Fatal("sample has no titled strings")
	}
	if blank != 0 {
		t.Fatalf("%d strings had bytes and decoded empty (%d kept)", blank, titled)
	}
}

func mssHasBytes(b []byte) bool {
	if len(b) < 1 {
		return false
	}
	n := int(b[0])
	pos := 1
	for i := 0; i < n && pos+4 <= len(b); i++ {
		pos += 3
		segs := int(b[pos])
		pos++
		for s := 0; s < segs && pos+3 <= len(b); s++ {
			nbytes := int(b[pos+2])
			pos += 3
			if nbytes > 0 {
				return true
			}
			if pos+nbytes > len(b) {
				return false
			}
			pos += nbytes
		}
	}
	return false
}

func eitTitleFields(b []byte) [][]byte {
	if len(b) < 11 {
		return nil
	}
	pos := 9
	n := int(b[pos])
	pos++
	var out [][]byte
	for i := 0; i < n && pos+8 <= len(b)-4; i++ {
		if pos+9 >= len(b)-4 {
			break
		}
		pos += 9
		if pos >= len(b)-4 {
			break
		}
		titleLen := int(b[pos])
		pos++
		if pos+titleLen > len(b)-4 {
			break
		}
		out = append(out, b[pos:pos+titleLen])
		pos += titleLen
		if pos+2 > len(b)-4 {
			break
		}
		descLen := int(binary.BigEndian.Uint16(b[pos:pos+2])) & 0x0FFF
		pos += 2
		if pos+descLen > len(b)-4 {
			break
		}
		pos += descLen
	}
	return out
}

func oneString(lang string, comp, mode byte, payload []byte) []byte {
	b := []byte{1}
	b = append(b, langBytes(lang)...)
	b = append(b, 1, comp, mode, byte(len(payload)))
	return append(b, payload...)
}

func twoStrings(langA, textA, langB, textB string) []byte {
	a := []byte(textA)
	b := []byte(textB)
	out := []byte{2}
	out = append(out, langBytes(langA)...)
	out = append(out, 1, 0, 0, byte(len(a)))
	out = append(out, a...)
	out = append(out, langBytes(langB)...)
	out = append(out, 1, 0, 0, byte(len(b)))
	return append(out, b...)
}

func multiSegment(lang string, parts ...[]byte) []byte {
	out := []byte{1}
	out = append(out, langBytes(lang)...)
	out = append(out, byte(len(parts)))
	for _, p := range parts {
		out = append(out, 0, 0, byte(len(p)))
		out = append(out, p...)
	}
	return out
}

func langBytes(lang string) []byte {
	b := []byte(lang)
	for len(b) < 3 {
		b = append(b, ' ')
	}
	return b[:3]
}

func eitSection(title []byte) []byte {
	sec := make([]byte, 20+len(title)+2+4)
	sec[0] = tableEIT
	binary.BigEndian.PutUint16(sec[3:5], 7)
	sec[9] = 1
	sec[16] = 0
	sec[17] = 0
	sec[18] = 60
	sec[19] = byte(len(title))
	copy(sec[20:], title)
	return sec
}

func ettSection(text []byte) []byte {
	// header 8 + protocol_version + ETM_id + multiple string + crc.
	sec := make([]byte, 13+len(text)+4)
	sec[0] = tableETT
	etm := uint32(7)<<16 | uint32(2)<<2
	binary.BigEndian.PutUint32(sec[9:13], etm)
	copy(sec[13:], text)
	return sec
}
