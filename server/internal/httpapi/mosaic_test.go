package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestMosaicRefusesWhatItCannotBuild(t *testing.T) {
	h := (&Server{Store: testStore(t), Hub: &live.Hub{Dir: t.TempDir()}}).Handler()
	for _, body := range []string{`{"channelIds":[1]}`, `{"channelIds":[1,1]}`, `{"channelIds":[1,2,3,4,5]}`, `{"channelIds":[0,2]}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mosaic", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Pick 2 to 4 different channels.") {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	// The media and export routes start nothing, and a key is only digits and dashes.
	for _, path := range []string{
		"/media/mosaic/1-2/index.m3u8",
		"/media/mosaic/1-2/init.mp4",
		"/media/mosaic/1/index.m3u8",
		"/media/mosaic/1-2/..%2f..%2fbroadwave.db",
		"/media/mosaic/..%2f1-2/init.mp4",
		"/export/mosaic/1",
		"/export/mosaic/a-b",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mosaic/1-2/stop", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("stop of a mosaic that is not running: %d", rec.Code)
	}
}

func TestSharedMosaicsAreChannelsInTheExports(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV", StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "5.1", GuideName: "WTST", StreamURL: "http://flex:5004/auto/v5.1"},
	}); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	chs, _ := st.Channels(ctx, false)
	for _, ch := range chs {
		ids[ch.GuideNumber] = ch.ID
	}
	a, b := strconv.FormatInt(ids["4.1"], 10), strconv.FormatInt(ids["5.1"], 10)
	h := (&Server{Store: st, Hub: &live.Hub{Dir: t.TempDir()}}).Handler()
	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, bad := range []string{`{"exportMosaics":"1"}`, `{"exportMosaics":"1-1"}`, `{"exportMosaics":"1-2,x"}`, `{"exportMosaics":"01-2"}`} {
		if rec := put(bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	// A key twice is listed once; a mosaic with a channel that left is skipped.
	if rec := put(`{"exportMosaics":" ` + a + `-` + b + `, 9998-9999,` + a + `-` + b + `"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"exportMosaics":"`+a+`-`+b+`,9998-9999"`) {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	m3u := httptest.NewRecorder()
	h.ServeHTTP(m3u, httptest.NewRequest(http.MethodGet, "/export/lineup.m3u", nil))
	want := `tvg-chno="990.1" tvg-name="Multiview: KBWV + WTST" channel-id="990.1" group-title="Multiview",Multiview: KBWV + WTST` + "\nhttp://example.com/export/mosaic/" + a + "-" + b + "\n"
	if !strings.Contains(m3u.Body.String(), want) || strings.Contains(m3u.Body.String(), "9998") {
		t.Fatalf("m3u:\n%s", m3u.Body)
	}
	guide := httptest.NewRecorder()
	h.ServeHTTP(guide, httptest.NewRequest(http.MethodGet, "/export/guide.xml", nil))
	if body := guide.Body.String(); !strings.Contains(body, `<channel id="mosaic.`+a+`-`+b+`">`) || !strings.Contains(body, "4.1 KBWV, 5.1 WTST, side by side. The sound is 4.1 KBWV.") {
		t.Fatalf("guide:\n%.2000s", body)
	}
	emu := &emuHandler{store: st}
	lineup := httptest.NewRecorder()
	emu.lineup(lineup, httptest.NewRequest(http.MethodGet, "/lineup.json", nil))
	if !strings.Contains(lineup.Body.String(), `"GuideName":"Multiview: KBWV + WTST","GuideNumber":"990.1","HD":1,"URL":"http://example.com/auto/m`+a+`-`+b+`"`) {
		t.Fatalf("lineup: %s", lineup.Body)
	}
	for _, path := range []string{"/auto/m1", "/auto/m" + b + "-" + a} {
		rec := httptest.NewRecorder()
		emu.stream(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	// Only listed mosaics play through the exports: another mix is not started.
	other := httptest.NewRecorder()
	h.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/export/mosaic/"+b+"-"+a, nil))
	if other.Code != http.StatusNotFound {
		t.Fatalf("an unlisted mosaic export: %d", other.Code)
	}

	// Removing the first leaves its slot empty, so the next keeps its number.
	if rec := put(`{"exportMosaics":",` + b + `-` + a + `,,"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"exportMosaics":",`+b+`-`+a+`"`) {
		t.Fatalf("save with a gap: %d %s", rec.Code, rec.Body)
	}
	name := `W"TST`
	if _, err := st.PatchChannel(ctx, ids["5.1"], store.ChannelPatch{CustomName: &name}); err != nil {
		t.Fatal(err)
	}
	m3u = httptest.NewRecorder()
	h.ServeHTTP(m3u, httptest.NewRequest(http.MethodGet, "/export/lineup.m3u", nil))
	if body := m3u.Body.String(); !strings.Contains(body, `tvg-chno="990.2" tvg-name="Multiview: W'TST + KBWV"`) || strings.Contains(body, "990.1") || strings.Contains(body, `\"`) {
		t.Fatalf("m3u after a removal:\n%s", body)
	}
}
