package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/guide"
	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// fakeSD answers like Schedules Direct for a lineup with 4.1 and 5.1: two
// weeks of hour-long airings with descriptions, counting schedule pulls.
func fakeSD(t *testing.T, pulls *atomic.Int32) *httptest.Server {
	t.Helper()
	start := time.Now().UTC().Truncate(24 * time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"code":0,"token":"tok"}`))
		case "/headends":
			_, _ = w.Write([]byte(`[{"headend":"80000","transport":"Antenna","location":"80000","lineups":[{"name":"Antenna","lineup":"USA-OTA80000"}]},
				{"headend":"CO1","transport":"Cable","location":"Plainsview","lineups":[{"name":"Valley Cable","lineup":"USA-CO1-X"}]}]`))
		case "/status":
			_, _ = w.Write([]byte(`{"account":{"maxLineups":4},"lineups":[]}`))
		case "/lineups/USA-OTA80000":
			if r.Method == http.MethodPut {
				_, _ = w.Write([]byte(`{"code":0}`))
				return
			}
			_, _ = w.Write([]byte(`{"map":[{"stationID":"90001","atscMajor":4,"atscMinor":1},{"stationID":"90003","atscMajor":5,"atscMinor":1}],
				"stations":[{"stationID":"90001","callsign":"KBWV","affiliate":"NBC Affiliate","logo":{"URL":"https://logos.example/kbwv.png"}},{"stationID":"90003","callsign":"WTST"}]}`))
		case "/schedules":
			pulls.Add(1)
			var req []struct {
				StationID string `json:"stationID"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			var blocks []string
			for _, st := range req {
				var progs []string
				for h := range 14 * 24 {
					at := start.Add(time.Duration(h) * time.Hour).Format(time.RFC3339)
					progs = append(progs, fmt.Sprintf(`{"programID":"EP%08d%04d","airDateTime":%q,"duration":3600,"md5":"m"}`, h%7, h, at))
				}
				blocks = append(blocks, fmt.Sprintf(`{"stationID":%q,"programs":[%s]}`, st.StationID, strings.Join(progs, ",")))
			}
			_, _ = w.Write([]byte("[" + strings.Join(blocks, ",") + "]"))
		case "/programs":
			var ids []string
			_ = json.NewDecoder(r.Body).Decode(&ids)
			var out []string
			for _, id := range ids {
				out = append(out, fmt.Sprintf(`{"programID":%q,"md5":"m","titles":[{"title120":"Show %s"}],"descriptions":{"description1000":[{"descriptionLanguage":"en","description":"About it."}]},"genres":["News"]}`, id, id[2:10]))
			}
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeTunerStore(t *testing.T) (*store.Store, *hdhr.Client) {
	t.Helper()
	ctx := context.Background()
	tuner := &fake.Server{}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	st := testStore(t)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	return st, client
}

// Without SiliconDust guide access, Schedules Direct fills two weeks; each
// source keeps its own schedule, and SiliconDust backs off.
func TestGuideSourcesKeepTheirOwnSchedules(t *testing.T) {
	ctx := context.Background()
	st, client := fakeTunerStore(t)
	var pulls atomic.Int32
	sd := fakeSD(t, &pulls)
	if err := st.PutSettings(ctx, map[string]string{"sdUser": "viewer", "sdPassword": "secret", "sdLineup": "USA-OTA80000"}); err != nil {
		t.Fatal(err)
	}
	api := &Server{Store: st, HDHR: client, SD: func(user, pass string) *guide.SD {
		return guide.NewSDWith(sd.Client(), sd.URL, user, pass)
	}}

	n, err := api.RefreshGuide(ctx)
	if err != nil || n == 0 {
		t.Fatalf("refresh %d %v", n, err)
	}
	if pulls.Load() != 1 {
		t.Fatalf("%d schedule pulls", pulls.Load())
	}
	far, err := st.Airings(ctx, time.Now().Add(12*24*time.Hour), time.Now().Add(12*24*time.Hour+2*time.Hour))
	if err != nil || len(far) == 0 || far[0].Description != "About it." || far[0].GuideSource != sourceSD {
		t.Fatalf("day 12: %+v %v", far, err)
	}
	states, _ := st.GuideSources(ctx)
	if sd := states[sourceSD]; sd.Fails != 0 || time.Until(sd.Next) < 23*time.Hour || sd.Rows == 0 {
		t.Fatalf("schedules direct %+v", sd)
	}
	silicon := states[sourceSilicon]
	if silicon.Fails != 1 || silicon.Error == "" {
		t.Fatalf("silicondust %+v", silicon)
	}
	if wait := api.GuideDelay(time.Now()); wait <= 0 || wait > 31*time.Minute {
		t.Fatalf("next look in %v", wait)
	}

	// Nothing is due: nothing is pulled.
	if _, err := api.RefreshGuide(ctx); err != nil {
		t.Fatal(err)
	}
	if pulls.Load() != 1 {
		t.Fatalf("%d schedule pulls", pulls.Load())
	}

	// SiliconDust's turn fails again and waits twice as long; Schedules
	// Direct is not pulled, and its listings stay.
	if err := st.SetNextGuidePull(ctx, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := api.RefreshGuide(ctx); err != nil {
		t.Fatal(err)
	}
	states, _ = st.GuideSources(ctx)
	if pulls.Load() != 1 || states[sourceSilicon].Fails != 2 {
		t.Fatalf("%d pulls, silicondust %+v", pulls.Load(), states[sourceSilicon])
	}
	_, next, _, _ := st.GuideSchedule(ctx)
	if wait := time.Until(next); wait < 59*time.Minute || wait > 61*time.Minute {
		t.Fatalf("SiliconDust next in %v", wait)
	}
	far, _ = st.Airings(ctx, time.Now().Add(12*24*time.Hour), time.Now().Add(12*24*time.Hour+2*time.Hour))
	if len(far) == 0 {
		t.Fatal("Schedules Direct listings went when SiliconDust failed")
	}

	// A new lineup is a settings change: pulled at once.
	if err := st.PutSettings(ctx, map[string]string{"sdUser": "viewer2"}); err != nil {
		t.Fatal(err)
	}
	if api.GuideDelay(time.Now()) != 0 {
		t.Fatal("a changed account was not due")
	}
	if _, err := api.RefreshGuide(ctx); err != nil {
		t.Fatal(err)
	}
	if pulls.Load() != 2 {
		t.Fatalf("%d schedule pulls", pulls.Load())
	}

	// Turning Schedules Direct off takes its listings away.
	if err := st.PutSettings(ctx, map[string]string{"sdUser": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.RefreshGuide(ctx); err != nil {
		t.Fatal(err)
	}
	far, _ = st.Airings(ctx, time.Now().Add(12*24*time.Hour), time.Now().Add(12*24*time.Hour+2*time.Hour))
	if len(far) != 0 {
		t.Fatalf("listings of a source turned off: %+v", far)
	}
}

// The hour between Refresh presses is SiliconDust's rule; a Schedules Direct
// account that was just changed is pulled anyway.
func TestManualRefreshLimitIsSiliconDustOnly(t *testing.T) {
	ctx := context.Background()
	st, client := fakeTunerStore(t)
	var pulls atomic.Int32
	sd := fakeSD(t, &pulls)
	api := &Server{Store: st, HDHR: client, SD: func(user, pass string) *guide.SD {
		return guide.NewSDWith(sd.Client(), sd.URL, user, pass)
	}}
	h := api.Handler()
	if rec := postJSON(t, h, "/api/v1/guide/refresh", "{}"); rec.Code == http.StatusTooManyRequests {
		t.Fatalf("first press: %d %s", rec.Code, rec.Body)
	}
	if rec := postJSON(t, h, "/api/v1/guide/refresh", "{}"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second press: %d %s", rec.Code, rec.Body)
	}
	if err := st.PutSettings(ctx, map[string]string{"sdUser": "viewer", "sdPassword": "secret", "sdLineup": "USA-OTA80000"}); err != nil {
		t.Fatal(err)
	}
	rec := postJSON(t, h, "/api/v1/guide/refresh", "{}")
	if rec.Code != http.StatusOK || pulls.Load() != 1 {
		t.Fatalf("after a settings change: %d %s, %d pulls", rec.Code, rec.Body, pulls.Load())
	}
	if rec := postJSON(t, h, "/api/v1/guide/refresh", "{}"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("again: %d %s", rec.Code, rec.Body)
	}
}

// Setting up Schedules Direct from a postal code: list lineups, pick one,
// and the guide fills right away.
func TestSchedulesDirectFromAPostalCode(t *testing.T) {
	st, client := fakeTunerStore(t)
	var pulls atomic.Int32
	sd := fakeSD(t, &pulls)
	api := &Server{Store: st, HDHR: client, SD: func(user, pass string) *guide.SD {
		return guide.NewSDWith(sd.Client(), sd.URL, user, pass)
	}}
	h := api.Handler()
	if rec := postJSON(t, h, "/api/v1/guide/sd/lineups", `{"username":"viewer","password":"secret","postalCode":"8000"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "ZIP") {
		t.Fatalf("bad postal code: %d %s", rec.Code, rec.Body)
	}
	if rec := postJSON(t, h, "/api/v1/guide/sd/lineups", `{"postalCode":"80000"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("no account: %d %s", rec.Code, rec.Body)
	}
	rec := postJSON(t, h, "/api/v1/guide/sd/lineups", `{"username":"viewer","password":"secret","postalCode":"80000"}`)
	var found struct {
		Country string           `json:"country"`
		Lineups []guide.SDLineup `json:"lineups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &found); err != nil || rec.Code != http.StatusOK || found.Country != "USA" || len(found.Lineups) != 2 || found.Lineups[0].ID != "USA-OTA80000" {
		t.Fatalf("lineups: %d %s", rec.Code, rec.Body)
	}
	rec = postJSON(t, h, "/api/v1/guide/sd", `{"username":"viewer","password":"secret","lineup":"USA-OTA80000"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"channels":2`) {
		t.Fatalf("use lineup: %d %s", rec.Code, rec.Body)
	}
	saved, _ := st.Settings(context.Background())
	if saved["sdUser"] != "viewer" || saved["sdLineup"] != "USA-OTA80000" || saved["sdPassword"] != "secret" {
		t.Fatalf("settings %v", saved)
	}
}

// An XMLTV link that lists nothing is not kept, and the answer says why.
func TestXMLTVLinkWithNoProgramsIsNotKept(t *testing.T) {
	st, client := fakeTunerStore(t)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<tv><channel id="a"><display-name>4.1</display-name></channel></tv>`))
	}))
	t.Cleanup(feed.Close)
	h := (&Server{Store: st, HDHR: client}).Handler()
	rec := postJSON(t, h, "/api/v1/guide/xmltv", `{"url":"`+feed.URL+`/guide.xml"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "gave no programs") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	saved, _ := st.Settings(context.Background())
	if saved["guideUrl"] != "" {
		t.Fatalf("kept %q", saved["guideUrl"])
	}
	if rec := postJSON(t, h, "/api/v1/guide/xmltv", `{"url":"ftp://x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("ftp: %d", rec.Code)
	}
}

// Setup never stops on the guide: with no listings it says why and flags
// the Ready step to offer a guide.
func TestSetupWithNoListingsOffersAGuide(t *testing.T) {
	st, client := fakeTunerStore(t)
	api := &Server{Store: st, HDHR: client}
	run := newFinishStatus()
	api.finishRun = &run
	api.stepGuide(context.Background())
	var step finishStep
	for _, s := range api.finishRun.Steps {
		if s.ID == "guide" {
			step = s
		}
	}
	if step.State != "check" || !api.finishRun.NeedsGuide || !strings.Contains(step.Detail, "Settings › Guide") {
		t.Fatalf("%+v needsGuide %v", step, api.finishRun.NeedsGuide)
	}
}
