package source

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"broadwave/internal/discovery"
	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// Sync reads discover.json and lineup.json. It does not open a tuner stream.
// An empty ip broadcasts for devices. Otherwise ip is a host or host:port.
func Sync(ctx context.Context, st *store.Store, client *hdhr.Client, ip string) (int, error) {
	if client == nil {
		client = &hdhr.Client{}
	}
	bases, err := basesFor(ctx, ip)
	if err != nil {
		return 0, err
	}
	if len(bases) == 0 {
		if strings.TrimSpace(ip) != "" {
			return 0, fmt.Errorf("No HDHomeRun answered at that address. Add the port if it is not 80.")
		}
		return 0, nil
	}
	return writeDevices(ctx, st, client, bases, true)
}

// Maintain is the unattended pass. A catalog with a tuner only refreshes
// those tuners. An empty catalog adopts the first one that answers.
func Maintain(ctx context.Context, st *store.Store, client *hdhr.Client, ip string) (int, error) {
	if client == nil {
		client = &hdhr.Client{}
	}
	bases, err := basesFor(ctx, ip)
	if err != nil {
		return 0, err
	}
	if len(bases) == 0 {
		return 0, nil
	}
	return writeDevices(ctx, st, client, bases, false)
}

// Auto is what the process runs on its own. A configured address is added.
// A broadcast adopts a tuner only when the catalog has none.
func Auto(ctx context.Context, st *store.Store, client *hdhr.Client, ip string) (int, error) {
	if strings.TrimSpace(ip) != "" {
		return Sync(ctx, st, client, ip)
	}
	return Maintain(ctx, st, client, ip)
}

func writeDevices(ctx context.Context, st *store.Store, client *hdhr.Client, bases []string, addAll bool) (int, error) {
	devices, err := st.Devices(ctx)
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	tuners := 0
	for _, device := range devices {
		known[strings.ToUpper(device.DeviceID)] = true
		if device.TunerCount > 0 {
			tuners++
		}
	}
	tookFirst := false
	for _, base := range bases {
		dev, err := client.FetchDevice(ctx, base)
		if err != nil {
			if addAll {
				return 0, err
			}
			continue
		}
		id := strings.ToUpper(dev.DeviceID)
		// A box with no tuner is not the one a new install adopts.
		if !known[id] && !addAll && (dev.TunerCount <= 0 || tuners > 0 || tookFirst) {
			continue
		}
		channels, err := client.FetchLineup(ctx, dev.LineupURL)
		if err != nil {
			if addAll {
				return 0, err
			}
			continue
		}
		if err := st.UpsertDevice(ctx, dev, channels); err != nil {
			return 0, err
		}
		if !known[id] {
			tookFirst = true
			if dev.TunerCount > 0 {
				tuners++
			}
		}
		known[id] = true
	}
	devices, err = st.Devices(ctx)
	if err != nil {
		return 0, err
	}
	return len(devices), nil
}

func basesFor(ctx context.Context, ip string) ([]string, error) {
	ip = strings.TrimSpace(ip)
	if ip != "" {
		ip = strings.TrimPrefix(ip, "http://")
		ip = strings.TrimPrefix(ip, "https://")
		ip = strings.TrimRight(ip, "/")
		if strings.Contains(ip, "/") || strings.Contains(ip, " ") {
			return nil, fmt.Errorf("enter a host or host:port")
		}
		if _, _, err := net.SplitHostPort(ip); err == nil {
			return []string{"http://" + ip}, nil
		}
		return probeCompatible(ctx, ip)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ssdpCh := make(chan []discovery.Found, 1)
	go func() {
		extra, _ := discovery.SearchSSDP(ctx, "")
		ssdpCh <- extra
	}()
	found, err := hdhr.Discover(4 * time.Second)
	if err != nil {
		cancel()
		return nil, err
	}
	var bases []string
	seen := map[string]bool{}
	add := func(base string) {
		base = strings.TrimRight(base, "/")
		if base == "" || seen[base] {
			return
		}
		seen[base] = true
		bases = append(bases, base)
	}
	for _, base := range broadcastBases(found, discovery.LocalNets()) {
		add(base)
	}
	var extra []discovery.Found
	select {
	case extra = <-ssdpCh:
	default:
		cancel()
		extra = <-ssdpCh
	}
	nets := discovery.LocalNets()
	for _, item := range extra {
		if item.Kind != "hdhomerun" || !lanLiteral(item.Addr, nets) {
			continue
		}
		add("http://" + item.Addr)
	}
	if len(bases) == 0 {
		if hosts, err := net.LookupHost("hdhomerun.local"); err == nil {
			for _, host := range hosts {
				if lanLiteral(host, nets) {
					add("http://" + host)
				}
			}
		}
	}
	return bases, nil
}

// broadcastBases returns http://<sender> for replies on this LAN.
// The BaseURL inside the packet is not used.
func broadcastBases(replies []hdhr.Reply, nets []*net.IPNet) []string {
	var bases []string
	seen := map[string]bool{}
	for _, reply := range replies {
		host := reply.Addr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !lanLiteral(host, nets) {
			continue
		}
		base := "http://" + host
		if seen[base] {
			continue
		}
		seen[base] = true
		bases = append(bases, base)
	}
	return bases
}

func lanLiteral(host string, nets []*net.IPNet) bool {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return false
	}
	return ip.IsLoopback() || discovery.OnLAN(host, nets)
}

func probeCompatible(ctx context.Context, host string) ([]string, error) {
	candidates := []struct{ base string }{
		{"http://" + host},
		{"http://" + host + ":5004"},
		{"http://" + host + ":34400"},
		{"http://" + host + ":8409"},
		{"http://" + net.JoinHostPort(host, "9191") + "/hdhr"},
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var bases []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, cand := range candidates {
		wg.Add(1)
		go func(base string) {
			defer wg.Done()
			if compatible(ctx, base) {
				mu.Lock()
				bases = append(bases, base)
				mu.Unlock()
			}
		}(cand.base)
	}
	wg.Wait()
	return bases, nil
}

func compatible(ctx context.Context, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/discover.json", nil)
	if err != nil {
		return false
	}
	res, err := (&http.Client{Timeout: 700 * time.Millisecond}).Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return res.StatusCode == http.StatusOK && strings.Contains(string(body), "DeviceID")
}
