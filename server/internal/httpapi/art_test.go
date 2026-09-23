package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"waveguide/internal/hdhr"
	"waveguide/internal/live"
	"waveguide/internal/store"
)

func TestArtResizesAndFallsBack(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF"},
	}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatal(err, channels)
	}
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 200, G: 20, B: 20, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	defer imgSrv.Close()
	if err := st.SetChannelArt(ctx, map[int64]string{channels[0].ID: imgSrv.URL + "/logo.png"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceAirings(ctx, []store.Airing{{
		ChannelID: channels[0].ID, Title: "News", Category: "News", Start: time.Now(), End: time.Now().Add(time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Dir: t.TempDir()}}).Handler()
	res := get(t, h, "/media/art/channel/"+strconv.FormatInt(channels[0].ID, 10)+"?w=40")
	if res.Code != 200 || res.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("%d %s", res.Code, res.Header().Get("Content-Type"))
	}
	got, _, err := image.Decode(res.Body)
	if err != nil || got.Bounds().Dx() != 40 {
		t.Fatal(err, got.Bounds())
	}
	airings, err := st.Airings(ctx, time.Now().Add(-time.Minute), time.Now().Add(2*time.Hour))
	if err != nil || len(airings) != 1 {
		t.Fatal(err, airings)
	}
	blank := get(t, h, "/media/art/airing/"+strconv.FormatInt(airings[0].ID, 10))
	if blank.Code != 200 || blank.Header().Get("Content-Type") != "image/svg+xml" || !bytes.Contains(blank.Body.Bytes(), []byte("News")) {
		t.Fatalf("%d %s %s", blank.Code, blank.Header().Get("Content-Type"), blank.Body.String())
	}
}
