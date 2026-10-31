package hdhr

import "testing"

func TestModelNote(t *testing.T) {
	cases := []struct{ model, note string }{
		{"HDHR5-4K", "ATSC 3.0 channels stay on the tuner."},
		{"HDHR3-CC", "Copy protected channels stay off the guide."},
		{"HDTC-2US", "This tuner can convert the picture."},
		{"HDHR5-2US", ""},
	}
	for _, tc := range cases {
		if got := ModelNote(tc.model); got != tc.note {
			t.Fatalf("%s: %q", tc.model, got)
		}
	}
	if ExtendQuery("HDTC-2US") != "transcode=mobile" || ExtendQuery("HDHR5-2US") != "" {
		t.Fatal(ExtendQuery("HDTC-2US"), ExtendQuery("HDHR5-2US"))
	}
}

func TestBroadcastGuideIsAnATSCAntenna(t *testing.T) {
	for model, want := range map[string]bool{
		"HDHR5-4US": true, "HDFX-4K": true, "HDTC-2US": true, "HDHR3-US": true, "": true,
		"HDHR5-4DT": false, "HDHR4-2DT": false, "HDHR3-4DC": false, "HDHR3-EU": false, "HDHR3-CC": false,
	} {
		if got := BroadcastGuide(model); got != want {
			t.Errorf("BroadcastGuide(%q) = %v", model, got)
		}
	}
}
