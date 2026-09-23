package source

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	found, err := hdhr.Discover(4 * time.Second)
	if err != nil {
		return nil, err
	}
	var bases []string
	seen := map[string]bool{}
	for _, reply := range found {
		base := reply.BaseURL
		if base == "" && reply.Addr != "" {
			base = "http://" + reply.Addr
		}
		base = strings.TrimRight(base, "/")
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		bases = append(bases, base)
	}
	return bases, nil
}
