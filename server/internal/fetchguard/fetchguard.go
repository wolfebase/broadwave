// Package fetchguard is the HTTP client for addresses a person or a playlist
// asked the server to open. http and https are allowed. A tuner or a playlist
// server on the home network is allowed. Link-local addresses, cloud metadata
// addresses, and any other scheme are not.
package fetchguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrRefused is an address this server will not fetch.
// The error text does not include the URL. A playlist address can carry a password.
var ErrRefused = errors.New("this server does not fetch that address")

// FFmpegProtocols are the only protocols a remote input may use.
// file and concat stay off so a playlist cannot point the encoder at a local path.
const FFmpegProtocols = "http,https,tcp,tls,crypto"

// metadataNames are hostnames that answer cloud instance metadata.
var metadataNames = []string{
	"metadata.google.internal",
	"metadata.goog",
}

// metadataIPs are metadata addresses that are not link-local.
// 169.254.169.254 is link-local and is refused with the rest of that range.
var metadataIPs = []net.IP{
	net.ParseIP("fd00:ec2::254"),
	net.ParseIP("100.100.100.200"),
	net.ParseIP("192.0.0.192"),
}

var dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

// transport is shared. Proxy is off: an environment proxy would dial a
// different host and skip the address check.
var transport = &http.Transport{
	Proxy: nil,
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialAllowed(ctx, network, addr)
	},
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          32,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// Transport is the dialer that refuses a link-local or metadata address.
// Callers that already refuse redirects, such as the tuner client, use it.
func Transport() *http.Transport { return transport }

// Client fetches with Allowed, including after a redirect.
// A timeout of 0 waits for the request context. The header wait is still bounded.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: checkRedirect}
}

// Do sends req. A refused address, and a URL inside a transport error, are
// replaced so the error can be logged.
func Do(req *http.Request, timeout time.Duration) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, ErrRefused
	}
	if err := Allowed(req.URL.String()); err != nil {
		return nil, err
	}
	res, err := Client(timeout).Do(req)
	if err != nil {
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
		return nil, dropURL(err)
	}
	return res, nil
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return ErrRefused
	}
	return Allowed(req.URL.String())
}

// Allowed reports whether rawURL may be fetched.
// A private LAN address and loopback are allowed. A link-local address,
// a metadata address, and any scheme other than http or https are not.
func Allowed(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return ErrRefused
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return ErrRefused
	}
	return hostPolicy(u.Hostname())
}

func hostPolicy(host string) error {
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.Contains(host, "%") {
		return ErrRefused
	}
	if metadataName(host) || ambiguous(host) {
		return ErrRefused
	}
	if ip := net.ParseIP(host); ip != nil && ipBlocked(ip) {
		return ErrRefused
	}
	return nil
}

func metadataName(host string) bool {
	host = strings.ToLower(host)
	for _, name := range metadataNames {
		if host == name || strings.HasSuffix(host, "."+name) {
			return true
		}
	}
	return false
}

// ambiguous is a numeric host some clients treat as an address and Go does not,
// such as a decimal, hex, or octal form of a metadata address.
// A label written 0x is hex. ffmpeg and glibc read 169.254.169.0xfe as
// 169.254.169.254. A name with an ordinary letter, such as tv.local, is not.
func ambiguous(host string) bool {
	if strings.Contains(host, ":") {
		return false
	}
	low := strings.ToLower(host)
	if strings.HasPrefix(low, "0x") {
		return true
	}
	labels := strings.Split(host, ".")
	if len(labels) == 1 {
		_, err := strconv.ParseUint(host, 10, 64)
		return err == nil
	}
	hexLabel := false
	for _, label := range labels {
		if label == "" {
			return true
		}
		switch numericLabel(label) {
		case numDecimal:
			if len(label) > 1 && label[0] == '0' {
				return true
			}
		case numHex:
			hexLabel = true
		default:
			return false
		}
	}
	if hexLabel {
		return true
	}
	return len(labels) != 4
}

const (
	numOther = iota
	numDecimal
	numHex
)

// numericLabel reports whether one dotted label is decimal or 0x hex.
// 0x with no digits, and a word that merely contains 0x, are neither.
func numericLabel(label string) int {
	if label == "" {
		return numOther
	}
	low := strings.ToLower(label)
	if strings.HasPrefix(low, "0x") {
		rest := low[2:]
		if rest == "" {
			return numOther
		}
		for _, c := range rest {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return numOther
			}
		}
		return numHex
	}
	for _, c := range label {
		if c < '0' || c > '9' {
			return numOther
		}
	}
	return numDecimal
}

func ipBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, other := range metadataIPs {
		if ip.Equal(other) {
			return true
		}
	}
	return false
}

func dialAllowed(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := hostPolicy(host); err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, ErrRefused
	}
	for _, ip := range ips {
		if ipBlocked(ip.IP) {
			return nil, ErrRefused
		}
	}
	chosen := ips[0].IP
	family := "tcp4"
	if chosen.To4() == nil {
		family = "tcp6"
	}
	if network != "tcp" && network != family {
		return nil, ErrRefused
	}
	return dialer.DialContext(ctx, family, net.JoinHostPort(chosen.String(), port))
}

func dropURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		if errors.Is(uerr.Err, ErrRefused) {
			return ErrRefused
		}
		return uerr.Err
	}
	if errors.Is(err, ErrRefused) {
		return ErrRefused
	}
	return err
}

// OneLine removes characters that would start another header line.
func OneLine(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.TrimSpace(s)
}
