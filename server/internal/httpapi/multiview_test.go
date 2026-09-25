package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestPickerNamesTheCostAndTheRecording(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:9", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF"},
		{GuideNumber: "4.2", GuideName: "WDAF2"},
		{GuideNumber: "5.1", GuideName: "KCTV"},
		{GuideNumber: "9.1", GuideName: "KMBC"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		guide string
		hz    int
	}{
		{"4.1", 593000000},
		{"4.2", 593000000},
		{"5.1", 533000000},
		{"9.1", 575000000},
	} {
		if err := st.RememberProgram(ctx, "D", row.guide, row.hz, 1); err != nil {
			t.Fatal(err)
		}
	}
	channels, err := st.Channels(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	id := map[string]int64{}
	for _, ch := range channels {
		id[ch.GuideNumber] = ch.ID
	}
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	h := (&Server{Store: st, Clock: func() time.Time { return now }}).Handler()

	one := postPlan(t, h, []int64{id["4.1"]}, true)
	got := map[int64]live.Offer{}
	for _, o := range one.Offers {
		got[o.ChannelID] = o
	}
	if got[id["4.2"]].Label != "Same tune as 4.1" || got[id["5.1"]].Label != "Uses a tuner" {
		t.Fatalf("%+v", one.Offers)
	}
	if len(one.Stops) != 0 {
		t.Fatalf("no recording yet %+v", one.Stops)
	}

	start := time.Date(2026, 9, 25, 15, 1, 30, 0, time.Local)
	if err := st.ReplaceAirings(ctx, []store.Airing{{
		ChannelID: id["5.1"], Title: "Jeopardy", Start: start, End: start.Add(30 * time.Minute),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Jeopardy", id["5.1"], 1, 2); err != nil {
		t.Fatal(err)
	}
	both := postPlan(t, h, []int64{id["4.1"], id["9.1"]}, true)
	if len(both.Stops) != 1 || both.Stops[0].ChannelID != id["9.1"] {
		t.Fatalf("%+v", both.Stops)
	}
	if both.Stops[0].Reason != "9.1 stops at 3:00 PM. Jeopardy is recording." {
		t.Fatalf("reason %q", both.Stops[0].Reason)
	}
	got = map[int64]live.Offer{}
	for _, o := range both.Offers {
		got[o.ChannelID] = o
	}
	if got[id["4.2"]].Label != "Same tune as 4.1" {
		t.Fatalf("share %+v", got[id["4.2"]])
	}
	if got[id["5.1"]].Cost != "tuner" || got[id["5.1"]].Label != "Uses a tuner" {
		t.Fatalf("recording channel %+v", got[id["5.1"]])
	}
	if got[id["4.1"]].Cost != "on" || got[id["9.1"]].Cost != "on" {
		t.Fatalf("on screen %+v %+v", got[id["4.1"]], got[id["9.1"]])
	}
}

func postPlan(t *testing.T, h http.Handler, ids []int64, picker bool) live.MultiviewPlan {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"channelIds": ids, "picker": picker})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/multiview/plan", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var plan live.MultiviewPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}
