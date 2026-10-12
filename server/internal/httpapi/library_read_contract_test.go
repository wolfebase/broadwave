package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// Search, storage, the library schedule, events, airings, a file move, and
// record-again are JSON the store can produce. None of them tunes or encodes.
func TestLibraryReadsMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	ctx := t.Context()
	st := testStore(t)
	dir := t.TempDir()
	hub := &live.Hub{Dir: dir}
	api := (&Server{Store: st, Hub: hub, Version: "dev", Staging: true}).Handler()

	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "north-pier", FriendlyName: "North Pier", TunerCount: 1,
		BaseURL: "http://north-pier.invalid", LineupURL: "http://north-pier.invalid/lineup.json",
	}, []hdhr.Channel{{
		GuideNumber: "7.1", GuideName: "North Pier",
		StreamURL: "http://north-pier.invalid/auto/v7.1",
	}}); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 || !chs[0].Present || !chs[0].Enabled || chs[0].Hidden || chs[0].SameAs != 0 {
		t.Fatalf("channel seed %+v %v", chs, err)
	}
	ch := chs[0]
	start := time.Now().Add(-15 * time.Minute).UTC().Truncate(time.Second)
	end := start.Add(time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{{
		ChannelID: ch.ID, Title: "Harbor Watch", Start: start, End: end,
	}}); err != nil {
		t.Fatal(err)
	}

	blank := successBody(t, doc, http.MethodGet, "/search", sendJSON(t, api, http.MethodGet, "/api/v1/search?q=", ""))
	if blank["query"] != "" || len(numberList(t, blank["airings"])) != 0 || len(numberList(t, blank["recordings"])) != 0 {
		t.Fatalf("blank q %#v", blank)
	}
	found := successBody(t, doc, http.MethodGet, "/search", sendJSON(t, api, http.MethodGet, "/api/v1/search?q=Harbor", ""))
	if found["query"] != "Harbor" || len(numberList(t, found["recordings"])) != 0 {
		t.Fatalf("search %#v", found)
	}
	hits := numberList(t, found["airings"])
	if len(hits) != 1 {
		t.Fatalf("hits %#v", found["airings"])
	}
	hit, ok := hits[0].(map[string]any)
	if !ok || hit["guideNumber"] != "7.1" || hit["channelName"] != "North Pier" || hit["title"] != "Harbor Watch" || hit["channelId"] != float64(ch.ID) {
		t.Fatalf("hit %#v", hits[0])
	}

	emptyEvents := successBody(t, doc, http.MethodGet, "/events", sendJSON(t, api, http.MethodGet, "/api/v1/events", ""))
	if len(numberList(t, emptyEvents["events"])) != 0 {
		t.Fatalf("events %#v", emptyEvents)
	}
	if err := st.AddEvent(ctx, "guide", "Harbor Watch listings loaded"); err != nil {
		t.Fatal(err)
	}
	oneEvent := successBody(t, doc, http.MethodGet, "/events", sendJSON(t, api, http.MethodGet, "/api/v1/events", ""))
	evs := numberList(t, oneEvent["events"])
	if len(evs) != 1 {
		t.Fatalf("events %#v", oneEvent)
	}
	ev, ok := evs[0].(map[string]any)
	if !ok || ev["kind"] != "guide" || ev["message"] != "Harbor Watch listings loaded" || ev["id"] == nil || ev["at"] == "" {
		t.Fatalf("event %#v", evs[0])
	}

	listed := successBody(t, doc, http.MethodGet, "/airings", sendJSON(t, api, http.MethodGet, "/api/v1/airings", ""))
	rows := numberList(t, listed["airings"])
	if len(rows) != 1 {
		t.Fatalf("airings %#v", listed)
	}
	row, ok := rows[0].(map[string]any)
	if !ok || row["title"] != "Harbor Watch" || row["channelId"] != float64(ch.ID) || row["guideNumber"] != nil || row["channelName"] != nil {
		t.Fatalf("airing %#v", rows[0])
	}
	gotStart, err := time.Parse(time.RFC3339, row["start"].(string))
	gotEnd, err2 := time.Parse(time.RFC3339, row["end"].(string))
	if err != nil || err2 != nil || !gotStart.Before(time.Now()) || !gotEnd.After(time.Now()) {
		t.Fatalf("window %v %v %v", row["start"], row["end"], err)
	}

	storage := successBody(t, doc, http.MethodGet, "/storage", sendJSON(t, api, http.MethodGet, "/api/v1/storage", ""))
	if storage["path"] != hub.Recordings() || storage["watermarkGB"] != float64(10) {
		t.Fatalf("storage %#v", storage)
	}
	free, _ := storage["freeBytes"].(float64)
	total, _ := storage["totalBytes"].(float64)
	if free <= 0 || total <= 0 {
		t.Fatalf("space %#v", storage)
	}

	showPath := filepath.Join(hub.Recordings(), "harbor.ts")
	if err := os.MkdirAll(filepath.Dir(showPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(showPath, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)
	recID, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: ch.ID, GuideNumber: "7.1", Title: "Harbor Watch",
		Path: showPath, Status: "complete", StartedAt: started, ProgramID: "harbor-watch",
	})
	if err != nil {
		t.Fatal(err)
	}
	shows := successBody(t, doc, http.MethodGet, "/storage/shows", sendJSON(t, api, http.MethodGet, "/api/v1/storage/shows", ""))
	groups := numberList(t, shows["shows"])
	if len(groups) != 1 {
		t.Fatalf("shows %#v", shows)
	}
	group, ok := groups[0].(map[string]any)
	if !ok || group["pass"] != nil || group["title"] != "Harbor Watch" || group["count"] != float64(1) || group["bytes"] != float64(1) || group["oldest"] != "2026-10-08T15:00:00Z" || group["newest"] != "2026-10-08T15:00:00Z" {
		t.Fatalf("show %#v", groups[0])
	}

	before := successBody(t, doc, http.MethodGet, "/virtuals/schedule", sendJSON(t, api, http.MethodGet, "/api/v1/virtuals/schedule", ""))
	if len(numberList(t, before["virtuals"])) != 0 {
		t.Fatalf("schedule %#v", before)
	}
	if _, err := st.CreateVirtual(ctx, "900", "Harbor Lights", []int64{recID}); err != nil {
		t.Fatal(err)
	}
	sched := successBody(t, doc, http.MethodGet, "/virtuals/schedule", sendJSON(t, api, http.MethodGet, "/api/v1/virtuals/schedule", ""))
	virts := numberList(t, sched["virtuals"])
	if len(virts) != 1 {
		t.Fatalf("schedule %#v", sched)
	}
	virt, ok := virts[0].(map[string]any)
	if !ok {
		t.Fatalf("virtual %#v", virts[0])
	}
	slots := numberList(t, virt["slots"])
	if virt["name"] != "Harbor Lights" || virt["orderMode"] != "custom" || len(numberList(t, virt["recordings"])) != 1 || len(slots) == 0 {
		t.Fatalf("virtual %#v", virt)
	}
	slot, ok := slots[0].(map[string]any)
	if !ok {
		t.Fatalf("slot %#v", slots[0])
	}
	slotStart, err := time.Parse(time.RFC3339, slot["start"].(string))
	slotEnd, err2 := time.Parse(time.RFC3339, slot["end"].(string))
	if err != nil || err2 != nil || slot["recordingId"] != float64(recID) || slot["title"] != "Harbor Watch" || slotEnd.Sub(slotStart) != 30*time.Minute || slotStart.After(time.Now()) {
		t.Fatalf("slot %#v %v", slot, err)
	}

	src := filepath.Join(hub.Recordings(), "south.ts")
	if err := os.WriteFile(src, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	moveID, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: ch.ID, GuideNumber: "7.1", Title: "South Pier",
		Path: src, Status: "complete", StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	moved := successBody(t, doc, http.MethodPost, "/recordings/{id}/move",
		sendJSON(t, api, http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(moveID, 10)+"/move", `{"name":"Harbor Lights/Pier"}`))
	file, _ := moved["file"].(string)
	if moved["ok"] != true || file != "Harbor Lights/Pier.ts" || strings.HasPrefix(file, "/") || strings.Contains(file, dir) {
		t.Fatalf("move %#v", moved)
	}
	got, err := os.ReadFile(filepath.Join(hub.Recordings(), filepath.FromSlash(file)))
	if err != nil || string(got) != "\x47" || hub.FFmpeg != "" {
		t.Fatalf("moved bytes %q %v", got, err)
	}

	again := successBody(t, doc, http.MethodGet, "/recordings/{id}/again",
		sendJSON(t, api, http.MethodGet, "/api/v1/recordings/"+strconv.FormatInt(recID, 10)+"/again", ""))
	if _, ok := again["airing"]; !ok || again["airing"] != nil {
		t.Fatalf("again %#v", again)
	}
}
