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
