package psip

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode/utf16"
)

// huffCode is one row of an A/65 Annex C encode table.
type huffCode struct {
	prev byte
	sym  byte
	n    uint8
	bits uint16
}

type huffNode struct {
	leaf bool
	sym  byte
	down [2]*huffNode
}

var (
	titleCodes []huffCode
	descCodes  []huffCode
	titleTrees [128]*huffNode
	descTrees  [128]*huffNode
)

func init() {
	titleCodes = unpackCodes(titlePacked)
	descCodes = unpackCodes(descPacked)
	titleTrees = buildHuffman(titleCodes)
	descTrees = buildHuffman(descCodes)
}

func unpackCodes(s string) []huffCode {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw)%5 != 0 {
		panic("psip: huffman table")
	}
	out := make([]huffCode, len(raw)/5)
	for i := range out {
		p := raw[i*5:]
		out[i] = huffCode{prev: p[0], sym: p[1], n: p[2], bits: binary.BigEndian.Uint16(p[3:5])}
	}
	return out
}

func buildHuffman(codes []huffCode) [128]*huffNode {
	var roots [128]*huffNode
	for _, e := range codes {
		if e.prev > 127 || e.n == 0 || e.n > 16 {
			continue
		}
		if roots[e.prev] == nil {
			roots[e.prev] = &huffNode{}
		}
		n := roots[e.prev]
		for i := int(e.n) - 1; i >= 0; i-- {
			bit := byte((e.bits >> i) & 1)
			if n.down[bit] == nil {
				n.down[bit] = &huffNode{}
			}
			n = n.down[bit]
		}
		n.leaf = true
		n.sym = e.sym
	}
	return roots
}

// decodeHuffman reads an Annex C string. Bits are most-significant-bit first.
// Symbol 0 ends the string. Symbol 27 is the order-1 escape: the next eight
// bits are one uncompressed Latin-1 character. A character 128-255 is followed
// by another uncompressed character. An uncompressed 27 returns to the Huffman
// tree of the last character below 128.
func decodeHuffman(trees [128]*huffNode, chunk []byte) string {
	br := bitReader{b: chunk}
	var out strings.Builder
	prev := byte(0)
	ctx := byte(0)
	rawNext := false
	for range len(chunk)*8 + 1 {
		takeRaw := rawNext || prev >= 128
		rawNext = false
		before := br.i
		var c byte
		var ok bool
		if takeRaw {
			c, ok = br.u8()
		} else {
			c, ok = readSymbol(trees[ctx], &br)
			if ok && br.i == before {
				break
			}
		}
		if !ok || c == 0 {
			break
		}
		if c == 27 {
			if takeRaw {
				prev = ctx
			} else {
				rawNext = true
			}
			continue
		}
		out.WriteRune(rune(c))
		prev = c
		if c < 128 {
			ctx = c
		} else {
			rawNext = true
		}
	}
	return out.String()
}

func readSymbol(root *huffNode, br *bitReader) (byte, bool) {
	n := root
	if n == nil {
		return 0, false
	}
	for !n.leaf {
		bit, ok := br.bit()
		if !ok {
			return 0, false
		}
		n = n.down[bit]
		if n == nil {
			return 0, false
		}
	}
	return n.sym, true
}

type bitReader struct {
	b []byte
	i int
}

func (r *bitReader) bit() (byte, bool) {
	if r.i >= len(r.b)*8 {
		return 0, false
	}
	bit := (r.b[r.i/8] >> (7 - r.i%8)) & 1
	r.i++
	return bit, true
}

func (r *bitReader) u8() (byte, bool) {
	var v byte
	for range 8 {
		bit, ok := r.bit()
		if !ok {
			return 0, false
		}
		v = v<<1 | bit
	}
	return v, true
}

func latin1(chunk []byte) string {
	var out strings.Builder
	for _, c := range chunk {
		if c == 0 {
			continue
		}
		out.WriteRune(rune(c))
	}
	return out.String()
}

func utf16be(chunk []byte) string {
	if len(chunk) < 2 {
		return ""
	}
	if len(chunk)%2 == 1 {
		chunk = chunk[:len(chunk)-1]
	}
	units := make([]uint16, len(chunk)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(chunk[i*2:])
	}
	if len(units) > 0 && units[0] == 0xFEFF {
		units = units[1:]
	}
	var out strings.Builder
	for _, r := range utf16.Decode(units) {
		if r != 0 {
			out.WriteRune(r)
		}
	}
	return out.String()
}
