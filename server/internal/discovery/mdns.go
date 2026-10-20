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

// unescapeDNS undoes RFC 1035 master-file escapes: \X is X, and \DDD is the
// byte with that decimal value, so UTF-8 names arrive as several \DDD bytes.
// Bytes that do not make valid UTF-8 are replaced.
func unescapeDNS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if v, ok := decimalEscape(s[i+1:]); ok {
				b.WriteByte(v)
				i += 3
				continue
			}
			i++
		}
		b.WriteByte(s[i])
	}
	return strings.ToValidUTF8(b.String(), "\uFFFD")
}

// decimalEscape reads the DDD of a \DDD escape (000–255).
func decimalEscape(s string) (byte, bool) {
	if len(s) < 3 {
		return 0, false
	}
	n := 0
	for _, c := range s[:3] {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n > 255 {
		return 0, false
	}
	return byte(n), true
}
