package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
)

// When the SiliconDust pull fails, a guide address still fills the guide, and
// SiliconDust is tried again soon instead of a day later.
func TestGuideAddressFillsTheGuideWhenSiliconDustFails(t *testing.T) {
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
	start := time.Now().UTC().Truncate(time.Hour)
	stamp := func(t time.Time) string { return t.Format("20060102150405 -0700") }
	xmltv := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
<channel id="fox"><display-name>4.1</display-name></channel>
<programme start="%s" stop="%s" channel="fox"><title>Evening News</title></programme>
</tv>`, stamp(start), stamp(start.Add(time.Hour)))
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xmltv))
	}))
	t.Cleanup(feed.Close)
	if err := st.PutSettings(ctx, map[string]string{"guideUrl": feed.URL + "/guide.xml"}); err != nil {
		t.Fatal(err)
	}

	api := &Server{Store: st, HDHR: client}
	n, err := api.RefreshGuide(ctx)
	if err != nil || n != 1 {
		t.Fatalf("refresh %d %v", n, err)
	}
	airings, err := st.Airings(ctx, start.Add(-time.Minute), start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(airings) != 1 || airings[0].Title != "Evening News" || airings[0].GuideSource != "xmltv" {
		t.Fatalf("airings %+v", airings)
	}
	_, next, _, err := st.GuideSchedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(next); wait <= 0 || wait > time.Hour {
		t.Fatalf("SiliconDust next tried in %v", wait)
	}
}
