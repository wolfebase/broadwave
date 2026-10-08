package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/store"
)

// Record it again names the next airing of the same episode: by program id on
// any channel, by name on the same channel, and nothing for a show with neither.
func TestRecordAgainNamesTheNextAiring(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	rows := []store.Airing{
		{ChannelID: 1, Title: "Show", ProgramID: "EP1", Start: now.Add(-time.Hour), End: now},
		{ChannelID: 2, Title: "Show", ProgramID: "EP1", Start: now.Add(26 * time.Hour), End: now.Add(27 * time.Hour)},
		{ChannelID: 1, Title: "Show", ProgramID: "EP1", Start: now.Add(50 * time.Hour), End: now.Add(51 * time.Hour)},
		{ChannelID: 1, Title: "Show", ProgramID: "EP2", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{ChannelID: 2, Title: "Quiz", Subtitle: "Finals", Start: now.Add(5 * time.Hour), End: now.Add(6 * time.Hour)},
		{ChannelID: 3, Title: "Quiz", Subtitle: "Finals", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)},
	}
	if err := st.ReplaceAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	add := func(rec store.Recording) int64 {
		rec.Status, rec.StartedAt = "complete", now.Add(-48*time.Hour)
		id, err := st.CreateRecording(ctx, rec)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	h := (&Server{Store: st}).Handler()
	again := func(id int64) *store.Airing {
		t.Helper()
		var body struct {
			Airing *store.Airing `json:"airing"`
		}
		res := get(t, h, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/again")
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Airing
	}
	if got := again(add(store.Recording{ChannelID: 1, Title: "Show", ProgramID: "EP1"})); got == nil || got.ChannelID != 2 || !got.Start.Equal(now.Add(26*time.Hour)) {
		t.Fatalf("by program id: %+v", got)
	}
	if got := again(add(store.Recording{ChannelID: 2, Title: "Quiz", Subtitle: "Finals"})); got == nil || got.ChannelID != 2 {
		t.Fatalf("by name: %+v", got)
	}
	if got := again(add(store.Recording{ChannelID: 1, Title: "News"})); got != nil {
		t.Fatalf("no episode: %+v", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/999/again", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing recording %d", rec.Code)
	}
}
