package cloudflare_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/cloudflare"
	"broadwave/internal/cloudflare/cftest"
)

type fakeHost struct {
	mu         sync.Mutex
	saved      cloudflare.Secret
	installs   int
	uninstalls int
}

func (h *fakeHost) Install(_ context.Context, s cloudflare.Secret) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.saved = s
	h.installs++
	return nil
}

func (h *fakeHost) Connected(context.Context) error { return nil }

func (h *fakeHost) Installed() (cloudflare.Secret, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saved, nil
}

func (h *fakeHost) Uninstall(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.saved = cloudflare.Secret{}
	h.uninstalls++
	return nil
}

func (h *fakeHost) secret() cloudflare.Secret {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saved
}

func setup(t *testing.T, change func(*cftest.Fake)) (*cloudflare.Manager, *cftest.Fake, *fakeHost) {
	t.Helper()
	fake := cftest.New()
	fake.AuthorizeAfter = 3
	if change != nil {
		change(fake)
	}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	host := &fakeHost{}
	m := &cloudflare.Manager{
		Endpoints: cloudflare.Endpoints{Dash: srv.URL, Login: srv.URL + "/login", API: srv.URL + "/client/v4"},
		Host:      host,
		Retry:     10 * time.Millisecond,
	}
	t.Cleanup(m.Cancel)
	return m, fake, host
}

func waitState(t *testing.T, m *cloudflare.Manager, want string) cloudflare.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := m.Status()
		if st.State == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %q, want %q (%+v)", st.State, want, st)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func signIn(t *testing.T, m *cloudflare.Manager, purpose string) cloudflare.Status {
	t.Helper()
	st, err := m.StartLogin(purpose)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != cloudflare.StateWaiting || st.ExpiresAt.IsZero() {
		t.Fatalf("start %+v", st)
	}
	return st
}

func TestLoginPollsUntilAuthorizedAndReadsTheZone(t *testing.T) {
	m, fake, _ := setup(t, nil)
	st := signIn(t, m, "")
	u, err := url.Parse(st.LoginURL)
	if err != nil || u.Path != "/argotunnel" {
		t.Fatalf("login url %q", st.LoginURL)
	}
	cb := u.Query().Get("callback")
	if !strings.Contains(cb, "/login/") || !u.Query().Has("aud") {
		t.Fatalf("callback %q", st.LoginURL)
	}
	key := cb[strings.LastIndex(cb, "/")+1:]
	got := waitState(t, m, cloudflare.StateAuthorized)
	if got.Zone == nil || got.Zone.ID != "zone1" || got.Zone.Name != "example.com" || got.Purpose != cloudflare.PurposeSetup {
		t.Fatalf("authorized %+v", got)
	}
	if n := fake.Polls(key); n != 3 {
		t.Fatalf("polled %d times, want 404, 404, 200", n)
	}
	if got.LoginURL != "" {
		t.Fatal("the login link stays after sign-in")
	}
}

func TestSetupMakesTheTunnelRoutesItAndDropsTheCertificate(t *testing.T) {
	m, fake, host := setup(t, nil)
	signIn(t, m, "")
	waitState(t, m, cloudflare.StateAuthorized)
	st, err := m.Setup(cloudflare.Plan{Label: "TV", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"})
	if err != nil {
		t.Fatal(err)
	}
	if st.State != cloudflare.StateProvisioning || st.Hostname != "tv.example.com" || st.Step != cloudflare.StepTunnel {
		t.Fatalf("provisioning %+v", st)
	}
	waitState(t, m, cloudflare.StateIdle)
	saved := host.secret()
	id, remote, config, ok := fake.Tunnel("broadwave-abc123")
	if !ok || !remote {
		t.Fatalf("tunnel %v remote %v", ok, remote)
	}
	if saved.Token != cftest.RunToken("acct1", id) || saved.Source != cloudflare.SourceBroadwave || saved.Routes != "cloudflare" ||
		saved.TunnelID != id || saved.Hostname != "tv.example.com" || saved.AccountID != "acct1" || saved.ZoneID != "zone1" {
		t.Fatalf("saved %+v", saved)
	}
	if !strings.Contains(config, `"hostname":"tv.example.com"`) || !strings.Contains(config, `"service":"http://127.0.0.1:8477"`) {
		t.Fatalf("config %s", config)
	}
	if got := fake.DNS("tv.example.com"); got != id+".cfargotunnel.com" {
		t.Fatalf("dns %q", got)
	}
	// The certificate is gone: nothing more can be made with it.
	if _, err := m.Setup(cloudflare.Plan{Label: "tv2", TunnelName: "x"}); !isProblem(err, "not_signed_in") {
		t.Fatalf("second setup %v", err)
	}
	if _, err := m.Remove(context.Background()); !isProblem(err, "not_signed_in") {
		t.Fatalf("remove without sign-in %v", err)
	}
}

func isProblem(err error, code string) bool {
	var p *cloudflare.Problem
	return errors.As(err, &p) && p.Code == code
}

func TestATakenNameAsksThenReplacesAndReusesTheTunnel(t *testing.T) {
	m, fake, host := setup(t, func(f *cftest.Fake) { f.AddDNS("tv.example.com", "203.0.113.10") })
	signIn(t, m, "")
	waitState(t, m, cloudflare.StateAuthorized)
	plan := cloudflare.Plan{Label: "tv", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"}
	if _, err := m.Setup(plan); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var st cloudflare.Status
	for {
		st = m.Status()
		if st.State == cloudflare.StateAuthorized && st.Error != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no name_taken: %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if st.Error.Code != "name_taken" || !strings.Contains(st.Error.Message, "tv.example.com") || st.Hostname != "tv.example.com" {
		t.Fatalf("taken %+v", st.Error)
	}
	if fake.DNS("tv.example.com") != "203.0.113.10" || host.secret().Token != "" {
		t.Fatal("a taken name was changed or saved")
	}
	plan.Overwrite = true
	if _, err := m.Setup(plan); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, cloudflare.StateIdle)
	if names := fake.Tunnels(); len(names) != 1 {
		t.Fatalf("tunnels %v", names)
	}
	id, _, _, _ := fake.Tunnel("broadwave-abc123")
	if fake.DNS("tv.example.com") != id+".cfargotunnel.com" || host.secret().TunnelID != id {
		t.Fatalf("replace did not point at the tunnel: %q", fake.DNS("tv.example.com"))
	}
}

func TestRefusedRemoteRoutesFallBackToLocal(t *testing.T) {
	m, fake, host := setup(t, func(f *cftest.Fake) { f.ConfigRefused = true })
	signIn(t, m, "")
	waitState(t, m, cloudflare.StateAuthorized)
	if _, err := m.Setup(cloudflare.Plan{Label: "tv", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"}); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, cloudflare.StateIdle)
	id, remote, _, ok := fake.Tunnel("broadwave-abc123")
	if !ok || remote || len(fake.Tunnels()) != 1 {
		t.Fatalf("tunnel %v remote %v %v", ok, remote, fake.Tunnels())
	}
	if s := host.secret(); s.Routes != "local" || s.TunnelID != id {
		t.Fatalf("saved %+v", s)
	}
}

func TestAZoneThatMayNotMakeTunnelsSaysSoAndKeepsTheSignIn(t *testing.T) {
	m, fake, host := setup(t, func(f *cftest.Fake) { f.TunnelsRefused = true })
	signIn(t, m, "")
	waitState(t, m, cloudflare.StateAuthorized)
	if _, err := m.Setup(cloudflare.Plan{Label: "tv", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := m.Status()
		if st.State == cloudflare.StateAuthorized && st.Error != nil {
			if st.Error.Code != "not_permitted" {
				t.Fatalf("error %+v", st.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(fake.Tunnels()) != 0 || host.secret().Token != "" {
		t.Fatalf("made %v saved %+v", fake.Tunnels(), host.secret())
	}
}

func TestAHiddenZoneNeedsTheWholeName(t *testing.T) {
	m, _, host := setup(t, func(f *cftest.Fake) { f.ZoneHidden = true })
	signIn(t, m, "")
	st := waitState(t, m, cloudflare.StateAuthorized)
	if st.Zone == nil || st.Zone.ID != "zone1" || st.Zone.Name != "" {
		t.Fatalf("zone %+v", st.Zone)
	}
	if _, err := m.Setup(cloudflare.Plan{Label: "tv", TunnelName: "broadwave-abc123"}); !isProblem(err, "bad_name") {
		t.Fatalf("label alone %v", err)
	}
	if _, err := m.Setup(cloudflare.Plan{Label: "tv.example.com", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"}); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, cloudflare.StateIdle)
	if host.secret().Hostname != "tv.example.com" {
		t.Fatalf("saved %+v", host.secret())
	}
}

func TestSignInProblems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*cftest.Fake)
		mgr    func(*cloudflare.Manager)
		code   string
	}{
		{"no domain", func(f *cftest.Fake) { f.NoZone = true }, nil, "no_zone"},
		{"login service down", func(f *cftest.Fake) { f.LoginDown = true }, nil, "login_failed"},
		{"owner never authorizes", func(f *cftest.Fake) { f.AuthorizeAfter = 0; f.Hold = 20 * time.Millisecond }, func(m *cloudflare.Manager) { m.LoginTTL = 150 * time.Millisecond }, "timeout"},
		{"sign-in left unused", nil, func(m *cloudflare.Manager) { m.HoldTTL = 100 * time.Millisecond }, "timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := setup(t, tc.change)
			if tc.mgr != nil {
				tc.mgr(m)
			}
			signIn(t, m, "")
			st := waitState(t, m, cloudflare.StateError)
			if st.Error == nil || st.Error.Code != tc.code || st.Error.Message == "" {
				t.Fatalf("error %+v", st.Error)
			}
			if _, err := m.Setup(cloudflare.Plan{Label: "tv"}); !isProblem(err, "not_signed_in") {
				t.Fatalf("setup after %s: %v", tc.code, err)
			}
		})
	}
}

func TestCancelStopsWaiting(t *testing.T) {
	m, fake, _ := setup(t, func(f *cftest.Fake) { f.AuthorizeAfter = 0; f.Hold = time.Second })
	st := signIn(t, m, "")
	m.Cancel()
	if got := m.Status(); got.State != cloudflare.StateIdle || got.LoginURL != "" {
		t.Fatalf("after cancel %+v", got)
	}
	u, _ := url.Parse(st.LoginURL)
	cb := u.Query().Get("callback")
	fake.Authorize(cb[strings.LastIndex(cb, "/")+1:])
	time.Sleep(50 * time.Millisecond)
	if got := m.Status(); got.State != cloudflare.StateIdle {
		t.Fatalf("a cancelled sign-in finished: %+v", got)
	}
}

func TestRemoveDeletesTheTunnelAndItsName(t *testing.T) {
	for _, dnsRefused := range []bool{false, true} {
		m, fake, host := setup(t, func(f *cftest.Fake) { f.DNSRefused = dnsRefused })
		signIn(t, m, "")
		waitState(t, m, cloudflare.StateAuthorized)
		if _, err := m.Setup(cloudflare.Plan{Label: "tv", TunnelName: "broadwave-abc123", Service: "http://127.0.0.1:8477"}); err != nil {
			t.Fatal(err)
		}
		waitState(t, m, cloudflare.StateIdle)
		// Remove signs in again; the sign-in deletes as soon as it lands.
		signIn(t, m, cloudflare.PurposeRemove)
		deadline := time.Now().Add(5 * time.Second)
		for host.uninstallsNow() == 0 || m.Status().State != cloudflare.StateIdle {
			if time.Now().After(deadline) {
				t.Fatalf("not removed: %+v", m.Status())
			}
			time.Sleep(5 * time.Millisecond)
		}
		st := m.Status()
		if len(fake.Tunnels()) != 0 || host.secret().Token != "" {
			t.Fatalf("left %v %+v", fake.Tunnels(), host.secret())
		}
		if dnsRefused {
			if st.DNSLeft != "tv.example.com" || fake.DNS("tv.example.com") == "" {
				t.Fatalf("refused dns: left %q", st.DNSLeft)
			}
		} else if st.DNSLeft != "" || fake.DNS("tv.example.com") != "" {
			t.Fatalf("dns left %q %q", st.DNSLeft, fake.DNS("tv.example.com"))
		}
	}
}

func (h *fakeHost) uninstallsNow() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.uninstalls
}

func TestHostname(t *testing.T) {
	for _, tc := range []struct{ label, zone, want string }{
		{"tv", "example.com", "tv.example.com"},
		{" TV ", "example.com", "tv.example.com"},
		{"tv.example.com", "example.com", "tv.example.com"},
		{"my-tv", "example.com", "my-tv.example.com"},
		{"a.b", "example.com", ""},
		{"-tv", "example.com", ""},
		{"t v", "example.com", ""},
		{"", "example.com", ""},
		{"tv.example.com", "", "tv.example.com"},
		{"tv", "", ""},
		{"example.com", "", ""},
	} {
		got, err := cloudflare.Hostname(tc.label, tc.zone)
		if got != tc.want || (tc.want == "") != (err != nil) {
			t.Errorf("Hostname(%q, %q) = %q, %v", tc.label, tc.zone, got, err)
		}
	}
}

func TestDecodeCert(t *testing.T) {
	c, err := cloudflare.DecodeCert(cftest.New().Cert())
	if err != nil || c.ZoneID != "zone1" || c.AccountID != "acct1" || c.APIToken != "cert-api-token" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := cloudflare.DecodeCert([]byte("cert not found")); err == nil {
		t.Fatal("plain text decoded")
	}
}

func TestVaultIsPrivateAndOutsideTheCatalog(t *testing.T) {
	dir := t.TempDir()
	v := cloudflare.NewVault(dir)
	if s, err := v.Load(); err != nil || s.Token != "" {
		t.Fatalf("empty %+v %v", s, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(cloudflare.Secret{Token: "tok", Source: cloudflare.SourcePasted}); err != nil {
		t.Fatal(err)
	}
	if v.Path != filepath.Join(dir, "secrets", "cloudflare.json") {
		t.Fatalf("path %s", v.Path)
	}
	fi, err := os.Stat(v.Path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v %v", fi.Mode(), err)
	}
	di, err := os.Stat(filepath.Dir(v.Path))
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v %v", di.Mode(), err)
	}
	if s, _ := v.Load(); s.Token != "tok" || s.Made() {
		t.Fatalf("load %+v", s)
	}
	if err := v.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(v.Path); !os.IsNotExist(err) {
		t.Fatal("clear left the file")
	}
	if err := v.Clear(); err != nil {
		t.Fatal(err)
	}
}
