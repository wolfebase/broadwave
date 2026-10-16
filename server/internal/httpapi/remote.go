package httpapi

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"broadwave/internal/store"
	"broadwave/internal/tunnel"
)

// remoteTTL bounds how long a changed public address takes to apply when it
// was changed without this process, such as by restoring a backup.
const remoteTTL = 5 * time.Second

// Tunnel is the cloudflared runner. Tests leave it nil.
type Tunnel interface {
	Apply(token string)
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
// Cloudflare, or by the public name. It always needs a paired device. A
// device on the home network that adds the header only locks itself out.
func (s *Server) isRemote(r *http.Request) bool {
	if cloudflareRequest(r) {
		return true
	}
	host := store.RemoteHost(s.remoteURL(r.Context()))
	if host == "" {
		return false
	}
	name, _, _ := splitHost(r.Host)
	return name == host
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
	writeJSON(w, http.StatusOK, remoteStatus{
		RemoteURL:  s.remoteURL(r.Context()),
		Configured: values[store.SettingRemoteURL],
		TokenSet:   strings.TrimSpace(values[store.SettingTunnelToken]) != "",
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

// applyTunnel starts or stops cloudflared for the saved token.
func (s *Server) applyTunnel(ctx context.Context) {
	if s.Tunnel == nil || s.Store == nil {
		return
	}
	values, err := s.Store.Settings(ctx)
	if err != nil {
		return
	}
	s.Tunnel.Apply(strings.TrimSpace(values[store.SettingTunnelToken]))
	s.forgetRemote()
}

// StartTunnel runs cloudflared when a token is saved. main calls it once.
func (s *Server) StartTunnel(ctx context.Context) { s.applyTunnel(ctx) }

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
