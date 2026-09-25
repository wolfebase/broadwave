package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestAffiliationGuideNameWins(t *testing.T) {
	if got := Affiliation([]string{"ABC"}, "WDAF"); got != "ABC" {
		t.Fatalf("guide name ABC lost to WDAF, got %q", got)
	}
	if got := Affiliation([]string{"FOX 4"}, "KXYZ"); got != "FOX" {
		t.Fatalf("FOX 4: %q", got)
	}
	if got := Affiliation([]string{"WDAF (FOX)"}, "KXYZ"); got != "FOX" {
		t.Fatalf("parenthetical: %q", got)
	}
	if got := Affiliation([]string{"NBC"}); got != "NBC" {
		t.Fatalf("exact NBC: %q", got)
	}
}

func TestAffiliationCallSign(t *testing.T) {
	if calls["WDAF"] != "FOX" || calls["KCTV"] != "CBS" || calls["KMBC"] != "ABC" || calls["KSHB"] != "NBC" {
		t.Fatal("Kansas City affiliates missing from the table")
	}
	if got := Affiliation(nil, "WDAF-DT"); got != "FOX" {
		t.Fatalf("WDAF-DT: %q", got)
	}
	if got := Affiliation(nil, "KCTVDT"); got != "CBS" {
		t.Fatalf("KCTVDT: %q", got)
	}
	if got := Affiliation(nil, "KMBC-HD"); got != "ABC" {
		t.Fatalf("KMBC-HD: %q", got)
	}
	if got := Affiliation([]string{"4.1", "WDAFDT"}, "HD"); got != "FOX" {
		t.Fatalf("guide call sign: %q", got)
	}
}

func TestAffiliationDoesNotGuess(t *testing.T) {
	for _, name := range []string{"FOX NEWS", "SHOPNBC", "WDAF2", "WFOX", "NOTNBC", "Jewelry"} {
		if got := Affiliation([]string{name}); got != "" {
			t.Fatalf("%s guessed %q", name, got)
		}
	}
}

func TestNetworksFromGuide(t *testing.T) {
	raw := []byte(`<tv><channel id="4.1"><display-name>4.1</display-name><display-name>WDAFDT</display-name></channel></tv>`)
	got := Networks(raw, []store.Channel{{ID: 9, GuideNumber: "4.1", GuideName: "HD"}})
	if got[9] != "FOX" {
		t.Fatalf("networks: %v", got)
	}
	raw = []byte(`<tv><channel id="9.1"><display-name>9.1</display-name><display-name>ABC</display-name></channel></tv>`)
	got = Networks(raw, []store.Channel{{ID: 3, GuideNumber: "9.1", GuideName: "WDAF"}})
	if got[3] != "ABC" {
		t.Fatalf("guide ABC should beat the call sign, got %v", got)
	}
}
