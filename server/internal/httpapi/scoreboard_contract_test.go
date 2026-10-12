package httpapi

import (
	"net/http"
	"reflect"
	"testing"

	"broadwave/internal/sports"
)

// The scoreboard query names the leagues the provider knows. Blank asks for
// every league. An unknown or uppercase id is refused before anyone is dialed.
func TestScoreboardLeagueMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	got, err := stringList(querySchema(t, doc, "get", "/sports/scoreboard", "league")["enum"])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{""}
	for _, league := range sports.Leagues {
		want = append(want, league.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("league enum %v, want %v", got, want)
	}

	st := testStore(t)
	if err := st.PutSettings(t.Context(), map[string]string{"liveScores": "0"}); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	for _, league := range want {
		path := "/api/v1/sports/scoreboard"
		if league != "" {
			path += "?league=" + league
		}
		body := documentedBody(t, doc, "get", "/sports/scoreboard", "200", call(t, api, http.MethodGet, path, "", ""))
		games, ok := body["games"].([]any)
		if !ok || len(games) != 0 {
			t.Fatalf("league %q %#v", league, body["games"])
		}
	}

	for _, league := range []string{"NFL", "quidditch"} {
		bad := assertErrorBody(t, doc, "get", "/sports/scoreboard", "400", call(t, api, http.MethodGet, "/api/v1/sports/scoreboard?league="+league, "", ""))
		if bad["code"] != "bad_request" || bad["message"] != "unknown league" {
			t.Fatalf("%s %#v", league, bad)
		}
	}
}

func querySchema(t *testing.T, doc *apiDoc, method, path, name string) map[string]any {
	t.Helper()
	op := operation(t, doc, method, path)
	params, ok := op["parameters"].([]any)
	if !ok {
		t.Fatalf("%s %s parameters are %T", method, path, op["parameters"])
	}
	for _, raw := range params {
		param, ok := asMap(raw)
		if !ok || param["name"] != name {
			continue
		}
		schema, ok := asMap(param["schema"])
		if !ok {
			t.Fatalf("%s schema is %T", name, param["schema"])
		}
		return schema
	}
	t.Fatalf("no %s parameter on %s %s", name, method, path)
	return nil
}
