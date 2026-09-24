package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProbePort is one well-known port and the path that identifies it.
type ProbePort struct {
	Port int
	Path string
	Kind string
}

// KnownPorts are the servers Look Harder checks. Local subnets only.
var KnownPorts = []ProbePort{
	{80, "/discover.json", "hdhomerun"},
	{5004, "/discover.json", "hdhomerun"},
	{9981, "/playlist/channels.m3u", "tvheadend"},
	{34400, "/discover.json", "hdhr-compatible"},
	{8409, "/discover.json", "hdhr-compatible"},
	{9191, "/hdhr/discover.json", "hdhr-compatible"},
	{8089, "/devices/ANY/channels.m3u", "channels-dvr"},
	{5523, "/feeds/default/m3u", "fastchannels"},
	{7777, "/playlist.m3u", "pluto"},
	{8182, "/playlist.m3u8?regions=us", "samsung"},
	{8885, "/", "tablo"},
	{32400, "/identity", "plex"},
	{8096, "/System/Info/Public", "jellyfin"},
}

// ProbeHost asks one host on the given ports. An empty ports list uses KnownPorts.
func ProbeHost(ctx context.Context, host string, ports []ProbePort) (Found, bool) {
	if len(ports) == 0 {
		ports = KnownPorts
	}
	client := &http.Client{Timeout: 400 * time.Millisecond}
	for _, port := range ports {
		if ctx.Err() != nil {
			return Found{}, false
		}
		url := fmt.Sprintf("http://%s%s", net.JoinHostPort(host, fmt.Sprintf("%d", port.Port)), port.Path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode >= 400 {
			continue
		}
		found, ok := matchProbe(port, body)
		if !ok {
			continue
		}
		found.Addr = host
		return found, true
	}
	return Found{}, false
}

func matchProbe(port ProbePort, body []byte) (Found, bool) {
	switch port.Kind {
	case "hdhomerun", "hdhr-compatible":
		var doc struct {
			FriendlyName string `json:"FriendlyName"`
			DeviceID     string `json:"DeviceID"`
		}
		if json.Unmarshal(body, &doc) != nil || doc.DeviceID == "" {
			return Found{}, false
		}
		name := doc.FriendlyName
		if name == "" {
			name = "HDHomeRun"
		}
		return Found{Kind: port.Kind, Name: name, ID: doc.DeviceID}, true
	case "tvheadend", "channels-dvr", "fastchannels", "pluto", "samsung":
		if !bytes.Contains(body, []byte("#EXTM3U")) {
			return Found{}, false
		}
		name := port.Kind
		switch port.Kind {
		case "fastchannels":
			name = "FastChannels"
		case "pluto":
			name = "Pluto"
		case "samsung":
			name = "Samsung TV Plus"
		}
		return Found{Kind: port.Kind, Name: name}, true
	default:
		if !strings.Contains(strings.ToLower(string(body)), port.Kind) {
			return Found{}, false
		}
		return Found{Kind: port.Kind, Name: port.Kind}, true
	}
}

// Look probes hosts on KnownPorts. It stays on the addresses it is given.
func Look(ctx context.Context, hosts []string) []Found {
	if len(hosts) == 0 {
		return nil
	}
	jobs := make(chan string)
	out := make(chan Found)
	var wg sync.WaitGroup
	workers := 32
	if len(hosts) < workers {
		workers = len(hosts)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if found, ok := ProbeHost(ctx, host, nil); ok {
					out <- found
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, host := range hosts {
			select {
			case <-ctx.Done():
				return
			case jobs <- host:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	var found []Found
	for item := range out {
		found = append(found, item)
	}
	return found
}

// LocalHosts lists addresses on each local /24 through /30, skipping this machine.
func LocalHosts() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	mine := map[string]bool{}
	var nets []*net.IPNet
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
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
			mine[ipnet.IP.String()] = true
			n := *ipnet
			n.IP = ipnet.IP.Mask(ipnet.Mask)
			nets = append(nets, &n)
		}
	}
	seen := map[string]bool{}
	var hosts []string
	for _, n := range nets {
		for _, host := range HostsIn(n) {
			if mine[host] || seen[host] {
				continue
			}
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// HostsIn lists usable addresses in a network, skipping the network and broadcast.
func HostsIn(n *net.IPNet) []string {
	ip := n.IP.To4()
	if ip == nil {
		return nil
	}
	mask := net.IP(n.Mask).To4()
	if mask == nil {
		return nil
	}
	ones, bits := n.Mask.Size()
	if bits != 32 || ones < 24 || ones > 30 {
		return nil
	}
	base := binaryIP(ip) & binaryIP(mask)
	last := base | ^binaryIP(mask)
	var out []string
	for cur := base + 1; cur < last; cur++ {
		out = append(out, net.IP(fromBinary(cur)).String())
	}
	return out
}

func binaryIP(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func fromBinary(v uint32) net.IP {
	return net.IP{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}
