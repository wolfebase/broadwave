// Package cftest is a fake Cloudflare for tests and the local harness: the
// sign-in page, the login service that holds the certificate, and the parts
// of the API setup uses. It never talks to Cloudflare.
package cftest

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Fake serves the dash page at /argotunnel, the login service under /login,
// and the API under /client/v4.
type Fake struct {
	Account  string
	ZoneID   string
	ZoneName string
	APIToken string
	// NoZone hands out a certificate without a zone.
	NoZone bool
	// ZoneHidden refuses the zone lookup, as a token that may not read zones.
	ZoneHidden bool
	// ConfigRefused refuses remotely managed routes.
	ConfigRefused bool
	// DNSRefused refuses DNS record reads, so a removed name stays.
	DNSRefused bool
	// TunnelsRefused refuses making tunnels, as a sign-in for a zone in an
	// account the owner may not change.
	TunnelsRefused bool
	// LoginDown answers the login poll with 503.
	LoginDown bool
	// Hold is how long a poll waits before 404. Zero answers at once.
	Hold time.Duration
	// AuthorizeAfter authorizes a key on its Nth poll. Zero waits for Authorize.
	AuthorizeAfter int

	mu      sync.Mutex
	polls   map[string]int
	ready   map[string]bool
	wake    chan struct{}
	tunnels map[string]*tunnel
	dns     map[string]record
	nextID  int
	// Calls lists "METHOD path" for every API call, in order.
	Calls []string
}

type tunnel struct {
	ID     string
	Name   string
	Remote bool
	Config string
}

type record struct {
	ID      string
	Content string
}

// New is a fake with one account and the zone example.com.
func New() *Fake {
	return &Fake{
		Account:  "acct1",
		ZoneID:   "zone1",
		ZoneName: "example.com",
		APIToken: "cert-api-token",
		polls:    map[string]int{},
		ready:    map[string]bool{},
		wake:     make(chan struct{}),
		tunnels:  map[string]*tunnel{},
		dns:      map[string]record{},
	}
}

// Authorize marks a sign-in key as authorized, as the owner clicking
// Authorize does.
func (f *Fake) Authorize(key string) {
	f.mu.Lock()
	f.ready[key] = true
	close(f.wake)
	f.wake = make(chan struct{})
	f.mu.Unlock()
}

// AddDNS puts a record in the zone that points elsewhere.
func (f *Fake) AddDNS(name, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.dns[strings.ToLower(name)] = record{ID: fmt.Sprintf("rec%d", f.nextID), Content: content}
}

// AddTunnel puts a tunnel in the account, as an earlier setup left it.
func (f *Fake) AddTunnel(name string, remote bool) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.nextID)
	f.tunnels[id] = &tunnel{ID: id, Name: name, Remote: remote}
	return id
}

// DNS returns where name points, or "".
func (f *Fake) DNS(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dns[strings.ToLower(name)].Content
}

// Tunnels returns the live tunnels' names.
func (f *Fake) Tunnels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, t := range f.tunnels {
		out = append(out, t.Name)
	}
	return out
}

// Tunnel returns one live tunnel by name.
func (f *Fake) Tunnel(name string) (id string, remote bool, config string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tunnels {
		if t.Name == name {
			return t.ID, t.Remote, t.Config, true
		}
	}
	return "", false, "", false
}

// Polls is how many times key was polled.
func (f *Fake) Polls(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls[key]
}

// Cert is the PEM the login service hands out.
func (f *Fake) Cert() []byte {
	zone := f.ZoneID
	if f.NoZone {
		zone = ""
	}
	body, _ := json.Marshal(map[string]string{"zoneID": zone, "accountID": f.Account, "apiToken": f.APIToken})
	return pem.EncodeToMemory(&pem.Block{Type: "ARGO TUNNEL TOKEN", Bytes: body})
}

// RunToken is the token the fake hands out for a tunnel.
func RunToken(account, id string) string {
	body, _ := json.Marshal(map[string]string{"a": account, "t": id, "s": base64.StdEncoding.EncodeToString([]byte("secret-" + id))})
	return base64.StdEncoding.EncodeToString(body)
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/argotunnel" && r.Method == http.MethodGet:
		f.page(w, r)
	case r.URL.Path == "/argotunnel/authorize" && r.Method == http.MethodPost:
		f.authorizeForm(w, r)
	case strings.HasPrefix(r.URL.Path, "/login/"):
		f.poll(w, r, strings.TrimPrefix(r.URL.Path, "/login/"))
	case strings.HasPrefix(r.URL.Path, "/client/v4/"):
		f.api(w, r, strings.TrimPrefix(r.URL.Path, "/client/v4"))
	default:
		http.NotFound(w, r)
	}
}

func keyOf(callback string) string {
	u, err := url.Parse(callback)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/login/")
}

func (f *Fake) page(w http.ResponseWriter, r *http.Request) {
	cb := r.URL.Query().Get("callback")
	zone := f.ZoneName
	if f.NoZone {
		zone = "(no domains in this account)"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><title>Fake Cloudflare</title>
<body style="font-family:system-ui;background:#f4f4f4;padding:40px">
<h1>Authorize Cloudflare Tunnel (fake)</h1>
<p>Zone: <b>%s</b></p>
<form method="post" action="/argotunnel/authorize">
<input type="hidden" name="callback" value="%s">
<button type="submit" style="font-size:18px;padding:10px 20px">Authorize</button>
</form></body>`, html.EscapeString(zone), html.EscapeString(cb))
}

func (f *Fake) authorizeForm(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.Authorize(keyOf(r.FormValue("callback")))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><body style="font-family:system-ui;padding:40px"><h1>Success</h1><p>You can close this tab.</p></body>`)
}

func (f *Fake) poll(w http.ResponseWriter, r *http.Request, key string) {
	f.mu.Lock()
	f.polls[key]++
	if f.AuthorizeAfter > 0 && f.polls[key] >= f.AuthorizeAfter {
		f.ready[key] = true
	}
	ready, down, wake, hold := f.ready[key], f.LoginDown, f.wake, f.Hold
	f.mu.Unlock()
	if down {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ready && hold > 0 {
		select {
		case <-wake:
		case <-time.After(hold):
		case <-r.Context().Done():
			return
		}
		f.mu.Lock()
		ready = f.ready[key]
		f.mu.Unlock()
	}
	if !ready {
		http.Error(w, "cert not found", http.StatusNotFound)
		return
	}
	_, _ = w.Write(f.Cert())
}

func ok(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result})
}

func fail(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []any{map[string]any{"code": code, "message": msg}}, "result": nil})
}

func (f *Fake) api(w http.ResponseWriter, r *http.Request, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, r.Method+" "+path)
	if r.Header.Get("Authorization") != "Bearer "+f.APIToken {
		fail(w, http.StatusForbidden, 10000, "Authentication error")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "zones":
		if f.ZoneHidden {
			fail(w, http.StatusForbidden, 9109, "Unauthorized to access requested resource")
			return
		}
		ok(w, map[string]string{"id": parts[1], "name": f.ZoneName})
	case len(parts) == 3 && parts[0] == "accounts" && parts[2] == "cfd_tunnel":
		if r.Method == http.MethodGet {
			name := r.URL.Query().Get("name")
			list := []map[string]any{}
			for _, t := range f.tunnels {
				if name == "" || t.Name == name {
					list = append(list, map[string]any{"id": t.ID, "name": t.Name, "remote_config": t.Remote})
				}
			}
			ok(w, list)
			return
		}
		if f.TunnelsRefused {
			fail(w, http.StatusForbidden, 10000, "Authentication error")
			return
		}
		var body struct {
			Name      string `json:"name"`
			Secret    []byte `json:"tunnel_secret"`
			ConfigSrc string `json:"config_src"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" || len(body.Secret) < 32 {
			fail(w, http.StatusBadRequest, 1001, "bad tunnel")
			return
		}
		for _, t := range f.tunnels {
			if t.Name == body.Name {
				fail(w, http.StatusConflict, 1013, "You already have a tunnel with this name")
				return
			}
		}
		f.nextID++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.nextID)
		t := &tunnel{ID: id, Name: body.Name, Remote: body.ConfigSrc == "cloudflare"}
		f.tunnels[id] = t
		ok(w, map[string]any{"id": id, "name": t.Name, "remote_config": t.Remote})
	case len(parts) == 5 && parts[0] == "accounts" && parts[4] == "configurations" && r.Method == http.MethodPut:
		t := f.tunnels[parts[3]]
		if t == nil {
			fail(w, http.StatusNotFound, 1003, "tunnel not found")
			return
		}
		if f.ConfigRefused {
			fail(w, http.StatusForbidden, 10000, "Authentication error")
			return
		}
		var body json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		t.Config = string(body)
		ok(w, map[string]any{"tunnel_id": t.ID})
	case len(parts) == 5 && parts[0] == "accounts" && parts[4] == "token" && r.Method == http.MethodGet:
		t := f.tunnels[parts[3]]
		if t == nil {
			fail(w, http.StatusNotFound, 1003, "tunnel not found")
			return
		}
		ok(w, RunToken(f.Account, t.ID))
	case len(parts) == 4 && parts[0] == "accounts" && parts[2] == "cfd_tunnel" && r.Method == http.MethodDelete:
		if f.tunnels[parts[3]] == nil {
			fail(w, http.StatusNotFound, 1003, "tunnel not found")
			return
		}
		delete(f.tunnels, parts[3])
		ok(w, map[string]any{"id": parts[3]})
	case len(parts) == 5 && parts[0] == "zones" && parts[2] == "tunnels" && parts[4] == "routes" && r.Method == http.MethodPut:
		var body struct {
			Type      string `json:"type"`
			Host      string `json:"user_hostname"`
			Overwrite bool   `json:"overwrite_existing"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := strings.ToLower(body.Host)
		if !strings.HasSuffix(name, "."+f.ZoneName) {
			fail(w, http.StatusBadRequest, 1001, "hostname is not in this zone")
			return
		}
		target := parts[3] + ".cfargotunnel.com"
		change := "new"
		if rec, exists := f.dns[name]; exists {
			if rec.Content == target {
				change = "unchanged"
			} else if !body.Overwrite {
				fail(w, http.StatusBadRequest, 1003, "Failed to create record "+name+" with err An A, AAAA, or CNAME record with that host already exists.")
				return
			}
		}
		f.nextID++
		f.dns[name] = record{ID: fmt.Sprintf("rec%d", f.nextID), Content: target}
		ok(w, map[string]string{"cname": change, "name": name})
	case len(parts) == 3 && parts[0] == "zones" && parts[2] == "dns_records" && r.Method == http.MethodGet:
		if f.DNSRefused {
			fail(w, http.StatusForbidden, 10000, "Authentication error")
			return
		}
		name := strings.ToLower(r.URL.Query().Get("name"))
		list := []map[string]string{}
		if rec, exists := f.dns[name]; exists {
			list = append(list, map[string]string{"id": rec.ID, "name": name, "content": rec.Content})
		}
		ok(w, list)
	case len(parts) == 4 && parts[0] == "zones" && parts[2] == "dns_records" && r.Method == http.MethodDelete:
		for name, rec := range f.dns {
			if rec.ID == parts[3] {
				delete(f.dns, name)
				ok(w, map[string]string{"id": rec.ID})
				return
			}
		}
		fail(w, http.StatusNotFound, 81044, "Record does not exist.")
	default:
		fail(w, http.StatusNotFound, 7003, "Could not route to "+path)
	}
}
