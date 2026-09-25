package discovery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLaterArrivalBannersOnce(t *testing.T) {
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	duo := Place{ID: "tuner|10611B4C", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun CONNECT DUO", Action: "added"}
	flex := Place{ID: "tuner|FLEX4K", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun FLEX 4K", Action: "add"}
	plex := Place{ID: "plex|192.168.1.2", Group: "server", Kind: "plex", Name: "TUS", Action: "use"}
	tv := Place{ID: "here|appletv|Living Room", Group: "screen", Kind: "appletv", Name: "Living Room", Action: "here"}
	browser := Place{ID: "here|web|This browser", Group: "screen", Kind: "web", Name: "This browser", Action: "here"}
	room := Place{ID: "screen|airplay|192.168.1.192", Group: "screen", Kind: "airplay", Name: "Living Room", Action: "found", Addr: "192.168.1.192"}

	var arrivals Arrivals
	if notes := arrivals.Observe(nil, start); notes != nil {
		t.Fatalf("an empty look is not the baseline: %v", notes)
	}
	if notes := arrivals.Observe([]Place{duo, browser}, start); len(notes) != 0 {
		t.Fatalf("a saved tuner and this browser are not a scan: %v", notes)
	}
	if notes := arrivals.Observe([]Place{duo, browser, room}, start); len(notes) != 0 {
		t.Fatalf("the house at startup stays quiet: %v", notes)
	}
	if notes := arrivals.Observe([]Place{duo, browser, room, flex}, start.Add(time.Second)); len(notes) != 1 || notes[0] != "New tuner found: HDHomeRun FLEX 4K. Add it?" {
		t.Fatalf("one banner: %v", notes)
	}
	if notes := arrivals.Observe([]Place{duo, browser, room, flex}, start.Add(2*time.Second)); len(notes) != 0 {
		t.Fatalf("the same tuner banners again: %v", notes)
	}
	// It leaves and comes back. Still one banner for the run.
	if notes := arrivals.Observe([]Place{duo}, start.Add(3*time.Second)); len(notes) != 0 {
		t.Fatal(notes)
	}
	if notes := arrivals.Observe([]Place{duo, flex}, start.Add(4*time.Second)); len(notes) != 0 {
		t.Fatalf("a return is not a new device: %v", notes)
	}

	// A reconnecting app during the first minute is not a new screen.
	// A server, and an Apple TV that appears after the grace, each get one line.
	var later Arrivals
	later.Observe([]Place{duo, room}, start)
	if notes := later.Observe([]Place{duo, room, tv}, start.Add(time.Minute)); len(notes) != 0 {
		t.Fatalf("grace: %v", notes)
	}
	den := Place{ID: "here|appletv|Den", Group: "screen", Kind: "appletv", Name: "Den", Action: "here"}
	notes := later.Observe([]Place{duo, room, tv, plex, den}, start.Add(2*time.Hour))
	if len(notes) != 2 || notes[0] != "New Plex server found: TUS. Use it as a tuner?" || notes[1] != "New Apple TV found: Den." {
		t.Fatalf("%v", notes)
	}
	if again := later.Observe([]Place{duo, room, tv, plex, den}, start.Add(3*time.Hour)); len(again) != 0 {
		t.Fatalf("second pass: %v", again)
	}
}

func TestSameServerOnTwoAddressesIsOne(t *testing.T) {
	_, lanNet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	_, dock, err := net.ParseCIDR("172.17.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	nets := []*net.IPNet{lanNet, dock}
	found := []Found{
		{Kind: "channels", Name: "tus", Addr: "192.168.1.2", ID: "ch"},
		{Kind: "channels", Name: "tus", Addr: "172.17.0.1", ID: "ch"},
	}
	places := Assemble(found, nil, nil, nets)
	if len(places) != 1 || places[0].ID != "channels|tus" {
		t.Fatalf("%+v", places)
	}
	var arrivals Arrivals
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if notes := arrivals.Observe(places, start); len(notes) != 0 {
		t.Fatal(notes)
	}
	again := Assemble([]Found{{Kind: "channels", Name: "tus", Addr: "172.17.0.1", ID: "ch"}}, nil, nil, nets)
	if len(again) != 1 {
		t.Fatalf("bridge address was dropped: %+v", again)
	}
	if notes := arrivals.Observe(again, start.Add(2*time.Hour)); len(notes) != 0 {
		t.Fatalf("a bridge address is not a new server: %v", notes)
	}
}

func TestAMissedSpeakerStaysQuietAtStartup(t *testing.T) {
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	room := Place{ID: "screen|airplay|192.168.1.192", Group: "screen", Kind: "airplay", Name: "Living Room", Action: "found", Addr: "192.168.1.192"}
	bed := Place{ID: "screen|airplay|192.168.1.245", Group: "screen", Kind: "airplay", Name: "Bedroom", Action: "found", Addr: "192.168.1.245"}
	var arrivals Arrivals
	if notes := arrivals.Observe([]Place{room}, start); len(notes) != 0 {
		t.Fatal(notes)
	}
	if notes := arrivals.Observe([]Place{room, bed}, start.Add(45*time.Second)); len(notes) != 0 {
		t.Fatalf("a speaker the first scan missed: %v", notes)
	}
	if notes := arrivals.Observe([]Place{room, bed}, start.Add(2*time.Hour)); len(notes) != 0 {
		t.Fatalf("already seen: %v", notes)
	}
}

func TestTunerAnsweredTwoWaysIsOneBanner(t *testing.T) {
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	house := Place{ID: "plex|tus", Group: "server", Kind: "plex", Name: "TUS", Action: "use", Addr: "192.168.1.2"}
	udp := Place{ID: "tuner|F1E40001", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun FLEX 4K", Action: "add", Addr: "192.168.1.60"}
	ssdp := Place{ID: "tuner|uuid:F1E40001", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun", Action: "add", Addr: "192.168.1.60"}
	var arrivals Arrivals
	if notes := arrivals.Observe([]Place{house}, start); len(notes) != 0 {
		t.Fatal(notes)
	}
	notes := arrivals.Observe([]Place{house, udp}, start.Add(time.Hour))
	if len(notes) != 1 || notes[0] != "New tuner found: HDHomeRun FLEX 4K. Add it?" {
		t.Fatalf("%v", notes)
	}
	if notes := arrivals.Observe([]Place{house, ssdp}, start.Add(2*time.Hour)); len(notes) != 0 {
		t.Fatalf("the same tuner on a second id: %v", notes)
	}
}

func TestNoticeSkipsWhatIsAlreadyHere(t *testing.T) {
	if _, ok := notice(Place{Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun", Action: "added"}); ok {
		t.Fatal("an added tuner is not news")
	}
	if _, ok := notice(Place{Group: "screen", Kind: "web", Name: "This browser", Action: "here"}); ok {
		t.Fatal("this browser is not a new device")
	}
	msg, ok := notice(Place{Group: "server", Kind: "plex", Name: "Plex", Action: "use"})
	if !ok || msg != "New Plex server found. Use it as a tuner?" {
		t.Fatalf("%q %v", msg, ok)
	}
}

func TestTunerLabelUsesTheFriendlyName(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"FriendlyName":"HDHomeRun FLEX 4K","ModelNumber":"HDHR5-4US","DeviceID":"FLEX4K","BaseURL":"%s","TunerCount":4,"DeviceAuth":"secret"}`, srv.URL)
	}))
	defer srv.Close()
	if got := tunerLabel(context.Background(), srv.URL); got != "HDHomeRun FLEX 4K" {
		t.Fatalf("%q", got)
	}
	if got := tunerLabel(context.Background(), "http://127.0.0.1:1"); got != "HDHomeRun" {
		t.Fatalf("%q", got)
	}
	if got := tunerLabel(context.Background(), ""); got != "HDHomeRun" {
		t.Fatalf("%q", got)
	}
}
