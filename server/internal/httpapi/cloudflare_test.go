package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"broadwave/internal/cloudflare"
	"broadwave/internal/cloudflare/cftest"
	"broadwave/internal/store"
	"broadwave/internal/tunnel"
)

type cfRig struct {
	api    *Server
	h      http.Handler
	fake   *cftest.Fake
	runner *fakeTunnel
	dir    string
}

func cloudflareServer(t *testing.T, change func(*cftest.Fake)) cfRig {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake := cftest.New()
	fake.AuthorizeAfter = 2
	if change != nil {
		change(fake)
	}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	runner := &fakeTunnel{status: tunnel.Status{State: "connected", Connections: 4}}
	api := &Server{
		Store:   st,
		Assets:  fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}},
		Version: "dev",
		Tunnel:  runner,
		Vault:   cloudflare.NewVault(dir),
		Origin:  "http://127.0.0.1:18477",
	}
	m := &cloudflare.Manager{
		Endpoints: cloudflare.Endpoints{Dash: srv.URL, Login: srv.URL + "/login", API: srv.URL + "/client/v4"},
		Retry:     10 * time.Millisecond,
	}
	api.UseCloudflare(m)
	t.Cleanup(m.Cancel)
	return cfRig{api: api, h: api.Handler(), fake: fake, runner: runner, dir: dir}
}

func cfState(t *testing.T, rec *httptest.ResponseRecorder) cloudflareView {
	t.Helper()
	var v cloudflareView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	return v
}

func (c cfRig) waitFor(t *testing.T, want string) cloudflareView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		v := cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", ""))
		if v.State == want {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %q, want %q: %+v", v.State, want, v)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c cfRig) turnOn(t *testing.T, label string) cloudflareView {
	t.Helper()
	start := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/login", "", "")
	if v := cfState(t, start); start.Code != http.StatusOK || v.State != "waiting" || v.LoginURL == "" {
		t.Fatalf("login %d %s", start.Code, start.Body.String())
	}
	c.waitFor(t, "authorized")
	got := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/setup", `{"label":"`+label+`"}`, "")
	if got.Code != http.StatusAccepted {
		t.Fatalf("setup %d %s", got.Code, got.Body.String())
	}
	return c.waitFor(t, "connected")
}

func TestSignInWithCloudflareTurnsOnAwayFromHome(t *testing.T) {
	c := cloudflareServer(t, nil)
	if v := cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", "")); v.State != "idle" || v.Managed {
		t.Fatalf("fresh %+v", v)
	}
	v := c.turnOn(t, "tv")
	if !v.Managed || v.Hostname != "tv.example.com" || v.Tunnel.State != "connected" {
		t.Fatalf("connected %+v", v)
	}
	id, remote, _, _ := c.fake.Tunnel("broadwave-" + serverPrefix(t, c.api))
	if id == "" || !remote {
		t.Fatalf("tunnel %v %v", c.fake.Tunnels(), remote)
	}
	token := cftest.RunToken("acct1", id)
	last := len(c.runner.tokens) - 1
	if last < 0 || c.runner.tokens[last] != token || c.runner.origins[last] != "" {
		t.Fatalf("runner %q %q", c.runner.tokens, c.runner.origins)
	}
	remoteNow := call(t, c.h, http.MethodGet, "/api/v1/remote", "", "").Body.String()
	if !strings.Contains(remoteNow, `"remoteUrl":"https://tv.example.com"`) || strings.Contains(remoteNow, token) || !strings.Contains(remoteNow, `"tokenSet":false`) {
		t.Fatalf("remote %s", remoteNow)
	}
	settings := call(t, c.h, http.MethodGet, "/api/v1/settings", "", "").Body.String()
	if strings.Contains(settings, token) || !strings.Contains(settings, `"tunnelTokenSet":"1"`) {
		t.Fatalf("settings %s", settings)
	}
	values, _ := c.api.Store.Settings(context.Background())
	for k, v := range values {
		if strings.Contains(v, token) {
			t.Fatalf("token in the catalog under %s", k)
		}
	}
	saved, err := c.api.Vault.Load()
	if err != nil || saved.Token != token || saved.Routes != "cloudflare" {
		t.Fatalf("vault %+v %v", saved, err)
	}
	// A second setup is refused until it is turned off.
	if got := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/login", "", ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "already_on") {
		t.Fatalf("again %d %s", got.Code, got.Body.String())
	}

	// Turning off here keeps the tunnel in Cloudflare and says the name stays.
	off := call(t, c.h, http.MethodDelete, "/api/v1/remote/cloudflare", "", "")
	ov := cfState(t, off)
	if off.Code != http.StatusOK || ov.State != "idle" || ov.Managed || ov.DNSLeft != "tv.example.com" {
		t.Fatalf("off %d %s", off.Code, off.Body.String())
	}
	if c.runner.tokens[len(c.runner.tokens)-1] != "" {
		t.Fatalf("runner still has %q", c.runner.tokens)
	}
	if s, _ := c.api.Vault.Load(); s.Token != "" {
		t.Fatal("vault kept the token")
	}
	if !strings.Contains(call(t, c.h, http.MethodGet, "/api/v1/remote", "", "").Body.String(), `"remoteUrl":""`) {
		t.Fatal("public address kept")
	}
	if len(c.fake.Tunnels()) != 1 {
		t.Fatal("turning off here deleted the tunnel")
	}
	// Removing in Cloudflare needs a fresh sign-in.
	if got := call(t, c.h, http.MethodDelete, "/api/v1/remote/cloudflare?cloudflare=1", "", ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "not_signed_in") {
		t.Fatalf("remove without sign-in %d %s", got.Code, got.Body.String())
	}
}

func serverPrefix(t *testing.T, api *Server) string {
	t.Helper()
	name, err := api.tunnelName(context.Background())
	if err != nil || len(name) != len("broadwave-")+6 {
		t.Fatalf("name %q %v", name, err)
	}
	return strings.TrimPrefix(name, "broadwave-")
}

func TestLocalRoutesRunWithTheServersAddress(t *testing.T) {
	c := cloudflareServer(t, func(f *cftest.Fake) { f.ConfigRefused = true })
	c.turnOn(t, "tv")
	last := len(c.runner.tokens) - 1
	if c.runner.origins[last] != "http://127.0.0.1:18477" {
		t.Fatalf("origin %q", c.runner.origins)
	}
	// A restart runs it the same way.
	c.runner.tokens, c.runner.origins = nil, nil
	c.api.StartTunnel(context.Background())
	if len(c.runner.origins) != 1 || c.runner.origins[0] != "http://127.0.0.1:18477" || c.runner.tokens[0] == "" {
		t.Fatalf("restart %q %q", c.runner.tokens, c.runner.origins)
	}
}

func TestRemoveWithAFreshSignInDeletesTheTunnel(t *testing.T) {
	c := cloudflareServer(t, nil)
	c.turnOn(t, "tv")
	start := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/login", `{"purpose":"remove"}`, "")
	if start.Code != http.StatusOK {
		t.Fatalf("remove sign-in %d %s", start.Code, start.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(c.fake.Tunnels()) != 0 || cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", "")).State != "idle" {
		if time.Now().After(deadline) {
			t.Fatalf("not removed %v", c.fake.Tunnels())
		}
		time.Sleep(5 * time.Millisecond)
	}
	v := cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", ""))
	if v.Managed || v.DNSLeft != "" || c.fake.DNS("tv.example.com") != "" {
		t.Fatalf("after remove %+v dns %q", v, c.fake.DNS("tv.example.com"))
	}
}

func TestCloudflareSetupErrorsComeBackWithCodes(t *testing.T) {
	c := cloudflareServer(t, func(f *cftest.Fake) { f.AddDNS("tv.example.com", "203.0.113.10") })
	if got := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/setup", `{"label":"tv"}`, ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "not_signed_in") {
		t.Fatalf("setup before sign-in %d %s", got.Code, got.Body.String())
	}
	call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/login", "", "")
	c.waitFor(t, "authorized")
	if got := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/setup", `{"label":"t v"}`, ""); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "bad_name") {
		t.Fatalf("bad label %d %s", got.Code, got.Body.String())
	}
	call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/setup", `{"label":"tv"}`, "")
	deadline := time.Now().Add(5 * time.Second)
	var v cloudflareView
	for {
		v = cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", ""))
		if v.State == "authorized" && v.Error != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no error %+v", v)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if v.Error.Code != "name_taken" || v.Managed {
		t.Fatalf("taken %+v", v)
	}
	if got := call(t, c.h, http.MethodPost, "/api/v1/remote/cloudflare/setup", `{"label":"tv","overwrite":true}`, ""); got.Code != http.StatusAccepted {
		t.Fatalf("replace %d %s", got.Code, got.Body.String())
	}
	c.waitFor(t, "connected")
	// Cancel clears a finished sign-in's leftovers and keeps the tunnel.
	cancel := cfState(t, call(t, c.h, http.MethodDelete, "/api/v1/remote/cloudflare/login", "", ""))
	if cancel.State != "connected" {
		t.Fatalf("cancel %+v", cancel)
	}
}

func TestCloudflareIsAdminAndHomeOnly(t *testing.T) {
	c := cloudflareServer(t, nil)
	turned := call(t, c.h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	admin := jsonField(t, turned.Body.Bytes(), "deviceToken")
	if err := c.api.Store.PutSettings(context.Background(), map[string]string{store.SettingRemoteURL: "https://tv.example.net"}); err != nil {
		t.Fatal(err)
	}
	c.api.forgetRemote()
	routes := [][2]string{
		{http.MethodGet, "/api/v1/remote/cloudflare"},
		{http.MethodPost, "/api/v1/remote/cloudflare/login"},
		{http.MethodDelete, "/api/v1/remote/cloudflare/login"},
		{http.MethodPost, "/api/v1/remote/cloudflare/setup"},
		{http.MethodDelete, "/api/v1/remote/cloudflare"},
	}
	for _, rt := range routes {
		body := ""
		if rt[0] == http.MethodPost {
			body = `{"label":"tv"}`
		}
		if got := call(t, c.h, rt[0], rt[1], body, ""); got.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a device %d", rt[0], rt[1], got.Code)
		}
		if got := remoteCall(t, c.h, rt[0], rt[1], body, admin); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "not_home") {
			t.Fatalf("%s %s from outside %d %s", rt[0], rt[1], got.Code, got.Body.String())
		}
	}
	if got := call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", admin); got.Code != http.StatusOK {
		t.Fatalf("admin at home %d", got.Code)
	}
	if v := cfState(t, call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", admin)); v.State != "idle" {
		t.Fatalf("an outside request started something: %+v", v)
	}
}

func TestPastedTokenLivesInTheSecretsFileNotTheCatalog(t *testing.T) {
	c := cloudflareServer(t, nil)
	token := testTunnelToken()
	if got := call(t, c.h, http.MethodPut, "/api/v1/settings", `{"tunnelToken":"cloudflared service install `+token+`"}`, ""); got.Code != http.StatusOK {
		t.Fatalf("save %d %s", got.Code, got.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(c.dir, "secrets", "cloudflare.json"))
	if err != nil || !strings.Contains(string(raw), token) || !strings.Contains(string(raw), `"source": "pasted"`) {
		t.Fatalf("secrets file %s %v", raw, err)
	}
	fi, _ := os.Stat(filepath.Join(c.dir, "secrets", "cloudflare.json"))
	di, _ := os.Stat(filepath.Join(c.dir, "secrets"))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatalf("modes %v %v", fi.Mode(), di.Mode())
	}
	if c.runner.tokens[len(c.runner.tokens)-1] != token || c.runner.origins[len(c.runner.origins)-1] != "" {
		t.Fatalf("runner %q %q", c.runner.tokens, c.runner.origins)
	}
	// A catalog backup does not carry it.
	backup := filepath.Join(t.TempDir(), "catalog.db")
	if err := c.api.Store.BackupTo(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(backup)
	if err != nil || len(copied) == 0 {
		t.Fatal(err)
	}
	if bytes.Contains(copied, []byte(token)) {
		t.Fatal("the backup holds the tunnel token")
	}
	if got := call(t, c.h, http.MethodGet, "/api/v1/remote", "", "").Body.String(); !strings.Contains(got, `"tokenSet":true`) {
		t.Fatalf("remote %s", got)
	}
	if got := call(t, c.h, http.MethodGet, "/api/v1/remote/cloudflare", "", "").Body.String(); !strings.Contains(got, `"state":"idle"`) || !strings.Contains(got, `"managed":false`) {
		t.Fatalf("a pasted token is not a Broadwave tunnel: %s", got)
	}
}

// putCatalogToken writes a token the way an older version did.
func putCatalogToken(t *testing.T, dbPath, token string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('tunnelToken', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, token); err != nil {
		t.Fatal(err)
	}
}

func TestAnOldCatalogTokenMovesToTheSecretsFileAtStart(t *testing.T) {
	c := cloudflareServer(t, nil)
	token := testTunnelToken()
	putCatalogToken(t, filepath.Join(c.dir, "broadwave.db"), token)
	c.api.StartTunnel(context.Background())
	saved, err := c.api.Vault.Load()
	if err != nil || saved.Token != token || saved.Source != cloudflare.SourcePasted {
		t.Fatalf("vault %+v %v", saved, err)
	}
	values, _ := c.api.Store.Settings(context.Background())
	if _, ok := values[store.SettingTunnelToken]; ok {
		t.Fatal("the catalog kept the token")
	}
	if c.runner.tokens[len(c.runner.tokens)-1] != token {
		t.Fatalf("runner %q", c.runner.tokens)
	}
}

func TestARestoredBackupDoesNotStartATunnel(t *testing.T) {
	c := cloudflareServer(t, nil)
	// An older backup that still has a token.
	backup := filepath.Join(t.TempDir(), "old.db")
	if err := c.api.Store.BackupTo(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	token := testTunnelToken()
	putCatalogToken(t, backup, token)
	if err := c.api.Store.RestoreFrom(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	c.api.applyTunnel(context.Background())
	for _, got := range c.runner.tokens {
		if got != "" {
			t.Fatalf("restore started a tunnel with %q", got)
		}
	}
	values, _ := c.api.Store.Settings(context.Background())
	if _, ok := values[store.SettingTunnelToken]; ok {
		t.Fatal("the restored token stayed in the catalog")
	}
	if s, _ := c.api.Vault.Load(); s.Token != "" {
		t.Fatal("the restored token was saved")
	}
}

func TestLoopbackOrigin(t *testing.T) {
	for addr, want := range map[string]string{
		":8477":           "http://127.0.0.1:8477",
		"0.0.0.0:9000":    "http://127.0.0.1:9000",
		"[::]:8477":       "http://127.0.0.1:8477",
		"127.0.0.1:18970": "http://127.0.0.1:18970",
		"192.168.1.5:80":  "http://192.168.1.5:80",
		"bad":             "http://127.0.0.1:8477",
	} {
		if got := LoopbackOrigin(addr); got != want {
			t.Errorf("LoopbackOrigin(%q) = %q, want %q", addr, got, want)
		}
	}
}
