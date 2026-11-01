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

func TestBroadcastGuideIsAnAntennaOrDVBTuner(t *testing.T) {
	for model, want := range map[string]bool{
		"HDHR5-4US": true, "HDFX-4K": true, "HDTC-2US": true, "HDHR3-US": true, "": true,
		"HDHR5-4DT": true, "HDHR4-2DT": true, "HDHR3-4DC": true, "HDHR3-EU": true,
		"HDHR3-CC": false, "HDHR PRIME": false,
	} {
		if got := BroadcastGuide(model); got != want {
			t.Errorf("BroadcastGuide(%q) = %v", model, got)
		}
	}
	for model, want := range map[string]bool{
		"HDHR5-4DT": true, "HDHR4-2DT": true, "HDHR3-4DC": true, "HDHR3-EU": true, "hdhr5-2dt": true,
		"HDHR5-4US": false, "HDFX-4K": false, "HDHR3-CC": false, "": false, "m3u": false,
	} {
		if got := DVB(model); got != want {
			t.Errorf("DVB(%q) = %v", model, got)
		}
	}
}
