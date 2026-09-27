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
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestScheduleSuggestsALaterAiringAndFixSkipsTheConflict(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "BOX", FriendlyName: "Lab", TunerCount: 1,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "FOX"},
		{GuideNumber: "5.1", GuideName: "NBC"},
	}); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	nbc := guideID(t, st, "5.1")
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: fox, Title: "News", ProgramID: "NEWS", Start: start, End: start.Add(time.Hour)},
		{ChannelID: nbc, Title: "Game", ProgramID: "GAME", Start: start, End: start.Add(time.Hour)},
		{ChannelID: fox, Title: "News", ProgramID: "NEWS", Start: start.Add(2 * time.Hour), End: start.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "News", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Game", nbc, 0, 0); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pass := range passes {
		if pass.Title == "Game" {
			if err := st.UpdatePass(ctx, pass.ID, 0, 0, 5); err != nil {
				t.Fatal(err)
			}
		}
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/schedule")
	var body struct {
		TunerCount int           `json:"tunerCount"`
		Items      []dvr.Planned `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TunerCount != 1 {
		t.Fatalf("tuners %d", body.TunerCount)
	}
	var skipped dvr.Planned
	found := false
	for _, item := range body.Items {
		if item.Airing.Title == "News" && item.Skipped {
			skipped = item
			found = true
		}
		if item.Airing.Title == "News" && !item.Skipped && item.Airing.Start.Equal(start) {
			t.Fatalf("early news should be skipped: %+v", item)
		}
	}
	if !found || skipped.Suggestion == nil || skipped.Suggestion.ChannelID != fox || skipped.Suggestion.GuideNumber != "4.1" || !skipped.Suggestion.Start.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("%+v", body.Items)
	}
	fix := postJSON(t, h, "/api/v1/schedule/fix", fmt.Sprintf(
		`{"passId":%d,"channelId":%d,"start":%q,"suggestionChannelId":%d,"suggestionStart":%q}`,
		skipped.PassID, skipped.Airing.ChannelID, skipped.Airing.Start.Format(time.RFC3339),
		skipped.Suggestion.ChannelID, skipped.Suggestion.Start.Format(time.RFC3339),
	))
	if fix.Code != http.StatusOK {
		t.Fatalf("fix %d %s", fix.Code, fix.Body.String())
	}
	for _, pass := range passes {
		if pass.Title == "Game" {
			if err := st.DeletePass(ctx, pass.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	again := get(t, h, "/api/v1/schedule")
	body.Items = nil
	if err := json.Unmarshal(again.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var early, late bool
	for _, item := range body.Items {
		if item.Airing.Title != "News" {
			continue
		}
		if item.Airing.Start.Equal(start) {
			early = item.Skipped && item.Reason == "Skipped once"
		}
		if item.Airing.Start.Equal(start.Add(2 * time.Hour)) {
			late = !item.Skipped
		}
	}
	if !early || !late {
		t.Fatalf("skip did not stick: %+v", body.Items)
	}
}

func TestLiveWatchDoesNotStarveARecording(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	tuner := &fake.Server{}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	dev.TunerCount = 1
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	nbc := guideID(t, st, "5.1")
	now := time.Now().UTC().Truncate(time.Second)
	soon := now.Add(30 * time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: nbc, Title: "News", ProgramID: "NEWS", Start: soon, End: soon.Add(30 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "News", nbc, 2, 2); err != nil {
		t.Fatal(err)
	}
	api := &Server{
		Store: st,
		HDHR:  client,
		Hub:   &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264"},
		Clock: func() time.Time { return now },
	}
	h := api.Handler()
	warn := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, fox))
	if warn.Code != http.StatusConflict {
		t.Fatalf("warn %d %s", warn.Code, warn.Body.String())
	}
	var problem struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(warn.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "recording_soon" || !strings.Contains(problem.Message, "News") || !strings.Contains(problem.Message, "Watching stops when that recording starts.") {
		t.Fatalf("%+v", problem)
	}
	for _, path := range tuner.Requests() {
		if strings.Contains(path, "/tuner") || strings.Contains(path, "/auto/") {
			t.Fatalf("watch tuned on the warning: %v", tuner.Requests())
		}
	}

	ok := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d,"confirmLive":true}`, fox))
	if ok.Code == http.StatusConflict || strings.Contains(ok.Body.String(), "recording_soon") {
		t.Fatalf("Watch anyway still blocked: %d %s", ok.Code, ok.Body.String())
	}
	tuned := false
	for _, path := range tuner.Requests() {
		if strings.Contains(path, "/tuner") || strings.Contains(path, "/auto/") {
			tuned = true
			break
		}
	}
	if !tuned {
		t.Fatalf("Watch anyway did not tune (%d %s): %v", ok.Code, ok.Body.String(), tuner.Requests())
	}
	// The lab tuner has no MPEG-TS and turns the tune down after the guard opens.
	if ok.Code != http.StatusServiceUnavailable || !strings.Contains(ok.Body.String(), "tuner_refused") {
		t.Fatalf("Watch anyway %d %s", ok.Code, ok.Body.String())
	}

	later := now.Add(3 * time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: nbc, Title: "News", ProgramID: "NEWS", Start: later, End: later.Add(30 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	outside := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d}`, fox))
	if outside.Code == http.StatusConflict {
		t.Fatalf("outside the pad: %s", outside.Body.String())
	}
}

func TestAConflictFixRecordsOnlyTheLaterAiring(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "BOX", FriendlyName: "Lab", TunerCount: 1,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "FOX"},
		{GuideNumber: "5.1", GuideName: "NBC"},
		{GuideNumber: "9.1", GuideName: "ABC"},
	}); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	nbc := guideID(t, st, "5.1")
	abc := guideID(t, st, "9.1")
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	later := start.Add(2 * time.Hour)
	local := later.In(time.Local)
	again := time.Date(local.Year(), local.Month(), local.Day()+1, local.Hour(), local.Minute(), local.Second(), 0, time.Local).UTC()
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: fox, Title: "News", Category: "Sports", ProgramID: "NEWS1", Start: start, End: start.Add(time.Hour)},
		{ChannelID: nbc, Title: "Game", ProgramID: "GAME", Start: start, End: start.Add(time.Hour)},
		{ChannelID: abc, Title: "News", Category: "Sports", ProgramID: "NEWS2", Start: later, End: later.Add(time.Hour)},
		{ChannelID: abc, Title: "News", Category: "Sports", ProgramID: "NEWS3", Start: again, End: again.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Sports", fox, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Game", nbc, 0, 0); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pass := range passes {
		switch pass.Title {
		case "Sports":
			pass.MatchKind = "category"
			pass.ChannelID = fox
			if err := st.UpdatePassRules(ctx, pass); err != nil {
				t.Fatal(err)
			}
		case "Game":
			if err := st.UpdatePass(ctx, pass.ID, 0, 0, 5); err != nil {
				t.Fatal(err)
			}
		}
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/schedule")
	var body struct {
		Items []dvr.Planned `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var skipped dvr.Planned
	found := false
	for _, item := range body.Items {
		if item.Airing.Title == "News" && item.Skipped && item.Suggestion != nil {
			skipped = item
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", body.Items)
	}
	payload := fmt.Sprintf(
		`{"passId":%d,"channelId":%d,"start":%q,"suggestionChannelId":%d,"suggestionStart":%q}`,
		skipped.PassID, skipped.Airing.ChannelID, skipped.Airing.Start.Format(time.RFC3339),
		skipped.Suggestion.ChannelID, skipped.Suggestion.Start.Format(time.RFC3339),
	)
	fixed := postJSON(t, h, "/api/v1/schedule/fix", payload)
	if fixed.Code != http.StatusOK {
		t.Fatalf("fix %d %s", fixed.Code, fixed.Body.String())
	}
	body.Items = nil
	if err := json.Unmarshal(fixed.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var early, kept bool
	for _, item := range body.Items {
		if item.Airing.Title != "News" {
			continue
		}
		switch {
		case item.Airing.Start.Equal(start):
			early = item.Skipped && item.Reason == "Skipped once"
		case item.Airing.Start.Equal(later):
			kept = !item.Skipped
		case item.Airing.Start.Equal(again):
			t.Fatalf("the next day's airing in the same slot is planned: %+v", item)
		}
	}
	if !early || !kept {
		t.Fatalf("early %v kept %v items %+v", early, kept, body.Items)
	}
	passes, err = st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var once []store.Pass
	var sports store.Pass
	for _, pass := range passes {
		switch pass.Title {
		case "News":
			once = append(once, pass)
		case "Sports":
			sports = pass
		}
	}
	if len(once) != 1 || once[0].Kind != "once" || once[0].ChannelID != abc || !once[0].AiringStart.Equal(later) || once[0].Priority != sports.Priority {
		t.Fatalf("%+v", once)
	}
	// A second press of the same fix keeps one pass.
	if again := postJSON(t, h, "/api/v1/schedule/fix", payload); again.Code == http.StatusOK {
		passes, _ = st.Passes(ctx)
		n := 0
		for _, pass := range passes {
			if pass.Title == "News" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%d News passes", n)
		}
	}
}

func guideID(t *testing.T, st *store.Store, number string) int64 {
	t.Helper()
	list, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range list {
		if ch.GuideNumber == number {
			return ch.ID
		}
	}
	t.Fatalf("missing %s", number)
	return 0
}

func postJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAddPassForOneAiring(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	if rec := postJSON(t, h, "/api/v1/passes", `{"title":"Jeopardy","airingStart":"2026-09-28T16:00:00Z"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no channel: %d %s", rec.Code, rec.Body.String())
	}
	rec := postJSON(t, h, "/api/v1/passes", `{"title":"Jeopardy","channelId":3,"airingStart":"2026-09-28T11:00:00-05:00"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Passes []store.Pass `json:"passes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)
	if len(body.Passes) != 1 || body.Passes[0].Kind != "once" || body.Passes[0].ChannelID != 3 || !body.Passes[0].AiringStart.Equal(want) {
		t.Fatalf("passes %+v", body.Passes)
	}
	if !strings.Contains(rec.Body.String(), `"airingStart":"2026-09-28T16:00:00Z"`) {
		t.Fatal(rec.Body.String())
	}
	patch := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", body.Passes[0].ID),
		strings.NewReader(`{"padAfter":5,"channelId":9,"keepMode":"last","keepCount":1,"timeStart":"10:00","timeEnd":"11:00"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, patch)
	body.Passes = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if p := body.Passes[0]; p.PadAfter != 5 || p.ChannelID != 3 || p.KeepMode == "last" || p.TimeStart != "" || !p.AiringStart.Equal(want) {
		t.Fatalf("a once pass takes only its pads: %+v", p)
	}
	var series struct {
		Passes []map[string]any `json:"passes"`
	}
	rec = postJSON(t, h, "/api/v1/passes", `{"title":"News"}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &series)
	for _, pass := range series.Passes {
		if pass["title"] == "News" {
			if _, ok := pass["airingStart"]; ok {
				t.Fatalf("a series pass has no airing: %v", pass)
			}
		}
	}
}
