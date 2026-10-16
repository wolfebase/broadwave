package cloudflare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// States a sign-in moves through. Connected is reported by the server once
// a tunnel Broadwave made is saved.
const (
	StateIdle         = "idle"
	StateWaiting      = "waiting"
	StateAuthorized   = "authorized"
	StateProvisioning = "provisioning"
	StateConnected    = "connected"
	StateError        = "error"
)

// Steps while provisioning.
const (
	StepTunnel  = "tunnel"
	StepDNS     = "dns"
	StepConnect = "connect"
)

// Purposes of a sign-in.
const (
	PurposeSetup  = "setup"
	PurposeRemove = "remove"
)

// Zone is the domain the sign-in picked. Name is empty when the sign-in may
// not read it.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Problem is a failure with a code the app words for itself.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (p *Problem) Error() string { return p.Message }

func problem(code, message string) *Problem { return &Problem{Code: code, Message: message} }

// Status is what Settings shows.
type Status struct {
	State     string    `json:"state"`
	Purpose   string    `json:"purpose,omitempty"`
	LoginURL  string    `json:"loginUrl,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
	Zone      *Zone     `json:"zone,omitempty"`
	Hostname  string    `json:"hostname,omitempty"`
	Step      string    `json:"step,omitempty"`
	Error     *Problem  `json:"error,omitempty"`
	// DNSLeft is a name that may still point at a removed tunnel.
	DNSLeft string `json:"dnsLeft,omitempty"`
}

// Host is the server side of setup: where the result is kept and run.
type Host interface {
	// Install keeps what setup made and starts the tunnel.
	Install(ctx context.Context, s Secret) error
	// Connected returns once the tunnel connects, or with ctx's error.
	Connected(ctx context.Context) error
	// Installed is the saved secret, or the zero Secret.
	Installed() (Secret, error)
	// Uninstall stops the tunnel and forgets the secret.
	Uninstall(ctx context.Context) error
}

// Plan is what setup makes.
type Plan struct {
	// Label is the first part of the name, such as tv. When the sign-in may
	// not read the domain's name, it is the whole name.
	Label     string
	Overwrite bool
	// TunnelName is the tunnel's name in the account.
	TunnelName string
	// Service is where the tunnel sends requests, such as http://127.0.0.1:8477.
	Service string
}

// Manager runs one sign-in at a time. The certificate stays in memory and is
// dropped when setup finishes, the owner cancels, or it sits unused.
type Manager struct {
	Endpoints Endpoints
	HTTP      *http.Client
	Host      Host
	// OnChange runs after every state change, outside the lock.
	OnChange func()
	Now      func() time.Time
	// LoginTTL bounds the wait for the owner to authorize. Zero is 9 minutes.
	LoginTTL time.Duration
	// HoldTTL bounds how long an unused certificate is kept. Zero is 15 minutes.
	HoldTTL time.Duration
	// Retry is the pause after a network error while waiting. Zero is 3 seconds.
	Retry time.Duration
	// ConnectWait bounds the Connecting step. Zero is 45 seconds.
	ConnectWait time.Duration

	mu     sync.Mutex
	st     Status
	cert   *Cert
	gen    int
	cancel context.CancelFunc
}

// NewManager uses Cloudflare's own addresses unless the test variables are set.
func NewManager(host Host) *Manager {
	return &Manager{Endpoints: DefaultEndpoints(), Host: host}
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return http.DefaultClient
}

func (m *Manager) api(token string) *API {
	return &API{Base: m.Endpoints.API, Token: token, HTTP: m.HTTP}
}

func (m *Manager) changed() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

// Status returns a copy of the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.st
	if st.State == "" {
		st.State = StateIdle
	}
	if st.Zone != nil {
		z := *st.Zone
		st.Zone = &z
	}
	if st.Error != nil {
		e := *st.Error
		st.Error = &e
	}
	return st
}

// stopLocked ends whatever runs now and drops the certificate.
func (m *Manager) stopLocked() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.cert = nil
	m.gen++
}

// StartLogin makes a new sign-in link and waits in the background for the
// owner to authorize it.
func (m *Manager) StartLogin(purpose string) (Status, error) {
	if purpose == "" {
		purpose = PurposeSetup
	}
	if purpose != PurposeSetup && purpose != PurposeRemove {
		return Status{}, problem("bad_request", "purpose is setup or remove")
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return Status{}, err
	}
	key := base64.URLEncoding.EncodeToString(buf)
	q := url.Values{}
	q.Set("aud", "")
	q.Set("callback", m.Endpoints.Login+"/"+key)
	m.mu.Lock()
	if m.st.State == StateProvisioning {
		m.mu.Unlock()
		return Status{}, problem("busy", "Setup is already running.")
	}
	m.stopLocked()
	gen := m.gen
	expires := m.now().Add(orDefault(m.LoginTTL, 9*time.Minute)).UTC().Truncate(time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	m.cancel = cancel
	m.st = Status{
		State:     StateWaiting,
		Purpose:   purpose,
		LoginURL:  m.Endpoints.Dash + "/argotunnel?" + q.Encode(),
		ExpiresAt: expires,
	}
	m.mu.Unlock()
	go m.wait(ctx, gen, key, purpose)
	m.changed()
	return m.Status(), nil
}

// Cancel stops a sign-in, drops the certificate, and clears an error.
func (m *Manager) Cancel() {
	m.mu.Lock()
	if m.st.State == StateProvisioning {
		// Setup finishes or fails on its own; stopping halfway would leave a
		// tunnel with no name, or a name with no tunnel.
		m.mu.Unlock()
		return
	}
	m.stopLocked()
	m.st = Status{State: StateIdle}
	m.mu.Unlock()
	m.changed()
}

// Forget clears the state after the tunnel was turned off here, and names a
// DNS entry that may remain.
func (m *Manager) Forget(dnsLeft string) {
	m.mu.Lock()
	m.stopLocked()
	m.st = Status{State: StateIdle, DNSLeft: dnsLeft}
	m.mu.Unlock()
	m.changed()
}

// fail ends the sign-in with a problem, if gen is still the current one.
func (m *Manager) fail(gen int, p *Problem) {
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return
	}
	m.stopLocked()
	m.st = Status{State: StateError, Error: p}
	m.mu.Unlock()
	m.changed()
}

var errNotYet = errors.New("not authorized yet")

// fetch asks once for the certificate. The login service holds the request
// for about a minute, then answers 404 until the owner authorizes.
func (m *Manager) fetch(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Endpoints.Login+"/"+key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Broadwave")
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return body, nil
	case resp.StatusCode >= 500:
		return nil, problem("login_failed", "Cloudflare's sign-in stopped answering. Try again in a minute.")
	default:
		return nil, errNotYet
	}
}

func (m *Manager) wait(ctx context.Context, gen int, key, purpose string) {
	var raw []byte
	for {
		got, err := m.fetch(ctx, key)
		if err == nil {
			raw = got
			break
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				m.fail(gen, problem("timeout", "Sign-in timed out. Try again."))
			}
			return
		}
		var p *Problem
		if errors.As(err, &p) {
			m.fail(gen, p)
			return
		}
		if errors.Is(err, errNotYet) {
			continue
		}
		select {
		case <-ctx.Done():
			continue
		case <-time.After(orDefault(m.Retry, 3*time.Second)):
		}
	}
	cert, err := DecodeCert(raw)
	if err != nil {
		m.fail(gen, problem("bad_cert", "Cloudflare's answer was not a sign-in Broadwave can use. Try again."))
		return
	}
	if cert.ZoneID == "" {
		m.fail(gen, problem("no_zone", "That Cloudflare account has no domain yet. Add one to Cloudflare, or use Tailscale."))
		return
	}
	zone := &Zone{ID: cert.ZoneID}
	lookup, cancel := context.WithTimeout(ctx, 15*time.Second)
	name, err := m.api(cert.APIToken).ZoneName(lookup, cert.ZoneID)
	cancel()
	if err == nil {
		zone.Name = name
	} else {
		slog.Info(fmt.Sprintf("cloudflare: the sign-in may not read its domain's name: %v", err))
	}
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	hold := m.now().Add(orDefault(m.HoldTTL, 15*time.Minute)).UTC().Truncate(time.Second)
	holdCtx, stop := context.WithDeadline(context.Background(), hold)
	m.cancel = stop
	m.cert = &cert
	m.st = Status{State: StateAuthorized, Purpose: purpose, Zone: zone, ExpiresAt: hold}
	m.mu.Unlock()
	m.changed()
	go m.holdFor(holdCtx, gen)
	if purpose == PurposeRemove {
		if _, err := m.Remove(context.Background()); err != nil {
			slog.Warn(fmt.Sprintf("cloudflare: remove: %v", err))
		}
	}
}

// holdFor drops an unused certificate when its time is up.
func (m *Manager) holdFor(ctx context.Context, gen int) {
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		m.mu.Lock()
		current := gen == m.gen && m.st.State == StateAuthorized
		m.mu.Unlock()
		if current {
			m.fail(gen, problem("timeout", "Sign-in timed out. Try again."))
		}
	}
}

// Hostname turns what the owner typed into the name setup makes. With the
// domain known, one label goes in front of it. Without, the whole name is
// needed.
func Hostname(label, zone string) (string, error) {
	label = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(label), ".")))
	bad := problem("bad_name", "Use letters, numbers, and dashes, such as tv.")
	if zone != "" {
		label = strings.TrimSuffix(label, "."+zone)
		if label == "" || strings.Contains(label, ".") || !validLabel(label) {
			return "", bad
		}
		return label + "." + zone, nil
	}
	parts := strings.Split(label, ".")
	if len(parts) < 3 || len(label) > 253 {
		return "", problem("bad_name", "Type the whole name, such as tv.example.com.")
	}
	for _, p := range parts {
		if !validLabel(p) {
			return "", bad
		}
	}
	return label, nil
}

func validLabel(s string) bool {
	if s == "" || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Setup makes the tunnel and its name in the background. The certificate
// is dropped once the tunnel's own token is saved.
func (m *Manager) Setup(plan Plan) (Status, error) {
	m.mu.Lock()
	if m.st.State == StateProvisioning {
		m.mu.Unlock()
		return Status{}, problem("busy", "Setup is already running.")
	}
	if m.cert == nil || m.st.State != StateAuthorized {
		m.mu.Unlock()
		return Status{}, problem("not_signed_in", "Sign in with Cloudflare first.")
	}
	zone := Zone{ID: m.cert.ZoneID}
	if m.st.Zone != nil {
		zone = *m.st.Zone
	}
	host, err := Hostname(plan.Label, zone.Name)
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	cert := *m.cert
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	// Setup holds the only copy now; a failure hands it back.
	m.cert = nil
	m.gen++
	gen := m.gen
	m.st = Status{State: StateProvisioning, Purpose: PurposeSetup, Zone: &zone, Hostname: host, Step: StepTunnel}
	m.mu.Unlock()
	m.changed()
	go m.provision(gen, cert, host, plan)
	return m.Status(), nil
}

func (m *Manager) step(gen int, step string) {
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return
	}
	m.st.Step = step
	m.mu.Unlock()
	m.changed()
}

// setupFailed goes back to signed in, with the certificate kept so the owner
// can pick another name or replace the one in use.
func (m *Manager) setupFailed(gen int, cert Cert, p *Problem) {
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return
	}
	hold := m.now().Add(orDefault(m.HoldTTL, 15*time.Minute)).UTC().Truncate(time.Second)
	ctx, stop := context.WithDeadline(context.Background(), hold)
	m.cancel = stop
	m.cert = &cert
	host := m.st.Hostname
	m.st = Status{State: StateAuthorized, Purpose: PurposeSetup, Zone: m.st.Zone, Hostname: host, ExpiresAt: hold, Error: p}
	m.mu.Unlock()
	m.changed()
	go m.holdFor(ctx, gen)
}

func (m *Manager) provision(gen int, cert Cert, host string, plan Plan) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	api := m.api(cert.APIToken)
	t, err := m.makeTunnel(ctx, api, cert, plan.TunnelName, host, plan.Service)
	if err != nil {
		m.setupFailed(gen, cert, setupProblem(err))
		return
	}
	m.step(gen, StepDNS)
	name, err := api.RouteDNS(ctx, cert.ZoneID, t.ID, host, plan.Overwrite)
	if errors.Is(err, ErrHostTaken) {
		m.setupFailed(gen, cert, problem("name_taken", host+" is already in use. Pick another name, or replace it."))
		return
	}
	if err != nil {
		m.setupFailed(gen, cert, setupProblem(err))
		return
	}
	if name != host && t.RemoteConfig {
		// Cloudflare named it differently; the route must match.
		if err := api.Configure(ctx, cert.AccountID, t.ID, name, plan.Service); err != nil {
			m.setupFailed(gen, cert, setupProblem(err))
			return
		}
	}
	m.step(gen, StepConnect)
	token, err := api.TunnelToken(ctx, cert.AccountID, t.ID)
	if err != nil {
		m.setupFailed(gen, cert, setupProblem(err))
		return
	}
	routes := "local"
	if t.RemoteConfig {
		routes = "cloudflare"
	}
	secret := Secret{
		Token:     token,
		Source:    SourceBroadwave,
		Routes:    routes,
		TunnelID:  t.ID,
		AccountID: cert.AccountID,
		ZoneID:    cert.ZoneID,
		Hostname:  name,
		CreatedAt: m.now().UTC().Truncate(time.Second),
	}
	if err := m.Host.Install(ctx, secret); err != nil {
		m.setupFailed(gen, cert, problem("setup_failed", "The tunnel is made, but this server could not save it: "+err.Error()))
		return
	}
	// The certificate controls every tunnel in the account. From here on
	// only this tunnel's token is kept; this copy is the last and goes when
	// provision returns.
	m.mu.Lock()
	if gen == m.gen {
		m.st.Hostname = name
	}
	m.mu.Unlock()
	m.changed()
	wait, stop := context.WithTimeout(context.Background(), orDefault(m.ConnectWait, 45*time.Second))
	_ = m.Host.Connected(wait)
	stop()
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return
	}
	m.st = Status{State: StateIdle}
	m.mu.Unlock()
	slog.Info("cloudflare: away from home is set up at https://" + name)
	m.changed()
}

// makeTunnel makes the tunnel with its route kept in Cloudflare when the
// sign-in allows that, and else one that cloudflared routes with --url. A
// tunnel this server made before is used again.
func (m *Manager) makeTunnel(ctx context.Context, api *API, cert Cert, name, host, service string) (Tunnel, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Tunnel{}, err
	}
	t, err := api.CreateTunnel(ctx, cert.AccountID, name, secret, "cloudflare")
	if refused(err) {
		t, err = api.CreateTunnel(ctx, cert.AccountID, name, secret, "")
	}
	if errors.Is(err, ErrNameTaken) {
		found, ok, ferr := api.FindTunnel(ctx, cert.AccountID, name)
		if ferr != nil {
			return Tunnel{}, ferr
		}
		if !ok {
			return Tunnel{}, err
		}
		t, err = found, nil
	}
	if err != nil {
		return Tunnel{}, err
	}
	if !t.RemoteConfig {
		return t, nil
	}
	cerr := api.Configure(ctx, cert.AccountID, t.ID, host, service)
	if cerr == nil {
		return t, nil
	}
	if !refused(cerr) {
		return Tunnel{}, cerr
	}
	// The sign-in may make tunnels but not route them in Cloudflare. Make
	// one cloudflared routes instead.
	slog.Info(fmt.Sprintf("cloudflare: routes stay on this server: %v", cerr))
	if err := api.DeleteTunnel(ctx, cert.AccountID, t.ID); err != nil {
		return Tunnel{}, err
	}
	t, err = api.CreateTunnel(ctx, cert.AccountID, name, secret, "local")
	if refused(err) {
		t, err = api.CreateTunnel(ctx, cert.AccountID, name, secret, "")
	}
	t.RemoteConfig = false
	return t, err
}

func setupProblem(err error) *Problem {
	var p *Problem
	if errors.As(err, &p) {
		return p
	}
	var ae *APIError
	if errors.As(err, &ae) && (ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden) {
		return problem("not_permitted", "Cloudflare did not let this sign-in make a tunnel. Sign in with the account that owns the domain.")
	}
	if errors.Is(err, ErrNameTaken) {
		return problem("setup_failed", "Cloudflare has a tunnel with this server's name that Broadwave can't use. Remove it in the dashboard, then try again.")
	}
	return problem("setup_failed", "Cloudflare did not finish: "+err.Error())
}

// Remove stops the tunnel here, then deletes it in Cloudflare with the
// fresh sign-in. It returns a DNS name that may remain.
func (m *Manager) Remove(ctx context.Context) (string, error) {
	m.mu.Lock()
	if m.cert == nil || m.st.State != StateAuthorized {
		m.mu.Unlock()
		return "", problem("not_signed_in", "Sign in with Cloudflare first.")
	}
	cert := *m.cert
	gen := m.gen
	m.mu.Unlock()
	saved, err := m.Host.Installed()
	if err != nil {
		return "", err
	}
	if !saved.Made() {
		m.Forget("")
		return "", nil
	}
	if saved.AccountID != cert.AccountID {
		p := problem("wrong_account", "That Cloudflare account did not make this tunnel. Sign in with the one that did.")
		m.setupFailedKeep(gen, p)
		return "", p
	}
	if err := m.Host.Uninstall(ctx); err != nil {
		return "", err
	}
	api := m.api(cert.APIToken)
	if err := api.DeleteTunnel(ctx, cert.AccountID, saved.TunnelID); err != nil {
		p := problem("remove_failed", "Turned off here. Cloudflare did not remove the tunnel: "+err.Error())
		m.fail(gen, p)
		return saved.Hostname, p
	}
	left := ""
	if saved.ZoneID != cert.ZoneID {
		left = saved.Hostname
	} else if err := api.DeleteRoute(ctx, cert.ZoneID, saved.TunnelID, saved.Hostname); err != nil {
		slog.Info(fmt.Sprintf("cloudflare: %s stays in DNS: %v", saved.Hostname, err))
		left = saved.Hostname
	}
	m.Forget(left)
	return left, nil
}

// setupFailedKeep shows a problem and keeps the sign-in.
func (m *Manager) setupFailedKeep(gen int, p *Problem) {
	m.mu.Lock()
	if gen == m.gen {
		m.st.Error = p
	}
	m.mu.Unlock()
	m.changed()
}
