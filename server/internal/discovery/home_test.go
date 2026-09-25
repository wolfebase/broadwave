package discovery

import (
	"net"
	"strings"
	"testing"
)

func lan() []*net.IPNet {
	_, n, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		panic(err)
	}
	return []*net.IPNet{n}
}

func TestClassifySSDPKeepsScreensAndServers(t *testing.T) {
	fire, ok := classifySSDP("HTTP/1.1 200 OK\r\nST: urn:dial-multiscreen-org:service:dial:1\r\nSERVER: Linux/3.0 UPnP/1.0 Fire TV\r\nLOCATION: http://192.168.1.40:8008/dd.xml\r\nUSN: uuid:fire\r\n\r\n", "192.168.1.40:8008")
	if !ok || fire.Kind != "firetv" || fire.Addr != "192.168.1.40" || fire.Name != "Fire TV" {
		t.Fatalf("%+v ok=%v", fire, ok)
	}
	tv, ok := classifySSDP("HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\nSERVER: Samsung TV\r\nLOCATION: http://192.168.1.41/desc.xml\r\n\r\n", "192.168.1.41:1900")
	if !ok || tv.Kind != "tv" || tv.Name != "Samsung TV" {
		t.Fatalf("%+v ok=%v", tv, ok)
	}
	plex, ok := classifySSDP("HTTP/1.1 200 OK\r\nSERVER: Plex\r\nLOCATION: http://192.168.1.2:32400/\r\nUSN: uuid:plex\r\n\r\n", "192.168.1.2:1900")
	if !ok || plex.Kind != "plex" || plex.Addr != "192.168.1.2" {
		t.Fatalf("%+v ok=%v", plex, ok)
	}
	if _, ok := classifySSDP("HTTP/1.1 200 OK\r\nSERVER: HP Printer\r\nST: upnp:rootdevice\r\nLOCATION: http://192.168.1.9/\r\n\r\n", "192.168.1.9:1900"); ok {
		t.Fatal("a printer is not part of the house")
	}
}

func TestParsePlexAndJellyfin(t *testing.T) {
	plex, ok := parsePlex("HTTP/1.0 200 OK\r\nContent-Type: plex/media-server\r\nName: Living Room\r\nResource-Identifier: abc\r\nPort: 32400\r\n\r\n", "192.168.1.2:32414")
	if !ok || plex.Kind != "plex" || plex.Name != "Living Room" || plex.Addr != "192.168.1.2" || plex.ID != "abc" {
		t.Fatalf("%+v ok=%v", plex, ok)
	}
	if _, ok := parsePlex("HTTP/1.0 200 OK\r\nName: Something else\r\n\r\n", "192.168.1.2:1"); ok {
		t.Fatal("a packet that is not Plex must be ignored")
	}
	jelly, ok := parseMedia([]byte(`{"Address":"http://192.168.1.2:8096","Id":"j1","Name":"Films"}`), "jellyfin")
	if !ok || jelly.Kind != "jellyfin" || jelly.Addr != "192.168.1.2" || jelly.Name != "Films" {
		t.Fatalf("%+v", jelly)
	}
	emby, ok := parseMedia([]byte(`{"Address":"http://192.168.1.8:8096","Id":"e1","Name":"Emby"}`), "jellyfin")
	if !ok || emby.Kind != "emby" {
		t.Fatalf("%+v", emby)
	}
}

func TestUnescapeDNSNames(t *testing.T) {
	if got := unescapeDNS(`Entertainment\ Room`); got != "Entertainment Room" {
		t.Fatalf("%q", got)
	}
	if got := unescapeDNS(`MacBook\ Pro\ 16\"`); got != `MacBook Pro 16"` {
		t.Fatalf("%q", got)
	}
}

func TestOnLANDropsThePublicInternet(t *testing.T) {
	nets := lan()
	if !OnLAN("192.168.1.252", nets) || OnLAN("8.8.8.8", nets) || OnLAN("not-an-ip", nets) {
		t.Fatal("only an address on the given subnet counts")
	}
}

func TestAssembleOneActionEach(t *testing.T) {
	nets := lan()
	found := []Found{
		{Kind: "hdhomerun", Name: "HDHomeRun", Addr: "192.168.1.252", ID: "10611B4C"},
		{Kind: "hdhomerun", Name: "HDHomeRun", Addr: "192.168.1.252", ID: "uuid:10611B4C"},
		{Kind: "hdhomerun", Name: "Other", Addr: "192.168.1.60", ID: "NEWDEVICE"},
		{Kind: "plex", Name: "TUS", Addr: "192.168.1.2", ID: "plex1"},
		{Kind: "channels", Name: "tus", Addr: "192.168.1.2", ID: "ch1"},
		{Kind: "plex", Name: "Stranger", Addr: "8.8.8.8", ID: "nope"},
		{Kind: "firetv", Name: "Fire TV", Addr: "192.168.1.40"},
		{Kind: "tv", Name: "TV", Addr: "192.168.1.40"},
		{Kind: "airplay", Name: "Den", Addr: "192.168.1.41"},
	}
	known := []Known{{ID: "10611B4C", Name: "CONNECT DUO", Addr: "192.168.1.252", Tuners: 2}}
	screens := []Screen{{Name: "Broadwave Staging TV", Kind: "appletv", Addr: "192.168.1.20"}}
	places := Assemble(found, known, screens, nets)
	byID := map[string]Place{}
	for _, place := range places {
		if _, ok := byID[place.ID]; ok {
			t.Fatalf("duplicate %s", place.ID)
		}
		byID[place.ID] = place
		if place.Action == "" {
			t.Fatalf("no action: %+v", place)
		}
	}
	duo := byID["tuner|10611B4C"]
	if duo.Action != "added" || duo.Name != "CONNECT DUO" || duo.Detail != "2 tuners" {
		t.Fatalf("duo %+v", duo)
	}
	if byID["tuner|NEWDEVICE"].Action != "add" {
		t.Fatal("a new tuner waits for a tap")
	}
	if byID["plex|192.168.1.2"].Action != "use" || byID["channels|192.168.1.2"].Action != "use" {
		t.Fatalf("%+v", places)
	}
	for _, place := range places {
		if strings.Contains(place.Addr, "8.8.8.8") || place.Name == "Stranger" {
			t.Fatal("off-subnet result leaked")
		}
	}
	if _, ok := byID["screen|tv|192.168.1.40"]; ok {
		t.Fatal("a generic TV on the same address as a Fire TV is one screen")
	}
	if byID["screen|firetv|192.168.1.40"].Action != "found" {
		t.Fatal("fire tv")
	}
	if byID["here|appletv|Broadwave Staging TV"].Action != "here" {
		t.Fatal("simulator screen")
	}
	if byID["plex|192.168.1.2"].Detail != "Plex" || byID["channels|192.168.1.2"].Detail != "Channels" {
		t.Fatalf("server kind %+v %+v", byID["plex|192.168.1.2"], byID["channels|192.168.1.2"])
	}
	if places[0].Group != "tuner" {
		t.Fatalf("tuners first, got %+v", places[0])
	}
}
