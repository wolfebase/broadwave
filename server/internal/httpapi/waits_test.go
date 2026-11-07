package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// The plan line tells a client the airing is waiting, in words it can show.
func TestScheduleSaysAnAiringIsWaiting(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "BOX", FriendlyName: "Lab", TunerCount: 1}, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "FOX"}}); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	start := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	end := start.Add(time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: fox, Title: "News", ProgramID: "NEWS", Start: start, End: end},
		{ChannelID: fox, Title: "News", ProgramID: "NEWS2", Start: end, End: end.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: fox, Start: start, End: end, Until: end, PassID: 1, Title: "News",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start,
	}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/schedule")
	var body struct {
		Items []struct {
			Waiting bool   `json:"waiting"`
			Skipped bool   `json:"skipped"`
			Reason  string `json:"reason"`
			Airing  struct {
				ProgramID string `json:"programId"`
			} `json:"airing"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items %+v", body.Items)
	}
	if !body.Items[0].Waiting || body.Items[0].Reason != "Waiting for a tuner" || body.Items[0].Skipped {
		t.Fatalf("waiting line %+v", body.Items[0])
	}
	if body.Items[1].Waiting {
		t.Fatalf("the next airing is not waiting: %+v", body.Items[1])
	}
}
