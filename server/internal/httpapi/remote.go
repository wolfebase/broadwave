package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"broadwave/internal/store"
	"broadwave/internal/tunnel"
)

// remoteTTL bounds how long a changed public address takes to apply when it
// was changed without this process, such as by restoring a backup.
const remoteTTL = 5 * time.Second

// Tunnel is the cloudflared runner. Tests leave it nil.
type Tunnel interface {
	Apply(token, origin string)
	Status() tunnel.Status
}

// remoteURL is the address that reaches this server from outside: the
// setting, or else the hostname the running tunnel reported.
func (s *Server) remoteURL(ctx context.Context) string {
	s.remoteMu.Lock()
	if !s.remoteAt.IsZero() && s.now().Sub(s.remoteAt) < remoteTTL {
		v := s.remoteVal
		s.remoteMu.Unlock()
		return v
	}
	s.remoteMu.Unlock()
	v := ""
	if s.Store != nil {
		if values, err := s.Store.Settings(ctx); err == nil {
			v = values[store.SettingRemoteURL]
		}
	}
	if v == "" && s.Tunnel != nil {
		if local := s.Tunnel.Status().Local; local != "" {
			v = "https://" + local
		}
	}
	s.remoteMu.Lock()
	s.remoteVal, s.remoteAt = v, s.now()
	s.remoteMu.Unlock()
	return v
}

func (s *Server) forgetRemote() {
	s.remoteMu.Lock()
	s.remoteAt = time.Time{}
	s.remoteMu.Unlock()
}

// isRemote reports a request that came in from outside the house: through
// Cloudflare, by a public name, through a reverse proxy for a client that is
// not on the home network, or straight from such a client. It always needs a
// paired device. The headers can only make a request more restricted: a
// device on the home network that adds them only locks itself out.
func (s *Server) isRemote(r *http.Request) bool {
	if cloudflareRequest(r) || r.Header.Get("Tailscale-Funnel-Request") != "" || !homePeer(r) {
		return true
	}
	host := store.RemoteHost(s.remoteURL(r.Context()))
	name, _, _ := splitHost(r.Host)
	if host != "" && name == host {
		return true
	}
	tailnet := tailnetRequest(r)
	for _, h := range []string{r.Host, firstValue(r.Header.Get("X-Forwarded-Host"))} {
		if s.publicName(h) && !(tailnet && tailnetName(h)) {
			return true
		}
	}
	return forwardedOutside(r)
}

// tailnetIPv6 is the range Tailscale gives each device beside its 100.x one.
var tailnetIPv6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

func tailnetAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return sharedSpace.Contains(ip) || tailnetIPv6.Contains(ip)
}

// tailnetName reports a MagicDNS name. Only a device on the tailnet can
// resolve it, unless Funnel publishes it, and Funnel is refused above.
func tailnetName(hostport string) bool {
	name, _, ok := splitHost(strings.TrimSpace(hostport))
	return ok && strings.HasSuffix(name, ".ts.net")
}

// tailnetRequest reports a request from a device on the tailnet: straight
// from a tailnet address, or through Tailscale Serve on this machine, which
// connects from loopback and names the tailnet client in X-Forwarded-For.
// Every address it names has to be on the tailnet.
func tailnetRequest(r *http.Request) bool {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	clients := forwardedClients(r)
	ip := peer.Addr().Unmap()
	if !tailnetAddr(ip) && !(ip.IsLoopback() && len(clients) > 0) {
		return false
	}
	for _, raw := range clients {
		c, ok := parseForwardedAddr(raw)
		if !ok || !tailnetAddr(c) {
			return false
		}
	}
	return true
}

// publicName reports a Host that the internet can resolve, such as a name a
// proxy with a real certificate answers for. The secure name points only at
// home network addresses, so it stays home.
func (s *Server) publicName(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	name, _, ok := splitHost(hostport)
	if !ok {
		return true
	}
	if name == "" || localName(name) || s.directHost(name) {
		return false
	}
	for _, h := range testHosts {
		if name == h {
			return false
		}
	}
	return true
}

func firstValue(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return first
}

// forwardedOutside reports a proxied request whose client is not on the home
// network. Every address a proxy names counts, so a client cannot hide behind
// a private one it wrote itself; one that cannot be read counts as outside.
func forwardedOutside(r *http.Request) bool {
	for _, raw := range forwardedClients(r) {
		ip, ok := parseForwardedAddr(raw)
		if !ok || !homeAddr(ip) {
			return true
		}
	}
	return false
}

// forwardedClients lists the client addresses in Forwarded, X-Forwarded-For,
// and X-Real-IP, oldest hop first.
func forwardedClients(r *http.Request) []string {
	var out []string
	for _, line := range r.Header.Values("Forwarded") {
		for _, hop := range strings.Split(line, ",") {
			for _, pair := range strings.Split(hop, ";") {
				k, v, ok := strings.Cut(strings.TrimSpace(pair), "=")
				if ok && strings.EqualFold(strings.TrimSpace(k), "for") {
					out = append(out, v)
				}
			}
		}
	}
	for _, line := range r.Header.Values("X-Forwarded-For") {
		out = append(out, strings.Split(line, ",")...)
	}
	for _, line := range r.Header.Values("X-Real-Ip") {
		out = append(out, line)
	}
	return out
}

// parseForwardedAddr reads "1.2.3.4", "1.2.3.4:80", "[2001:db8::1]:80", or a
// quoted Forwarded value. "unknown" and obfuscated names do not parse.
func parseForwardedAddr(raw string) (netip.Addr, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), `"`)
	if ip, err := netip.ParseAddr(strings.Trim(raw, "[]")); err == nil {
		return ip.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(raw); err == nil {
		return ap.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}

// homePeer reports a connection from the home network, or from a proxy on it.
func homePeer(r *http.Request) bool {
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	return err == nil && homeAddr(addr.Addr().Unmap())
}

// sharedSpace is 100.64.0.0/10, which Tailscale hands out to a tailnet.
var sharedSpace = netip.MustParsePrefix("100.64.0.0/10")

// testPeers holds httptest's default client address, 192.0.2.1.
var testPeers []netip.Prefix

// homeAddr reports an address on the home network: private, loopback, link
// local, a tailnet, or an IPv6 address on one of this machine's own networks
// (a home's IPv6 addresses are public ones).
func homeAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || sharedSpace.Contains(ip) {
		return true
	}
	for _, p := range testPeers {
		if p.Contains(ip) {
			return true
		}
	}
	return ip.Is6() && localIPv6.has(ip)
}

// localIPv6 caches this machine's IPv6 networks. A router can hand out a new
// prefix at any time, so they are read again after a minute.
var localIPv6 localV6

type localV6 struct {
	mu       sync.Mutex
	at       time.Time
	prefixes []netip.Prefix
}

func (l *localV6) has(ip netip.Addr) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.at.IsZero() || time.Since(l.at) > time.Minute {
		l.prefixes, l.at = interfaceIPv6(), time.Now()
	}
	for _, p := range l.prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// interfaceIPv6 lists the global IPv6 networks of this machine, at most a /48
// wide so a misconfigured short prefix cannot take in the internet.
func interfaceIPv6() []netip.Prefix {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(n.IP)
		if !ok || !ip.Is6() || ip.Is4In6() || !ip.IsGlobalUnicast() {
			continue
		}
		bits, _ := n.Mask.Size()
		if bits < 48 {
			continue
		}
		out = append(out, netip.PrefixFrom(ip, bits).Masked())
	}
	return out
}
func cloudflareRequest(r *http.Request) bool {
	return r.Header.Get("Cf-Connecting-Ip") != "" || r.Header.Get("Cf-Ray") != ""
}

func loopbackPeer(r *http.Request) bool {
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	return err == nil && addr.Addr().Unmap().IsLoopback()
}

// trustedProxyPeer is a peer that may name the real client: cloudflared on
// this machine or on the home network. A public peer cannot.
func trustedProxyPeer(r *http.Request) bool {
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := addr.Addr().Unmap()
	return ip.IsLoopback() || ip.IsPrivate()
}

// tunnelClientIP is the address Cloudflare saw, when the request came through
// a tunnel on this network.
func tunnelClientIP(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip"))
	if raw == "" || !trustedProxyPeer(r) {
		return ""
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	return "cf:" + ip.Unmap().String()
}

// proxyClientIP is the client a reverse proxy on this network names: its
// X-Real-IP, or else the last X-Forwarded-For hop, which the proxy itself
// added. Pairing counts guesses by it, so strangers behind one proxy do not
// share a single limit.
func proxyClientIP(r *http.Request) string {
	if !trustedProxyPeer(r) {
		return ""
	}
	raw := strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if raw == "" {
		hops := r.Header.Values("X-Forwarded-For")
		if len(hops) == 0 {
			return ""
		}
		parts := strings.Split(hops[len(hops)-1], ",")
		raw = parts[len(parts)-1]
	}
	ip, ok := parseForwardedAddr(raw)
	if !ok {
		return ""
	}
	return "fwd:" + ip.String()
}

type remoteStatus struct {
	RemoteURL  string         `json:"remoteUrl"`
	Configured string         `json:"configured"`
	TokenSet   bool           `json:"tokenSet"`
	Tunnel     tunnel.Status  `json:"tunnel"`
	DeviceAuth bool           `json:"deviceAuth"`
	Invites    []store.Invite `json:"invites"`
	Here       bool           `json:"here"`
}

func (s *Server) remoteStatus(w http.ResponseWriter, r *http.Request) {
	values, err := s.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	invites, err := s.Store.Invites(r.Context(), s.now())
	if err != nil {
		writeError(w, err)
		return
	}
	st := tunnel.Status{State: "off"}
	if s.Tunnel != nil {
		st = s.Tunnel.Status()
	}
	saved, err := s.vault().Load()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, remoteStatus{
		RemoteURL:  s.remoteURL(r.Context()),
		Configured: values[store.SettingRemoteURL],
		TokenSet:   saved.Token != "" && !saved.Made(),
		Tunnel:     st,
		DeviceAuth: values[store.SettingDeviceAuth] == "1",
		Invites:    invites,
		Here:       s.isRemote(r),
	})
}

type inviteBody struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	var body inviteBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	inv, code, err := s.Store.CreateInvite(r.Context(), body.Name, body.Scopes, s.now())
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := map[string]any{"invite": inv, "code": code}
	if base := s.remoteURL(r.Context()); base != "" {
		out["link"] = base + "/pair?invite=" + url.QueryEscape(code)
		out["appLink"] = "broadwave://join?server=" + url.QueryEscape(base) + "&code=" + url.QueryEscape(code)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.Store.Invites(r.Context(), s.now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": invites})
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteInvite(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	s.listInvites(w, r)
}

// applyTunnel starts or stops cloudflared for the token in the secrets file.
// A token in the catalog came back with a restored backup and is not used.
func (s *Server) applyTunnel(ctx context.Context) {
	s.dropCatalogToken(ctx)
	if s.Tunnel == nil {
		return
	}
	saved, err := s.vault().Load()
	if err != nil {
		slog.Error(fmt.Sprintf("tunnel: %v", err))
		return
	}
	origin := ""
	if saved.Made() && saved.Routes != "cloudflare" {
		origin = s.origin()
	}
	s.Tunnel.Apply(saved.Token, origin)
	s.forgetRemote()
}

// StartTunnel moves an older catalog's token into the secrets file, then runs
// cloudflared when a token is saved. main calls it once.
func (s *Server) StartTunnel(ctx context.Context) {
	s.migrateTunnelToken(ctx)
	s.applyTunnel(ctx)
}

// privateCache keeps a CDN from storing a response that answered a signed-in
// request from outside. A zone rule such as Cache Everything would otherwise
// serve it to anyone who asks for the same URL.
type privateCache struct{ http.ResponseWriter }

func (p privateCache) WriteHeader(code int) {
	h := p.Header()
	cc := h.Get("Cache-Control")
	switch {
	case cc == "":
		h.Set("Cache-Control", "private")
	case strings.Contains(cc, "public"):
		h.Set("Cache-Control", strings.ReplaceAll(cc, "public", "private"))
	case !strings.Contains(cc, "private") && !strings.Contains(cc, "no-store"):
		h.Set("Cache-Control", "private, "+cc)
	}
	p.ResponseWriter.WriteHeader(code)
}

func (p privateCache) Write(b []byte) (int, error) {
	if p.Header().Get("Cache-Control") == "" {
		p.Header().Set("Cache-Control", "private")
	}
	return p.ResponseWriter.Write(b)
}

func (p privateCache) Flush() {
	if f, ok := p.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (p privateCache) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := p.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

func (p privateCache) Unwrap() http.ResponseWriter { return p.ResponseWriter }

type outsideKey struct{}

func withOutside(ctx context.Context) context.Context {
	return context.WithValue(ctx, outsideKey{}, true)
}

// outside reports a request that came in through the tunnel or the public name.
func outside(ctx context.Context) bool {
	v, _ := ctx.Value(outsideKey{}).(bool)
	return v
}
