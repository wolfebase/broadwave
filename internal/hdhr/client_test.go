package hdhr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchDropsDeviceAuth(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			_, _ = w.Write([]byte(`{
				"FriendlyName":"HDHomeRun CONNECT DUO",
				"ModelNumber":"HDHR5-2US",
				"FirmwareName":"hdhomerun5_atsc",
				"FirmwareVersion":"20250815",
				"UpgradeAvailable":"20260326",
				"DeviceID":"10611b4c",
				"DeviceAuth":"should-not-survive",
				"BaseURL":"` + srv.URL + `",
				"LineupURL":"` + srv.URL + `/lineup.json",
				"TunerCount":2
			}`))
		case "/lineup.json":
			_, _ = w.Write([]byte(`[
				{"GuideNumber":"14.10","GuideName":"ZLiving","URL":"http://tuner:5004/auto/v14.10","VideoCodec":"H264","AudioCodec":"AC3"},
				{"GuideNumber":"4.1","GuideName":"WDAF-DT","URL":"http://tuner:5004/auto/v4.1","VideoCodec":"MPEG2","AudioCodec":"AC3","HD":1,"Favorite":1}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	dev, err := c.FetchDevice(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if dev.DeviceID != "10611B4C" || dev.TunerCount != 2 || dev.ModelNumber != "HDHR5-2US" {
		t.Fatalf("%+v", dev)
	}
	encoded, err := json.Marshal(dev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "should-not-survive") {
		t.Fatalf("device auth leaked: %s", encoded)
	}
	channels, err := c.FetchLineup(context.Background(), dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 2 || !channels[1].Favorite || !channels[1].HD || channels[0].Favorite {
		t.Fatalf("%+v", channels)
	}
	if !strings.Contains(channels[1].StreamURL, ":5004/") {
		t.Fatalf("stream url not kept server-side: %+v", channels[1])
	}
}
