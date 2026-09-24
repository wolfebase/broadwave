package doctor

import (
	"strings"
	"testing"
	"time"
)

func TestEachCheck(t *testing.T) {
	ok := Facts{IPs: []string{"192.168.1.10"}, BroadcastOK: true, HostHasGPU: true, DevDri: true, RecordingsPath: "/data/recordings", Mounts: "/dev/sda /data ext4 rw 0 0\n", FreeBytes: 40e9, Timezone: "UTC", Now: time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), UID: 99, PUID: "99", PGID: "100"}
	if n := Notes(ok); len(n) != 0 {
		t.Fatalf("healthy server: %+v", n)
	}
	cases := []struct {
		id   string
		edit func(*Facts)
	}{
		{"bridge", func(f *Facts) { f.IPs = []string{"172.17.0.2"}; f.BroadcastOK = false }},
		{"dri", func(f *Facts) { f.DevDri = false }},
		{"volume", func(f *Facts) { f.RecordingsPath = "/config/work/recordings" }},
		{"disk", func(f *Facts) { f.FreeBytes = 5e9 }},
		{"tz", func(f *Facts) { f.Timezone = "" }},
		{"clock", func(f *Facts) { f.Now = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"owner", func(f *Facts) { f.UID = 0; f.PUID = ""; f.PGID = "" }},
		{"tuner", func(f *Facts) { f.TunerQuiet = true }},
	}
	for _, tc := range cases {
		f := ok
		tc.edit(&f)
		notes := Notes(f)
		if len(notes) != 1 || notes[0].ID != tc.id || !strings.Contains(notes[0].Message, ".") {
			t.Fatalf("%s: %+v", tc.id, notes)
		}
	}
}

func TestHomeLANIsNotDocker(t *testing.T) {
	f := Facts{IPs: []string{"172.16.1.20"}, BroadcastOK: false, Timezone: "UTC", Now: time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), UID: 99, PUID: "99", PGID: "100", FreeBytes: 40e9, DevDri: true}
	for _, n := range Notes(f) {
		if n.ID == "bridge" {
			t.Fatal("172.16 is a home network")
		}
	}
}
