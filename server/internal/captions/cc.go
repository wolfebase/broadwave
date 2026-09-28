package captions

import "bytes"

var ga94 = []byte("GA94")

// fromMPEG2 finds ATSC A/53 caption data in picture user data
// (00 00 01 B2 "GA94" 03 cc_data) and returns its field-1 byte pairs.
func fromMPEG2(es []byte) []byte {
	var out []byte
	for {
		i := bytes.Index(es, []byte{0, 0, 1, 0xb2})
		if i < 0 {
			return out
		}
		es = es[i+4:]
		if len(es) > 5 && bytes.Equal(es[:4], ga94) && es[4] == 0x03 {
			out = ccData(es[5:], out)
		}
	}
}

// fromH264 finds the same caption data in SEI messages
// (user_data_registered_itu_t_t35, country 0xB5, provider 0x0031, "GA94").
func fromH264(es []byte) []byte {
	var out []byte
	for {
		i := bytes.Index(es, []byte{0, 0, 1})
		if i < 0 || i+3 >= len(es) {
			return out
		}
		es = es[i+3:]
		if es[0]&0x1f != 6 {
			continue
		}
		end := bytes.Index(es, []byte{0, 0, 1})
		if end < 0 {
			end = len(es)
		}
		out = seiCaptions(unescape(es[1:end]), out)
	}
}

func seiCaptions(b []byte, out []byte) []byte {
	for len(b) > 2 {
		kind, size := 0, 0
		for len(b) > 0 && b[0] == 0xff {
			kind += 255
			b = b[1:]
		}
		if len(b) == 0 {
			return out
		}
		kind += int(b[0])
		b = b[1:]
		for len(b) > 0 && b[0] == 0xff {
			size += 255
			b = b[1:]
		}
		if len(b) == 0 {
			return out
		}
		size += int(b[0])
		b = b[1:]
		if size > len(b) {
			return out
		}
		msg := b[:size]
		b = b[size:]
		if kind == 4 && len(msg) > 8 && msg[0] == 0xb5 && msg[1] == 0x00 && msg[2] == 0x31 && bytes.Equal(msg[3:7], ga94) && msg[7] == 0x03 {
			out = ccData(msg[8:], out)
		}
	}
	return out
}

// ccData reads cc_data() and keeps the valid NTSC field-1 pairs, parity stripped.
func ccData(b []byte, out []byte) []byte {
	if len(b) < 2 || b[0]&0x40 == 0 {
		return out
	}
	n := int(b[0] & 0x1f)
	b = b[2:]
	for i := 0; i < n && len(b) >= 3; i++ {
		valid := b[0]&0x04 != 0
		kind := b[0] & 0x03
		if valid && kind == 0 {
			out = append(out, b[1]&0x7f, b[2]&0x7f)
		}
		b = b[3:]
	}
	return out
}

// unescape drops H.264 emulation prevention bytes (00 00 03).
func unescape(b []byte) []byte {
	if !bytes.Contains(b, []byte{0, 0, 3}) {
		return b
	}
	out := make([]byte, 0, len(b))
	zeros := 0
	for _, c := range b {
		if zeros >= 2 && c == 3 {
			zeros = 0
			continue
		}
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out = append(out, c)
	}
	return out
}
