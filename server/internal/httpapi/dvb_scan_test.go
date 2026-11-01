package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"broadwave/internal/dvbsi"
	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// The guide scan tunes a DVB-T tuner and its listings reach the guide: the
// tuned multiplex's two services by program number, and the other
// multiplex's service by the name the SDT gives it.
func TestTheGuideScanReadsDVBListings(t *testing.T) {
	ctx := context.Background()
	tuner := &fake.Server{Profile: fake.ProfileDVBT}
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
	was := scanDwell
	scanDwell = 3 * time.Second
	t.Cleanup(func() { scanDwell = was })
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	srv := &Server{Store: st, HDHR: client, Hub: hub, Version: "dev", Staging: true}
	hub.OnDVB = func(freq int, channelID int64, g dvbsi.Guide) {
		if _, err := srv.ApplyDVB(context.Background(), freq, channelID, g); err != nil {
			t.Error(err)
		}
	}

	srv.scanMissing(ctx)

	res := call(t, srv.Handler(), http.MethodGet, "/api/v1/airings?hours=3", "", "")
	if res.Code != http.StatusOK {
		t.Fatalf("airings %d %s", res.Code, res.Body)
	}
	var doc struct {
		Airings []struct {
			ChannelID   int64     `json:"channelId"`
			Title       string    `json:"title"`
			Category    string    `json:"category"`
			GuideSource string    `json:"guideSource"`
			Start       time.Time `json:"start"`
		} `json:"airings"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &doc); err != nil {
		t.Fatal(err, res.Body.String())
	}
	rows := doc.Airings
	all, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	number := map[int64]string{}
	for _, ch := range all {
		number[ch.ID] = ch.GuideNumber
	}
	titles := map[string][]string{}
	for _, row := range rows {
		if row.GuideSource != "broadcast" {
			t.Fatalf("%+v", row)
		}
		titles[number[row.ChannelID]] = append(titles[number[row.ChannelID]], row.Title)
	}
	known := map[string]map[string]bool{
		"1":  {"Quiz Night": true, "Harbour Lights": true, "Café Stories": true},
		"2":  {"Little Explorers": true, "Studio Kitchen": true, "Night Match": true},
		"21": {"Evening Bulletin": true, "Weather Watch ☀": true},
	}
	for guide, want := range known {
		if len(titles[guide]) == 0 {
			t.Fatalf("channel %s has no listings: %v (%s)", guide, titles, res.Body)
		}
		for _, title := range titles[guide] {
			if !want[title] {
				t.Fatalf("channel %s lists %q: %v", guide, title, titles)
			}
		}
	}
	// The scan tuned the first multiplex once: the second one's service
	// already had its now and next from the "other" tables.
	tunes := map[string]int{}
	for _, req := range tuner.Requests() {
		if strings.HasPrefix(req, "/tuner") && strings.Contains(req, "/ch") {
			tunes[req[strings.LastIndex(req, "/ch")+3:]]++
		}
	}
	if len(titles["1"]) < 4 || tunes["474000000"] != 1 || len(tunes) != 1 {
		t.Fatalf("channel 1 %v; tunes %v: %v", titles["1"], tunes, tuner.Requests())
	}
}
