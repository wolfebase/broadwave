package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/direct"
	"broadwave/internal/store"
)

type fakeDirect struct {
	host, url string
	on        bool
}

func (f *fakeDirect) Host() string { return f.host }
func (f *fakeDirect) URL() string {
	if !f.on {
		return ""
	}
	return f.url
}

func (f *fakeDirect) Status() direct.Status {
	st := direct.Status{Enabled: f.on, State: "ready", Name: f.host, Addresses: []string{"192.168.1.2"}}
	if !f.on {
		st.State = "off"
	}
	st.URL = f.URL()
	return st
}

func (f *fakeDirect) SetEnabled(on bool) error {
	f.on = on
	return nil
}

func directServer(t *testing.T) (*Server, *fakeDirect) {
	st := testStore(t)
	d := &fakeDirect{host: "lan.abcdefghij.direct.wolfeup.com", url: "https://lan.abcdefghij.direct.wolfeup.com:8443", on: true}
	return &Server{Store: st, Version: "dev", Direct: d}, d
}

func hostStatus(h http.Handler, host string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestOwnSecureNameIsAnswered(t *testing.T) {
	api, _ := directServer(t)
	h := api.Handler()
	for _, host := range []string{"lan.abcdefghij.direct.wolfeup.com:8443", "LAN.abcdefghij.direct.wolfeup.com.:8443", "lan.abcdefghij.direct.wolfeup.com"} {
		if code := hostStatus(h, host); code != http.StatusOK {
			t.Fatalf("%s: %d", host, code)
		}
	}
	// Another server's name under the same domain may point here too. It is
	// not this server's, so it is refused like any rebinding name.
	for _, host := range []string{"lan.zzzzzzzzzz.direct.wolfeup.com:8443", "abcdefghij.direct.wolfeup.com:8443", "x.lan.abcdefghij.direct.wolfeup.com"} {
		if code := hostStatus(h, host); code != http.StatusMisdirectedRequest {
			t.Fatalf("%s: %d", host, code)
		}
	}
	// Without the service nothing under the domain is answered.
	bare := &Server{Store: api.Store, Version: "dev"}
	if code := hostStatus(bare.Handler(), "lan.abcdefghij.direct.wolfeup.com:8443"); code != http.StatusMisdirectedRequest {
		t.Fatalf("no service: %d", code)
	}
}

func TestSecureNameIsHome(t *testing.T) {
	api, _ := directServer(t)
	ctx := context.Background()
	if err := api.Store.PutSettings(ctx, map[string]string{store.SettingDeviceAuth: "0"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	req.Host = "lan.abcdefghij.direct.wolfeup.com:8443"
	if api.isRemote(req) {
		t.Fatal("the secure name counted as away from home")
	}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("home request by the secure name: %d %s", rec.Code, rec.Body.String())
	}
}

func TestServerInfoOffersTheSecureURL(t *testing.T) {
	api, d := directServer(t)
	var info map[string]any
	_ = json.Unmarshal(get(t, api.Handler(), "/api/v1/server").Body.Bytes(), &info)
	if info["secureUrl"] != "https://lan.abcdefghij.direct.wolfeup.com:8443" {
		t.Fatalf("secureUrl %v", info["secureUrl"])
	}
	d.on = false
	info = nil
	_ = json.Unmarshal(get(t, api.Handler(), "/api/v1/server").Body.Bytes(), &info)
	if _, ok := info["secureUrl"]; ok {
		t.Fatalf("secureUrl while off: %v", info["secureUrl"])
	}
}

func TestDirectSwitch(t *testing.T) {
	api, d := directServer(t)
	h := api.Handler()
	var st direct.Status
	_ = json.Unmarshal(get(t, h, "/api/v1/direct").Body.Bytes(), &st)
	if st.State != "ready" || st.URL == "" {
		t.Fatalf("status %+v", st)
	}
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/direct", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := put(`{"enabled":false}`); rec.Code != http.StatusOK || d.on {
		t.Fatalf("off: %d %v", rec.Code, d.on)
	}
	if rec := put(`{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty: %d", rec.Code)
	}
	bare := &Server{Store: api.Store, Version: "dev"}
	_ = json.Unmarshal(get(t, bare.Handler(), "/api/v1/direct").Body.Bytes(), &st)
	if st.State != "unavailable" || !strings.Contains(st.Reason, "BROADWAVE_DIRECT=0") {
		t.Fatalf("no service: %+v", st)
	}
	bridged := &Server{Store: api.Store, Version: "dev", DirectOff: "The secure name is off: the container is on a bridge network."}
	st = direct.Status{}
	_ = json.Unmarshal(get(t, bridged.Handler(), "/api/v1/direct").Body.Bytes(), &st)
	if st.State != "unavailable" || st.Reason != bridged.DirectOff {
		t.Fatalf("bridged: %+v", st)
	}
}

func TestDirectNeedsAdminWhenSignInIsOn(t *testing.T) {
	if got := requiredScope(http.MethodGet, "/api/v1/direct"); got != store.ScopeAdmin {
		t.Fatalf("GET scope %q", got)
	}
	if got := requiredScope(http.MethodPut, "/api/v1/direct"); got != store.ScopeAdmin {
		t.Fatalf("PUT scope %q", got)
	}
}
