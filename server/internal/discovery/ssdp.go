package discovery

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Found is one device or server seen on the network.
type Found struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Addr string `json:"addr"`
	ID   string `json:"id,omitempty"`
}

const ssdpSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 1\r\n" +
	"ST: upnp:rootdevice\r\n" +
	"\r\n"

// SearchSSDP sends one M-SEARCH. An empty dest uses the SSDP multicast address.
func SearchSSDP(ctx context.Context, dest string) ([]Found, error) {
	if dest == "" {
		dest = "239.255.255.250:1900"
	}
	if !strings.Contains(dest, ":") {
		dest += ":1900"
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("ssdp socket is not udp")
	}
	remote, err := net.ResolveUDPAddr("udp4", dest)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = udp.SetDeadline(deadline)
	if _, err := udp.WriteToUDP([]byte(ssdpSearch), remote); err != nil {
		return nil, err
	}
	seen := map[string]Found{}
	buf := make([]byte, 4096)
	for {
		n, addr, err := udp.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, err
		}
		found, ok := parseSSDP(string(buf[:n]), addr.String())
		if !ok {
			continue
		}
		seen[found.Addr+"|"+found.Kind] = found
	}
	out := make([]Found, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	return out, nil
}

func parseSSDP(raw, from string) (Found, bool) {
	lines := strings.Split(raw, "\n")
	header := map[string]string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		header[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
	}
	server := header["server"]
	st := header["st"]
	location := header["location"]
	kind := ""
	blob := strings.ToLower(server + " " + st + " " + location)
	switch {
	case strings.Contains(blob, "hdhomerun"):
		kind = "hdhomerun"
	case strings.Contains(blob, "channels"):
		kind = "channels-dvr"
	case strings.Contains(blob, "tvheadend"):
		kind = "tvheadend"
	default:
		return Found{}, false
	}
	addr := from
	if host, _, err := net.SplitHostPort(from); err == nil {
		addr = host
	}
	if location != "" {
		if u, err := url.Parse(location); err == nil && u.Hostname() != "" {
			addr = u.Hostname()
		}
	}
	name := server
	if name == "" {
		name = kind
	}
	return Found{Kind: kind, Name: name, Addr: addr, ID: header["usn"]}, true
}
