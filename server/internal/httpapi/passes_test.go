package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func sendJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func passList(t *testing.T, rec *httptest.ResponseRecorder) []store.Pass {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Passes []store.Pass `json:"passes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Passes
}

func TestAKeywordPassKeepsItsRules(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	passes := passList(t, postJSON(t, h, "/api/v1/passes",
		`{"title":"bears","matchKind":"contains","channelId":4,"days":[5,1,1],"timeStart":"18:00","timeEnd":"23:30",
		"episodes":"new","keepMode":"last","keepCount":3,"limitCount":5,"rerecord":true,"commercials":false,"padAfter":15}`))
	if len(passes) != 1 {
		t.Fatalf("%+v", passes)
	}
	p := passes[0]
	if p.Kind != "series" || p.MatchKind != "contains" || p.ChannelID != 4 || fmt.Sprint(p.Days) != "[1 5]" ||
		p.TimeStart != "18:00" || p.TimeEnd != "23:30" || p.Episodes != "new" || p.KeepMode != "last" || p.KeepCount != 3 ||
		p.LimitCount != 5 || !p.Rerecord || p.Commercials || p.PadBefore != 1 || p.PadAfter != 15 {
		t.Fatalf("rules not kept: %+v", p)
	}
	passes = passList(t, sendJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", p.ID),
		`{"title":"Chiefs","matchKind":"title","days":[],"timeStart":"","timeEnd":""}`))
	if p := passes[0]; p.Title != "Chiefs" || p.MatchKind != "title" || len(p.Days) != 0 || p.TimeStart != "" || p.KeepCount != 3 {
		t.Fatalf("edit: %+v", p)
	}
	for _, bad := range []string{
		`{"title":"x","matchKind":"team"}`,
		`{"title":"x","matchKind":"regex"}`,
		`{"title":"x","timeStart":"25:00","timeEnd":"26:00"}`,
		`{"title":"x","timeStart":"8:00","timeEnd":"09:00"}`,
		`{"title":"x","timeStart":"20:00"}`,
		`{"title":"x","timeStart":"20:00","timeEnd":"20:00"}`,
		`{"title":"x","days":[7]}`,
		`{"title":"x","days":"mon"}`,
		`{"title":"x","keepMode":"forever"}`,
		`{"title":"x","episodes":"old"}`,
		`{"title":"  "}`,
	} {
		if rec := postJSON(t, h, "/api/v1/passes", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, rec.Code, rec.Body.String())
		}
		if rec := sendJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", p.ID), bad); bad != `{"title":"  "}` && rec.Code != http.StatusBadRequest {
			t.Errorf("patch %s: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	if got := passList(t, get(t, h, "/api/v1/passes")); len(got) != 1 || got[0].Title != "Chiefs" {
		t.Fatalf("a refused change saved something: %+v", got)
	}
}

func TestATeamPassKeepsItsTeam(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.FollowTeam(ctx, store.TeamFollow{Name: "Kansas City Chiefs", Short: "Chiefs", Record: true}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	passes := passList(t, get(t, h, "/api/v1/passes"))
	if len(passes) != 1 || passes[0].Kind != "team" {
		t.Fatalf("%+v", passes)
	}
	passes = passList(t, sendJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", passes[0].ID),
		`{"title":"Bears","matchKind":"contains","padAfter":30,"keepMode":"last","keepCount":2}`))
	if p := passes[0]; p.Title != "Chiefs" || p.MatchKind != "team" || p.PadAfter != 30 || p.KeepCount != 2 {
		t.Fatalf("%+v", p)
	}
}

func TestOrderingPassesSetsTheirPriority(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	for _, title := range []string{"A", "B", "C"} {
		postJSON(t, h, "/api/v1/passes", fmt.Sprintf(`{"title":%q}`, title))
	}
	ids := map[string]int64{}
	for _, p := range passList(t, get(t, h, "/api/v1/passes")) {
		ids[p.Title] = p.ID
	}
	passes := passList(t, sendJSON(t, h, http.MethodPut, "/api/v1/passes/order",
		fmt.Sprintf(`{"ids":[%d,%d,%d]}`, ids["C"], ids["A"], ids["B"])))
	var got []string
	for _, p := range passes {
		got = append(got, fmt.Sprintf("%s%d", p.Title, p.Priority))
	}
	if strings.Join(got, " ") != "C3 A2 B1" {
		t.Fatalf("order %v", got)
	}
	for _, body := range []string{
		fmt.Sprintf(`{"ids":[%d,%d]}`, ids["A"], ids["B"]),
		fmt.Sprintf(`{"ids":[%d,%d,%d]}`, ids["A"], ids["A"], ids["B"]),
		fmt.Sprintf(`{"ids":[%d,%d,999]}`, ids["A"], ids["B"]),
	} {
		if rec := sendJSON(t, h, http.MethodPut, "/api/v1/passes/order", body); rec.Code != http.StatusConflict {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	if got := passList(t, get(t, h, "/api/v1/passes")); got[0].Title != "C" || got[0].Priority != 3 {
		t.Fatalf("a refused order changed priorities: %+v", got)
	}
}

func TestPreviewNamesWhatAPassWouldPushOut(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "BOX", FriendlyName: "Lab", TunerCount: 1},
		[]hdhr.Channel{{GuideNumber: "4.1", GuideName: "FOX"}, {GuideNumber: "5.1", GuideName: "NBC"}}); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	nbc := guideID(t, st, "5.1")
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Minute)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: fox, Title: "News", ProgramID: "NEWS1", Start: start, End: start.Add(time.Hour)},
		{ChannelID: nbc, Title: "Game", ProgramID: "GAME1", Start: start.Add(30 * time.Minute), End: start.Add(3 * time.Hour)},
		{ChannelID: nbc, Title: "Game Night", ProgramID: "GAME2", Start: start.Add(24 * time.Hour), End: start.Add(25 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	news := passList(t, postJSON(t, h, "/api/v1/passes", `{"title":"News","padBefore":0,"padAfter":0}`))[0]
	type preview struct {
		Items []dvr.Planned `json:"items"`
		Bumps []dvr.Planned `json:"bumps"`
	}
	read := func(body string) preview {
		t.Helper()
		rec := postJSON(t, h, "/api/v1/passes/preview", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
		var p preview
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	low := read(`{"title":"game","matchKind":"contains"}`)
	if len(low.Items) != 2 || !low.Items[0].Skipped || !low.Items[0].Conflict || low.Items[1].Skipped || len(low.Bumps) != 0 {
		t.Fatalf("a lower pass loses the overlap and bumps nothing: %+v", low)
	}
	high := read(`{"title":"game","matchKind":"contains","priority":5}`)
	if len(high.Items) != 2 || high.Items[0].Skipped || len(high.Bumps) != 1 || high.Bumps[0].PassID != news.ID || high.Bumps[0].Airing.Title != "News" {
		t.Fatalf("a higher pass pushes News out: %+v", high)
	}
	if one := read(`{"title":"game","matchKind":"contains","priority":5,"days":[]}`); len(one.Items) != 2 {
		t.Fatalf("%+v", one)
	}
	// An edit previews in place of the saved pass, and nothing is saved.
	edit := read(fmt.Sprintf(`{"id":%d,"title":"Game Night"}`, news.ID))
	if len(edit.Items) != 1 || edit.Items[0].Airing.Title != "Game Night" || edit.Items[0].PassID != news.ID {
		t.Fatalf("edit preview %+v", edit)
	}
	if got := passList(t, get(t, h, "/api/v1/passes")); len(got) != 1 || got[0].Title != "News" {
		t.Fatalf("preview saved: %+v", got)
	}
	if rec := postJSON(t, h, "/api/v1/passes/preview", `{"title":"x","timeStart":"9"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad rule: %d", rec.Code)
	}
	if rec := postJSON(t, h, "/api/v1/passes/preview", `{"id":999}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing pass: %d", rec.Code)
	}
}
