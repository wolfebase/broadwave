package network

import (
	"testing"
)

func TestAffiliationGuideNameWins(t *testing.T) {
	if got := Of([]string{"ABC"}, "KDVR"); got != "ABC" {
		t.Fatalf("guide name ABC lost to KDVR, got %q", got)
	}
	if got := Of([]string{"FOX 4"}, "KXYZ"); got != "FOX" {
		t.Fatalf("FOX 4: %q", got)
	}
	if got := Of([]string{"KDVR (FOX)"}, "KXYZ"); got != "FOX" {
		t.Fatalf("parenthetical: %q", got)
	}
	if got := Of([]string{"NBC"}); got != "NBC" {
		t.Fatalf("exact NBC: %q", got)
	}
}

func TestAffiliationCallSign(t *testing.T) {
	if calls["KDVR"] != "FOX" || calls["KCNC"] != "CBS" || calls["KMGH"] != "ABC" || calls["KUSA"] != "NBC" {
		t.Fatal("Denver affiliates missing from the table")
	}
	if got := Of(nil, "KDVR-DT"); got != "FOX" {
		t.Fatalf("KDVR-DT: %q", got)
	}
	if got := Of(nil, "KCNCDT"); got != "CBS" {
		t.Fatalf("KCNCDT: %q", got)
	}
	if got := Of(nil, "KMGH-HD"); got != "ABC" {
		t.Fatalf("KMGH-HD: %q", got)
	}
	if got := Of([]string{"4.1", "KDVRDT"}, "HD"); got != "FOX" {
		t.Fatalf("guide call sign: %q", got)
	}
}

func TestAffiliationNumberedDTSubchannels(t *testing.T) {
	if got := Of(nil, "KDVR-DT"); got != "FOX" {
		t.Fatalf("KDVR-DT: %q", got)
	}
	if got := Of(nil, "KDVR-DT1"); got != "FOX" {
		t.Fatalf("KDVR-DT1: %q", got)
	}
	for _, name := range []string{"KDVR-DT2", "KDVR-DT3", "KDVR-DT4", "KCNC-DT2"} {
		if got := Of(nil, name); got != "" {
			t.Fatalf("%s matched %q", name, got)
		}
	}
	if got := Of([]string{"KDVRDT2"}); got != "" {
		t.Fatalf("guide name KDVRDT2 matched %q", got)
	}
}

func TestAffiliationDoesNotGuess(t *testing.T) {
	for _, name := range []string{"FOX NEWS", "SHOPNBC", "KDVR2", "WFOX", "NOTNBC", "Jewelry"} {
		if got := Of([]string{name}); got != "" {
			t.Fatalf("%s guessed %q", name, got)
		}
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
		"KBWV-NBC": "NBC", "WBWV CW": "CW", "KBWV-DT (PBS)": "PBS", "CBWV CTV": "CTV", "ABC7 3.0": "ABC", "FOX 3.0": "FOX",
	}
	for name, want := range cases {
		if got := Of([]string{name}, "KQQQ"); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

func TestAffiliationLeavesDiginetsAndLookalikesAlone(t *testing.T) {
	for _, name := range []string{
		"PBS KIDS", "PBSKIDS", "CBS NEWS", "ION PLUS", "IONMYST", "CBC NN", "CTV NEWS",
		"GLOBALNEWS", "KCITY", "CITY NEWS", "MYSTERY", "MYTVLIVE", "NBCLX", "TVA SPORTS", "NOTPBS",
	} {
		if got := Of([]string{name}); got != "" {
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
		if got := Of(nil, call); got != want {
			t.Errorf("%s: %q, want %q", call, got, want)
		}
	}
}

func TestMainNetworksByCountry(t *testing.T) {
	if got := Main("US"); len(got) != 4 || got[0] != "ABC" {
		t.Fatalf("US: %v", got)
	}
	if got := Main("CA"); got[0] != "CBC" || got[1] != "CTV" {
		t.Fatalf("CA: %v", got)
	}
	if !IsMain("Global") || !IsMain("NBC") || IsMain("PBS") || IsMain("") {
		t.Fatal("IsMainNetwork")
	}
}
