package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestNowAndNext(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "AAAA0001", FriendlyName: "FLEX", BaseURL: "http://127.0.0.1:1", TunerCount: 2},
		[]hdhr.Channel{{GuideNumber: "4.1", GuideName: "KNBC"}, {GuideNumber: "5.1", GuideName: "KTLA"}}); err != nil {
		t.Fatal(err)
	}
	chs, _ := st.Channels(ctx, true)
	at := time.Date(2026, 10, 15, 20, 15, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []store.Airing{
		{ChannelID: chs[0].ID, Title: "Earlier", Start: at.Add(-2 * time.Hour), End: at.Add(-time.Hour)},
		{ChannelID: chs[0].ID, Title: "News", Description: "long", ImageURL: "https://img.example/n.jpg", Start: at.Add(-15 * time.Minute), End: at.Add(45 * time.Minute)},
		{ChannelID: chs[0].ID, Title: "Late Show", Start: at.Add(45 * time.Minute), End: at.Add(105 * time.Minute)},
		{ChannelID: chs[0].ID, Title: "After", Start: at.Add(105 * time.Minute), End: at.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Clock: func() time.Time { return at.Add(20 * time.Second) }}
	rec := get(t, s.Handler(), "/api/v1/now")
	var body struct {
		At       time.Time    `json:"at"`
		Channels []nowChannel `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.At.Equal(at) || len(body.Channels) != 2 {
		t.Fatalf("%s", rec.Body)
	}
	first := body.Channels[0]
	if first.Now == nil || first.Now.Title != "News" || first.Next == nil || first.Next.Title != "Late Show" {
		t.Fatalf("now/next %s", rec.Body)
	}
	if first.Progress != 0.25 || first.Now.Description != "" || first.Art != "/media/art/airing/"+itoa(first.Now.ID) {
		t.Fatalf("row %+v", first)
	}
	empty := body.Channels[1]
	if empty.Now != nil || empty.Next != nil || empty.Art != "/media/art/channel/"+itoa(chs[1].ID) {
		t.Fatalf("empty channel %+v", empty)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
