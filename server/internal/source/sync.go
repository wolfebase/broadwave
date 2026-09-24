package source

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"waveguide/internal/discovery"
	"waveguide/internal/hdhr"
	"waveguide/internal/store"
)

// Sync reads discover.json and lineup.json. It does not open a tuner stream.
// An empty ip broadcasts for devices. Otherwise ip is a host or host:port.
func Sync(ctx context.Context, st *store.Store, client *hdhr.Client, ip string) (int, error) {
	if client == nil {
		client = &hdhr.Client{}
	}
	bases, err := basesFor(ip)
	if err != nil {
		return 0, err
	}
	if len(bases) == 0 {
		return 0, nil
	}
	for _, base := range bases {
		dev, err := client.FetchDevice(ctx, base)
		if err != nil {
			return 0, err
		}
		channels, err := client.FetchLineup(ctx, dev.LineupURL)
		if err != nil {
			return 0, err
		}
		if err := st.UpsertDevice(ctx, dev, channels); err != nil {
			return 0, err
		}
	}
	devices, err := st.Devices(ctx)
	if err != nil {
		return 0, err
	}
	return len(devices), nil
}

func basesFor(ip string) ([]string, error) {
	ip = strings.TrimSpace(ip)
	if ip != "" {
		ip = strings.TrimPrefix(ip, "http://")
		ip = strings.TrimPrefix(ip, "https://")
		ip = strings.TrimRight(ip, "/")
		if strings.Contains(ip, "/") || strings.Contains(ip, " ") {
			return nil, fmt.Errorf("enter a host or host:port")
		}
		return []string{"http://" + ip}, nil
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
	for _, reply := range found {
		base := reply.BaseURL
		if base == "" && reply.Addr != "" {
			base = "http://" + reply.Addr
		}
		add(base)
	}
	var extra []discovery.Found
	select {
	case extra = <-ssdpCh:
	default:
		cancel()
		extra = <-ssdpCh
	}
	for _, item := range extra {
		if item.Kind == "hdhomerun" && item.Addr != "" {
			add("http://" + item.Addr)
		}
	}
	if len(bases) == 0 {
		if hosts, err := net.LookupHost("hdhomerun.local"); err == nil {
			for _, host := range hosts {
				add("http://" + host)
			}
		}
	}
	return bases, nil
}
