package discovery

import (
	"context"
	"strings"

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
			out = append(out, Found{Kind: service, Name: friendlyName(entry), Addr: addr, ID: entry.HostName})
		}
	}
}

// friendlyName prefers a Chromecast fn= label and undoes DNS-SD escapes.
func friendlyName(entry *zeroconf.ServiceEntry) string {
	name := unescapeDNS(entry.Instance)
	for _, raw := range entry.Text {
		key, val, ok := strings.Cut(raw, "=")
		if ok && key == "fn" {
			if fn := unescapeDNS(val); fn != "" {
				name = fn
			}
		}
	}
	return name
}

func unescapeDNS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
