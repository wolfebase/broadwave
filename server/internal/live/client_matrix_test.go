package live

import "testing"

// TestClientMatrix is the HW5 table. Quality is auto on the LAN, so Decide
// keeps the original picture when the client can play it.
// Apple TV HD is 1080p and does not advertise HEVC. Browsers do not
// advertise HEVC. Chrome, Edge, and Firefox do not advertise AC-3.
func TestClientMatrix(t *testing.T) {
	ota := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	p720 := Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true}
	atv4k := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}, MaxHeight: 2160}
	phone := Caps{Platform: "ios", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}}
	rows := []struct {
		name string
		caps Caps
		ota  string
		prog string
	}{
		{"Apple TV HD", Caps{Platform: "tvos", Video: []string{"h264"}, Audio: []string{"aac", "ac3", "eac3"}, MaxHeight: 1080}, "1080.copy.broadcast", "copy.copy"},
		{"Apple TV 4K (1st generation)", atv4k, "1080.copy.broadcast.hevc", "copy.copy"},
		{"Apple TV 4K (2nd generation)", atv4k, "1080.copy.broadcast.hevc", "copy.copy"},
		{"Apple TV 4K (3rd generation)", atv4k, "1080.copy.broadcast.hevc", "copy.copy"},
		{"iPhone 6s", Caps{Platform: "ios", Video: []string{"h264"}, Audio: []string{"aac", "ac3", "eac3"}}, "1080.copy.broadcast", "copy.copy"},
		{"iPhone SE (2nd generation)", phone, "1080.copy.broadcast.hevc", "copy.copy"},
		{"iPhone 11", phone, "1080.copy.broadcast.hevc", "copy.copy"},
		{"iPad", phone, "1080.copy.broadcast.hevc", "copy.copy"},
		{"Safari", Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac", "ac3"}}, "1080.copy.broadcast", "copy.copy"},
		{"Chrome", Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}, "1080.aac2.broadcast", "copy.copy"},
		{"Edge", Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}, "1080.aac2.broadcast", "copy.copy"},
		{"Firefox", Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}, "1080.aac2.broadcast", "copy.copy"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			assertClientKey(t, "1080i mpeg-2 ac-3", ota, row.caps, row.ota)
			assertClientKey(t, "720p h264 aac", p720, row.caps, row.prog)
		})
	}
}

func assertClientKey(t *testing.T, label string, src Source, caps Caps, want string) {
	t.Helper()
	got := Decide(src, caps, Prefs{Quality: "auto"})
	if got.Rendition.Key() != want {
		t.Errorf("%s: %s (%s), want %s", label, got.Rendition.Key(), got.Reason, want)
	}
}
