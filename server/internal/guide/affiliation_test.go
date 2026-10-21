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

func TestAffiliationReadsPSIPShortNames(t *testing.T) {
	cases := map[string]string{
		"FOX 5": "FOX", "NBC4": "NBC", "ABC7": "ABC", "CBS 12": "CBS", "abc 7 hd": "ABC",
		"PBS": "PBS", "PBS 8": "PBS", "CW": "CW", "CW50": "CW", "THE CW": "CW",
		"MY20": "MyNetworkTV", "MY 9": "MyNetworkTV", "MNTV": "MyNetworkTV",
		"ION": "ION", "ION TV": "ION", "UNIVSN": "Univision", "TELEMUN": "Telemundo",
		"UNIMAS": "UniMás", "TELEMUNDO 40": "Telemundo",
		"CBC": "CBC", "CTV": "CTV", "CTV2": "CTV 2", "CTV 2": "CTV 2", "GLOBAL": "Global",
		"CITYTV": "Citytv", "TVO": "TVO", "TÉLÉ-QUÉBEC": "Télé-Québec", "Télé-Québec HD": "Télé-Québec",
		"ICI TELE": "Radio-Canada", "TVA": "TVA", "NOOVO": "Noovo",
		"KBWV-NBC": "NBC", "WBWV CW": "CW", "KBWV-DT (PBS)": "PBS", "CBWV CTV": "CTV",
	}
	for name, want := range cases {
		if got := Affiliation([]string{name}, "KQQQ"); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

func TestAffiliationLeavesDiginetsAndLookalikesAlone(t *testing.T) {
	for _, name := range []string{
		"PBS KIDS", "PBSKIDS", "CBS NEWS", "ION PLUS", "IONMYST", "CBC NN", "CTV NEWS",
		"GLOBALNEWS", "KCITY", "CITY NEWS", "MYSTERY", "MYTVLIVE", "NBCLX", "TVA SPORTS", "NOTPBS",
	} {
		if got := Affiliation([]string{name}); got != "" {
			t.Errorf("%s guessed %q", name, got)
		}
	}
}

func TestAffiliationCanadianCallSignRules(t *testing.T) {
	cases := map[string]string{
		"CBQT": "CBC", "CBQT-DT": "CBC", "CBQFT": "Radio-Canada", "CBQFT-DT": "Radio-Canada",
		"CBFT": "Radio-Canada", "CBVT": "Radio-Canada",
		"CBQX": "", "CBQQFT": "", "CKQT": "",
	}
	for call, want := range cases {
		if got := Affiliation(nil, call); got != want {
			t.Errorf("%s: %q, want %q", call, got, want)
		}
	}
}

func TestMainNetworksByCountry(t *testing.T) {
	if got := MainNetworks("US"); len(got) != 4 || got[0] != "ABC" {
		t.Fatalf("US: %v", got)
	}
	if got := MainNetworks("CA"); got[0] != "CBC" || got[1] != "CTV" {
		t.Fatalf("CA: %v", got)
	}
	if !IsMainNetwork("Global") || !IsMainNetwork("NBC") || IsMainNetwork("PBS") || IsMainNetwork("") {
		t.Fatal("IsMainNetwork")
	}
}
