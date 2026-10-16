package direct

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// fakeService is the Worker's contract: it checks every signature.
type fakeService struct {
	mu        sync.Mutex
	keys      map[string]string // id -> public key
	addresses map[string][]string
	txt       map[string]string
	calls     []string
	now       func() time.Time
	forget    bool
}

func newFakeService(now func() time.Time) (*fakeService, *httptest.Server) {
	f := &fakeService{keys: map[string]string{}, addresses: map[string][]string{}, txt: map[string]string{}, now: now}
	return f, httptest.NewServer(f)
}

func (f *fakeService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	unix := r.Header.Get("X-Broadwave-Time")
	sec, _ := strconv.ParseInt(unix, 10, 64)
	if d := f.now().Unix() - sec; d > 300 || d < -300 {
		http.Error(w, `{"error":"stale"}`, http.StatusUnauthorized)
		return
	}
	sig, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Broadwave-Signature"))
	check := func(pub string) bool {
		raw, _ := base64.StdEncoding.DecodeString(pub)
		return len(raw) == ed25519.PublicKeySize && ed25519.Verify(raw, message(r.Method, r.URL.Path, unix, body), sig)
	}
	if r.URL.Path == "/v1/register" {
		var in struct{ PublicKey, ServerID string }
		_ = json.Unmarshal(body, &in)
		if !check(in.PublicKey) {
			http.Error(w, `{"error":"bad signature"}`, http.StatusUnauthorized)
			return
		}
		id := "abcdefgh" + strconv.Itoa(22+len(f.keys))
		for k, v := range f.keys {
			if v == in.PublicKey {
				id = k
			}
		}
		f.keys[id] = in.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "name": "lan." + id + ".direct.test"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/"), "/")
	pub, ok := f.keys[parts[0]]
	if !ok || f.forget {
		http.Error(w, `{"error":"unknown server"}`, http.StatusNotFound)
		return
	}
	if !check(pub) {
		http.Error(w, `{"error":"bad signature"}`, http.StatusUnauthorized)
		return
	}
	switch parts[1] {
	case "addresses":
		var in struct{ IPv4, IPv6 []string }
		_ = json.Unmarshal(body, &in)
		f.addresses[parts[0]] = append(in.IPv4, in.IPv6...)
	case "acme":
		if r.Method == http.MethodDelete {
			delete(f.txt, parts[0])
		} else {
			var in struct{ Value string }
			_ = json.Unmarshal(body, &in)
			f.txt[parts[0]] = in.Value
		}
	}
	_, _ = w.Write([]byte(`{}`))
}

// testCA issues certificates that the test root pool trusts.
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newCA(t *testing.T) *testCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"},
		NotBefore: t0.Add(-time.Hour), NotAfter: t0.Add(10 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{cert: cert, key: key, pool: pool}
}

// fakeIssuer plays the CA: it sets the challenge, checks the service has it,
// and signs a certificate valid for life.
type fakeIssuer struct {
	ca     *testCA
	svc    *fakeService
	life   time.Duration
	issued []string
	fail   error
}

func (fi *fakeIssuer) Issue(ctx context.Context, name string, ch Challenge) ([]byte, []byte, error) {
	if fi.fail != nil {
		return nil, nil, fi.fail
	}
	value := strings.Repeat("v", 43)
	if err := ch.Set(ctx, value); err != nil {
		return nil, nil, err
	}
	fi.svc.mu.Lock()
	seen := slices.Contains(mapValues(fi.svc.txt), value)
	fi.svc.mu.Unlock()
	if !seen {
		return nil, nil, errors.New("challenge not published")
	}
	if err := ch.Clear(ctx); err != nil {
		return nil, nil, err
	}
	fi.issued = append(fi.issued, name)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(int64(len(fi.issued) + 1)), Subject: pkix.Name{CommonName: name},
		DNSNames: []string{name}, NotBefore: t0.Add(-time.Hour), NotAfter: t0.Add(fi.life),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, fi.ca.cert, &key.PublicKey, fi.ca.key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, _ := ecPEM(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyPEM, nil
}

func mapValues(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

type rig struct {
	svc    *Service
	fake   *fakeService
	issuer *fakeIssuer
	now    time.Time
	addrs  []netip.Addr
	dir    string
	ca     *testCA
	api    string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{now: t0, dir: t.TempDir(), addrs: []netip.Addr{netip.MustParseAddr("192.168.1.2")}, ca: newCA(t)}
	fake, srv := newFakeService(func() time.Time { return r.now })
	t.Cleanup(srv.Close)
	r.fake = fake
	r.api = srv.URL
	r.issuer = &fakeIssuer{ca: r.ca, svc: fake, life: 90 * 24 * time.Hour}
	r.svc = r.open(t)
	return r
}

func (r *rig) open(t *testing.T) *Service {
	t.Helper()
	svc, err := New(Config{
		Dir: r.dir, API: r.api, ServerID: "srv-1", Port: 8443, Issuer: r.issuer, Directory: "test-ca",
		Addrs: func() []netip.Addr { return r.addrs }, Roots: r.ca.pool, Now: func() time.Time { return r.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFirstRoundRegistersPointsAndCertifies(t *testing.T) {
	r := newRig(t)
	if r.svc.URL() != "" {
		t.Fatal("a URL before any certificate")
	}
	if wait := r.svc.Step(context.Background()); wait != checkEvery {
		t.Fatalf("wait %v, status %+v", wait, r.svc.Status())
	}
	st := r.svc.Status()
	if st.State != "ready" || st.Name != "lan.abcdefgh22.direct.test" || !st.Trusted {
		t.Fatalf("status %+v", st)
	}
	if got := r.svc.URL(); got != "https://lan.abcdefgh22.direct.test:8443" {
		t.Fatalf("url %q", got)
	}
	if got := r.fake.addresses["abcdefgh22"]; !slices.Equal(got, []string{"192.168.1.2"}) {
		t.Fatalf("addresses %v", got)
	}
	if len(r.fake.txt) != 0 {
		t.Fatalf("challenge left behind: %v", r.fake.txt)
	}
	cert, err := r.svc.GetCertificate(&tls.ClientHelloInfo{ServerName: st.Name})
	if err != nil || cert.Leaf.DNSNames[0] != st.Name {
		t.Fatalf("certificate %v %v", cert, err)
	}
	for _, name := range []string{"server.key", "state.json", "cert.pem", "cert.key", "account.key"} {
		info, err := os.Stat(filepath.Join(r.dir, name))
		if name == "account.key" {
			continue // the fake issuer has no ACME account
		}
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
}

func TestQuietRoundCallsNothing(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	r.fake.calls = nil
	r.now = r.now.Add(time.Hour)
	r.svc.Step(context.Background())
	if len(r.fake.calls) != 0 || len(r.issuer.issued) != 1 {
		t.Fatalf("calls %v issued %v", r.fake.calls, r.issuer.issued)
	}
}

func TestAddressChangeIsReported(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	r.addrs = []netip.Addr{netip.MustParseAddr("10.0.0.7"), netip.MustParseAddr("fd00::7")}
	r.fake.calls = nil
	r.svc.Step(context.Background())
	if !slices.Equal(r.fake.calls, []string{"PUT /v1/abcdefgh22/addresses"}) {
		t.Fatalf("calls %v", r.fake.calls)
	}
	if got := r.fake.addresses["abcdefgh22"]; !slices.Equal(got, []string{"10.0.0.7", "fd00::7"}) {
		t.Fatalf("addresses %v", got)
	}
	// A day later the same list is sent again, in case the record was lost.
	r.fake.calls = nil
	r.now = r.now.Add(25 * time.Hour)
	r.svc.Step(context.Background())
	if !slices.Equal(r.fake.calls, []string{"PUT /v1/abcdefgh22/addresses"}) {
		t.Fatalf("calls %v", r.fake.calls)
	}
}

func TestRenewsThirtyDaysBeforeExpiry(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	r.now = t0.Add(59 * 24 * time.Hour)
	r.svc.Step(context.Background())
	if len(r.issuer.issued) != 1 {
		t.Fatalf("renewed early: %v", r.issuer.issued)
	}
	r.now = t0.Add(61 * 24 * time.Hour)
	r.issuer.life = 150 * 24 * time.Hour
	r.svc.Step(context.Background())
	if len(r.issuer.issued) != 2 {
		t.Fatalf("not renewed: %v", r.issuer.issued)
	}
	if exp := r.svc.Status().Expires; !exp.Equal(t0.Add(150 * 24 * time.Hour)) {
		t.Fatalf("expires %v", exp)
	}
}

func TestRestartKeepsNameAndCertificate(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	url := r.svc.URL()
	again := r.open(t)
	if again.URL() != url || again.Host() != "lan.abcdefgh22.direct.test" {
		t.Fatalf("after restart %q %q", again.URL(), again.Host())
	}
	r.fake.calls = nil
	again.Step(context.Background())
	if len(r.fake.calls) != 0 || len(r.issuer.issued) != 1 {
		t.Fatalf("calls %v issued %v", r.fake.calls, r.issuer.issued)
	}
}

func TestNewDirectoryGetsANewCertificate(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	svc, err := New(Config{
		Dir: r.dir, API: r.api, ServerID: "srv-1", Port: 8443, Issuer: r.issuer, Directory: "other-ca",
		Addrs: func() []netip.Addr { return r.addrs }, Roots: r.ca.pool, Now: func() time.Time { return r.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.Step(context.Background())
	if len(r.issuer.issued) != 2 {
		t.Fatalf("issued %v", r.issuer.issued)
	}
}

func TestUntrustedCertificateIsNotAdvertised(t *testing.T) {
	r := newRig(t)
	other := newCA(t)
	r.issuer.ca = other
	r.svc.Step(context.Background())
	st := r.svc.Status()
	if st.Trusted || st.URL != "" || r.svc.URL() != "" {
		t.Fatalf("status %+v", st)
	}
	// It still serves the certificate, for testing against a staging CA.
	if _, err := r.svc.GetCertificate(nil); err != nil {
		t.Fatal(err)
	}
}

func TestOffStopsEverything(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	if err := r.svc.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if r.svc.URL() != "" || r.svc.Status().State != "off" {
		t.Fatalf("still on: %+v", r.svc.Status())
	}
	if _, err := r.svc.GetCertificate(nil); err == nil {
		t.Fatal("serves a certificate while off")
	}
	r.fake.calls = nil
	r.now = r.now.Add(48 * time.Hour)
	r.svc.Step(context.Background())
	if len(r.fake.calls) != 0 {
		t.Fatalf("calls while off: %v", r.fake.calls)
	}
	// Off survives a restart.
	if again := r.open(t); again.Enabled() || again.Status().State != "off" {
		t.Fatal("came back on")
	}
	if err := r.svc.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	r.svc.Step(context.Background())
	if r.svc.URL() == "" {
		t.Fatal("not back on")
	}
}

func TestFailureBacksOffAndKeepsAWorkingCertificate(t *testing.T) {
	r := newRig(t)
	r.issuer.fail = errors.New("ca down")
	w1 := r.svc.Step(context.Background())
	w2 := r.svc.Step(context.Background())
	if w1 != minBackoff || w2 != 2*minBackoff {
		t.Fatalf("backoff %v %v", w1, w2)
	}
	if st := r.svc.Status(); st.State != "error" || !strings.Contains(st.LastError, "ca down") {
		t.Fatalf("status %+v", st)
	}
	r.issuer.fail = nil
	r.svc.Step(context.Background())
	// Renewal fails, but the current certificate still works.
	r.now = t0.Add(70 * 24 * time.Hour)
	r.issuer.fail = errors.New("ca down")
	r.svc.Step(context.Background())
	if st := r.svc.Status(); st.State != "ready" || st.URL == "" || st.LastError == "" {
		t.Fatalf("status %+v", st)
	}
}

func TestForgottenLabelRegistersAgain(t *testing.T) {
	r := newRig(t)
	r.svc.Step(context.Background())
	r.fake.forget = true
	r.addrs = []netip.Addr{netip.MustParseAddr("192.168.1.9")}
	r.svc.Step(context.Background())
	r.fake.forget = false
	r.fake.calls = nil
	r.svc.Step(context.Background())
	if !slices.Equal(r.fake.calls, []string{"POST /v1/register", "PUT /v1/abcdefgh22/addresses"}) {
		t.Fatalf("calls %v", r.fake.calls)
	}
	// The same key gets the same name back, so the certificate still fits.
	if len(r.issuer.issued) != 1 || r.svc.URL() == "" {
		t.Fatalf("issued %v url %q", r.issuer.issued, r.svc.URL())
	}
}

func TestNoHomeAddress(t *testing.T) {
	r := newRig(t)
	r.addrs = nil
	r.svc.Step(context.Background())
	if st := r.svc.Status(); st.State != "error" || len(r.issuer.issued) != 0 {
		t.Fatalf("status %+v", st)
	}
}

func TestPublishable(t *testing.T) {
	for raw, want := range map[string]bool{
		"10.1.2.3": true, "172.16.0.1": true, "172.31.255.255": true, "192.168.0.1": true,
		"100.64.0.1": true, "100.127.0.1": true, "fd12::1": true,
		"8.8.8.8": false, "172.32.0.1": false, "100.128.0.1": false, "127.0.0.1": false,
		"169.254.0.1": false, "fe80::1": false, "fc00::1": false, "2001:db8::1": false, "::ffff:8.8.8.8": false,
		"::ffff:192.168.1.2": true,
	} {
		if got := Publishable(netip.MustParseAddr(raw)); got != want {
			t.Errorf("%s: %v", raw, got)
		}
	}
}

func TestPickPrefersTheRoutedInterface(t *testing.T) {
	a := netip.MustParseAddr
	ifaces := []iface{
		{Name: "docker0", Addrs: []netip.Addr{a("172.17.0.1")}},
		{Name: "br0", Addrs: []netip.Addr{a("192.168.1.2"), a("fd00::2"), a("fe80::2"), a("2600::2")}},
		{Name: "wg0", Addrs: []netip.Addr{a("10.253.0.1")}},
		{Name: "tailscale0", Addrs: []netip.Addr{a("100.100.1.1")}},
	}
	got := addrStrings(pick(a("192.168.1.2"), ifaces))
	if !slices.Equal(got, []string{"192.168.1.2", "fd00::2"}) {
		t.Fatalf("got %v", got)
	}
	// No route: every real interface's home IPv4, without bridges and tunnels.
	ifaces = append(ifaces, iface{Name: "eth1", Addrs: []netip.Addr{a("10.0.0.5")}})
	got = addrStrings(pick(netip.Addr{}, ifaces))
	if !slices.Equal(got, []string{"10.0.0.5", "192.168.1.2"}) {
		t.Fatalf("got %v", got)
	}
	// A public primary falls back the same way.
	got = addrStrings(pick(a("203.0.113.5"), ifaces))
	if !slices.Equal(got, []string{"10.0.0.5", "192.168.1.2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseAddrs(t *testing.T) {
	got := addrStrings(ParseAddrs(" 192.168.1.2, 8.8.8.8 ,fd00::1,junk,192.168.1.2"))
	if !slices.Equal(got, []string{"192.168.1.2", "fd00::1"}) {
		t.Fatalf("got %v", got)
	}
}

func TestLoadKeyKeepsTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k")
	a, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := LoadKey(path)
	if !a.Equal(b) {
		t.Fatal("a new key on the second load")
	}
}

// The Worker's tests verify this exact signature (infra/direct/test/handler.test.ts).
func TestSignatureVector(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = 1
	}
	key := ed25519.NewKeyFromSeed(seed)
	msg := message("put", "/v1/abcdefghij/addresses", "1800000000", []byte(`{"ipv4":["192.168.1.2"]}`))
	pub := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg))
	const wantPub = "iojj3XQJ8ZX9UtstPLpdcspnCb8dlBIb83SIAbQPb1w="
	const wantSig = "yV6lV+uJjlRL48NsMOC1RR6MkJJbjwsuu5eLt8l5nMvuddpQSj76ljZeRVM1NfTdaMEcAaQxK6BMT3BZCgbyBw=="
	if pub != wantPub || sig != wantSig {
		t.Fatalf("vector changed:\npub %s\nsig %s", pub, sig)
	}
}

func TestClientRefusalCarriesTheReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"too many new certificates this week"}`))
	}))
	defer srv.Close()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c := &Client{API: srv.URL, Key: key}
	err := c.SetChallenge(context.Background(), "abcdefghij", "x")
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 429 || !strings.Contains(err.Error(), "this week") {
		t.Fatalf("err %v", err)
	}
}
