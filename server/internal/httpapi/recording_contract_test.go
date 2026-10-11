package httpapi

import (
	"testing"
)

// TestRecordingResponsesMatchTheBodies checks the recording replies whose
// published shape left out a field the handler always sends.
func TestRecordingResponsesMatchTheBodies(t *testing.T) {
	doc := loadOpenAPI(t)
	root := fixtureDir(t)

	play, ok, err := responseSchema(doc, "post", "/recordings/{id}/play", "200")
	if err != nil || !ok {
		t.Fatalf("play schema: ok=%v %v", ok, err)
	}
	playRequired, err := stringList(play["required"])
	if err != nil {
		t.Fatal(err)
	}
	if !listHas(playRequired, "recording") {
		t.Fatal("play does not require recording")
	}
	playBody := readFixture(t, root, "recording-play")
	if err := validateSchema(doc, play, playBody, "$", 0); err != nil {
		t.Fatal(err)
	}
	played, _ := playBody.(map[string]any)
	recording, ok := asMap(doc.schemas["Recording"])
	if !ok {
		t.Fatal("no Recording schema")
	}
	if err := validateSchema(doc, recording, played["recording"], "$.recording", 0); err != nil {
		t.Fatal(err)
	}

	watched := flagSchema(t, doc, "put", "/recordings/{id}/watched", "watched")
	if err := validateSchema(doc, watched, readFixture(t, root, "recording-watched"), "$", 0); err != nil {
		t.Fatal(err)
	}
	keep := flagSchema(t, doc, "put", "/recordings/{id}/keep", "keep")
	if err := validateSchema(doc, keep, readFixture(t, root, "recording-keep"), "$", 0); err != nil {
		t.Fatal(err)
	}

	again, ok, err := responseSchema(doc, "get", "/recordings/{id}/again", "200")
	if err != nil || !ok {
		t.Fatalf("again schema: ok=%v %v", ok, err)
	}
	againRequired, err := stringList(again["required"])
	if err != nil {
		t.Fatal(err)
	}
	if !listHas(againRequired, "airing") {
		t.Fatal("record again does not require airing")
	}
	props, _ := asMap(again["properties"])
	airing, _ := asMap(props["airing"])
	if !allowsNull(airing) {
		t.Fatal("record again airing does not allow null")
	}
	if err := validateSchema(doc, again, readFixture(t, root, "again"), "$", 0); err != nil {
		t.Fatal(err)
	}
}

func flagSchema(t *testing.T, doc *apiDoc, method, path, flag string) map[string]any {
	t.Helper()
	schema, ok, err := responseSchema(doc, method, path, "200")
	if err != nil || !ok {
		t.Fatalf("%s %s schema: ok=%v %v", method, path, ok, err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("%s %s still allows fields it does not name", method, path)
	}
	required, err := stringList(schema["required"])
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", flag} {
		if !listHas(required, key) {
			t.Errorf("%s %s does not require %s", method, path, key)
		}
	}
	return schema
}
