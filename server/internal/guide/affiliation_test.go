package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestAffiliationGuideNameWins(t *testing.T) {
	if got := Affiliation([]string{"ABC"}, "KDVR"); got != "ABC" {
		t.Fatalf("guide name ABC lost to KDVR, got %q", got)
	}
	if got := Affiliation([]string{"FOX 4"}, "KXYZ"); got != "FOX" {
		t.Fatalf("FOX 4: %q", got)
	}
	if got := Affiliation([]string{"KDVR (FOX)"}, "KXYZ"); got != "FOX" {
		t.Fatalf("parenthetical: %q", got)
	}
	if got := Affiliation([]string{"NBC"}); got != "NBC" {
		t.Fatalf("exact NBC: %q", got)
	}
}

func TestAffiliationCallSign(t *testing.T) {
	if calls["KDVR"] != "FOX" || calls["KCNC"] != "CBS" || calls["KMGH"] != "ABC" || calls["KUSA"] != "NBC" {
		t.Fatal("Denver affiliates missing from the table")
	}
	if got := Affiliation(nil, "KDVR-DT"); got != "FOX" {
		t.Fatalf("KDVR-DT: %q", got)
	}
	if got := Affiliation(nil, "KCNCDT"); got != "CBS" {
		t.Fatalf("KCNCDT: %q", got)
	}
	if got := Affiliation(nil, "KMGH-HD"); got != "ABC" {
		t.Fatalf("KMGH-HD: %q", got)
	}
	if got := Affiliation([]string{"4.1", "KDVRDT"}, "HD"); got != "FOX" {
		t.Fatalf("guide call sign: %q", got)
	}
}

func TestAffiliationNumberedDTSubchannels(t *testing.T) {
	if got := Affiliation(nil, "KDVR-DT"); got != "FOX" {
		t.Fatalf("KDVR-DT: %q", got)
	}
	if got := Affiliation(nil, "KDVR-DT1"); got != "FOX" {
		t.Fatalf("KDVR-DT1: %q", got)
	}
	for _, name := range []string{"KDVR-DT2", "KDVR-DT3", "KDVR-DT4", "KCNC-DT2"} {
		if got := Affiliation(nil, name); got != "" {
			t.Fatalf("%s matched %q", name, got)
		}
	}
	if got := Affiliation([]string{"KDVRDT2"}); got != "" {
		t.Fatalf("guide name KDVRDT2 matched %q", got)
	}
}

func TestAffiliationDoesNotGuess(t *testing.T) {
	for _, name := range []string{"FOX NEWS", "SHOPNBC", "KDVR2", "WFOX", "NOTNBC", "Jewelry"} {
		if got := Affiliation([]string{name}); got != "" {
			t.Fatalf("%s guessed %q", name, got)
		}
	}
}

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
