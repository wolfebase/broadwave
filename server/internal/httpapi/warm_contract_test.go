package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/live"
)

// Warm decodes the same body as watch, including the older profile, audio, and
// pictureMode fields. A nil hub answers before that body is read.
func TestWarmBodyBeforeDecide(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	nilHub := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	withHub := (&Server{
		Store:   st,
		Hub:     &live.Hub{Store: st},
		Version: "dev",
		Staging: true,
	}).Handler()

	t.Run("spec", func(t *testing.T) {
		op := operation(t, doc, "post", "/watch/{id}/warm")
		desc, _ := op["description"].(string)
		if !strings.Contains(desc, "`profile`/`audio`/`pictureMode` are the older web fields and still work.") {
			t.Fatalf("description %q", desc)
		}
		body, ok := asMap(op["requestBody"])
		if !ok || body["required"] != false {
			t.Fatalf("request body %#v", op["requestBody"])
		}
		schema := requestSchema(t, doc, "post", "/watch/{id}/warm")
		props, ok := asMap(schema["properties"])
		if !ok {
			t.Fatal("no properties")
		}
		rendition, ok := asMap(props["rendition"])
		if !ok || rendition["description"] != "Force a rendition key, for example copy.copy or 720.aac2.broadcast" {
			t.Fatalf("rendition %#v", props["rendition"])
		}
		assertStringEnum(t, props, "profile", true, "transparent", "balanced", "saver")
		assertStringEnum(t, props, "audio", true, "stereo", "surround")
		picture, ok := asMap(props["pictureMode"])
		if !ok || picture["$ref"] != "#/components/schemas/PictureMode" {
			t.Fatalf("pictureMode %#v", props["pictureMode"])
		}
		good := jsonValue(t, `{"profile":"saver","audio":"surround","pictureMode":"film","rendition":"copy.copy"}`)
		if err := validateSchema(doc, schema, good, "$", 0); err != nil {
			t.Fatal(err)
		}
		if err := validateSchema(doc, schema, jsonValue(t, `{"profile":"sideways"}`), "$", 0); err == nil {
			t.Fatal("profile sideways validated")
		}
		if err := validateSchema(doc, schema, jsonValue(t, `{"audio":"mono"}`), "$", 0); err == nil {
			t.Fatal("audio mono validated")
		}
		if err := validateSchema(doc, schema, jsonValue(t, `{"pictureMode":"sideways"}`), "$", 0); err == nil {
			t.Fatal("pictureMode sideways validated")
		}
	})

	t.Run("live", func(t *testing.T) {
		// A nil hub answers before the body is read, so bad JSON is still 200.
		req := httptest.NewRequest(http.MethodPost, "/api/v1/watch/1/warm", strings.NewReader("{"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		nilHub.ServeHTTP(rec, req)
		warm := successBody(t, doc, "post", "/watch/{id}/warm", rec)
		if warm["warm"] != false {
			t.Fatalf("warm %#v", warm["warm"])
		}

		// A hub is present, so the bad JSON is refused before decide runs.
		bad := httptest.NewRequest(http.MethodPost, "/api/v1/watch/1/warm", strings.NewReader("{"))
		bad.Header.Set("Content-Type", "application/json")
		badRec := httptest.NewRecorder()
		withHub.ServeHTTP(badRec, bad)
		body := assertErrorBody(t, doc, "post", "/watch/{id}/warm", "400", badRec)
		if body["code"] != "bad_request" {
			t.Fatalf("code %#v", body["code"])
		}

		idReq := httptest.NewRequest(http.MethodPost, "/api/v1/watch/nope/warm", strings.NewReader("{"))
		idReq.Header.Set("Content-Type", "application/json")
		idRec := httptest.NewRecorder()
		nilHub.ServeHTTP(idRec, idReq)
		idBody := assertErrorBody(t, doc, "post", "/watch/{id}/warm", "400", idRec)
		if idBody["code"] != "bad_request" {
			t.Fatalf("code %#v", idBody["code"])
		}
	})
}

func assertStringEnum(t *testing.T, props map[string]any, name string, deprecated bool, want ...string) {
	t.Helper()
	prop, ok := asMap(props[name])
	if !ok {
		t.Fatalf("no %s", name)
	}
	if prop["type"] != "string" || prop["deprecated"] != deprecated {
		t.Fatalf("%s %#v", name, prop)
	}
	got, err := stringList(prop["enum"])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s enum %v", name, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s enum %v", name, got)
		}
	}
}
