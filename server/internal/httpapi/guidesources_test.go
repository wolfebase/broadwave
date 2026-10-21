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
		case "/lineups/USA-OTA80000":
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
