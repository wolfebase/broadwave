package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"broadwave/internal/store"
)

func TestDeviceAuthStaysOffUntilAsked(t *testing.T) {
	h := (&Server{Store: testStore(t), Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}}}).Handler()
	res := get(t, h, "/api/v1/settings")
	if !strings.Contains(res.Body.String(), `"deviceAuth":"0"`) {
		t.Fatalf("settings %s", res.Body.String())
	}
	if res.Code != http.StatusOK {
		t.Fatal(res.Code)
	}
	channels := get(t, h, "/api/v1/channels")
	if !strings.Contains(channels.Body.String(), `"channels"`) {
		t.Fatalf("%s", channels.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/media/live/missing", "", "").Code; got != http.StatusNotFound && got != http.StatusBadRequest && got != http.StatusOK {
		// Sign-in is off, so a missing stream is not a 401.
		if got == http.StatusUnauthorized {
			t.Fatal("media required a token while sign-in is off")
		}
	}
}

func TestDeviceAuthRequiresTokenAndKeepsExportsOpen(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	api := &Server{
		Store:   testStore(t),
		Clock:   func() time.Time { return now },
		Assets:  fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Broadwave")}},
		Version: "dev",
	}
	h := api.Handler()
	turned := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	if turned.Code != http.StatusOK {
		t.Fatalf("enable %d %s", turned.Code, turned.Body.String())
	}
	admin := jsonField(t, turned.Body.Bytes(), "deviceToken")
	if !strings.HasPrefix(admin, "bw_") {
		t.Fatalf("minted %q", admin)
	}
	if turned.Header().Get("Set-Cookie") == "" || strings.Contains(turned.Header().Get("Set-Cookie"), "Secure") {
		t.Fatalf("cookie %q", turned.Header().Get("Set-Cookie"))
	}
	if strings.Contains(call(t, h, http.MethodGet, "/api/v1/settings", "", "").Body.String(), "deviceToken") && call(t, h, http.MethodGet, "/api/v1/settings", "", "").Code == http.StatusOK {
		t.Fatal("token returned without sign-in")
	}
	locked := call(t, h, http.MethodGet, "/api/v1/settings", "", "")
	if locked.Code != http.StatusUnauthorized {
		t.Fatalf("open settings %d %s", locked.Code, locked.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/api/v1/health", "", ""); got.Code != http.StatusOK {
		t.Fatalf("health %d", got.Code)
	}
	if got := call(t, h, http.MethodGet, "/api/v1/server", "", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "devicePairing") {
		t.Fatalf("server %d %s", got.Code, got.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/export/lineup.m3u", "", ""); got.Code != http.StatusOK {
		t.Fatalf("export %d %s", got.Code, got.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/media/live/missing", "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("media %d %s", got.Code, got.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/", "", ""); got.Code != http.StatusOK {
		t.Fatalf("ui %d", got.Code)
	}
	opened := call(t, h, http.MethodGet, "/api/v1/settings", "", admin)
	if opened.Code != http.StatusOK || strings.Contains(opened.Body.String(), admin) {
		t.Fatalf("bearer %d %s", opened.Code, opened.Body.String())
	}
	var c *http.Cookie
	for _, item := range turned.Result().Cookies() {
		if item.Name == deviceCookie {
			c = item
		}
	}
	if c == nil || c.Value != admin {
		t.Fatalf("cookie %+v", turned.Result().Cookies())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clients/me", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"auth":"device"`) {
		t.Fatalf("cookie me %d %s", rec.Code, rec.Body.String())
	}
	q := call(t, h, http.MethodGet, "/api/v1/clients/me?access_token="+admin+"&x=1", "", "")
	if q.Code != http.StatusOK || strings.Contains(q.Body.String(), admin) {
		t.Fatalf("query %d %s", q.Code, q.Body.String())
	}

	watchCode := call(t, h, http.MethodPost, "/api/v1/pair/code", `{"scopes":["watch"],"kind":"phone"}`, admin)
	if watchCode.Code != http.StatusCreated {
		t.Fatalf("code %d %s", watchCode.Code, watchCode.Body.String())
	}
	code := jsonField(t, watchCode.Body.Bytes(), "code")
	claimed := call(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+code+`","name":"Pocket","kind":"phone"}`, "")
	if claimed.Code != http.StatusCreated {
		t.Fatalf("claim %d %s", claimed.Code, claimed.Body.String())
	}
	watch := jsonField(t, claimed.Body.Bytes(), "token")
	if call(t, h, http.MethodPut, "/api/v1/settings", `{"hideScores":"1"}`, watch).Code != http.StatusForbidden {
		t.Fatal("watch token changed settings")
	}
	if got := call(t, h, http.MethodPost, "/api/v1/passes", `{}`, watch); got.Code != http.StatusForbidden {
		t.Fatalf("watch pass %d %s", got.Code, got.Body.String())
	}
	recordCode := call(t, h, http.MethodPost, "/api/v1/pair/code", `{"scopes":["record"]}`, admin)
	recordClaim := call(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+jsonField(t, recordCode.Body.Bytes(), "code")+`","name":"DVR","kind":"other"}`, "")
	record := jsonField(t, recordClaim.Body.Bytes(), "token")
	if got := call(t, h, http.MethodPost, "/api/v1/passes", `{}`, record); got.Code != http.StatusBadRequest {
		t.Fatalf("record pass %d %s", got.Code, got.Body.String())
	}
	if got := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, record); got.Code != http.StatusForbidden {
		t.Fatalf("record settings %d", got.Code)
	}

	me := call(t, h, http.MethodGet, "/api/v1/clients", "", admin)
	var listed struct {
		Devices []store.ClientDevice `json:"devices"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var pocket string
	for _, dev := range listed.Devices {
		if dev.Name == "Pocket" {
			pocket = dev.ID
		}
		if strings.Contains(me.Body.String(), watch) {
			t.Fatal("list included a raw token")
		}
	}
	if pocket == "" {
		t.Fatalf("list %s", me.Body.String())
	}
	revoked := call(t, h, http.MethodDelete, "/api/v1/clients/"+pocket, "", admin)
	if revoked.Code != http.StatusOK || strings.Contains(revoked.Body.String(), "Pocket") {
		t.Fatalf("revoke %d %s", revoked.Code, revoked.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/api/v1/channels", "", watch); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token %d", got.Code)
	}

	shown := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Den","kind":"tv","scopes":["watch"]}`, "")
	if shown.Code != http.StatusCreated {
		t.Fatalf("show %d %s", shown.Code, shown.Body.String())
	}
	showCode := jsonField(t, shown.Body.Bytes(), "code")
	secret := jsonField(t, shown.Body.Bytes(), "pollSecret")
	pairID := jsonField(t, shown.Body.Bytes(), "id")
	pending := call(t, h, http.MethodGet, "/api/v1/pair/"+pairID+"?secret="+secret, "", "")
	if !strings.Contains(pending.Body.String(), `"state":"pending"`) {
		t.Fatalf("poll %s", pending.Body.String())
	}
	approved := call(t, h, http.MethodPost, "/api/v1/pair/approve", `{"code":"`+showCode+`","scopes":["watch","record"]}`, admin)
	if approved.Code != http.StatusOK || strings.Contains(approved.Body.String(), "bw_") {
		t.Fatalf("approve %d %s", approved.Code, approved.Body.String())
	}
	delivered := call(t, h, http.MethodGet, "/api/v1/pair/"+pairID+"?secret="+secret, "", "")
	tv := jsonField(t, delivered.Body.Bytes(), "token")
	if !strings.HasPrefix(tv, "bw_") || !strings.Contains(delivered.Body.String(), `"state":"approved"`) {
		t.Fatalf("deliver %s", delivered.Body.String())
	}
	again := call(t, h, http.MethodGet, "/api/v1/pair/"+pairID+"?secret="+secret, "", "")
	if strings.Contains(again.Body.String(), tv) {
		t.Fatal("token delivered twice")
	}
	if got := call(t, h, http.MethodGet, "/api/v1/recordings", "", tv); got.Code != http.StatusOK {
		t.Fatalf("tv watch %d %s", got.Code, got.Body.String())
	}

	if got := call(t, h, http.MethodPost, "/api/v1/pair/approve", `{"code":"000000"}`, admin); got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "That code does not match.") {
		t.Fatalf("bad code %d %s", got.Code, got.Body.String())
	}
	echo := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceToken":"`+admin+`","deviceAuth":"2"}`, admin)
	if echo.Code != http.StatusBadRequest {
		t.Fatalf("bad deviceAuth %d %s", echo.Code, echo.Body.String())
	}
	off := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"0","deviceToken":"`+admin+`"}`, admin)
	if off.Code != http.StatusOK || strings.Contains(off.Body.String(), admin) {
		t.Fatalf("disable %d %s", off.Code, off.Body.String())
	}
	if got := call(t, h, http.MethodGet, "/api/v1/settings", "", ""); got.Code != http.StatusOK {
		t.Fatalf("open again %d", got.Code)
	}
}

func TestOmittedApproveScopesStayWatch(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	h := (&Server{Store: testStore(t), Clock: func() time.Time { return now }}).Handler()
	on := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	if on.Code != http.StatusOK {
		t.Fatalf("enable %d %s", on.Code, on.Body.String())
	}
	admin := jsonField(t, on.Body.Bytes(), "deviceToken")
	shown := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Den","kind":"tv","scopes":["admin"]}`, "")
	if shown.Code != http.StatusCreated {
		t.Fatalf("show %d %s", shown.Code, shown.Body.String())
	}
	code := jsonField(t, shown.Body.Bytes(), "code")
	approved := call(t, h, http.MethodPost, "/api/v1/pair/approve", `{"code":"`+code+`"}`, admin)
	if approved.Code != http.StatusOK {
		t.Fatalf("approve %d %s", approved.Code, approved.Body.String())
	}
	if strings.Contains(approved.Body.String(), `"admin"`) || !strings.Contains(approved.Body.String(), `"watch"`) {
		t.Fatalf("scopes %s", approved.Body.String())
	}
}

func TestShowTokenSurvivesHeadAndALostWrite(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	h := (&Server{Store: testStore(t), Clock: func() time.Time { return now }}).Handler()
	shown := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Den","kind":"tv"}`, "")
	if shown.Code != http.StatusCreated {
		t.Fatalf("show %d %s", shown.Code, shown.Body.String())
	}
	secret := jsonField(t, shown.Body.Bytes(), "pollSecret")
	pairID := jsonField(t, shown.Body.Bytes(), "id")
	code := jsonField(t, shown.Body.Bytes(), "code")
	if got := call(t, h, http.MethodPost, "/api/v1/pair/approve", `{"code":"`+code+`"}`, ""); got.Code != http.StatusOK {
		t.Fatalf("approve %d %s", got.Code, got.Body.String())
	}
	poll := "/api/v1/pair/" + pairID + "?secret=" + secret
	head := call(t, h, http.MethodHead, poll, "", "")
	if head.Code != http.StatusOK || strings.Contains(head.Body.String(), "bw_") {
		t.Fatalf("head %d %s", head.Code, head.Body.String())
	}
	if head.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("head cache %q", head.Header().Get("Cache-Control"))
	}
	lost := &failBody{}
	h.ServeHTTP(lost, httptest.NewRequest(http.MethodGet, poll, nil))
	if lost.code != http.StatusOK {
		t.Fatalf("lost write %d", lost.code)
	}
	delivered := call(t, h, http.MethodGet, poll, "", "")
	tv := jsonField(t, delivered.Body.Bytes(), "token")
	if !strings.HasPrefix(tv, "bw_") || delivered.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("deliver %d %q %s", delivered.Code, delivered.Header().Get("Cache-Control"), delivered.Body.String())
	}
	again := call(t, h, http.MethodGet, poll, "", "")
	if strings.Contains(again.Body.String(), tv) || !strings.Contains(again.Body.String(), `"state":"approved"`) {
		t.Fatalf("second poll %s", again.Body.String())
	}
}

type failBody struct {
	h    http.Header
	code int
}

func (f *failBody) Header() http.Header {
	if f.h == nil {
		f.h = http.Header{}
	}
	return f.h
}

func (f *failBody) WriteHeader(code int) { f.code = code }

func (f *failBody) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestOverlappingPollsDeliverTheTokenOnce(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	h := (&Server{Store: testStore(t), Clock: func() time.Time { return now }}).Handler()
	shown := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Den","kind":"tv"}`, "")
	if shown.Code != http.StatusCreated {
		t.Fatalf("show %d %s", shown.Code, shown.Body.String())
	}
	secret := jsonField(t, shown.Body.Bytes(), "pollSecret")
	pairID := jsonField(t, shown.Body.Bytes(), "id")
	code := jsonField(t, shown.Body.Bytes(), "code")
	if got := call(t, h, http.MethodPost, "/api/v1/pair/approve", `{"code":"`+code+`"}`, ""); got.Code != http.StatusOK {
		t.Fatalf("approve %d %s", got.Code, got.Body.String())
	}
	poll := "/api/v1/pair/" + pairID + "?secret=" + secret
	const n = 8
	var wg sync.WaitGroup
	bodies := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, poll, nil))
			bodies[i] = rec.Body.String()
		}()
	}
	wg.Wait()
	got := 0
	for _, body := range bodies {
		if strings.Contains(body, "bw_") {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("token delivered %d times\n%s", got, strings.Join(bodies, "\n"))
	}
	again := call(t, h, http.MethodGet, poll, "", "")
	if strings.Contains(again.Body.String(), "bw_") {
		t.Fatalf("later poll %s", again.Body.String())
	}
}

func TestPairingRateLimit(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	h := (&Server{Store: testStore(t), Clock: func() time.Time { return now }}).Handler()
	for i := range pairIPLimit {
		res := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Room","kind":"tv"}`, "")
		if res.Code != http.StatusCreated {
			t.Fatalf("pair %d: %d %s", i, res.Code, res.Body.String())
		}
	}
	res := call(t, h, http.MethodPost, "/api/v1/pair", `{"name":"Room","kind":"tv"}`, "")
	if res.Code != http.StatusTooManyRequests || !strings.Contains(res.Body.String(), `"rate_limited"`) {
		t.Fatalf("limit %d %s", res.Code, res.Body.String())
	}
}

func TestRequiredScopeFailsClosed(t *testing.T) {
	if got := requiredScope(http.MethodGet, "/api/v1/health"); got != "public" {
		t.Fatalf("health %s", got)
	}
	if got := requiredScope(http.MethodGet, "/api/channels"); got != store.ScopeWatch {
		t.Fatalf("channels %s", got)
	}
	if got := requiredScope(http.MethodPost, "/api/v1/passes"); got != store.ScopeRecord {
		t.Fatalf("passes %s", got)
	}
	if got := requiredScope(http.MethodPut, "/api/v1/settings"); got != store.ScopeAdmin {
		t.Fatalf("settings %s", got)
	}
	if got := requiredScope(http.MethodPost, "/api/v1/not-a-route"); got != store.ScopeAdmin {
		t.Fatalf("unknown %s", got)
	}
	if got := requiredScope(http.MethodGet, "/export/guide.xml"); got != "public" {
		t.Fatalf("export %s", got)
	}
	if got := requiredScope(http.MethodGet, "/media/file/1/index.m3u8"); got != store.ScopeWatch {
		t.Fatalf("media %s", got)
	}
	if got := requiredScope(http.MethodGet, "/api/v1/tuners"); got != store.ScopeAdmin {
		t.Fatalf("tuners %s", got)
	}
	if got := requiredScope(http.MethodPost, "/api/v1/virtuals"); got != store.ScopeRecord {
		t.Fatalf("virtuals %s", got)
	}
	if got := requiredScope(http.MethodPatch, "/api/v1/virtuals/4"); got != store.ScopeRecord {
		t.Fatalf("virtual patch %s", got)
	}
	if got := requiredScope(http.MethodPost, "/api/v1/virtuals/4/play"); got != store.ScopeWatch {
		t.Fatalf("virtual play %s", got)
	}
	if got := requiredScope(http.MethodPost, "/api/v1/mosaic/a/b/stop"); got != store.ScopeAdmin {
		t.Fatalf("mosaic extra %s", got)
	}
	if got := requiredScope(http.MethodDelete, "/api/v1/markers/9/extra"); got != store.ScopeAdmin {
		t.Fatalf("marker extra %s", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels?access_token=bw_secret&n=1", nil)
	token, stripped := presentedToken(req)
	if token != "bw_secret" || stripped.URL.RawQuery != "n=1" || strings.Contains(stripped.RequestURI, "bw_secret") {
		t.Fatalf("token %q query %q uri %q", token, stripped.URL.RawQuery, stripped.RequestURI)
	}
	both := httptest.NewRequest(http.MethodGet, "/api/v1/ws?access_token=bw_query", nil)
	both.Header.Set("Authorization", "Bearer bw_header")
	token, stripped = presentedToken(both)
	if token != "bw_header" || stripped.URL.Query().Get("access_token") != "" || strings.Contains(stripped.RequestURI, "bw_query") {
		t.Fatalf("bearer %q query %q uri %q", token, stripped.URL.RawQuery, stripped.RequestURI)
	}
}

func call(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func jsonField(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("%v %s", err, raw)
	}
	s, _ := body[key].(string)
	if s == "" {
		t.Fatalf("missing %s in %s", key, raw)
	}
	return s
}
