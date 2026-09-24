package hdhr

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"strings"
	"time"
)

const (
	typeGetSetReq   = 0x0004
	typeGetSetRpy   = 0x0005
	tagGetSetName   = 0x03
	tagGetSetValue  = 0x04
	tagErrorMessage = 0x05
	tagLockKey      = 0x15
)

// Control talks to an HDHomeRun on TCP port 65001.
type Control struct {
	Addr string
}

func controlDial(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, "65001")
}

func (c Control) Get(name string) (string, error) {
	return c.exchange(name, "", 0, false)
}

func (c Control) Set(name, value string) (string, error) {
	return c.exchange(name, value, 0, true)
}

func (c Control) exchange(name, value string, lockKey uint32, set bool) (string, error) {
	conn, err := net.DialTimeout("tcp", controlDial(c.Addr), 3*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))

	payload := tlvString(tagGetSetName, name)
	if set {
		payload = append(payload, tlvString(tagGetSetValue, value)...)
	}
	if lockKey != 0 {
		payload = append(payload, tlvU32(tagLockKey, lockKey)...)
	}
	if _, err := conn.Write(seal(typeGetSetReq, payload)); err != nil {
		return "", err
	}
	packet, err := readPacket(conn)
	if err != nil {
		return "", err
	}
	kind := binary.BigEndian.Uint16(packet[0:2])
	if kind != typeGetSetRpy {
		return "", fmt.Errorf("control reply type %04x", kind)
	}
	length := int(binary.BigEndian.Uint16(packet[2:4]))
	body := packet[:4+length]
	got := binary.LittleEndian.Uint32(packet[4+length : 4+length+4])
	if crc32.ChecksumIEEE(body) != got {
		return "", fmt.Errorf("control crc mismatch")
	}
	var gotValue, errMsg string
	rest := packet[4 : 4+length]
	for len(rest) > 0 {
		tag := rest[0]
		rest = rest[1:]
		n, next, err := readVarLen(rest)
		if err != nil {
			return "", err
		}
		if len(next) < n {
			return "", fmt.Errorf("control tlv overrun")
		}
		val := next[:n]
		rest = next[n:]
		text := strings.TrimRight(string(val), "\x00")
		switch tag {
		case tagGetSetValue:
			gotValue = text
		case tagErrorMessage:
			errMsg = text
		}
	}
	if errMsg != "" {
		return "", fmt.Errorf("%s", errMsg)
	}
	return gotValue, nil
}

func tlvString(tag byte, value string) []byte {
	raw := append([]byte(value), 0)
	out := []byte{tag}
	out = append(out, varLen(len(raw))...)
	return append(out, raw...)
}

func tlvU32(tag byte, value uint32) []byte {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	out := []byte{tag}
	out = append(out, varLen(4)...)
	return append(out, raw[:]...)
}

func varLen(n int) []byte {
	if n <= 127 {
		return []byte{byte(n)}
	}
	return []byte{byte(n&0x7f) | 0x80, byte(n >> 7)}
}

func seal(kind uint16, payload []byte) []byte {
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(body[0:2], kind)
	binary.BigEndian.PutUint16(body[2:4], uint16(len(payload)))
	copy(body[4:], payload)
	out := make([]byte, len(body)+4)
	copy(out, body)
	binary.LittleEndian.PutUint32(out[len(body):], crc32.ChecksumIEEE(body))
	return out
}

func readPacket(r io.Reader) ([]byte, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(head[2:4]))
	rest := make([]byte, length+4)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}
	return append(head, rest...), nil
}
