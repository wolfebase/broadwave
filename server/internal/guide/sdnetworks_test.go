package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestSDNetworksReadAffiliates(t *testing.T) {
	body := []byte(`{
		"map": [
			{"stationID": "1001", "channel": "4.1"},
			{"stationID": "1002", "atscMajor": 9, "atscMinor": 1},
			{"stationID": "1003", "channel": "22.1"},
			{"stationID": "1004", "channel": "22.2"},
			{"stationID": "1005", "channel": "99.1"}
		],
		"stations": [
			{"stationID": "1001", "callsign": "KBWV", "affiliate": "NBC Affiliate"},
			{"stationID": "1002", "callsign": "WBWV", "affiliate": "TELEMUNDO"},
			{"stationID": "1003", "callsign": "CBQT", "name": "CBQT-DT"},
			{"stationID": "1004", "callsign": "KBWVLD2", "affiliate": "Antenna TV"},
			{"stationID": "1005", "callsign": "KQQQ", "affiliate": "CBS"}
		]
	}`)
	lineup := []store.Channel{{ID: 1, GuideNumber: "4.1"}, {ID: 2, GuideNumber: "9.1"}, {ID: 3, GuideNumber: "22.1"}, {ID: 4, GuideNumber: "22.2"}}
	got := SDNetworks(body, lineup)
	want := map[int64]string{1: "NBC", 2: "Telemundo", 3: "CBC"}
	if len(got) != len(want) {
		t.Fatalf("networks %v", got)
	}
	for id, net := range want {
		if got[id] != net {
			t.Fatalf("%d: %q, want %q (%v)", id, got[id], net, got)
		}
	}
	if SDNetworks([]byte("not json"), lineup) != nil {
		t.Fatal("bad body")
	}
}
