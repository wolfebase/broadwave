package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrNameTaken is a tunnel name already used in the account.
	ErrNameTaken = errors.New("a tunnel with that name already exists")
	// ErrHostTaken is a DNS name that already points somewhere else.
	ErrHostTaken = errors.New("that name is already in use")
)

// APIError is a failed Cloudflare API call.
type APIError struct {
	Status  int
	Codes   []int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("cloudflare answered %d", e.Status)
	}
	return fmt.Sprintf("cloudflare answered %d: %s", e.Status, e.Message)
}

// refused is a call the sign-in's token may not make, or a body this API
// version does not take.
func refused(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status >= 400 && ae.Status < 500 && ae.Status != http.StatusConflict && ae.Status != http.StatusTooManyRequests
}

// API is the part of Cloudflare's REST API that setup uses, with the token
// from the sign-in.
type API struct {
	Base  string
	Token string
	HTTP  *http.Client
}

// Tunnel is one cfd_tunnel.
type Tunnel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RemoteConfig bool   `json:"remote_config"`
}

type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (a *API) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (a *API) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Accept", "application/json;version=1")
	req.Header.Set("User-Agent", "Broadwave")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode/100 != 2 || !env.Success {
		ae := &APIError{Status: resp.StatusCode}
		var msgs []string
		for _, e := range env.Errors {
			ae.Codes = append(ae.Codes, e.Code)
			msgs = append(msgs, e.Message)
		}
		ae.Message = strings.Join(msgs, "; ")
		return ae
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// ZoneName looks up the domain the sign-in picked. The token may not be
// allowed to read zones; the caller then asks for the whole name.
func (a *API) ZoneName(ctx context.Context, zoneID string) (string, error) {
	var z struct {
		Name string `json:"name"`
	}
	if err := a.call(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID), nil, &z); err != nil {
		return "", err
	}
	return strings.ToLower(z.Name), nil
}

// CreateTunnel makes a tunnel. configSrc "cloudflare" keeps its routes in
// Cloudflare; "local" (or empty) leaves them to the cloudflared command line.
func (a *API) CreateTunnel(ctx context.Context, account, name string, secret []byte, configSrc string) (Tunnel, error) {
	body := map[string]any{"name": name, "tunnel_secret": secret}
	if configSrc != "" {
		body["config_src"] = configSrc
	}
	var t Tunnel
	err := a.call(ctx, http.MethodPost, "/accounts/"+url.PathEscape(account)+"/cfd_tunnel", body, &t)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusConflict {
		return Tunnel{}, ErrNameTaken
	}
	if err == nil && configSrc == "cloudflare" {
		t.RemoteConfig = true
	}
	return t, err
}

// FindTunnel returns the live tunnel with this name.
func (a *API) FindTunnel(ctx context.Context, account, name string) (Tunnel, bool, error) {
	q := url.Values{"name": {name}, "is_deleted": {"false"}}
	var list []Tunnel
	if err := a.call(ctx, http.MethodGet, "/accounts/"+url.PathEscape(account)+"/cfd_tunnel?"+q.Encode(), nil, &list); err != nil {
		return Tunnel{}, false, err
	}
	for _, t := range list {
		if t.Name == name {
			return t, true, nil
		}
	}
	return Tunnel{}, false, nil
}

// Configure sends a remotely managed tunnel its one route.
func (a *API) Configure(ctx context.Context, account, id, hostname, service string) error {
	body := map[string]any{"config": map[string]any{"ingress": []map[string]string{
		{"hostname": hostname, "service": service},
		{"service": "http_status:404"},
	}}}
	return a.call(ctx, http.MethodPut, "/accounts/"+url.PathEscape(account)+"/cfd_tunnel/"+url.PathEscape(id)+"/configurations", body, nil)
}

// RouteDNS points hostname at the tunnel with a CNAME, the same call as
// cloudflared tunnel route dns. It returns the name Cloudflare made.
func (a *API) RouteDNS(ctx context.Context, zone, id, hostname string, overwrite bool) (string, error) {
	body := map[string]any{"type": "dns", "user_hostname": hostname, "overwrite_existing": overwrite}
	var res struct {
		CName string `json:"cname"`
		Name  string `json:"name"`
	}
	err := a.call(ctx, http.MethodPut, "/zones/"+url.PathEscape(zone)+"/tunnels/"+url.PathEscape(id)+"/routes", body, &res)
	if err != nil {
		if hostTaken(err) {
			return "", ErrHostTaken
		}
		return "", err
	}
	if res.Name == "" {
		res.Name = hostname
	}
	return strings.ToLower(res.Name), nil
}

func hostTaken(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	if ae.Status == http.StatusConflict {
		return true
	}
	for _, c := range ae.Codes {
		// 1003: a record with that host already exists. 81053 and 81057: the
		// DNS API's own words for the same thing.
		if c == 1003 || c == 81053 || c == 81057 {
			return true
		}
	}
	return strings.Contains(strings.ToLower(ae.Message), "already exist")
}

// TunnelToken is the token cloudflared runs this one tunnel with.
func (a *API) TunnelToken(ctx context.Context, account, id string) (string, error) {
	var token string
	if err := a.call(ctx, http.MethodGet, "/accounts/"+url.PathEscape(account)+"/cfd_tunnel/"+url.PathEscape(id)+"/token", nil, &token); err != nil {
		return "", err
	}
	if token == "" {
		return "", errors.New("cloudflare sent an empty tunnel token")
	}
	return token, nil
}

// DeleteTunnel removes the tunnel and its connections. One already gone is fine.
func (a *API) DeleteTunnel(ctx context.Context, account, id string) error {
	err := a.call(ctx, http.MethodDelete, "/accounts/"+url.PathEscape(account)+"/cfd_tunnel/"+url.PathEscape(id)+"?cascade=true", nil, nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// DeleteRoute removes the CNAME that points hostname at the tunnel. The
// sign-in's token may not be allowed to; the caller then says the name stays.
func (a *API) DeleteRoute(ctx context.Context, zone, id, hostname string) error {
	q := url.Values{"type": {"CNAME"}, "name": {hostname}}
	var recs []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	if err := a.call(ctx, http.MethodGet, "/zones/"+url.PathEscape(zone)+"/dns_records?"+q.Encode(), nil, &recs); err != nil {
		return err
	}
	target := id + ".cfargotunnel.com"
	for _, rec := range recs {
		// A record someone pointed elsewhere is theirs.
		if !strings.EqualFold(strings.TrimSuffix(rec.Content, "."), target) {
			continue
		}
		if err := a.call(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zone)+"/dns_records/"+url.PathEscape(rec.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}
