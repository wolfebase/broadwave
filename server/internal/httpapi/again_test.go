package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/hdhr"
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

// A 1.0 and 3.0 pair records on one channel. Record it again names that
// channel when the damaged recording is the copy and the episode is known
// only by its title and subtitle. Another station's earlier listing of the
// same name stays off the result.
func TestRecordAgainFollowsASimulcastToTheChannelThatRecords(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
		{GuideNumber: "8.1", GuideName: "WTST-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v8.1"},
	}); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var c1, c3, other int64
	for _, ch := range chs {
		switch ch.GuideNumber {
		case "4.1":
			c1 = ch.ID
		case "104.1":
			c3 = ch.ID
		case "8.1":
			other = ch.ID
		}
	}
	if c1 == 0 || c3 == 0 || other == 0 {
		t.Fatalf("channels 4.1=%d 104.1=%d 8.1=%d", c1, c3, other)
	}
	now := time.Now().UTC().Truncate(time.Second)
	soon := now.Add(2 * time.Hour)
	later := now.Add(4 * time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: other, Title: "City Desk", Subtitle: "The Vote", Start: soon, End: soon.Add(time.Hour)},
		{ChannelID: c1, Title: "City Desk", Subtitle: "The Vote", Start: later, End: later.Add(time.Hour)},
		{ChannelID: c3, Title: "City Desk", Subtitle: "The Vote", Start: later, End: later.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	add := func(channelID int64) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			ChannelID: channelID, Title: "City Desk", Subtitle: "The Vote",
			Status: "complete", StartedAt: now.Add(-48 * time.Hour),
		})
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
	// 3.0 is the one that shows, so the 1.0 recording records there.
	if got := again(add(c1)); got == nil || got.ChannelID != c3 || !got.Start.Equal(later) {
		t.Fatalf("copy on 4.1: %+v, want 104.1 at %s", got, later)
	}
	if got := again(add(c3)); got == nil || got.ChannelID != c3 || !got.Start.Equal(later) {
		t.Fatalf("recording on 104.1: %+v, want that channel at %s", got, later)
	}
	if err := st.SetTwinChoice(ctx, c3, store.TwinBoth); err != nil {
		t.Fatal(err)
	}
	// Both show, so the pair records on 4.1. The earlier listing on 8.1 is another station.
	if got := again(add(c3)); got == nil || got.ChannelID != c1 || !got.Start.Equal(later) {
		t.Fatalf("copy on 104.1 with both shown: %+v, want 4.1 at %s", got, later)
	}
	if got := again(add(c1)); got == nil || got.ChannelID != c1 || !got.Start.Equal(later) {
		t.Fatalf("recording on 4.1 with both shown: %+v, want that channel at %s", got, later)
	}
}
