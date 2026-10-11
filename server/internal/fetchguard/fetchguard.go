// Package fetchguard is the HTTP client for addresses a person or a playlist
// asked the server to open. http and https are allowed. A tuner or a playlist
// server on the home network is allowed. Cloud metadata addresses and any
// other scheme are not. Link-local addresses are refused too, except on the
// tuner client: a tuner plugged straight into the server has one.
package fetchguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
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

// metadataIPs are cloud metadata and credential addresses. They are refused
// on every client, including the tuner client that allows link-local.
var metadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"),
	net.ParseIP("169.254.170.2"),
	net.ParseIP("169.254.170.23"),
	net.ParseIP("fd00:ec2::254"),
	net.ParseIP("fd00:ec2::23"),
	net.ParseIP("100.100.100.200"),
	net.ParseIP("192.0.0.192"),
}

// nat64 is the well-known NAT64 prefix. 64:ff9b::a9fe:a9fe reaches
// 169.254.169.254 on a network that translates it.
var nat64 = &net.IPNet{IP: net.ParseIP("64:ff9b::"), Mask: net.CIDRMask(96, 128)}

var dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

// policy is what one client refuses. The tuner client allows link-local.
type policy struct {
	linkLocal bool
}

var (
	strict = policy{}
	tuner  = policy{linkLocal: true}
)

// newTransport builds a shared transport for p. Proxy is off: an environment
// proxy would dial a different host and skip the address check. The header
// wait only catches a server that never answers; a large guide can be slow.
func newTransport(p policy) *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialAllowed(ctx, network, addr, p)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

var (
	transport      = newTransport(strict)
	tunerTransport = newTransport(tuner)
)

// Transport is the dialer that refuses a link-local or metadata address.
func Transport() *http.Transport { return transport }

// TunerTransport is the dialer for a tuner or another device on the home
// network. It allows link-local and still refuses metadata addresses.
// Callers that already refuse redirects, such as the tuner client, use it.
func TunerTransport() *http.Transport { return tunerTransport }

// Client fetches with Allowed, including after a redirect.
// A timeout of 0 waits for the request context. The header wait is still bounded.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: checkRedirect(strict)}
}

// TunerClient is Client for a tuner or another device on the home network.
func TunerClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: tunerTransport, CheckRedirect: checkRedirect(tuner)}
}

// Do sends req. A refused address, and a URL inside a transport error, are
// replaced so the error can be logged.
func Do(req *http.Request, timeout time.Duration) (*http.Response, error) {
	return do(req, strict, Client(timeout))
}

// TunerDo is Do for a tuner or another device on the home network.
func TunerDo(req *http.Request, timeout time.Duration) (*http.Response, error) {
	return do(req, tuner, TunerClient(timeout))
}

func do(req *http.Request, p policy, client *http.Client) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, ErrRefused
	}
	if err := p.allowed(req.URL.String()); err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
		return nil, dropURL(err)
	}
	return res, nil
}

func checkRedirect(p policy) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return ErrRefused
		}
		return p.allowed(req.URL.String())
	}
}

// Allowed reports whether rawURL may be fetched.
// A private LAN address and loopback are allowed. A link-local address,
// a metadata address, and any scheme other than http or https are not.
func Allowed(rawURL string) error {
	return strict.allowed(rawURL)
}

func (p policy) allowed(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return ErrRefused
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return ErrRefused
	}
	return p.host(u.Hostname())
}

// lookupTimeout bounds name resolution. A caller may hold a lock across Reachable.
const lookupTimeout = 2 * time.Second

// lookupIP resolves a hostname. Tests replace it so they do not dial.
var lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// Reachable reports whether rawURL may be opened by a program that resolves
// the name itself. Allowed runs first. A literal address is not looked up.
// A name is refused when any answer is blocked. A name this resolver cannot
// find is left to the program: a .local name may resolve there and not here.
// The URL is left unchanged so TLS still sees the name.
func Reachable(ctx context.Context, rawURL string) error {
	if err := Allowed(rawURL); err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ErrRefused
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if host == "" {
		return ErrRefused
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	ips, err := lookupIP(lookupCtx, host)
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	if err != nil || lookupCtx.Err() != nil {
		return nil
	}
	for _, ip := range ips {
		if strict.blocked(ip) {
			return ErrRefused
		}
	}
	return nil
}

func (p policy) host(host string) error {
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.Contains(host, "%") {
		return ErrRefused
	}
	if metadataName(host) || ambiguous(host) {
		return ErrRefused
	}
	if ip := net.ParseIP(host); ip != nil && p.blocked(ip) {
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
	labels := strings.Split(host, ".")
	if len(labels) == 1 {
		return numericLabel(host) != numOther
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

func (p policy) blocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if nat64.Contains(ip) {
		ip = net.IP(ip[12:16])
	}
	if ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() && !p.linkLocal {
		return true
	}
	for _, other := range metadataIPs {
		if ip.Equal(other) {
			return true
		}
	}
	return false
}

// dialAllowed checks every answer, then dials the checked addresses in order,
// so a name cannot change between the check and the dial. A host whose first
// answer is unreachable, such as IPv6 in a container without it, still works.
func dialAllowed(ctx context.Context, network, addr string, p policy) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := p.host(host); err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if p.blocked(ip.IP) {
			return nil, ErrRefused
		}
	}
	var last error = ErrRefused
	for _, ip := range ips {
		family := "tcp4"
		if ip.IP.To4() == nil {
			family = "tcp6"
		}
		if network != "tcp" && network != family {
			continue
		}
		conn, err := dialer.DialContext(ctx, family, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, last
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
