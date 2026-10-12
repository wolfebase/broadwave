package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The documented limit is 63 characters. A name of two-byte characters used
// to be refused once its bytes passed 63.
func TestServerNameIsCountedInCharacters(t *testing.T) {
	doc := loadOpenAPI(t)
	schema := requestSchema(t, doc, http.MethodPatch, "/server")
	props, _ := asMap(schema["properties"])
	nameSchema, ok := asMap(props["name"])
	if !ok {
		t.Fatal("name is not a schema")
	}
	max, ok := asNumber(nameSchema["maxLength"])
	if !ok || max != 63 {
		t.Fatalf("maxLength %#v", nameSchema["maxLength"])
	}
	name := strings.Repeat("é", 63)
	if err := validateSchema(doc, schema, map[string]any{"name": name}, "$", 0); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(doc, schema, map[string]any{"name": name + "é"}, "$", 0); err == nil {
		t.Fatal("64 characters matched the spec")
	}

	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	raw, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	saved := documentedBody(t, doc, http.MethodPatch, "/server", "200", sendJSON(t, api, http.MethodPatch, "/api/v1/server", string(raw)))
	if saved["name"] != name {
		t.Fatalf("name %#v", saved["name"])
	}
	longer, err := json.Marshal(map[string]string{"name": name + "é"})
	if err != nil {
		t.Fatal(err)
	}
	bad := assertErrorBody(t, doc, http.MethodPatch, "/server", "400", sendJSON(t, api, http.MethodPatch, "/api/v1/server", string(longer)))
	if bad["code"] != "bad_request" {
		t.Fatalf("long %#v", bad)
	}
	blank, err := json.Marshal(map[string]string{"name": " "})
	if err != nil {
		t.Fatal(err)
	}
	bad = assertErrorBody(t, doc, http.MethodPatch, "/server", "400", sendJSON(t, api, http.MethodPatch, "/api/v1/server", string(blank)))
	if bad["code"] != "bad_request" {
		t.Fatalf("blank %#v", bad)
	}
	still := documentedBody(t, doc, http.MethodGet, "/server", "200", call(t, api, http.MethodGet, "/api/v1/server", "", ""))
	if still["name"] != name {
		t.Fatalf("stored %#v", still["name"])
	}
}
