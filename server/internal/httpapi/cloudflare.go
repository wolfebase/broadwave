package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/cloudflare"
	"broadwave/internal/store"
	"broadwave/internal/tunnel"
)

// cloudflareView is GET /remote/cloudflare: the sign-in, or the tunnel it made.
type cloudflareView struct {
	cloudflare.Status
	// Managed is a tunnel Broadwave made and runs.
	Managed bool          `json:"managed"`
	Tunnel  tunnel.Status `json:"tunnel"`
}

// UseCloudflare wires a sign-in manager to this server. main calls it once.
func (s *Server) UseCloudflare(m *cloudflare.Manager) {
	m.Host = cfHost{s}
	m.OnChange = s.RemoteChanged
	s.Cloudflare = m
}

func (s *Server) cf() *cloudflare.Manager {
	s.cfOnce.Do(func() {
		if s.Cloudflare == nil {
			s.UseCloudflare(cloudflare.NewManager(nil))
		}
	})
	return s.Cloudflare
}

func (s *Server) vault() *cloudflare.Vault {
	s.vaultOnce.Do(func() {
		if s.Vault == nil {
			s.Vault = &cloudflare.Vault{}
		}
	})
	return s.Vault
}

// RemoteChanged tells every app that Away from home changed. The event has
// no details; an admin reads them from /remote/cloudflare.
func (s *Server) RemoteChanged() {
	s.forgetRemote()
	if s.Bus != nil {
		s.Bus.Publish("remote.changed", nil)
	}
}

// origin is where a tunnel without routes in Cloudflare sends requests.
func (s *Server) origin() string {
	if s.Origin != "" {
		return s.Origin
	}
	return "http://127.0.0.1:8477"
}

// LoopbackOrigin turns the listen address into the address cloudflared on
// this machine uses to reach the server.
func LoopbackOrigin(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://127.0.0.1:8477"
	}
	if ip := net.ParseIP(host); host == "" || ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// homeOnly refuses a request from outside the house. Signing in hands this
// server a key to the owner's Cloudflare account; that starts at home.
func (s *Server) homeOnly(w http.ResponseWriter, r *http.Request) bool {
	if s.isRemote(r) {
		apiError(w, http.StatusForbidden, "not_home", "Set this up from the home network.", nil)
		return false
	}
	return true
}

func (s *Server) cloudflareView(ctx context.Context) (cloudflareView, error) {
	st := s.cf().Status()
	v := cloudflareView{Status: st, Tunnel: tunnel.Status{State: "off"}}
	if s.Tunnel != nil {
		v.Tunnel = s.Tunnel.Status()
	}
	saved, err := s.vault().Load()
	if err != nil {
		return v, err
	}
	if saved.Made() {
		v.Managed = true
		if st.State == cloudflare.StateIdle || st.State == cloudflare.StateError {
			v.State = cloudflare.StateConnected
		}
		if v.Hostname == "" {
			v.Hostname = saved.Hostname
		}
	}
	return v, nil
}

func (s *Server) writeCloudflare(w http.ResponseWriter, r *http.Request, status int) {
	v, err := s.cloudflareView(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, v)
}

func cloudflareError(w http.ResponseWriter, err error) {
	var p *cloudflare.Problem
	if !errors.As(err, &p) {
		writeError(w, err)
		return
	}
	status := http.StatusInternalServerError
	switch p.Code {
	case "bad_name", "bad_request":
		status = http.StatusBadRequest
	case "busy", "not_signed_in", "already_on", "wrong_account", "name_taken":
		status = http.StatusConflict
	case "remove_failed":
		status = http.StatusBadGateway
	}
	apiError(w, status, p.Code, p.Message, nil)
}

func (s *Server) cloudflareStatus(w http.ResponseWriter, r *http.Request) {
	if !s.homeOnly(w, r) {
		return
	}
	s.writeCloudflare(w, r, http.StatusOK)
}

func (s *Server) cloudflareLogin(w http.ResponseWriter, r *http.Request) {
	if !s.homeOnly(w, r) {
		return
	}
	var body struct {
		Purpose string `json:"purpose"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			httpError(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	if body.Purpose != cloudflare.PurposeRemove {
		saved, err := s.vault().Load()
		if err != nil {
			writeError(w, err)
			return
		}
		if saved.Made() {
			cloudflareError(w, &cloudflare.Problem{Code: "already_on", Message: "Away from home is already on. Turn it off first."})
			return
		}
	}
	if _, err := s.cf().StartLogin(body.Purpose); err != nil {
		cloudflareError(w, err)
		return
	}
	s.writeCloudflare(w, r, http.StatusOK)
}

func (s *Server) cloudflareCancel(w http.ResponseWriter, r *http.Request) {
	if !s.homeOnly(w, r) {
		return
	}
	s.cf().Cancel()
	s.writeCloudflare(w, r, http.StatusOK)
}

func (s *Server) cloudflareSetup(w http.ResponseWriter, r *http.Request) {
	if !s.homeOnly(w, r) {
		return
	}
	var body struct {
		Label     string `json:"label"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	saved, err := s.vault().Load()
	if err != nil {
		writeError(w, err)
		return
	}
	if saved.Made() {
		cloudflareError(w, &cloudflare.Problem{Code: "already_on", Message: "Away from home is already on. Turn it off first."})
		return
	}
	name, err := s.tunnelName(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.cf().Setup(cloudflare.Plan{Label: body.Label, Overwrite: body.Overwrite, TunnelName: name, Service: s.origin()}); err != nil {
		cloudflareError(w, err)
		return
	}
	s.writeCloudflare(w, r, http.StatusAccepted)
}

// tunnelName is broadwave- and the start of the server's id, so two servers
// in one account get two tunnels, and the same server finds its own again.
func (s *Server) tunnelName(ctx context.Context) (string, error) {
	id, err := s.Store.Identity(ctx, DefaultServerName())
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range strings.ToLower(id.ID) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
		if b.Len() == 6 {
			break
		}
	}
	return "broadwave-" + b.String(), nil
}

func (s *Server) cloudflareRemove(w http.ResponseWriter, r *http.Request) {
	if !s.homeOnly(w, r) {
		return
	}
	if r.URL.Query().Get("cloudflare") == "1" {
		if _, err := s.cf().Remove(r.Context()); err != nil {
			cloudflareError(w, err)
			return
		}
		s.writeCloudflare(w, r, http.StatusOK)
		return
	}
	saved, err := s.vault().Load()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := (cfHost{s}).Uninstall(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	left := ""
	if saved.Made() {
		left = saved.Hostname
	}
	s.cf().Forget(left)
	s.writeCloudflare(w, r, http.StatusOK)
}

// cfHost is the server side of setup.
type cfHost struct{ s *Server }

func (h cfHost) Install(ctx context.Context, sec cloudflare.Secret) error {
	if err := h.s.vault().Save(sec); err != nil {
		return err
	}
	if h.s.Store != nil {
		if err := h.s.Store.PutSettings(ctx, map[string]string{store.SettingRemoteURL: "https://" + sec.Hostname}); err != nil {
			return err
		}
	}
	h.s.applyTunnel(ctx)
	h.s.RemoteChanged()
	return nil
}

func (h cfHost) Connected(ctx context.Context) error {
	if h.s.Tunnel == nil {
		return nil
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		st := h.s.Tunnel.Status()
		switch st.State {
		case "connected":
			return nil
		case "missing", "off":
			return errors.New(st.Message)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (h cfHost) Installed() (cloudflare.Secret, error) { return h.s.vault().Load() }

func (h cfHost) Uninstall(ctx context.Context) error {
	saved, err := h.s.vault().Load()
	if err != nil {
		return err
	}
	if err := h.s.vault().Clear(); err != nil {
		return err
	}
	if saved.Hostname != "" && h.s.Store != nil {
		values, err := h.s.Store.Settings(ctx)
		if err == nil && values[store.SettingRemoteURL] == "https://"+saved.Hostname {
			if err := h.s.Store.PutSettings(ctx, map[string]string{store.SettingRemoteURL: ""}); err != nil {
				return err
			}
		}
	}
	h.s.applyTunnel(ctx)
	h.s.RemoteChanged()
	return nil
}

// savePastedToken keeps a token the owner pasted. An empty one stops the tunnel.
func (s *Server) savePastedToken(raw string) error {
	token, err := store.CleanTunnelToken(raw)
	if err != nil {
		return err
	}
	if token == "" {
		return s.vault().Clear()
	}
	return s.vault().Save(cloudflare.Secret{Token: token, Source: cloudflare.SourcePasted, CreatedAt: s.now().UTC().Truncate(time.Second)})
}

// migrateTunnelToken moves a token an older version kept in the catalog
// into the secrets file. It runs once at start, before the tunnel does. A
// token already in the secrets file wins.
func (s *Server) migrateTunnelToken(ctx context.Context) {
	if s.Store == nil {
		return
	}
	moved := false
	err := s.Store.TakeTunnelToken(ctx, func(token string) error {
		saved, err := s.vault().Load()
		if err != nil {
			return err
		}
		if saved.Token != "" {
			return nil
		}
		moved = true
		return s.vault().Save(cloudflare.Secret{Token: token, Source: cloudflare.SourcePasted, CreatedAt: s.now().UTC().Truncate(time.Second)})
	})
	if err != nil {
		slog.Error(fmt.Sprintf("tunnel: could not move the token out of the catalog: %v", err))
		return
	}
	if moved {
		slog.Info("tunnel: moved the tunnel token out of the catalog into the secrets folder")
	}
}

// dropCatalogToken removes a token a restored backup brought back. A restore
// never starts a tunnel; the owner pastes the token again or signs in.
func (s *Server) dropCatalogToken(ctx context.Context) {
	if s.Store == nil {
		return
	}
	found := false
	if err := s.Store.TakeTunnelToken(ctx, func(string) error { found = true; return nil }); err == nil && found {
		slog.Info("tunnel: the restored catalog had a tunnel token; it was not used. Paste it again, or sign in with Cloudflare.")
	}
}
