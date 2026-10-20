package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// proxyCall sends a request the way a reverse proxy on the home network
// (Nginx Proxy Manager, SWAG, Caddy) delivers it: from the proxy's address,
// with the given Host and forwarding headers.
func proxyCall(t *testing.T, h http.Handler, method, path, host string, headers map[string]string, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Host = host
	req.RemoteAddr = "192.168.1.3:44000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var closedPaths = []string{"/api/v1/channels", "/api/v1/settings", "/api/v1/recordings", "/api/v1/support", "/export/lineup.m3u", "/export/guide.xml"}

func TestProxyForAnOutsideClientNeedsAPairedDevice(t *testing.T) {
	_, h := remoteServer(t)
	cases := map[string]map[string]string{
		"x-forwarded-for":   {"X-Forwarded-For": "203.0.113.9"},
		"x-forwarded-chain": {"X-Forwarded-For": "203.0.113.9, 192.168.1.3"},
		"x-real-ip":         {"X-Real-IP": "203.0.113.9"},
		"forwarded":         {"Forwarded": `for=203.0.113.9;proto=https`},
		"forwarded-v6":      {"Forwarded": `for="[2001:db8:cafe::17]:4711"`},
		"unreadable":        {"Forwarded": "for=unknown"},
		"x-forwarded-host":  {"X-Forwarded-Host": "tv.example.org", "X-Forwarded-For": "192.168.1.20"},
	}
	for name, headers := range cases {
		// The proxy rewrites Host to the server's own address.
		for _, path := range closedPaths {
			if got := proxyCall(t, h, http.MethodGet, path, "192.168.1.2:8477", headers, "", ""); got.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: %d", name, path, got.Code)
			}
		}
		if got := proxyCall(t, h, http.MethodGet, "/api/v1/clients/me", "192.168.1.2:8477", headers, "", ""); !strings.Contains(got.Body.String(), `"auth":"device"`) {
			t.Fatalf("%s me %s", name, got.Body.String())
		}
	}
	// What a stranger needs to start pairing stays open.
	for _, path := range []string{"/api/v1/health", "/api/v1/server", "/"} {
		if got := proxyCall(t, h, http.MethodGet, path, "192.168.1.2:8477", map[string]string{"X-Real-IP": "203.0.113.9"}, "", ""); got.Code != http.StatusOK {
			t.Fatalf("open %s %d", path, got.Code)
		}
	}
}

func TestPublicNameThroughAProxyNeedsAPairedDevice(t *testing.T) {
	api, h := remoteServer(t)
	// A browser behind an HTTPS proxy on 443, with no forwarding headers.
	browser := map[string]string{"Sec-Fetch-Site": "same-origin"}
	for _, path := range closedPaths {
		if got := proxyCall(t, h, http.MethodGet, path, "tv.example.org", browser, "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("443 proxy %s %d", path, got.Code)
		}
	}
	// A name in BROADWAVE_HOSTS, even when the proxy says the client is at home:
	// a proxy that passes the client's own X-Forwarded-For would let anyone claim it.
	api.Hosts = []string{"tv.example.org"}
	for _, path := range closedPaths {
		if got := proxyCall(t, h, http.MethodGet, path, "tv.example.org:8477", map[string]string{"X-Forwarded-For": "192.168.1.20"}, "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("listed name %s %d", path, got.Code)
		}
	}
}

func TestProxyForAHomeClientStaysOpen(t *testing.T) {
	_, h := remoteServer(t)
	for _, headers := range []map[string]string{
		{"X-Forwarded-For": "192.168.1.20", "X-Real-IP": "192.168.1.20", "X-Forwarded-Proto": "http"},
		{"X-Forwarded-For": "10.0.0.5, 172.17.0.1"},
		{"Forwarded": `for="[fd00::20]:5000"`},
		{"X-Real-IP": "100.101.102.103"}, // a tailnet
		{"X-Forwarded-For": "127.0.0.1"},
	} {
		for _, host := range []string{"192.168.1.2:8477", "broadwave.local", "tv.home.arpa", "nas.lan:8477"} {
			for _, path := range []string{"/api/v1/channels", "/export/lineup.m3u"} {
				if got := proxyCall(t, h, http.MethodGet, path, host, headers, "", ""); got.Code != http.StatusOK {
					t.Fatalf("%v %s %s: %d %s", headers, host, path, got.Code, got.Body.String())
				}
			}
		}
	}
}

func TestHomeNetworkStaysOpen(t *testing.T) {
	_, h := remoteServer(t)
	for _, peer := range []string{"192.168.1.20:5000", "10.1.2.3:5000", "127.0.0.1:5000", "[::1]:5000", "[fe80::1]:5000", "[fd12::5]:5000"} {
		for _, host := range []string{"192.168.1.2:8477", "broadwave.local:8477", "tv.home.arpa", "broadwave:8477", "[fd12::2]:8477"} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
			req.Host, req.RemoteAddr = host, peer
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s via %s: %d", peer, host, rec.Code)
			}
		}
	}
	if got := call(t, h, http.MethodGet, "/api/v1/profile", "", ""); !strings.Contains(got.Body.String(), `"auth":"local-open"`) {
		t.Fatalf("lan profile %s", got.Body.String())
	}
}

func TestAPublicPeerNeedsAPairedDevice(t *testing.T) {
	_, h := remoteServer(t)
	// A port forwarded straight to the server: no proxy, no headers.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	req.Host = "198.51.100.20:8477"
	req.RemoteAddr = "203.0.113.9:5000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("public peer %d", rec.Code)
	}
	// It cannot claim to be at home with a header.
	req.Header.Set("X-Forwarded-For", "192.168.1.20")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("public peer with a home header %d", rec.Code)
	}
}

func TestForwardingHeadersOnlyRestrict(t *testing.T) {
	_, h := remoteServer(t)
	// A home device naming an outside address locks only itself out.
	if got := proxyCall(t, h, http.MethodGet, "/api/v1/channels", "192.168.1.2:8477", map[string]string{"X-Forwarded-For": "8.8.8.8"}, "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("home device naming an outside address %d", got.Code)
	}
	// With sign-in on, naming a home address or loopback gets nothing more.
	enabled := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable %d %s", enabled.Code, enabled.Body.String())
	}
	for _, headers := range []map[string]string{
		{"X-Forwarded-For": "127.0.0.1"},
		{"X-Real-IP": "192.168.1.2"},
		{"Forwarded": "for=127.0.0.1"},
	} {
		if got := proxyCall(t, h, http.MethodGet, "/api/v1/channels", "localhost:8477", headers, "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("%v with sign-in on: %d", headers, got.Code)
		}
	}
	// An outside client that writes a home address before the proxy's own hop
	// is still outside: every hop counts.
	off := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"0"}`, jsonField(t, enabled.Body.Bytes(), "deviceToken"))
	if off.Code != http.StatusOK {
		t.Fatalf("disable %d %s", off.Code, off.Body.String())
	}
	if got := proxyCall(t, h, http.MethodGet, "/api/v1/channels", "192.168.1.2:8477", map[string]string{"X-Forwarded-For": "192.168.1.20, 203.0.113.9"}, "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed first hop %d", got.Code)
	}
}

func TestCloudflareRequestsAreUnchanged(t *testing.T) {
	_, h := remoteServer(t)
	call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"https://tv.example.net"}`, "")
	for _, path := range closedPaths {
		if got := remoteCall(t, h, http.MethodGet, path, "", ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("cloudflare %s %d", path, got.Code)
		}
	}
	if got := remoteCall(t, h, http.MethodGet, "/api/v1/health", "", ""); got.Code != http.StatusOK {
		t.Fatalf("cloudflare health %d", got.Code)
	}
}

func TestInvitePairsADeviceThroughAProxy(t *testing.T) {
	_, h := remoteServer(t)
	call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"https://tv.example.org"}`, "")
	made := call(t, h, http.MethodPost, "/api/v1/invites", `{"name":"Mom","scopes":["watch"]}`, "")
	if made.Code != http.StatusCreated || !strings.Contains(made.Body.String(), `"link":"https://tv.example.org/pair?invite=`) {
		t.Fatalf("invite %d %s", made.Code, made.Body.String())
	}
	code := jsonField(t, made.Body.Bytes(), "code")
	outside := map[string]string{"X-Forwarded-For": "203.0.113.9", "X-Forwarded-Proto": "https", "Sec-Fetch-Site": "same-origin"}
	if got := proxyCall(t, h, http.MethodGet, "/api/v1/remote", "tv.example.org", outside, "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("remote status without a device %d", got.Code)
	}
	claimed := proxyCall(t, h, http.MethodPost, "/api/v1/pair/claim", "tv.example.org", outside, `{"code":"`+code+`","name":"Laptop","kind":"web"}`, "")
	if claimed.Code != http.StatusCreated {
		t.Fatalf("claim %d %s", claimed.Code, claimed.Body.String())
	}
	if c := claimed.Header().Get("Set-Cookie"); !strings.Contains(c, "Secure") {
		t.Fatalf("cookie over https without Secure: %q", c)
	}
	if cc := claimed.Header().Get("Cache-Control"); !strings.Contains(cc, "private") && !strings.Contains(cc, "no-store") {
		t.Fatalf("outside answer is shareable: %q", cc)
	}
	token := jsonField(t, claimed.Body.Bytes(), "token")
	if got := proxyCall(t, h, http.MethodGet, "/api/v1/channels", "tv.example.org", outside, "", token); got.Code != http.StatusOK {
		t.Fatalf("paired channels %d", got.Code)
	}
	if got := proxyCall(t, h, http.MethodGet, "/export/lineup.m3u?access_token="+token, "tv.example.org", outside, "", ""); got.Code != http.StatusOK {
		t.Fatalf("paired export %d", got.Code)
	}
	if got := proxyCall(t, h, http.MethodGet, "/api/v1/settings", "tv.example.org", outside, "", token); got.Code != http.StatusForbidden {
		t.Fatalf("watch token read settings %d", got.Code)
	}
}

func TestProxyClientsAreLimitedByTheirOwnAddress(t *testing.T) {
	_, h := remoteServer(t)
	try := func(client string) int {
		return proxyCall(t, h, http.MethodPost, "/api/v1/pair/claim", "192.168.1.2:8477", map[string]string{"X-Real-IP": client}, `{"code":"000000","name":"x"}`, "").Code
	}
	for i := range pairIPLimit {
		if got := try("203.0.113.1"); got == http.StatusTooManyRequests {
			t.Fatalf("limited early at %d", i)
		}
	}
	if got := try("203.0.113.1"); got != http.StatusTooManyRequests {
		t.Fatalf("one client past the cap %d", got)
	}
	if got := try("203.0.113.2"); got == http.StatusTooManyRequests {
		t.Fatal("second client behind the proxy shared the first one's cap")
	}
}

func TestTailnetNamesStayHome(t *testing.T) {
	api, h := remoteServer(t)
	api.Hosts = []string{"tv.tail1234.ts.net"}
	send := func(host, peer string, headers map[string]string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
		req.Host, req.RemoteAddr = host, peer
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	open := []struct {
		name, host, peer string
		headers          map[string]string
	}{
		{"magicdns", "tv.tail1234.ts.net:8477", "100.101.102.103:5000", nil},
		{"magicdns v6", "tv.tail1234.ts.net:8477", "[fd7a:115c:a1e0::9]:5000", nil},
		{"serve", "tv.tail1234.ts.net", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "100.101.102.103", "Sec-Fetch-Site": "same-origin"}},
		{"serve v6", "tv.tail1234.ts.net", "[::1]:5000", map[string]string{"X-Forwarded-For": "fd7a:115c:a1e0::9", "Sec-Fetch-Site": "same-origin"}},
	}
	for _, c := range open {
		if got := send(c.host, c.peer, c.headers); got != http.StatusOK {
			t.Fatalf("%s: %d", c.name, got)
		}
	}
	closed := []struct {
		name, host, peer string
		headers          map[string]string
	}{
		{"funnel", "tv.tail1234.ts.net", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "203.0.113.9", "Sec-Fetch-Site": "same-origin"}},
		{"funnel header", "tv.tail1234.ts.net", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "100.101.102.103", "Tailscale-Funnel-Request": "?1", "Sec-Fetch-Site": "same-origin"}},
		{"serve without a client", "tv.tail1234.ts.net", "127.0.0.1:5000", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"lan peer", "tv.tail1234.ts.net:8477", "192.168.1.20:5000", nil},
		{"tailnet peer naming an outsider", "tv.tail1234.ts.net:8477", "100.101.102.103:5000", map[string]string{"X-Forwarded-For": "203.0.113.9"}},
		{"tailnet peer, other public name", "tv.example.org", "100.101.102.103:5000", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"ts.net forwarded host from the lan", "192.168.1.2:8477", "192.168.1.3:5000", map[string]string{"X-Forwarded-Host": "tv.tail1234.ts.net", "X-Forwarded-For": "192.168.1.20"}},
	}
	for _, c := range closed {
		if got := send(c.host, c.peer, c.headers); got != http.StatusUnauthorized {
			t.Fatalf("%s: %d", c.name, got)
		}
	}
}
