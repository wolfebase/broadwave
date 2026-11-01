// Package dvbsi reads the DVB service information (ETSI EN 300 468) a DVB-T,
// T2, or C mux carries: the service names (SDT) and the listings (EIT).
package dvbsi

import (
	"encoding/binary"

	"broadwave/internal/psip"
)

const (
	pidSDT = 0x11
	pidEIT = 0x12
)

type section struct {
	PID  int
	Body []byte
}

type assembly struct {
	buf []byte
	cc  int
}

// sections reassembles the sections carried on the SDT and EIT PIDs. A
// section that fails its CRC, a length that cannot be a section, and a
// section broken by a lost packet are dropped.
func sections(raw []byte) []section {
	open := map[int]*assembly{}
	var out []section
	psip.Packets(nil, raw, func(pkt []byte) {
		if pkt[1]&0x80 != 0 {
			return
		}
		pid := int(pkt[1]&0x1F)<<8 | int(pkt[2])
		if pid != pidSDT && pid != pidEIT {
			return
		}
		afc := (pkt[3] >> 4) & 3
		if afc&1 == 0 {
			return
		}
		cc := int(pkt[3] & 0x0F)
		off := 4
		if afc&2 != 0 {
			off = 5 + int(pkt[4])
			if off >= 188 {
				return
			}
		}
		payload := pkt[off:]
		pusi := pkt[1]&0x40 != 0
		a := open[pid]
		if a != nil {
			switch {
			case cc == a.cc:
				// A repeated packet carries nothing new.
				return
			case cc != (a.cc+1)&0x0F:
				a.buf = nil
			}
			a.cc = cc
		}
		if pusi {
			pointer := int(payload[0])
			if 1+pointer > len(payload) {
				delete(open, pid)
				return
			}
			if a != nil && a.buf != nil {
				a.buf = append(a.buf, payload[1:1+pointer]...)
				out = appendDone(out, pid, a)
			}
			if a == nil {
				a = &assembly{cc: cc}
				open[pid] = a
			}
			a.buf = append(a.buf[:0:0], payload[1+pointer:]...)
			out = appendDone(out, pid, a)
			return
		}
		if a == nil || a.buf == nil {
			return
		}
		a.buf = append(a.buf, payload...)
		out = appendDone(out, pid, a)
	})
	return out
}

// appendDone moves every whole section at the front of a.buf to out. Stuffing
// (0xFF) after a section ends the packet's sections.
func appendDone(out []section, pid int, a *assembly) []section {
	for len(a.buf) >= 3 {
		if a.buf[0] == 0xFF {
			a.buf = nil
			return out
		}
		length := int(a.buf[1]&0x0F)<<8 | int(a.buf[2])
		total := 3 + length
		// A length that cannot be a section would pin every later packet
		// on this PID.
		if length < 5 || total > 4096 {
			a.buf = nil
			return out
		}
		if len(a.buf) < total {
			return out
		}
		body := append([]byte(nil), a.buf[:total]...)
		if crcOK(body) {
			out = append(out, section{PID: pid, Body: body})
		}
		a.buf = a.buf[total:]
	}
	if len(a.buf) == 0 {
		// The next section starts in a packet that says so.
		a.buf = nil
	}
	return out
}

func crcOK(section []byte) bool {
	if len(section) < 4 {
		return false
	}
	return CRC(section[:len(section)-4]) == binary.BigEndian.Uint32(section[len(section)-4:])
}

// CRC is the MPEG-2 CRC-32 (polynomial 0x04C11DB7, init 0xFFFFFFFF) that
// ends every SI section.
func CRC(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, by := range data {
		crc ^= uint32(by) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
