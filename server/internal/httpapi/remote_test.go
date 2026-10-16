package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"broadwave/internal/store"
	"broadwave/internal/tunnel"
)

type fakeTunnel struct {
	mu      sync.Mutex
	tokens  []string
	origins []string
	status  tunnel.Status
}

func (f *fakeTunnel) Apply(token, origin string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, token)
	f.origins = append(f.origins, origin)
}

func (f *fakeTunnel) Status() tunnel.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func remoteServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	api := &Server{
		Store:   testStore(t),
		Clock:   func() time.Time { return now },
		Assets:  fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}},
		Version: "dev",
	}
	return api, api.Handler()
}

// remoteCall sends a request the way cloudflared delivers it: the public
// name in Host, Cloudflare's headers, and a peer on this machine.
func remoteCall(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "tv.example.net"
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
	req.Header.Set("Cf-Ray", "8c0ffee-DFW")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPublicAddressNeedsAPairedDeviceAndTheHouseStaysOpen(t *testing.T) {
	_, h := remoteServer(t)
	// Before the address is saved, the public name is refused outright.
	plain := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	plain.Host = "tv.example.net"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, plain)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("unknown public name %d", rec.Code)
	}
	if got := call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"TV.example.net/"}`, ""); got.Code != http.StatusOK ||
		!strings.Contains(got.Body.String(), `"remoteUrl":"https://tv.example.net"`) {
		t.Fatalf("save %d %s", got.Code, got.Body.String())
	}
	// An app sends no Sec-Fetch-Site. The saved name is enough.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, plain)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("public name without a token %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/v1/channels", "/api/v1/settings", "/export/lineup.m3u", "/export/guide.xml", "/media/live/x/index.m3u8"} {
		if got := remoteCall(t, h, http.MethodGet, path, "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("remote %s %d", path, got.Code)
		}
	}
	for _, path := range []string{"/api/v1/health", "/api/v1/server", "/"} {
		if got := remoteCall(t, h, http.MethodGet, path, "", ""); got.Code != http.StatusOK {
			t.Fatalf("remote %s %d", path, got.Code)
		}
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/profile", "", ""); !strings.Contains(got.Body.String(), `"auth":"device"`) {
		t.Fatalf("profile %s", got.Body.String())
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/clients/me", "", ""); !strings.Contains(got.Body.String(), `"auth":"device"`) {
		t.Fatalf("me %s", got.Body.String())
	}
	// The home network is unchanged.
	for _, path := range []string{"/api/v1/channels", "/export/lineup.m3u"} {
		if got := call(t, h, http.MethodGet, path, "", ""); got.Code != http.StatusOK {
			t.Fatalf("lan %s %d", path, got.Code)
		}
	}
	if got := call(t, h, http.MethodGet, "/api/v1/profile", "", ""); !strings.Contains(got.Body.String(), `"auth":"local-open"`) {
		t.Fatalf("lan profile %s", got.Body.String())
	}
}

func TestCloudflareHeaderMakesAnyNameRemote(t *testing.T) {
	_, h := remoteServer(t)
	// A tunnel that rewrites Host to the server's address is still outside.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	req.Host = "192.168.1.2:8477"
	req.RemoteAddr = "172.17.0.5:41000"
	req.Header.Set("Cf-Connecting-Ip", "198.51.100.4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("cloudflare request %d", rec.Code)
	}
	// A name nobody saved yet, from the built-in cloudflared, asks for pairing.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	req.Host = "new-name.example.org"
	req.RemoteAddr = "127.0.0.1:41000"
	req.Header.Set("Cf-Ray", "8c0ffee-DFW")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsaved cloudflare name %d", rec.Code)
	}
	// A rebinding page on the home network that adds the header is still refused.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/server", nil)
	req.Host = "rebind.attacker.com"
	req.RemoteAddr = "192.168.1.50:41000"
	req.Header.Set("Cf-Ray", "x")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("rebinding page with a fake header %d", rec.Code)
	}
}

func TestTunnelEmulatorPortIgnoresCloudflareHeaders(t *testing.T) {
	h := rejectForeignHost(nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/lineup.json", nil)
	req.Host = "rebind.attacker.com:8478"
	req.RemoteAddr = "127.0.0.1:41000"
	req.Header.Set("Cf-Ray", "x")
	req.Header.Set("Cf-Connecting-Ip", "203.0.113.4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("emulator answered a foreign name %d", rec.Code)
	}
}

func TestOutsideGuessesDoNotBlockPairingAtHome(t *testing.T) {
	_, h := remoteServer(t)
	for i := range pairFailCap + 5 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pair/claim", strings.NewReader(`{"code":"000000","name":"x"}`))
		req.RemoteAddr = "127.0.0.1:1000"
		req.Header.Set("Cf-Connecting-Ip", "203.0.113."+strconv.Itoa(i%200+1))
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := remoteCall(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"000000","name":"x"}`, ""); got.Code != http.StatusTooManyRequests {
		t.Fatalf("outside cap %d", got.Code)
	}
	if got := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Kitchen TV","kind":"tv"}`, ""); got.Code != http.StatusCreated {
		t.Fatalf("home pairing blocked %d %s", got.Code, got.Body.String())
	}
}

func TestOutsideResponsesAreNotShareable(t *testing.T) {
	_, h := remoteServer(t)
	call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"tv.example.net"}`, "")
	made := call(t, h, http.MethodPost, "/api/v1/invites", `{"name":"Mom"}`, "")
	code := jsonField(t, made.Body.Bytes(), "code")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pair/claim", strings.NewReader(`{"code":"`+code+`","name":"Laptop","kind":"web"}`))
	req.Host = "tv.example.net"
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
	req.Header.Set("Cf-Visitor", `{"scheme":"https"}`)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim %d %s", rec.Code, rec.Body.String())
	}
	if c := rec.Header().Get("Set-Cookie"); !strings.Contains(c, "Secure") {
		t.Fatalf("cookie over https without Secure: %q", c)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "private") && !strings.Contains(cc, "no-store") {
		t.Fatalf("claim cache %q", cc)
	}
	token := jsonField(t, rec.Body.Bytes(), "token")
	for _, path := range []string{"/api/v1/channels", "/api/v1/health", "/"} {
		got := remoteCall(t, h, http.MethodGet, path, "", token)
		if cc := got.Header().Get("Cache-Control"); strings.Contains(cc, "public") || (!strings.Contains(cc, "private") && !strings.Contains(cc, "no-store") && !strings.Contains(cc, "no-cache")) {
			t.Fatalf("%s cache %q", path, cc)
		}
	}
	// At home the cookie stays usable over plain http.
	if c := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "").Header().Get("Set-Cookie"); strings.Contains(c, "Secure") {
		t.Fatalf("home cookie marked Secure: %q", c)
	}
}

func TestInvitePairsOneDeviceFromOutside(t *testing.T) {
	_, h := remoteServer(t)
	call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"https://tv.example.net"}`, "")
	made := call(t, h, http.MethodPost, "/api/v1/invites", `{"name":"Mom","scopes":["watch"]}`, "")
	if made.Code != http.StatusCreated {
		t.Fatalf("invite %d %s", made.Code, made.Body.String())
	}
	var out struct {
		Code    string       `json:"code"`
		Link    string       `json:"link"`
		AppLink string       `json:"appLink"`
		Invite  store.Invite `json:"invite"`
	}
	if err := json.Unmarshal(made.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Code) != 14 || strings.Count(out.Code, "-") != 2 {
		t.Fatalf("code %q", out.Code)
	}
	if out.Link != "https://tv.example.net/pair?invite="+out.Code {
		t.Fatalf("link %q", out.Link)
	}
	if !strings.HasPrefix(out.AppLink, "broadwave://join?server=https%3A%2F%2Ftv.example.net&code=") {
		t.Fatalf("app link %q", out.AppLink)
	}
	if !out.Invite.ExpiresAt.Equal(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expires %v", out.Invite.ExpiresAt)
	}
	listed := call(t, h, http.MethodGet, "/api/v1/invites", "", "")
	if !strings.Contains(listed.Body.String(), `"name":"Mom"`) || strings.Contains(listed.Body.String(), out.Code) {
		t.Fatalf("list %s", listed.Body.String())
	}
	// Someone typing on a TV: lower case, no dashes, and the device's own name.
	typed := strings.ToLower(strings.ReplaceAll(out.Code, "-", " "))
	claimed := remoteCall(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+typed+`","name":"Living Room","kind":"tv"}`, "")
	if claimed.Code != http.StatusCreated {
		t.Fatalf("claim %d %s", claimed.Code, claimed.Body.String())
	}
	token := jsonField(t, claimed.Body.Bytes(), "token")
	if !strings.Contains(claimed.Body.String(), `"scopes":["watch"]`) {
		t.Fatalf("scopes %s", claimed.Body.String())
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/channels", "", token); got.Code != http.StatusOK {
		t.Fatalf("paired channels %d", got.Code)
	}
	if got := remoteCall(t, h, http.MethodGet, "/export/lineup.m3u?access_token="+token, "", ""); got.Code != http.StatusOK {
		t.Fatalf("paired export %d", got.Code)
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/settings", "", token); got.Code != http.StatusForbidden {
		t.Fatalf("watch token read settings %d", got.Code)
	}
	if got := remoteCall(t, h, http.MethodPost, "/api/v1/invites", `{"name":"x"}`, token); got.Code != http.StatusForbidden {
		t.Fatalf("watch token made an invite %d", got.Code)
	}
	// One device per invite.
	again := remoteCall(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+out.Code+`","name":"Phone","kind":"phone"}`, "")
	if again.Code != http.StatusNotFound {
		t.Fatalf("second claim %d %s", again.Code, again.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/api/v1/invites", "", ""); strings.Contains(got.Body.String(), "Mom") {
		t.Fatalf("used invite still listed %s", got.Body.String())
	}
}

func TestDeletedInviteStopsWorking(t *testing.T) {
	_, h := remoteServer(t)
	made := call(t, h, http.MethodPost, "/api/v1/invites", `{"name":"Grandpa"}`, "")
	code := jsonField(t, made.Body.Bytes(), "code")
	if strings.Contains(made.Body.String(), `"link"`) {
		t.Fatal("link without a public address")
	}
	var out struct {
		Invite store.Invite `json:"invite"`
	}
	_ = json.Unmarshal(made.Body.Bytes(), &out)
	if got := call(t, h, http.MethodDelete, "/api/v1/invites/"+out.Invite.ID, "", ""); got.Code != http.StatusOK || strings.Contains(got.Body.String(), "Grandpa") {
		t.Fatalf("delete %d %s", got.Code, got.Body.String())
	}
	if got := remoteCall(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+code+`","name":"TV","kind":"tv"}`, ""); got.Code == http.StatusCreated {
		t.Fatal("deleted invite paired a device")
	}
}

func TestTunnelClientsAreLimitedByTheirOwnAddress(t *testing.T) {
	_, h := remoteServer(t)
	try := func(peer, cf string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pair/claim", strings.NewReader(`{"code":"000000","name":"x"}`))
		req.RemoteAddr = peer
		req.Header.Set("Cf-Connecting-Ip", cf)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := range pairIPLimit {
		if got := try("127.0.0.1:1000", "203.0.113.1"); got == http.StatusTooManyRequests {
			t.Fatalf("limited early at %d", i)
		}
	}
	if got := try("127.0.0.1:1000", "203.0.113.1"); got != http.StatusTooManyRequests {
		t.Fatalf("one client past the cap %d", got)
	}
	// Another person behind the same tunnel is not locked out by the first.
	if got := try("127.0.0.1:1000", "203.0.113.2"); got == http.StatusTooManyRequests {
		t.Fatal("second client shared the first one's cap")
	}
	// A public peer cannot pick its own address with the header.
	for i := range pairIPLimit {
		try("198.51.100.7:1000", "203.0.113."+strconv.Itoa(50+i))
	}
	if got := try("198.51.100.7:1000", "203.0.113.99"); got != http.StatusTooManyRequests {
		t.Fatalf("spoofed header escaped the cap %d", got)
	}
}

func testTunnelToken() string {
	return base64.StdEncoding.EncodeToString([]byte(`{"a":"acct","t":"6ff42ae2-765d-4adf-8112-31c55c1551ef","s":"c2VjcmV0"}`))
}

func TestTunnelTokenIsSecretAndStartsCloudflared(t *testing.T) {
	api, h := remoteServer(t)
	fake := &fakeTunnel{status: tunnel.Status{State: "connected", Hostnames: []string{"ha.example.net", "tv.example.net"}, Local: "tv.example.net", Connections: 4}}
	api.Tunnel = fake
	token := testTunnelToken()
	if got := call(t, h, http.MethodPut, "/api/v1/settings", `{"tunnelToken":"not a token"}`, ""); got.Code != http.StatusBadRequest {
		t.Fatalf("bad token %d", got.Code)
	}
	saved := call(t, h, http.MethodPut, "/api/v1/settings", `{"tunnelToken":"sudo cloudflared service install `+token+`"}`, "")
	if saved.Code != http.StatusOK || strings.Contains(saved.Body.String(), token) || !strings.Contains(saved.Body.String(), `"tunnelTokenSet":"1"`) {
		t.Fatalf("save %d %s", saved.Code, saved.Body.String())
	}
	if len(fake.tokens) != 1 || fake.tokens[0] != token {
		t.Fatalf("applied %q", fake.tokens)
	}
	status := call(t, h, http.MethodGet, "/api/v1/remote", "", "")
	if !strings.Contains(status.Body.String(), `"remoteUrl":"https://tv.example.net"`) ||
		!strings.Contains(status.Body.String(), `"state":"connected"`) || !strings.Contains(status.Body.String(), `"tokenSet":true`) ||
		strings.Contains(status.Body.String(), token) {
		t.Fatalf("status %s", status.Body.String())
	}
	// The hostname cloudflared reported is enough to answer to that name.
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/health", "", ""); got.Code != http.StatusOK {
		t.Fatalf("tunnel name %d", got.Code)
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/remote", "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("remote status from outside %d", got.Code)
	}
	cleared := call(t, h, http.MethodPut, "/api/v1/settings", `{"tunnelToken":""}`, "")
	if !strings.Contains(cleared.Body.String(), `"tunnelTokenSet":"0"`) || fake.tokens[len(fake.tokens)-1] != "" {
		t.Fatalf("clear %s %q", cleared.Body.String(), fake.tokens)
	}
}
