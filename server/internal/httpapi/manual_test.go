package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func manualServer(t *testing.T, tuners int) (http.Handler, *store.Store, int64, int64) {
	t.Helper()
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "BOX", FriendlyName: "Lab", TunerCount: tuners},
		[]hdhr.Channel{{GuideNumber: "4.1", GuideName: "FOX"}, {GuideNumber: "5.1", GuideName: "NBC"}}); err != nil {
		t.Fatal(err)
	}
	return (&Server{Store: st}).Handler(), st, guideID(t, st, "4.1"), guideID(t, st, "5.1")
}

func manualSchedule(t *testing.T, h http.Handler) []dvr.Planned {
	t.Helper()
	rec := get(t, h, "/api/v1/schedule")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []dvr.Planned `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}

func TestAManualPassRecordsATimeWithoutListings(t *testing.T) {
	h, _, fox, _ := manualServer(t, 2)
	start := time.Now().In(time.Local).Add(2 * time.Hour).Truncate(time.Minute)
	body := fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":%q,"timeStart":%q,"duration":90}`,
		fox, start.Format(time.DateOnly), start.Format("15:04"))
	passes := passList(t, postJSON(t, h, "/api/v1/passes", body))
	if len(passes) != 1 {
		t.Fatalf("%+v", passes)
	}
	p := passes[0]
	if p.Kind != "manual" || p.ChannelID != fox || p.Duration != 90 || p.Date != start.Format(time.DateOnly) ||
		p.TimeStart != start.Format("15:04") || !p.AiringStart.Equal(start) || p.PadBefore != 0 || p.PadAfter != 0 || p.Title != "" {
		t.Fatalf("pass: %+v", p)
	}
	items := manualSchedule(t, h)
	if len(items) != 1 || !items[0].Manual || items[0].PassID != p.ID || !items[0].Airing.Start.Equal(start) ||
		!items[0].Airing.End.Equal(start.Add(90*time.Minute)) || items[0].Airing.Title != "FOX" {
		t.Fatalf("schedule: %+v", items)
	}

	// Edit to repeat on two weekdays; the date goes away.
	passes = passList(t, sendJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/passes/%d", p.ID),
		`{"days":[1,3],"duration":30,"rename":"Late news","keepMode":"last","keepCount":2}`))
	p = passes[0]
	if fmt.Sprint(p.Days) != "[1 3]" || p.Date != "" || !p.AiringStart.IsZero() || p.Duration != 30 || p.Title != "Late news" || p.KeepCount != 2 {
		t.Fatalf("edit: %+v", p)
	}
	for _, item := range manualSchedule(t, h) {
		if wd := item.Airing.Start.In(time.Local).Weekday(); wd != time.Monday && wd != time.Wednesday || item.Airing.Title != "Late news" {
			t.Fatalf("repeat: %+v", item)
		}
	}
	if rec := sendJSON(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/passes/%d", p.ID), ""); len(passList(t, rec)) != 0 {
		t.Fatal("delete")
	}
}

func TestAManualPassIsChecked(t *testing.T) {
	h, _, fox, _ := manualServer(t, 2)
	past := time.Now().In(time.Local).Add(-3 * time.Hour)
	for _, bad := range []string{
		`{"kind":"manual","date":"2030-01-01","timeStart":"22:00","duration":30}`,
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":"2030-01-01","timeStart":"9:00","duration":30}`, fox),
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":"2030-01-01","timeStart":"22:00","duration":0}`, fox),
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":"2030-01-01","timeStart":"22:00","duration":481}`, fox),
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"timeStart":"22:00","duration":30}`, fox),
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"days":[9],"timeStart":"22:00","duration":30}`, fox),
		`{"kind":"manual","channelId":99999,"date":"2030-01-01","timeStart":"22:00","duration":30}`,
		fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":%q,"timeStart":%q,"duration":60}`, fox, past.Format(time.DateOnly), past.Format("15:04")),
	} {
		if rec := postJSON(t, h, "/api/v1/passes", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	if got := passList(t, get(t, h, "/api/v1/passes")); len(got) != 0 {
		t.Fatalf("saved: %+v", got)
	}
}

func TestAManualPassTakesTheListingAndPreviewsItsConflicts(t *testing.T) {
	h, st, fox, nbc := manualServer(t, 1)
	ctx := context.Background()
	start := time.Now().In(time.Local).Add(2 * time.Hour).Truncate(time.Minute)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: fox, Title: "Night News", Description: "Local news.", ProgramID: "EP1", Start: start, End: start.Add(time.Hour)},
		{ChannelID: nbc, Title: "Game", ProgramID: "GAME1", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	game := passList(t, postJSON(t, h, "/api/v1/passes", `{"title":"Game","padBefore":0,"padAfter":0}`))[0]
	type preview struct {
		Items    []dvr.Planned `json:"items"`
		Bumps    []dvr.Planned `json:"bumps"`
		TimeZone string        `json:"timeZone"`
	}
	body := fmt.Sprintf(`{"kind":"manual","channelId":%d,"date":%q,"timeStart":%q,"duration":60}`,
		fox, start.Format(time.DateOnly), start.Format("15:04"))
	rec := postJSON(t, h, "/api/v1/passes/preview", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var p preview
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].Airing.Title != "Night News" || p.Items[0].Airing.Description != "Local news." || p.TimeZone == "" {
		t.Fatalf("preview: %+v", p)
	}
	// Asked for by time, it ranks above the series, so it pushes the game out.
	if p.Items[0].Skipped || len(p.Bumps) != 1 || p.Bumps[0].PassID != game.ID {
		t.Fatalf("bumps: %+v", p)
	}
	if got := passList(t, get(t, h, "/api/v1/passes")); len(got) != 1 {
		t.Fatalf("preview saved: %+v", got)
	}
	passList(t, postJSON(t, h, "/api/v1/passes", body))
	items := manualSchedule(t, h)
	var manual dvr.Planned
	for _, item := range items {
		if item.Manual {
			manual = item
		} else if !item.Skipped || !item.Conflict {
			t.Fatalf("the game should lose the tuner: %+v", items)
		}
	}
	// Skipping the slot by pass leaves the listing's episode alone.
	skip := fmt.Sprintf(`{"passId":%d,"programId":"EP1","title":"Night News","channelId":%d,"start":%q}`,
		manual.PassID, fox, manual.Airing.Start.UTC().Format(time.RFC3339))
	if rec := postJSON(t, h, "/api/v1/schedule/skip", skip); rec.Code != http.StatusOK {
		t.Fatalf("skip: %d %s", rec.Code, rec.Body.String())
	}
	skips, _ := st.Skips(ctx)
	if !skips[dvr.ManualSkipKey(manual.PassID)+"|"+manual.Airing.Start.UTC().Format(time.RFC3339)] || skips["id:EP1|"+manual.Airing.Start.UTC().Format(time.RFC3339)] {
		t.Fatalf("skips: %v", skips)
	}
}
