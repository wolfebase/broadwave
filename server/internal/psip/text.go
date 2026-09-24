package psip

// decodeText reads a multiple_string_structure. Compression types 1 and 2
// (Huffman) are left as-is until a capture needs them; uncompressed text is
// what the local broadcasts use.
func decodeText(b []byte) string {
	if len(b) < 1 {
		return ""
	}
	n := int(b[0])
	pos := 1
	var out string
	for i := 0; i < n && pos+4 <= len(b); i++ {
		pos += 3 // ISO-639
		segs := int(b[pos])
		pos++
		for s := 0; s < segs && pos+3 <= len(b); s++ {
			comp := b[pos]
			mode := b[pos+1]
			nbytes := int(b[pos+2])
			pos += 3
			if pos+nbytes > len(b) {
				return out
			}
			chunk := b[pos : pos+nbytes]
			pos += nbytes
			if comp != 0 || mode != 0 {
				continue
			}
			out += string(chunk)
		}
	}
	return out
}
