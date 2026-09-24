package discovery

import (
	"context"

	"github.com/libp2p/zeroconf/v2"
)

// Browse asks the local network for one DNS-SD service, such as _htsp._tcp.
// It returns whatever answered before ctx ends.
func Browse(ctx context.Context, service string) ([]Found, error) {
	entries := make(chan *zeroconf.ServiceEntry, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- zeroconf.Browse(ctx, service, "local.", entries)
	}()
	var out []Found
	for {
		select {
		case <-ctx.Done():
			return out, nil
		case err := <-errCh:
			return out, err
		case entry := <-entries:
			addr := ""
			if len(entry.AddrIPv4) > 0 {
				addr = entry.AddrIPv4[0].String()
			}
			out = append(out, Found{Kind: service, Name: entry.Instance, Addr: addr, ID: entry.HostName})
		}
	}
}
