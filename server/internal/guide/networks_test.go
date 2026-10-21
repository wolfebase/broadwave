package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestNetworksFromGuide(t *testing.T) {
	raw := []byte(`<tv><channel id="4.1"><display-name>4.1</display-name><display-name>KDVRDT</display-name></channel></tv>`)
	got := Networks(raw, []store.Channel{{ID: 9, GuideNumber: "4.1", GuideName: "HD"}})
	if got[9] != "FOX" {
		t.Fatalf("networks: %v", got)
	}
	raw = []byte(`<tv><channel id="9.1"><display-name>9.1</display-name><display-name>ABC</display-name></channel></tv>`)
	got = Networks(raw, []store.Channel{{ID: 3, GuideNumber: "9.1", GuideName: "KDVR"}})
	if got[3] != "ABC" {
		t.Fatalf("guide ABC should beat the call sign, got %v", got)
	}
}
