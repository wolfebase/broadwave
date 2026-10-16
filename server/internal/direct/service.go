package direct

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

// RenewBefore is how long before expiry a certificate is replaced.
const RenewBefore = 30 * 24 * time.Hour

const (
	checkEvery  = 5 * time.Minute
	reportEvery = 24 * time.Hour
	minBackoff  = time.Minute
	maxBackoff  = 6 * time.Hour
)

// Config is what the service needs. Dir is the folder for its files, inside
// the config folder and outside the catalog and its backups.
type Config struct {
	Dir      string
	API      string
	ServerID string
	// Port is the HTTPS port the name is served on.
	Port int
	// Issuer gets certificates. Nil uses Let's Encrypt at Directory.
	Issuer    Issuer
	Directory string
	// Addrs lists the addresses to publish. Nil asks the network interfaces.
	Addrs func() []netip.Addr
	// Roots decides whether a certificate is trusted by devices. Nil is the system pool.
	Roots *x509.CertPool
	HTTP  *http.Client
	Now   func() time.Time
}

// Status is what Settings shows.
type Status struct {
	Enabled bool `json:"enabled"`
	// State is off, starting, ready, or error.
	State     string    `json:"state"`
	Name      string    `json:"name,omitempty"`
	URL       string    `json:"url,omitempty"`
	Expires   time.Time `json:"expires,omitzero"`
	Trusted   bool      `json:"trusted"`
	Addresses []string  `json:"addresses"`
	LastError string    `json:"lastError,omitempty"`
	ErrorAt   time.Time `json:"errorAt,omitzero"`
}

// state is saved as state.json in Dir.
type state struct {
	// Disabled is the owner's switch. The zero value is on.
	Disabled   bool      `json:"disabled,omitempty"`
	API        string    `json:"api,omitempty"`
	ID         string    `json:"id,omitempty"`
	Name       string    `json:"name,omitempty"`
	Addresses  []string  `json:"addresses,omitempty"`
	ReportedAt time.Time `json:"reportedAt,omitzero"`
	// Directory is the CA that issued the saved certificate.
	Directory string `json:"directory,omitempty"`
}

// Service keeps the name registered, pointed at this server, and certified.
type Service struct {
	cfg    Config
	client *Client

	mu      sync.Mutex
	st      state
	cert    *tls.Certificate
	expires time.Time
	trusted bool
	status  string
	lastErr string
	errAt   time.Time
	backoff time.Duration
	kick    chan struct{}
}

// New loads the saved key, state, and certificate.
func New(cfg Config) (*Service, error) {
	if cfg.API == "" {
		cfg.API = DefaultAPI
	}
	if cfg.Directory == "" {
		cfg.Directory = LetsEncrypt
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Addrs == nil {
		cfg.Addrs = LocalAddrs
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, err
	}
	key, err := LoadKey(filepath.Join(cfg.Dir, "server.key"))
	if err != nil {
		return nil, err
	}
	if cfg.Issuer == nil {
		cfg.Issuer = &ACME{Directory: cfg.Directory, AccountKey: filepath.Join(cfg.Dir, "account.key"), HTTP: cfg.HTTP}
	}
	s := &Service{
		cfg:    cfg,
		client: &Client{API: cfg.API, Key: key, HTTP: cfg.HTTP, Now: cfg.Now},
		status: "starting",
		kick:   make(chan struct{}, 1),
	}
	if raw, err := os.ReadFile(s.path("state.json")); err == nil {
		_ = json.Unmarshal(raw, &s.st)
	}
	if s.st.API != cfg.API {
		// Another service's label means nothing here.
		s.st = state{Disabled: s.st.Disabled, API: cfg.API}
	}
	s.loadCert()
	if s.st.Disabled {
		s.status = "off"
	}
	return s, nil
}

func (s *Service) path(name string) string { return filepath.Join(s.cfg.Dir, name) }

func (s *Service) loadCert() {
	chain, err1 := os.ReadFile(s.path("cert.pem"))
	key, err2 := os.ReadFile(s.path("cert.key"))
	if err1 != nil || err2 != nil {
		return
	}
	s.setCert(chain, key)
}

// setCert parses and installs a certificate. The caller holds no lock.
func (s *Service) setCert(chain, key []byte) bool {
	pair, err := tls.X509KeyPair(chain, key)
	if err != nil || len(pair.Certificate) == 0 {
		return false
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false
	}
	pair.Leaf = leaf
	inter := x509.NewCertPool()
	for _, der := range pair.Certificate[1:] {
		if c, err := x509.ParseCertificate(der); err == nil {
			inter.AddCert(c)
		}
	}
	s.mu.Lock()
	name := s.st.Name
	s.mu.Unlock()
	_, verr := leaf.Verify(x509.VerifyOptions{
		DNSName:       name,
		Roots:         s.cfg.Roots,
		Intermediates: inter,
		CurrentTime:   s.cfg.Now(),
	})
	s.mu.Lock()
	s.cert = &pair
	s.expires = leaf.NotAfter
	s.trusted = verr == nil
	s.mu.Unlock()
	return true
}

func (s *Service) save() error {
	s.mu.Lock()
	raw, err := json.MarshalIndent(s.st, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := s.path("state.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path("state.json"))
}

// Run keeps everything current until ctx ends.
func (s *Service) Run(ctx context.Context) {
	for {
		wait := s.Step(ctx)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-s.kick:
			t.Stop()
		}
	}
}

// Step does one round and returns how long to wait before the next.
func (s *Service) Step(ctx context.Context) time.Duration {
	s.mu.Lock()
	off := s.st.Disabled
	s.mu.Unlock()
	if off {
		s.setStatus("off")
		return time.Hour
	}
	if err := s.register(ctx); err != nil {
		return s.failed("register", err)
	}
	if err := s.report(ctx); err != nil {
		return s.failed("addresses", err)
	}
	if s.needsCert() {
		if err := s.issue(ctx); err != nil {
			return s.failed("certificate", err)
		}
	}
	s.mu.Lock()
	s.backoff = 0
	s.lastErr = ""
	s.errAt = time.Time{}
	s.mu.Unlock()
	s.setStatus("ready")
	return checkEvery
}

func (s *Service) setStatus(v string) {
	s.mu.Lock()
	s.status = v
	s.mu.Unlock()
}

// failed notes the error and backs off. A certificate that still works keeps
// the state ready: the name goes on serving while the next try waits.
func (s *Service) failed(what string, err error) time.Duration {
	msg := fmt.Sprintf("%s: %v", what, err)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backoff == 0 {
		s.backoff = minBackoff
	} else {
		s.backoff = min(s.backoff*2, maxBackoff)
	}
	var ae *apiError
	if errors.As(err, &ae) && ae.Status == http.StatusTooManyRequests {
		s.backoff = max(s.backoff, time.Hour)
	}
	if msg != s.lastErr {
		slog.Warn("direct: " + msg)
	}
	s.lastErr = msg
	s.errAt = s.cfg.Now()
	if s.cert != nil && s.cfg.Now().Before(s.expires) {
		s.status = "ready"
	} else {
		s.status = "error"
	}
	return s.backoff
}

func (s *Service) register(ctx context.Context) error {
	s.mu.Lock()
	have := s.st.ID != "" && s.st.Name != ""
	s.mu.Unlock()
	if have {
		return nil
	}
	reg, err := s.client.Register(ctx, s.cfg.ServerID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if reg.Name != s.st.Name {
		s.st.Directory = ""
	}
	s.st.API = s.cfg.API
	s.st.ID = reg.ID
	s.st.Name = reg.Name
	s.st.Addresses = nil
	s.mu.Unlock()
	slog.Info("direct: this server's secure name is " + reg.Name)
	return s.save()
}

func addrStrings(list []netip.Addr) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.String())
	}
	slices.Sort(out)
	return out
}

// report points the name at this server when its address changed, and once a
// day so a lost record comes back.
func (s *Service) report(ctx context.Context) error {
	list := s.cfg.Addrs()
	if len(list) == 0 {
		return errors.New("this server has no home network address")
	}
	now := addrStrings(list)
	s.mu.Lock()
	same := slices.Equal(now, s.st.Addresses) && s.cfg.Now().Sub(s.st.ReportedAt) < reportEvery
	id := s.st.ID
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := s.client.Addresses(ctx, id, list); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			// The service forgot this label. Register again next round.
			s.mu.Lock()
			s.st.ID = ""
			s.mu.Unlock()
			_ = s.save()
		}
		return err
	}
	s.mu.Lock()
	if !slices.Equal(now, s.st.Addresses) {
		slog.Info(fmt.Sprintf("direct: %s points at %v", s.st.Name, now))
	}
	s.st.Addresses = now
	s.st.ReportedAt = s.cfg.Now()
	s.mu.Unlock()
	return s.save()
}

func (s *Service) needsCert() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cert == nil || s.st.Directory != s.cfg.Directory {
		return true
	}
	if s.cert.Leaf == nil || !slices.Contains(s.cert.Leaf.DNSNames, s.st.Name) {
		return true
	}
	return s.expires.Sub(s.cfg.Now()) < RenewBefore
}

type challenge struct {
	c  *Client
	id string
}

func (c challenge) Set(ctx context.Context, value string) error {
	return c.c.SetChallenge(ctx, c.id, value)
}
func (c challenge) Clear(ctx context.Context) error { return c.c.ClearChallenge(ctx, c.id) }

func (s *Service) issue(ctx context.Context) error {
	s.mu.Lock()
	id, name := s.st.ID, s.st.Name
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	slog.Info("direct: asking for a certificate for " + name)
	chain, key, err := s.cfg.Issuer.Issue(ctx, name, challenge{c: s.client, id: id})
	if err != nil {
		return err
	}
	if !s.setCert(chain, key) {
		return errors.New("the certificate authority sent a certificate that does not load")
	}
	if err := os.WriteFile(s.path("cert.key"), key, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(s.path("cert.pem"), chain, 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	s.st.Directory = s.cfg.Directory
	exp := s.expires
	s.mu.Unlock()
	slog.Info(fmt.Sprintf("direct: certificate for %s until %s", name, exp.Format(time.DateOnly)))
	return s.save()
}

// Enabled reports the owner's switch.
func (s *Service) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.st.Disabled
}

// SetEnabled turns the secure name on or off. Off stops serving it at once;
// on starts a round now.
func (s *Service) SetEnabled(on bool) error {
	s.mu.Lock()
	s.st.Disabled = !on
	if !on {
		s.status = "off"
	} else if s.status == "off" {
		s.status = "starting"
	}
	s.mu.Unlock()
	if err := s.save(); err != nil {
		return err
	}
	select {
	case s.kick <- struct{}{}:
	default:
	}
	return nil
}

// Host is the name this server answers to, once registered.
func (s *Service) Host() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Name
}

// usable is a certificate devices trust, for the current name, not expired.
func (s *Service) usableLocked() bool {
	if s.st.Disabled || s.cert == nil || !s.trusted || s.st.Name == "" {
		return false
	}
	if s.cert.Leaf == nil || !slices.Contains(s.cert.Leaf.DNSNames, s.st.Name) {
		return false
	}
	return s.cfg.Now().Before(s.expires)
}

// URL is the secure address, once a trusted certificate is live. Empty before.
func (s *Service) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.usableLocked() {
		return ""
	}
	return s.urlLocked()
}

func (s *Service) urlLocked() string {
	u := "https://" + s.st.Name
	if s.cfg.Port != 443 {
		u += ":" + strconv.Itoa(s.cfg.Port)
	}
	return u
}

// GetCertificate is the TLS listener's certificate.
func (s *Service) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Disabled || s.cert == nil {
		return nil, errors.New("no certificate yet")
	}
	return s.cert, nil
}

// Status is the state for Settings.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{
		Enabled:   !s.st.Disabled,
		State:     s.status,
		Name:      s.st.Name,
		Expires:   s.expires,
		Trusted:   s.trusted,
		Addresses: append([]string{}, s.st.Addresses...),
		LastError: s.lastErr,
		ErrorAt:   s.errAt,
	}
	if s.usableLocked() {
		st.URL = s.urlLocked()
	}
	if s.st.Disabled {
		st.State = "off"
	}
	return st
}
