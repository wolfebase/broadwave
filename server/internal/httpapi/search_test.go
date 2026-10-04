package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestSearchListsNamedShowsThenWhatIsOnNowFirst(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:9", TunerCount: 2,
	}, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}, {GuideNumber: "5.1", GuideName: "WTST"}}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, true)
	if err != nil || len(channels) != 2 {
		t.Fatal(err, channels)
	}
	now := time.Date(2026, 10, 4, 7, 0, 0, 0, time.Local)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 4, h, m, 0, 0, time.Local) }
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: channels[0].ID, Title: "Farm News", Start: at(5, 0), End: at(5, 30)},
		{ChannelID: channels[1].ID, Title: "Evening News", Start: at(18, 0), End: at(18, 30)},
		{ChannelID: channels[0].ID, Title: "Morning News", Start: at(6, 0), End: at(9, 0)},
		{ChannelID: channels[1].ID, Title: "Noon News", Start: at(12, 0), End: at(12, 30)},
		{ChannelID: channels[1].ID, Title: "Ball Game", Description: "Scores and news at the half.", Start: at(6, 30), End: at(9, 30)},
	}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Clock: func() time.Time { return now }}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=news", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Airings []struct {
			Title string `json:"title"`
		} `json:"airings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range got.Airings {
		titles = append(titles, a.Title)
	}
	// Shows named "news" first, each group on now and then by start.
	want := []string{"Morning News", "Noon News", "Evening News", "Ball Game"}
	if len(titles) != len(want) {
		t.Fatalf("titles %v, want %v (a show that ended at 5:30 is not a result at 7:00)", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("titles %v, want %v", titles, want)
		}
	}
}
