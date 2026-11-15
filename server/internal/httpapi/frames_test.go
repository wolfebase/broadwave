package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func writePreview(t *testing.T, dir string, id int64, width int, age time.Duration) {
	t.Helper()
	path := live.FramePath(dir, id, width)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if age > 0 {
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

func listedFrames(t *testing.T, h http.Handler) []int64 {
	t.Helper()
	res := get(t, h, "/api/v1/frames")
	if res.Code != http.StatusOK {
		t.Fatalf("frames %d %s", res.Code, res.Body.String())
	}
	var body struct {
		Channels []int64 `json:"channels"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Channels == nil {
		t.Fatal("channels is null")
	}
	return body.Channels
}

func TestFramesListsAFreshPreview(t *testing.T) {
	dir := t.TempDir()
	writePreview(t, dir, 3, 480, 0)
	writePreview(t, dir, 7, 480, 0)
	// A tune's still outlives its tune as long as a swept one: one from a
	// game watched until ten minutes ago was dropped while the sweep, half
	// an hour behind, had not come back to it.
	writePreview(t, dir, 8, 480, 20*time.Minute)
	writePreview(t, dir, 9, 480, live.SweepStaleAfter+time.Minute)
	writePreview(t, dir, 11, 1280, 0)
	part := live.FramePath(dir, 12, 480) + ".part"
	if err := os.WriteFile(part, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	got := listedFrames(t, h)
	want := []int64{3, 7, 8}
	if len(got) != len(want) || got[0] != 3 || got[1] != 7 || got[2] != 8 {
		t.Fatalf("frames %v, want %v", got, want)
	}

	emptyDir := t.TempDir()
	none := listedFrames(t, (&Server{Hub: &live.Hub{Dir: emptyDir}}).Handler())
	if len(none) != 0 {
		t.Fatalf("empty dir %v", none)
	}
	missing := listedFrames(t, (&Server{}).Handler())
	if len(missing) != 0 {
		t.Fatalf("no hub %v", missing)
	}
}

func TestFrameIsStaleAfterTenMinutes(t *testing.T) {
	dir := t.TempDir()
	path := live.FramePath(dir, 7, 480)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	wide := live.FramePath(dir, 7, 1280)
	if err := os.WriteFile(wide, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	stale := get(t, h, "/api/v1/channels/7/frame")
	if stale.Code != 200 || stale.Header().Get("X-Frame-Stale") != "1" || stale.Header().Get("Last-Modified") == "" {
		t.Fatalf("stale %d %v", stale.Code, stale.Header())
	}
	fresh := get(t, h, "/api/v1/channels/7/frame?w=1280")
	if fresh.Code != 200 || fresh.Header().Get("X-Frame-Stale") != "" {
		t.Fatalf("fresh %d %q", fresh.Code, fresh.Header().Get("X-Frame-Stale"))
	}
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/9/frame", nil)
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, missingReq)
	if missing.Code != 404 {
		t.Fatalf("missing %d", missing.Code)
	}
}

func TestAFullScreenStillAndItsStamp(t *testing.T) {
	dir := t.TempDir()
	writePreview(t, dir, 7, 480, 0)
	writePreview(t, dir, 7, 1280, 0)
	full := live.FramePath(dir, 7, 1920)
	if err := os.WriteFile(full, []byte{0xff, 0xd8, 0x19, 0x20, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	writePreview(t, dir, 9, 480, 0)
	writePreview(t, dir, 9, 1280, 0)
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	if got := get(t, h, "/api/v1/channels/7/frame?w=1920"); got.Code != 200 || got.Body.Len() != 6 {
		t.Fatalf("1920: %d, %d bytes", got.Code, got.Body.Len())
	}
	// A grab from before the full-screen size answers with the wide one.
	if got := get(t, h, "/api/v1/channels/9/frame?w=1920"); got.Code != 200 || got.Body.Len() != 4 {
		t.Fatalf("1920 without one: %d", got.Code)
	}
	res := get(t, h, "/api/v1/frames")
	var body struct {
		Channels []int64 `json:"channels"`
		Frames   []struct {
			ID int64 `json:"id"`
			At int64 `json:"at"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(live.FramePath(dir, 7, 480))
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Frames) != 2 || body.Frames[0].ID != 7 || body.Frames[0].At != info.ModTime().Unix() || len(body.Channels) != 2 {
		t.Fatalf("frames %s", res.Body.String())
	}
	at := strconv.FormatInt(body.Frames[0].At, 10)
	if c := get(t, h, "/api/v1/channels/7/frame?w=1920&at="+at).Header().Get("Cache-Control"); !strings.Contains(c, "immutable") {
		t.Fatalf("a stamped still is not cached: %q", c)
	}
	if c := get(t, h, "/api/v1/channels/7/frame?w=1920&at=12").Header().Get("Cache-Control"); c != "no-cache" {
		t.Fatalf("an old stamp is cached: %q", c)
	}
}

type stampBody struct {
	Frames []struct {
		ID      int64 `json:"id"`
		At      int64 `json:"at"`
		Earlier bool  `json:"earlier"`
	} `json:"frames"`
}

func stamps(t *testing.T, h http.Handler) map[int64]int64 {
	t.Helper()
	var body stampBody
	if err := json.Unmarshal(get(t, h, "/api/v1/frames").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[int64]int64{}
	for _, f := range body.Frames {
		out[f.ID] = f.At
	}
	return out
}

// twinStore has a station on two tuners (a SameAs row) and a station whose
// 3.0 channel shows and whose 1.0 channel is hidden.
func twinStore(t *testing.T) (*store.Store, map[string][]store.Channel) {
	t.Helper()
	st := testStore(t)
	ctx := context.Background()
	duo := hdhr.Device{DeviceID: "DUO1", BaseURL: "http://duo", LineupURL: "http://duo/lineup.json", TunerCount: 2}
	if err := st.UpsertDevice(ctx, duo, []hdhr.Channel{
		{GuideNumber: "5.1", GuideName: "WTST", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://duo:5004/auto/v5.1"},
	}); err != nil {
		t.Fatal(err)
	}
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := st.UpsertDevice(ctx, flex, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "5.1", GuideName: "WTST", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v5.1"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
	}); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string][]store.Channel{}
	for _, ch := range chs {
		by[ch.GuideNumber] = append(by[ch.GuideNumber], ch)
	}
	return st, by
}

// A still of one row is the picture of every row of the same broadcast: a
// station's row on another tuner, and the other half of a 1.0/3.0 pair. A
// grab of the hidden 1.0 channel left the shown 3.0 channel with none.
func TestAStillServesTheSameBroadcastOnAnotherRow(t *testing.T) {
	st, by := twinStore(t)
	c3, c1 := by["104.1"][0], by["4.1"][0]
	if c3.Hidden || !c1.Hidden || c1.TwinID != c3.ID {
		t.Fatalf("104.1 %+v 4.1 %+v", c3, c1)
	}
	var shown, same store.Channel
	for _, ch := range by["5.1"] {
		if ch.SameAs == 0 {
			shown = ch
		} else {
			same = ch
		}
	}
	if same.ID == 0 || same.SameAs != shown.ID {
		t.Fatalf("5.1 rows %+v", by["5.1"])
	}
	dir := t.TempDir()
	writePreview(t, dir, c1.ID, 480, time.Minute)
	writePreview(t, dir, c1.ID, 1920, time.Minute)
	writePreview(t, dir, shown.ID, 480, 0)
	h := (&Server{Store: st, Hub: &live.Hub{Dir: dir}}).Handler()
	got := stamps(t, h)
	if got[c3.ID] == 0 || got[c3.ID] != got[c1.ID] {
		t.Fatalf("the 3.0 channel has its twin's still: %v", got)
	}
	if got[same.ID] == 0 || got[same.ID] != got[shown.ID] {
		t.Fatalf("the row on the other tuner has the shown row's still: %v", got)
	}
	at := strconv.FormatInt(got[c3.ID], 10)
	res := get(t, h, "/api/v1/channels/"+strconv.FormatInt(c3.ID, 10)+"/frame?w=1920&at="+at)
	if res.Code != 200 || !strings.Contains(res.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("the twin's picture: %d %v", res.Code, res.Header())
	}
	// Its own newer still wins.
	writePreview(t, dir, c3.ID, 480, 0)
	if now := stamps(t, h); now[c3.ID] == got[c3.ID] {
		t.Fatalf("its own newer still: %v", now)
	}
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/channels/999/frame", nil))
	if missing.Code != 404 {
		t.Fatalf("no still: %d", missing.Code)
	}
}

// A still from before the program on now, or its first minutes, is marked,
// so a card for that program does not show the one before.
func TestAStillFromTheProgramBeforeIsMarked(t *testing.T) {
	st, by := twinStore(t)
	c3, c1 := by["104.1"][0], by["4.1"][0]
	var five store.Channel
	for _, ch := range by["5.1"] {
		if ch.SameAs == 0 {
			five = ch
		}
	}
	now := time.Now()
	if err := st.InsertAirings(context.Background(), []store.Airing{
		{ChannelID: five.ID, Title: "News", Start: now.Add(-5 * time.Minute), End: now.Add(25 * time.Minute)},
		{ChannelID: c1.ID, Title: "Football", Start: now.Add(-time.Hour), End: now.Add(2 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writePreview(t, dir, five.ID, 480, 8*time.Minute)
	writePreview(t, dir, c3.ID, 480, time.Minute)
	h := (&Server{Store: st, Hub: &live.Hub{Dir: dir}}).Handler()
	var body stampBody
	if err := json.Unmarshal(get(t, h, "/api/v1/frames").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	earlier := map[int64]bool{}
	for _, f := range body.Frames {
		earlier[f.ID] = f.Earlier
	}
	if !earlier[five.ID] {
		t.Fatalf("a still from before the news: %+v", body)
	}
	if earlier[c3.ID] {
		t.Fatalf("a still of the game, listed on its twin: %+v", body)
	}
	writePreview(t, dir, five.ID, 480, 4*time.Minute)
	if err := json.Unmarshal(get(t, h, "/api/v1/frames").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, f := range body.Frames {
		if f.ID == five.ID && !f.Earlier {
			t.Fatal("a still from the first minute of the news is the program before")
		}
	}
}
