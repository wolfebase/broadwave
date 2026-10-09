package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// These tests speak the requests Plex, Jellyfin, Emby, and Channels send to an
// HDHomeRun. None of them starts the emulator listener, and none of them dials
// the device they were given as a tuner.

const (
	uaPlex     = "PlexMediaServer/1.41.0.8992"
	uaJellyfin = "Jellyfin-Server/10.10.0"
	uaEmby     = "Emby/4.8.0"
	uaChannels = "ChannelsDVR/2024.1"
)

type tunerFixture struct {
	t       *testing.T
	st      *store.Store
	hits    *atomic.Int32
	mux     http.Handler
	quiet   http.Handler
	api     http.Handler
	ids     map[string]int64
	mosaic  string
	virtual int64
}

func newTunerFixture(t *testing.T) *tunerFixture {
	t.Helper()
	st := testStore(t)
	ctx := t.Context()
	if err := st.SetIdentity(ctx, "ab12cd34ef567890", "Harbor", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	hits := &atomic.Int32{}
	tuner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(tuner.Close)
	// TunerCount 0 and a closed port make a failed tune return before any probe.
	dead := "http://127.0.0.1:1"
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "LABFLEX1", FriendlyName: "Lab", BaseURL: tuner.URL, LineupURL: tuner.URL + "/lineup.json", TunerCount: 0,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, Favorite: true, StreamURL: dead + "/auto/v4.1"},
		{GuideNumber: "5.2", GuideName: "Rivers", VideoCodec: "H264", AudioCodec: "AC3", StreamURL: dead + "/auto/v5.2"},
		{GuideNumber: "7.1", GuideName: "Coast", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: dead + "/auto/v7.1"},
		{GuideNumber: "8.1", GuideName: "Plains", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: dead + "/auto/v8.1"},
		{GuideNumber: "105.1", GuideName: "WTST", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, Protected: true, StreamURL: dead + "/auto/v105.1"},
	}); err != nil {
		t.Fatal(err)
	}
	fx := &tunerFixture{t: t, st: st, ids: map[string]int64{}}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chs {
		fx.ids[ch.GuideNumber] = ch.ID
	}
	off := false
	if _, err := st.PatchChannel(ctx, fx.ids["7.1"], store.ChannelPatch{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	hide := true
	if _, err := st.PatchChannel(ctx, fx.ids["8.1"], store.ChannelPatch{Hidden: &hide}); err != nil {
		t.Fatal(err)
	}
	fx.mosaic = strconv.FormatInt(fx.ids["4.1"], 10) + "-" + strconv.FormatInt(fx.ids["5.2"], 10)
	if err := st.PutSettings(ctx, map[string]string{"exportMosaics": fx.mosaic}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(clip), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clip, []byte{0x47, 0x40}, 0o644); err != nil {
		t.Fatal(err)
	}
	recID, err := st.CreateRecording(ctx, store.Recording{
		Title: "Night Desk", GuideNumber: "4.1", Status: "complete", Path: clip, StartedAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVirtual(ctx, "4.1", "Same number", []int64{recID}); err != nil {
		t.Fatal(err)
	}
	virt, err := st.CreateVirtual(ctx, "9001", "Night", []int64{recID})
	if err != nil {
		t.Fatal(err)
	}
	fx.virtual = virt.ID
	show := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{
			ChannelID: fx.ids["4.1"], Title: "Night Desk", Subtitle: "The storm", Description: "Weather and the town.",
			Category: "News, movies", ProgramID: "EP0001", SeriesID: "SH-NIGHT", Season: 2, Episode: 10, EpisodeLabel: "S2E10",
			Start: show, End: show.Add(time.Hour),
		},
		{
			ChannelID: fx.ids["5.2"], Title: "River Notes", EpisodeLabel: "Part 2",
			Start: show, End: show.Add(30 * time.Minute),
		},
	}); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir}
	fx.hits = hits
	fx.mux = newEmulatorMux(st, hub)
	fx.quiet = newEmulatorMux(st, nil)
	fx.api = (&Server{Store: st, Hub: hub}).Handler()
	return fx
}

func (fx *tunerFixture) ask(handler http.Handler, method, path, ua string) *httptest.ResponseRecorder {
	fx.t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func (fx *tunerFixture) quietHits() {
	fx.t.Helper()
	if n := fx.hits.Load(); n != 0 {
		fx.t.Fatalf("the tuner was contacted %d times", n)
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body)
	}
}

func TestEmulatorDeviceID(t *testing.T) {
	if got := emulatorDeviceID("ab12cd34ef"); got != "AB12CD34" {
		t.Fatalf("hex id %s", got)
	}
	for _, id := range []string{"", "short", "WAVEGD01", "not-hex!"} {
		if got := emulatorDeviceID(id); got != "B0AD0001" {
			t.Fatalf("%q → %s", id, got)
		}
	}
}

func TestSiliconDustDocument(t *testing.T) {
	fx := newTunerFixture(t)
	disc := fx.ask(fx.mux, http.MethodGet, "/discover.json", "")
	var device map[string]any
	decodeBody(t, disc, &device)
	if _, ok := device["DeviceAuth"]; ok {
		t.Fatalf("DeviceAuth must be omitted: %s", disc.Body)
	}
	if device["DeviceID"] != "AB12CD34" || device["FriendlyName"] != "Harbor" {
		t.Fatalf("identity %s", disc.Body)
	}
	model, _ := device["ModelNumber"].(string)
	if model != emulatorModel || strings.Contains(strings.ToLower(model), "hdtc") {
		t.Fatalf("model %q", model)
	}
	if device["FirmwareName"] != emulatorFirmware || device["FirmwareVersion"] != "20260101" {
		t.Fatalf("firmware %s", disc.Body)
	}
	if device["TunerCount"] != float64(emulatedTuners) {
		t.Fatalf("tuners %v", device["TunerCount"])
	}
	if device["LineupURL"] != "http://example.com/lineup.json" || device["BaseURL"] != "http://example.com" {
		t.Fatalf("urls %s", disc.Body)
	}

	var rows []lineupRow
	decodeBody(t, fx.ask(fx.mux, http.MethodGet, "/lineup.json?show=found&DeviceAuth=ignored", ""), &rows)
	byNum := map[string]lineupRow{}
	for _, row := range rows {
		byNum[row.GuideNumber] = row
	}
	var ch lineupRow
	for _, row := range rows {
		if row.GuideNumber == "4.1" && row.GuideName == "KBWV" {
			ch = row
		}
	}
	if ch.HD != 1 || ch.Favorite != 1 || ch.VideoCodec != "MPEG2" || ch.AudioCodec != "AC3" || ch.URL != "http://example.com/auto/v4.1" || !strings.Contains(ch.Tags, "favorite") {
		t.Fatalf("4.1 %+v", ch)
	}
	raw := fx.ask(fx.mux, http.MethodGet, "/lineup.json", "").Body.String()
	if strings.Contains(raw, `"HD":true`) || strings.Contains(raw, `"Favorite":true`) || !strings.Contains(raw, `"HD":1`) {
		t.Fatalf("hd and favorite must be numbers: %s", raw)
	}
	if _, ok := byNum["7.1"]; ok {
		t.Fatal("a disabled channel is in the lineup")
	}
	if _, ok := byNum["8.1"]; ok {
		t.Fatal("a hidden channel is in the lineup")
	}
	if _, ok := byNum["105.1"]; ok {
		t.Fatal("a protected channel stays hidden")
	}
	if byNum["9001"].URL != "http://example.com/auto/v9001" || byNum["990.1"].URL != "http://example.com/auto/v990.1" {
		t.Fatalf("virtual or mosaic missing: %s", raw)
	}
	if strings.TrimSpace(fx.ask(fx.mux, http.MethodGet, "/lineup.json", "").Body.String()) == "null" {
		t.Fatal("empty check used a populated lineup")
	}

	empty := newEmulatorMux(testStore(t), nil)
	blank := httptest.NewRecorder()
	empty.ServeHTTP(blank, httptest.NewRequest(http.MethodGet, "/lineup.json", nil))
	if strings.TrimSpace(blank.Body.String()) != "[]" {
		t.Fatalf("empty lineup %s", blank.Body)
	}

	var status struct {
		ScanInProgress int
		ScanPossible   int
		Source         string
		SourceList     []string
	}
	decodeBody(t, fx.ask(fx.mux, http.MethodGet, "/lineup_status.json", ""), &status)
	if status.ScanInProgress != 0 || status.ScanPossible != 0 || status.Source != "Antenna" || len(status.SourceList) != 1 || status.SourceList[0] != "Antenna" {
		t.Fatalf("status %+v", status)
	}
	if body := fx.ask(fx.mux, http.MethodGet, "/lineup_status.json", "").Body.String(); !strings.Contains(body, `"ScanPossible":0`) {
		t.Fatalf("scan possible omitted: %s", body)
	}
	if rec := fx.ask(fx.mux, http.MethodPost, "/lineup.post?scan=start", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("scan start %d", rec.Code)
	}
	if rec := fx.ask(fx.mux, http.MethodPost, "/lineup.post?scan=abort", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("scan abort %d", rec.Code)
	}
	if rec := fx.ask(fx.mux, http.MethodPost, "/lineup.post?scan=now", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad scan %d", rec.Code)
	}
	var tuners []struct{ Resource string }
	decodeBody(t, fx.ask(fx.mux, http.MethodGet, "/status.json", ""), &tuners)
	if len(tuners) != emulatedTuners || tuners[0].Resource != "tuner0" {
		t.Fatalf("status.json %+v", tuners)
	}
	page := fx.ask(fx.mux, http.MethodGet, "/tuners.html", "")
	if !strings.Contains(page.Body.String(), "Tuner 0 Channelnone") {
		t.Fatalf("tuners.html %s", page.Body)
	}
	xml := fx.ask(fx.mux, http.MethodGet, "/lineup.xml", "").Body.String()
	if !strings.Contains(xml, "<GuideNumber>4.1</GuideNumber>") || !strings.Contains(xml, "<HD>1</HD>") {
		t.Fatalf("lineup.xml %s", xml)
	}
	m3u := fx.ask(fx.mux, http.MethodGet, "/lineup.m3u", "").Body.String()
	if !strings.Contains(m3u, "#EXTM3U\n") || !strings.Contains(m3u, "#EXTINF:-1,4.1 KBWV\nhttp://example.com/auto/v4.1\n") {
		t.Fatalf("lineup.m3u %s", m3u)
	}
	fx.quietHits()
}

func TestPlexTunerClient(t *testing.T) {
	fx := newTunerFixture(t)
	disc := fx.ask(fx.mux, http.MethodGet, "/discover.json", uaPlex)
	var device struct {
		DeviceID  string
		LineupURL string
	}
	decodeBody(t, disc, &device)
	if len(device.DeviceID) != 8 || strings.Trim(device.DeviceID, "0123456789ABCDEF") != "" {
		t.Fatalf("plex rejects a device id that is not 8 hex: %s", disc.Body)
	}
	lineup := fx.ask(fx.mux, http.MethodGet, device.LineupURL[len("http://example.com"):], uaPlex)
	if !strings.Contains(lineup.Body.String(), `"GuideNumber":"4.1"`) {
		t.Fatalf("lineup %s", lineup.Body)
	}
	// Plex asks for /auto/v and the guide number, including through /tunerN.
	for _, path := range []string{"/auto/v4.1", "/tuner0/v4.1", "/auto/v4.1?duration=5"} {
		rec := fx.ask(fx.mux, http.MethodGet, path, uaPlex)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
			t.Fatalf("%s: %d %q", path, rec.Code, rec.Header().Get("X-HDHomeRun-Error"))
		}
	}
	// A virtual channel with the same number must not win.
	if body := fx.ask(fx.mux, http.MethodGet, "/auto/v4.1", uaPlex).Body.String(); strings.Contains(body, string([]byte{0x47, 0x40})) {
		t.Fatal("the virtual channel shadowed 4.1")
	}
	miss := fx.ask(fx.mux, http.MethodGet, "/auto/v99.1", uaPlex)
	if miss.Code != http.StatusNotFound || miss.Header().Get("X-HDHomeRun-Error") != "801 Unknown Channel" {
		t.Fatalf("unknown %d %s", miss.Code, miss.Header().Get("X-HDHomeRun-Error"))
	}
	busy := fx.ask(fx.mux, http.MethodGet, "/tuner8/v4.1", uaPlex)
	if busy.Code != http.StatusServiceUnavailable || busy.Header().Get("X-HDHomeRun-Error") != "804 Tuner In Use" {
		t.Fatalf("tuner 8 %d %s", busy.Code, busy.Header().Get("X-HDHomeRun-Error"))
	}
	play := fx.ask(fx.mux, http.MethodGet, "/auto/v9001", uaPlex)
	if play.Code != http.StatusOK || !strings.Contains(play.Body.String(), string([]byte{0x47})) {
		t.Fatalf("virtual %d %q", play.Code, play.Body.String())
	}
	fx.quietHits()
}

func TestJellyfinTunerClient(t *testing.T) {
	fx := newTunerFixture(t)
	disc := fx.ask(fx.mux, http.MethodGet, "/discover.json", uaJellyfin)
	var device struct {
		ModelNumber string
		BaseURL     string
		LineupURL   string
		TunerCount  int
	}
	decodeBody(t, disc, &device)
	if strings.Contains(strings.ToLower(device.ModelNumber), "hdtc") {
		t.Fatalf("jellyfin would request a transcode: %s", device.ModelNumber)
	}
	if device.TunerCount != emulatedTuners || device.LineupURL != device.BaseURL+"/lineup.json" {
		t.Fatalf("discover %+v", device)
	}
	show := false
	if _, err := fx.st.PatchChannel(t.Context(), fx.ids["105.1"], store.ChannelPatch{Hidden: &show}); err != nil {
		t.Fatal(err)
	}
	var rows []lineupRow
	decodeBody(t, fx.ask(fx.mux, http.MethodGet, "/lineup.json", uaJellyfin), &rows)
	var protected lineupRow
	var plain int
	for _, row := range rows {
		if row.DRM == 1 {
			protected = row
			continue
		}
		plain++
	}
	if protected.GuideNumber != "105.1" || !strings.Contains(protected.Tags, "drm") || protected.URL != "http://example.com/auto/v105.1" {
		t.Fatalf("drm row %+v", protected)
	}
	if plain < 2 {
		t.Fatal("favorites-only is the client's filter; the lineup still lists the rest")
	}
	// Jellyfin plays the URL from the lineup and skips DRM itself. If it asks anyway, refuse before a tune.
	got := fx.ask(fx.mux, http.MethodGet, "/auto/v105.1", uaJellyfin)
	if got.Code != http.StatusServiceUnavailable || got.Header().Get("X-HDHomeRun-Error") != "811 Content Protection Required" {
		t.Fatalf("drm stream %d %s", got.Code, got.Header().Get("X-HDHomeRun-Error"))
	}
	profile := fx.ask(fx.mux, http.MethodGet, "/auto/v4.1?transcode=heavy", uaJellyfin)
	if profile.Code != http.StatusServiceUnavailable || profile.Header().Get("X-HDHomeRun-Error") != "802 Unknown Transcode Profile" {
		t.Fatalf("transcode %d %s", profile.Code, profile.Header().Get("X-HDHomeRun-Error"))
	}
	fx.quietHits()
}

func TestEmbyTunerClient(t *testing.T) {
	fx := newTunerFixture(t)
	disc := fx.ask(fx.mux, http.MethodGet, "/discover.json", uaEmby)
	var device struct{ ModelNumber, LineupURL string }
	decodeBody(t, disc, &device)
	if strings.Contains(strings.ToLower(device.ModelNumber), "hdtc") || !strings.HasSuffix(device.LineupURL, "/lineup.json") {
		t.Fatalf("discover %s", disc.Body)
	}
	page := fx.ask(fx.mux, http.MethodGet, "/tuners.html", uaEmby).Body.String()
	idle := 0
	for _, line := range strings.Split(page, "\n") {
		i := strings.Index(line, "Channel")
		if i < 0 {
			continue
		}
		// Emby's 2018 parser takes the characters after "Channel" and compares them to "none".
		if line[i+len("Channel"):] == "none" {
			idle++
		}
	}
	if idle != emulatedTuners {
		t.Fatalf("emby would not see idle tuners:\n%s", page)
	}
	show := false
	if _, err := fx.st.PatchChannel(t.Context(), fx.ids["105.1"], store.ChannelPatch{Hidden: &show}); err != nil {
		t.Fatal(err)
	}
	var rows []lineupRow
	decodeBody(t, fx.ask(fx.mux, http.MethodGet, "/lineup.json", uaEmby), &rows)
	for _, row := range rows {
		if row.DRM == 1 {
			continue
		}
		path := strings.TrimPrefix(row.URL, "http://example.com")
		// A mosaic export would start ffmpeg. The nil hub answers 806 without one.
		handler := fx.mux
		if row.GuideNumber == "990.1" {
			handler = fx.quiet
		}
		rec := fx.ask(handler, http.MethodGet, path, uaEmby)
		switch row.GuideNumber {
		case "9001":
			if rec.Code != http.StatusOK {
				t.Fatalf("emby virtual %d %s", rec.Code, rec.Body)
			}
		case "990.1":
			if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
				t.Fatalf("mosaic %d %s", rec.Code, rec.Header().Get("X-HDHomeRun-Error"))
			}
		default:
			if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
				t.Fatalf("%s %d %s", row.URL, rec.Code, rec.Header().Get("X-HDHomeRun-Error"))
			}
		}
	}
	fx.quietHits()
}

func TestChannelsDVRTunerClient(t *testing.T) {
	fx := newTunerFixture(t)
	disc := fx.ask(fx.mux, http.MethodGet, "/discover.json", uaChannels)
	var device struct{ LineupURL string }
	decodeBody(t, disc, &device)
	// Channels ignores LineupURL and requests /auto/v plus the guide number.
	if !strings.Contains(device.LineupURL, "/lineup.json") {
		t.Fatal(disc.Body)
	}
	for _, path := range []string{"/auto/v4.1", "/auto/c" + strconv.FormatInt(fx.ids["4.1"], 10)} {
		rec := fx.ask(fx.mux, http.MethodGet, path, uaChannels)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
			t.Fatalf("%s %d %s body %s", path, rec.Code, rec.Header().Get("X-HDHomeRun-Error"), rec.Body)
		}
	}
	mosaic := fx.ask(fx.quiet, http.MethodGet, "/auto/v990.1", uaChannels)
	if mosaic.Code != http.StatusServiceUnavailable || mosaic.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
		t.Fatalf("mosaic number %d %s", mosaic.Code, mosaic.Header().Get("X-HDHomeRun-Error"))
	}
	alias := fx.ask(fx.quiet, http.MethodGet, "/auto/m"+fx.mosaic, uaChannels)
	if alias.Code != http.StatusServiceUnavailable || alias.Header().Get("X-HDHomeRun-Error") != "806 Tune Failed" {
		t.Fatalf("mosaic alias %d %s", alias.Code, alias.Header().Get("X-HDHomeRun-Error"))
	}
	unknown := fx.ask(fx.quiet, http.MethodGet, "/auto/m1", uaChannels)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown mosaic %d", unknown.Code)
	}
	fx.quietHits()
}

func TestChannelsGuideMatchesPlaylist(t *testing.T) {
	fx := newTunerFixture(t)
	m3u := fx.ask(fx.api, http.MethodGet, "/export/lineup.m3u", uaChannels).Body.String()
	guide := fx.ask(fx.api, http.MethodGet, "/export/guide.xml", uaChannels).Body.String()
	if !strings.Contains(m3u, `url-tvg="http://example.com/export/guide.xml"`) {
		t.Fatalf("header %s", m3u)
	}
	want := `tvg-id="ota.4-1" tvg-chno="4.1" tvg-name="KBWV" channel-id="ota.4-1" channel-number="4.1" tvc-stream-vcodec="mpeg2" tvc-stream-acodec="ac3",KBWV`
	if !strings.Contains(m3u, want) {
		t.Fatalf("playlist\n%s", m3u)
	}
	if strings.Contains(m3u, `channel-id="4.1"`) {
		t.Fatal("channel-id is still the display number")
	}
	if !strings.Contains(m3u, `tvg-id="ota.5-2" tvg-chno="5.2" tvg-name="Rivers" channel-id="ota.5-2" channel-number="5.2" tvc-stream-vcodec="h264" tvc-stream-acodec="ac3",Rivers`) {
		t.Fatalf("5.2 %s", m3u)
	}
	if strings.Contains(m3u, "7.1") || strings.Contains(m3u, "8.1") || strings.Contains(m3u, "105.1") || strings.Contains(m3u, "ac4") {
		t.Fatalf("hidden channels or an unknown codec leaked\n%s", m3u)
	}
	if !strings.Contains(m3u, `channel-id="mosaic.`+fx.mosaic+`" channel-number="990.1"`) {
		t.Fatalf("mosaic playlist\n%s", m3u)
	}
	if !strings.Contains(guide, `<channel id="ota.4-1">`) || !strings.Contains(guide, `<channel id="mosaic.`+fx.mosaic+`">`) {
		t.Fatalf("guide channels\n%s", guide)
	}
	if !strings.Contains(guide, `<series-id system="broadwave">SH-NIGHT</series-id>`) {
		t.Fatalf("series id\n%s", guide)
	}
	if !strings.Contains(guide, `<episode-num system="onscreen">S02E10</episode-num>`) || strings.Contains(guide, "S2E10") {
		t.Fatalf("episode\n%s", guide)
	}
	if !strings.Contains(guide, `<episode-num system="dd_progid">EP0001</episode-num>`) {
		t.Fatalf("program id\n%s", guide)
	}
	if !strings.Contains(guide, "<category>News</category>") || !strings.Contains(guide, "<category>Movie</category>") {
		t.Fatalf("categories\n%s", guide)
	}
	if !strings.Contains(guide, `<episode-num system="onscreen">Part 2</episode-num>`) {
		t.Fatalf("label only\n%s", guide)
	}
	// The id Channels stores from the playlist is the id the guide uses.
	if !strings.Contains(m3u, `channel-id="ota.4-1"`) || !strings.Contains(guide, `channel="ota.4-1"`) {
		t.Fatal("playlist channel-id and guide programme channel differ")
	}
}
