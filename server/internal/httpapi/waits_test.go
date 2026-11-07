package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
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

// A rule made while its airing is on doesn't claim that airing will record.
func TestScheduleDoesNotPromiseAnAiringAlreadyOn(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "BOX", FriendlyName: "Lab", TunerCount: 1}, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "FOX"}}); err != nil {
		t.Fatal(err)
	}
	fox := guideID(t, st, "4.1")
	start := time.Now().UTC().Add(-21 * time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Minute)
	if err := st.ReplaceAirings(ctx, []store.Airing{{ChannelID: fox, Title: "News", ProgramID: "NEWS", Start: start, End: end}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/schedule")
	var body struct {
		Items []struct {
			Skipped  bool   `json:"skipped"`
			Conflict bool   `json:"conflict"`
			Reason   string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || !body.Items[0].Skipped || body.Items[0].Conflict || body.Items[0].Reason != "Already on" {
		t.Fatalf("items %+v", body.Items)
	}
}

// A plan that can't read its waits says so instead of showing an airing as fine.
func TestScheduleFailsWhenWaitsCannotBeRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	db, err := sql.Open("sqlite", filepath.Join(dir, "broadwave.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE recording_waits`); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/schedule", nil))
	if rec.Code < 500 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}
