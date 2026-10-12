package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

// TestPassAndVirtualBodiesMatchTheHandler checks the request bodies the
// pass and library-channel handlers accept. A rename is the title change,
// a preview of a saved pass does not need a title, and an unknown library order is refused.
func TestPassAndVirtualBodiesMatchTheHandler(t *testing.T) {
	doc := loadOpenAPI(t)
	patchSchema := requestSchema(t, doc, http.MethodPatch, "/passes/{id}")
	previewSchema := requestSchema(t, doc, http.MethodPost, "/passes/preview")
	virtualSchema := requestSchema(t, doc, http.MethodPatch, "/virtuals/{id}")
	playSchema := requestSchema(t, doc, http.MethodPost, "/virtuals/{id}/play")

	t.Run("spec", func(t *testing.T) {
		if err := validateSchema(doc, patchSchema, jsonValue(t, `{"rename":"Harbor Watch"}`), "$", 0); err != nil {
			t.Fatalf("spec still requires fields the handler ignores: %v", err)
		}
		if err := validateSchema(doc, patchSchema, jsonValue(t, `{"id":1,"title":"Harbor Watch"}`), "$", 0); err != nil {
			t.Fatalf("spec rejects an ignored id and title: %v", err)
		}
		if err := validateSchema(doc, patchSchema, jsonValue(t, `{"matchKind":"team"}`), "$", 0); err == nil {
			t.Fatal("spec still accepts matchKind team on PATCH")
		}
		if err := validateSchema(doc, previewSchema, jsonValue(t, `{"id":1}`), "$", 0); err != nil {
			t.Fatalf("spec still requires title when id is set: %v", err)
		}
		if err := validateSchema(doc, previewSchema, jsonValue(t, `{"id":1,"title":"Harbor Watch","rename":"Harbor Watch"}`), "$", 0); err != nil {
			t.Fatalf("spec rejects the edit preview: %v", err)
		}
		if err := validateSchema(doc, virtualSchema, jsonValue(t, `{"orderMode":"sideways"}`), "$", 0); err == nil {
			t.Fatal("spec accepts an unknown orderMode")
		}
		play := operation(t, doc, http.MethodPost, "/virtuals/{id}/play")
		body, _ := asMap(play["requestBody"])
		if body["required"] != true {
			t.Fatal("spec still says the play body is optional")
		}
		if err := validateSchema(doc, playSchema, jsonValue(t, `{}`), "$", 0); err != nil {
			t.Fatalf("spec rejects an omitted index: %v", err)
		}
		if err := validateSchema(doc, playSchema, jsonValue(t, `{"index":-1}`), "$", 0); err == nil {
			t.Fatal("spec accepts an index below 0")
		}
	})

	t.Run("live", func(t *testing.T) {
		st := testStore(t)
		api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

		created := postJSON(t, api, "/api/v1/passes", `{"title":"Harbor Watch"}`)
		createdBody := successBody(t, doc, http.MethodPost, "/passes", created)
		id := passID(t, createdBody)
		if passField(t, createdBody, "title") != "Harbor Watch" {
			t.Fatalf("create title %#v", passField(t, createdBody, "title"))
		}

		renamed := sendJSON(t, api, http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", id), `{"rename":"Harbor Lights"}`)
		renamedBody := successBody(t, doc, http.MethodPatch, "/passes/{id}", renamed)
		if passField(t, renamedBody, "title") != "Harbor Lights" || passID(t, renamedBody) != id {
			t.Fatalf("rename %+v", renamedBody)
		}

		preview := postJSON(t, api, "/api/v1/passes/preview", fmt.Sprintf(`{"id":%d}`, id))
		successBody(t, doc, http.MethodPost, "/passes/preview", preview)
		listed := successBody(t, doc, http.MethodGet, "/passes", get(t, api, "/api/v1/passes"))
		if got := passField(t, listed, "title"); got != "Harbor Lights" {
			t.Fatalf("preview saved a title: %#v", got)
		}
		if rec := postJSON(t, api, "/api/v1/passes/preview", `{}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("preview without title: %d %s", rec.Code, rec.Body.String())
		}
		if rec := postJSON(t, api, "/api/v1/passes/preview", `{"id":0}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("preview id 0 without title: %d %s", rec.Code, rec.Body.String())
		}

		ready, err := st.CreateRecording(t.Context(), store.Recording{
			ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete",
			StartedAt: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		channel, err := st.CreateVirtual(t.Context(), "900", "Harbor Watch", []int64{ready})
		if err != nil {
			t.Fatal(err)
		}
		if rec := sendJSON(t, api, http.MethodPatch, fmt.Sprintf("/api/v1/virtuals/%d", channel.ID), `{"orderMode":"sideways"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("unknown orderMode: %d %s", rec.Code, rec.Body.String())
		}
		updated := sendJSON(t, api, http.MethodPatch, fmt.Sprintf("/api/v1/virtuals/%d", channel.ID), `{"orderMode":"random"}`)
		updatedBody := successBody(t, doc, http.MethodPatch, "/virtuals/{id}", updated)
		if updatedBody["orderMode"] != "random" {
			t.Fatalf("orderMode %#v", updatedBody["orderMode"])
		}
		cleared := sendJSON(t, api, http.MethodPatch, fmt.Sprintf("/api/v1/virtuals/%d", channel.ID), `{"ruleTitle":"Harbor Watch"}`)
		clearedBody := successBody(t, doc, http.MethodPatch, "/virtuals/{id}", cleared)
		if clearedBody["orderMode"] != "random" {
			t.Fatalf("omitted orderMode %#v", clearedBody["orderMode"])
		}

		play := fmt.Sprintf("/api/v1/virtuals/%d/play", channel.ID)
		if rec := sendJSON(t, api, http.MethodPost, play, ``); rec.Code != http.StatusBadRequest {
			t.Fatalf("empty play body: %d %s", rec.Code, rec.Body.String())
		}
		if rec := postJSON(t, api, play, `{}`); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("omitted index is 0, not a missing-index 400: %d %s", rec.Code, rec.Body.String())
		}
		if rec := postJSON(t, api, play, `{"index":-1}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("index below 0: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func requestSchema(t *testing.T, doc *apiDoc, method, path string) map[string]any {
	t.Helper()
	op := operation(t, doc, method, path)
	body, ok := asMap(op["requestBody"])
	if !ok {
		t.Fatalf("%s %s has no request body", method, path)
	}
	content, ok := asMap(body["content"])
	if !ok {
		t.Fatalf("%s %s has no content", method, path)
	}
	media, ok := asMap(content["application/json"])
	if !ok {
		t.Fatalf("%s %s has no json", method, path)
	}
	schema, ok := asMap(media["schema"])
	if !ok {
		t.Fatalf("%s %s schema is not an object", method, path)
	}
	return schema
}

func operation(t *testing.T, doc *apiDoc, method, path string) map[string]any {
	t.Helper()
	item, ok := asMap(doc.paths[path])
	if !ok {
		t.Fatalf("no path %s", path)
	}
	op, ok := asMap(item[strings.ToLower(method)])
	if !ok {
		t.Fatalf("no %s %s", method, path)
	}
	return op
}

func jsonValue(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func successBody(t *testing.T, doc *apiDoc, method, path string, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	schema, ok, err := responseSchema(doc, method, path, "200")
	if err != nil || !ok {
		t.Fatalf("%s %s 200 schema: ok=%v %v", method, path, ok, err)
	}
	if err := validateSchema(doc, schema, body, "$", 0); err != nil {
		t.Fatalf("%s %s body: %v", method, path, err)
	}
	obj, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("%s %s body is %T", method, path, body)
	}
	return obj
}

func passID(t *testing.T, body map[string]any) int64 {
	t.Helper()
	id, ok := passField(t, body, "id").(float64)
	if !ok {
		t.Fatalf("pass id %#v", passField(t, body, "id"))
	}
	return int64(id)
}

func passField(t *testing.T, body map[string]any, field string) any {
	t.Helper()
	list, ok := body["passes"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("passes %#v", body["passes"])
	}
	pass, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("pass %#v", list[0])
	}
	return pass[field]
}
