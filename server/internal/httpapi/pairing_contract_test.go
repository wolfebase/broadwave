package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// Pairing successes are JSON the store can produce with no tuner. Each body
// is checked against the documented schema, including the one poll that
// carries the token and the later poll that does not.
func TestPairingSuccessMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	api := (&Server{Store: testStore(t), Version: "dev", Staging: true}).Handler()

	started := documentedBody(t, doc, "post", "/pair", "201",
		postJSON(t, api, "/api/v1/pair", `{"name":"Harbor Watch","kind":"tv","scopes":["watch"]}`))
	id, _ := started["id"].(string)
	code, _ := started["code"].(string)
	secret, _ := started["pollSecret"].(string)
	if id == "" || code == "" || secret == "" {
		t.Fatalf("ticket %#v", started)
	}
	pollURL := "/api/v1/pair/" + url.PathEscape(id) + "?secret=" + url.QueryEscape(secret)

	pending := documentedBody(t, doc, "get", "/pair/{id}", "200", call(t, api, http.MethodGet, pollURL, "", ""))
	if pending["state"] != "pending" || pending["token"] != nil || pending["expiresAt"] == "" {
		t.Fatalf("pending %#v", pending)
	}

	approved := documentedBody(t, doc, "post", "/pair/approve", "200",
		postJSON(t, api, "/api/v1/pair/approve", `{"code":"`+code+`","scopes":["watch"]}`))
	device, ok := approved["device"].(map[string]any)
	if !ok || device["name"] != "Harbor Watch" || device["kind"] != "tv" {
		t.Fatalf("approve %#v", approved)
	}

	// HEAD is this GET route and must not take the token.
	head := documentedBody(t, doc, "get", "/pair/{id}", "200", call(t, api, http.MethodHead, pollURL, "", ""))
	if head["state"] != "approved" || head["token"] != nil {
		t.Fatalf("head %#v", head)
	}

	granted := documentedBody(t, doc, "get", "/pair/{id}", "200", call(t, api, http.MethodGet, pollURL, "", ""))
	token, _ := granted["token"].(string)
	if granted["state"] != "approved" || token == "" || granted["device"] == nil {
		t.Fatalf("granted %#v", granted)
	}

	again := documentedBody(t, doc, "get", "/pair/{id}", "200", call(t, api, http.MethodGet, pollURL, "", ""))
	if again["state"] != "approved" || again["token"] != nil || again["device"] == nil {
		t.Fatalf("second poll %#v", again)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/clients/me", nil)
	meReq.AddCookie(&http.Cookie{Name: deviceCookie, Value: token})
	meRec := httptest.NewRecorder()
	api.ServeHTTP(meRec, meReq)
	seat := documentedBody(t, doc, "get", "/clients/me", "200", meRec)
	if seat["auth"] != "local-open" {
		t.Fatalf("auth %#v", seat["auth"])
	}
	seated, ok := seat["device"].(map[string]any)
	if !ok || seated["id"] == "" || seated["name"] != "Harbor Watch" {
		t.Fatalf("seat %#v", seat)
	}

	listed := documentedBody(t, doc, "get", "/clients", "200", call(t, api, http.MethodGet, "/api/v1/clients", "", ""))
	devices, ok := listed["devices"].([]any)
	if !ok || len(devices) != 1 {
		t.Fatalf("clients %#v", listed)
	}

	ticket := documentedBody(t, doc, "post", "/pair/code", "201",
		postJSON(t, api, "/api/v1/pair/code", `{"kind":"phone","scopes":["watch"]}`))
	if _, present := ticket["pollSecret"]; present || ticket["code"] == "" {
		t.Fatalf("code ticket %#v", ticket)
	}
	claimCode, _ := ticket["code"].(string)
	claim := documentedBody(t, doc, "post", "/pair/claim", "201",
		postJSON(t, api, "/api/v1/pair/claim", `{"code":"`+claimCode+`","name":"Harbor Lights","kind":"phone"}`))
	if claim["token"] == "" {
		t.Fatalf("claim %#v", claim)
	}
	claimed, ok := claim["device"].(map[string]any)
	if !ok || claimed["name"] != "Harbor Lights" || claimed["kind"] != "phone" {
		t.Fatalf("claim device %#v", claim["device"])
	}
}

// documentedBody checks one JSON response against the status in the spec.
func documentedBody(t *testing.T, doc *apiDoc, method, path, status string, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	want, err := strconv.Atoi(status)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != want {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	schema, ok, err := responseSchema(doc, method, path, status)
	if err != nil || !ok {
		t.Fatalf("%s %s %s schema: ok=%v %v", method, path, status, ok, err)
	}
	if err := validateSchema(doc, schema, body, "$", 0); err != nil {
		t.Fatalf("%s %s %s: %v\n%s", method, path, status, err, rec.Body.String())
	}
	obj, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("%s %s body is %T", method, path, body)
	}
	return obj
}
