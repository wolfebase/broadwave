package hdhr

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net"
	"time"
)

const (
	discoverPort    = 65001
	typeDiscoverReq = 0x0002
	typeDiscoverRpy = 0x0003

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagBaseURL    = 0x2A
)

// Reply is a decoded HDHomeRun discovery datagram.
// Device auth tags are intentionally ignored and never returned.
type Reply struct {
	Addr       string
	DeviceID   string
	BaseURL    string
	TunerCount int
}

// DiscoverPacket builds a broadcast discovery request for tuner devices.
func DiscoverPacket() []byte {
	payload := []byte{
		tagDeviceType, 0x04, 0x00, 0x00, 0x00, 0x01,
		tagDeviceID, 0x04, 0xFF, 0xFF, 0xFF, 0xFF,
	}
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(body[0:2], typeDiscoverReq)
	binary.BigEndian.PutUint16(body[2:4], uint16(len(payload)))
	copy(body[4:], payload)
	out := make([]byte, len(body)+4)
	copy(out, body)
	binary.LittleEndian.PutUint32(out[len(body):], crc32.ChecksumIEEE(body))
	return out
}

// ParseReply decodes a discovery reply. Unknown tags are skipped.
func ParseReply(packet []byte, addr string) (Reply, error) {
	if len(packet) < 8 {
		return Reply{}, fmt.Errorf("discovery packet too short")
	}
	kind := binary.BigEndian.Uint16(packet[0:2])
	if kind != typeDiscoverRpy {
		return Reply{}, fmt.Errorf("unexpected packet type %04x", kind)
	}
	length := int(binary.BigEndian.Uint16(packet[2:4]))
	if len(packet) < 4+length+4 {
		return Reply{}, fmt.Errorf("discovery packet truncated")
	}
	body := packet[:4+length]
	want := binary.LittleEndian.Uint32(packet[4+length : 4+length+4])
	if crc32.ChecksumIEEE(body) != want {
		return Reply{}, fmt.Errorf("discovery crc mismatch")
	}
	reply := Reply{Addr: addr}
	payload := packet[4 : 4+length]
	for len(payload) > 0 {
		tag := payload[0]
		payload = payload[1:]
		n, rest, err := readVarLen(payload)
		if err != nil {
			return Reply{}, err
		}
		if len(rest) < n {
			return Reply{}, fmt.Errorf("tlv overruns packet")
		}
		val := rest[:n]
		payload = rest[n:]
		switch tag {
		case tagDeviceID:
			if len(val) == 4 {
				reply.DeviceID = fmt.Sprintf("%08X", binary.BigEndian.Uint32(val))
			}
		case tagBaseURL:
			reply.BaseURL = string(val)
		case tagTunerCount:
			if len(val) == 1 {
				reply.TunerCount = int(val[0])
			} else if len(val) == 4 {
				reply.TunerCount = int(binary.BigEndian.Uint32(val))
			}
		}
	}
	if reply.BaseURL == "" && reply.DeviceID == "" {
		return Reply{}, fmt.Errorf("discovery reply had no device")
	}
	return reply, nil
}

func readVarLen(b []byte) (int, []byte, error) {
	if len(b) == 0 {
		return 0, nil, fmt.Errorf("missing tlv length")
	}
	if b[0]&0x80 == 0 {
		return int(b[0]), b[1:], nil
	}
	if len(b) < 2 {
		return 0, nil, fmt.Errorf("truncated tlv length")
	}
	n := int(b[0]&0x7f) | int(b[1])<<7
	return n, b[2:], nil
}

// DiscoverHost asks one address, for a tuner that broadcast cannot reach.
func DiscoverHost(host string, timeout time.Duration) (Reply, error) {
	return discoverHostPort(host, fmt.Sprintf("%d", discoverPort), timeout)
}

func discoverHostPort(host, port string, timeout time.Duration) (Reply, error) {
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return Reply{}, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(DiscoverPacket()); err != nil {
		return Reply{}, err
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return Reply{}, err
	}
	return ParseReply(buf[:n], host)
}

// Discover broadcasts for HDHomeRun tuners and returns one reply per address.
func Discover(timeout time.Duration) ([]Reply, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("discovery socket is not udp")
	}
	packet := DiscoverPacket()
	for _, ip := range broadcastTargets() {
		dst, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(ip, fmt.Sprintf("%d", discoverPort)))
		if err != nil {
			continue
		}
		_, _ = udp.WriteToUDP(packet, dst)
	}
	deadline := time.Now().Add(timeout)
	_ = udp.SetReadDeadline(deadline)
	seen := map[string]Reply{}
	buf := make([]byte, 2048)
	for {
		n, addr, err := udp.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, err
		}
		reply, err := ParseReply(buf[:n], addr.String())
		if err != nil {
			continue
		}
		host, _, _ := net.SplitHostPort(addr.String())
		if host == "" {
			host = addr.String()
		}
		reply.Addr = host
		if prev, ok := seen[host]; ok && prev.BaseURL != "" {
			continue
		}
		seen[host] = reply
	}
	out := make([]Reply, 0, len(seen))
	for _, reply := range seen {
		out = append(out, reply)
	}
	return out, nil
}

func broadcastTargets() []string {
	targets := []string{"255.255.255.255"}
	ifaces, err := net.Interfaces()
	if err != nil {
		return targets
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			mask := ipnet.Mask
			if len(mask) == 16 {
				mask = mask[12:]
			}
			bcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bcast[i] = ip[i] | ^mask[i]
			}
			targets = append(targets, bcast.String())
		}
	}
	return targets
}
