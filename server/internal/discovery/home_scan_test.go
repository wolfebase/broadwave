package discovery

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

func TestSearchSSDPKindsReadsLoopbackReplies(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	const dial = "urn:dial-multiscreen-org:service:dial:1"
	const renderer = "urn:schemas-upnp-org:device:MediaRenderer:1"
	cases := []struct {
		server, st, location, usn string
		kind, name, addr          string
	}{
		{"Linux/5.15 UPnP/1.0", "upnp:rootdevice", "http://203.0.113.10/device.xml", "uuid:hdhomerun:1", "hdhomerun", "HDHomeRun", "203.0.113.10"},
		{"Lounge", "urn:plex:1", "http://203.0.113.20:32400/", "uuid:plex-lounge", "plex", "Lounge", "203.0.113.20"},
		{"Linux/6.1 UPnP/1.0", "urn:jellyfin:1", "http://203.0.113.21:8096/", "uuid:jellyfin-1", "jellyfin", "Jellyfin", "203.0.113.21"},
		{"Den TV", "urn:emby:1", "http://203.0.113.22:8096/", "uuid:den-emby", "emby", "Den TV", "203.0.113.22"},
		{"Lounge", "upnp:rootdevice", "http://203.0.113.23:8089/", "uuid:channels-lounge", "channels", "Lounge", "203.0.113.23"},
		{"UPnP/1.0", "upnp:rootdevice", "http://203.0.113.24:9981/", "uuid:tvheadend", "tvheadend", "tvheadend", "203.0.113.24"},
		{"Fire TV", dial, "http://203.0.113.40:8008/dd.xml", "uuid:fire-den", "firetv", "Fire TV", "203.0.113.40"},
		{"Den TV", dial, "http://203.0.113.41:8008/dd.xml", "uuid:shield", "androidtv", "Den TV", "203.0.113.41"},
		{"Amazon TV", dial, "http://203.0.113.42:8008/dd.xml", "uuid:amazon-1", "firetv", "Amazon TV", "203.0.113.42"},
		{"Lounge", renderer, "http://203.0.113.43/desc.xml", "uuid:tv-lounge", "tv", "Lounge", "203.0.113.43"},
	}
	ignored := []struct{ server, st, location, usn string }{
		{"Office Printer", "upnp:rootdevice", "http://203.0.113.50/", "uuid:printer"},
		{"Gateway", "upnp:rootdevice", "http://203.0.113.1/", "uuid:router"},
	}
	var bodies []string
	for _, tc := range cases {
		bodies = append(bodies, ssdpReply(tc.server, tc.st, tc.location, tc.usn))
	}
	for _, tc := range ignored {
		bodies = append(bodies, ssdpReply(tc.server, tc.st, tc.location, tc.usn))
	}

	var mu sync.Mutex
	var searches []string
	go func() {
		buf := make([]byte, 4096)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			raw := string(buf[:n])
			mu.Lock()
			searches = append(searches, raw)
			mu.Unlock()
			for _, body := range bodies {
				if _, err := conn.WriteTo([]byte(body), addr); err != nil {
					return
				}
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	found, err := searchSSDPKinds(ctx, conn.LocalAddr().String(), homeSearchTargets)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Found{}
	for _, item := range found {
		if item.Addr == "127.0.0.1" {
			t.Fatalf("location did not replace the sender: %+v", item)
		}
		if _, ok := got[item.ID]; ok {
			t.Fatalf("duplicate %s", item.ID)
		}
		got[item.ID] = item
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d %+v", len(got), found)
	}
	for _, tc := range cases {
		want := Found{Kind: tc.kind, Name: tc.name, Addr: tc.addr, ID: tc.usn}
		if got[tc.usn] != want {
			t.Fatalf("%s\n got %+v\nwant %+v", tc.usn, got[tc.usn], want)
		}
	}

	mu.Lock()
	seen := append([]string(nil), searches...)
	mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("no m-search")
	}
	gotST := map[string]bool{}
	for _, raw := range seen {
		if !strings.HasPrefix(raw, "M-SEARCH * HTTP/1.1") {
			t.Fatalf("query %q", raw)
		}
		if headerValue(raw, "HOST") != "239.255.255.250:1900" {
			t.Fatalf("HOST %q", raw)
		}
		st := headerValue(raw, "ST")
		if st == "" {
			t.Fatalf("no ST in %q", raw)
		}
		gotST[st] = true
	}
	for _, st := range homeSearchTargets {
		if !gotST[st] {
			t.Fatalf("missing %s in %v", st, gotST)
		}
	}
}

func TestSearchSSDPKindsRejectsAnUnusableAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := searchSSDPKinds(ctx, "::bad", []string{"upnp:rootdevice"})
	if err == nil || !strings.Contains(err.Error(), "::bad") {
		t.Fatalf("%v", err)
	}
}

func TestSearchSSDPKindsUsesTheSSDPPort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := searchSSDPKinds(ctx, "127.0.0.1", []string{"upnp:rootdevice"}); err != nil {
		t.Fatal(err)
	}
}

func TestKindLabelNamesEveryKind(t *testing.T) {
	labels := []struct{ kind, label string }{
		{"hdhomerun", "HDHomeRun"},
		{"plex", "Plex"},
		{"jellyfin", "Jellyfin"},
		{"emby", "Emby"},
		{"channels", "Channels"},
		{"channels-dvr", "Channels"},
		{"tvheadend", "tvheadend"},
		{"chromecast", "Chromecast"},
		{"airplay", "AirPlay"},
		{"firetv", "Fire TV"},
		{"androidtv", "Android TV"},
		{"tv", "TV"},
		{"iphone", "iPhone"},
		{"ipad", "iPad"},
		{"appletv", "Apple TV"},
		{"web", "This browser"},
		{"satellite", "satellite"},
	}
	for _, tc := range labels {
		if got := kindLabel(tc.kind); got != tc.label {
			t.Fatalf("%s: %q", tc.kind, got)
		}
	}
}

func TestScreenRankPrefersTheSetTopBox(t *testing.T) {
	if screenRank("firetv") != 3 || screenRank("androidtv") != 3 || screenRank("chromecast") != 3 {
		t.Fatal("a set-top box outranks a speaker")
	}
	if screenRank("airplay") != 2 {
		t.Fatal("airplay")
	}
	if screenRank("tv") != 1 || screenRank("iphone") != 1 || screenRank("web") != 1 || screenRank("fridge") != 1 {
		t.Fatal("everything else is a plain screen")
	}
}

func TestGroupsSeparateTunersServersAndScreens(t *testing.T) {
	groups := []struct{ kind, group string }{
		{"hdhomerun", "tuner"},
		{"hdhr-compatible", "tuner"},
		{"tvheadend", "tuner"},
		{"plex", "server"},
		{"jellyfin", "server"},
		{"emby", "server"},
		{"channels", "server"},
		{"channels-dvr", "server"},
		{"chromecast", "screen"},
		{"airplay", "screen"},
		{"firetv", "screen"},
		{"androidtv", "screen"},
		{"tv", "screen"},
		{"iphone", "screen"},
		{"ipad", "screen"},
		{"appletv", "screen"},
		{"web", "screen"},
		{"fridge", ""},
	}
	for _, tc := range groups {
		if got := groupOf(tc.kind); got != tc.group {
			t.Fatalf("%s: %q", tc.kind, got)
		}
	}
	if groupOrder("tuner") != 0 || groupOrder("server") != 1 || groupOrder("screen") != 2 || groupOrder("") != 2 {
		t.Fatalf("order %d %d %d", groupOrder("tuner"), groupOrder("server"), groupOrder("screen"))
	}
}

func TestTunerDetailCounts(t *testing.T) {
	if tunerDetail(0) != "0 tuners" || tunerDetail(1) != "1 tuner" || tunerDetail(2) != "2 tuners" {
		t.Fatalf("%q %q %q", tunerDetail(0), tunerDetail(1), tunerDetail(2))
	}
}

func TestLabelsDropNoiseAndKeepTheHost(t *testing.T) {
	if got := cleanLabel(" \x00Den\x1f TV\x7f "); got != "Den TV" {
		t.Fatalf("%q", got)
	}
	if cleanLabel("   ") != "" || cleanLabel("\x00\x7f") != "" {
		t.Fatal("blank")
	}
	long := strings.Repeat("å", 80)
	if got := cleanLabel("  " + long); len([]rune(got)) != 64 || got != strings.Repeat("å", 64) {
		t.Fatalf("len %d %q", len([]rune(got)), got)
	}
	if fallbackName("", "plex") != "Plex" || fallbackName(" \x00 ", "plex") != "Plex" {
		t.Fatal("empty name")
	}
	if fallbackName(" Lounge ", "plex") != "Lounge" {
		t.Fatal("kept name")
	}
	if displayName("jellyfin", "") != "Jellyfin" || displayName("jellyfin", "Linux") != "Jellyfin" {
		t.Fatal("linux header")
	}
	if displayName("tvheadend", "UPnP/1.0") != "tvheadend" {
		t.Fatal("upnp header")
	}
	if displayName("androidtv", "DIAL/1.0") != "Android TV" {
		t.Fatal("dial header")
	}
	if displayName("emby", "Den TV") != "Den TV" {
		t.Fatal("friendly name")
	}
	if hostOnly("") != "" || hostOnly("   ") != "" {
		t.Fatal("empty host")
	}
	if hostOnly("http://203.0.113.10:8008/dd.xml") != "203.0.113.10" {
		t.Fatal("http")
	}
	if hostOnly("https://203.0.113.10/a") != "203.0.113.10" {
		t.Fatal("https")
	}
	if hostOnly("203.0.113.10:1900") != "203.0.113.10" {
		t.Fatal("port")
	}
	if hostOnly("203.0.113.10") != "203.0.113.10" {
		t.Fatal("bare host")
	}
}

func TestLocalNetsSkipLoopback(t *testing.T) {
	for _, n := range LocalNets() {
		if n == nil || n.IP == nil || n.IP.To4() == nil || n.IP.IsLoopback() {
			t.Fatal("a local network was empty or a loopback")
		}
		if !n.IP.Mask(n.Mask).Equal(n.IP) {
			t.Fatal("a local network kept host bits")
		}
	}
}

func TestLocalIPsSkipLoopback(t *testing.T) {
	for _, ip := range localIPs() {
		parsed := net.ParseIP(ip)
		if parsed == nil || ip == "" || parsed.IsLoopback() || parsed.IsUnspecified() {
			t.Fatal("a local address was empty, unspecified, or a loopback")
		}
	}
}

func TestAdvertTextCarriesTheIdentity(t *testing.T) {
	got := Advert{ID: "srv-1", Name: "Lounge", Version: "1.4.0", Port: 8477}.txt()
	want := []string{"id=srv-1", "name=Lounge", "version=1.4.0", "api=1", "port=8477"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}

func TestEnableBroadcastOnLoopback(t *testing.T) {
	// A conn that was never listened on cannot take the option.
	enableBroadcast(new(net.UDPConn))
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	enableBroadcast(conn)
}

func TestMatchProbeReadsPlaylistsAndNames(t *testing.T) {
	got, ok := matchProbe(ProbePort{Kind: "hdhomerun"}, []byte(`{"DeviceID":"ABC123","FriendlyName":""}`))
	if !ok || got.Kind != "hdhomerun" || got.Name != "HDHomeRun" || got.ID != "ABC123" {
		t.Fatalf("%+v %v", got, ok)
	}
	got, ok = matchProbe(ProbePort{Kind: "hdhr-compatible"}, []byte(`{"DeviceID":"DEF456","FriendlyName":"Lounge"}`))
	if !ok || got.Kind != "hdhr-compatible" || got.Name != "Lounge" || got.ID != "DEF456" {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok := matchProbe(ProbePort{Kind: "hdhomerun"}, []byte(`{"FriendlyName":"Lounge"}`)); ok {
		t.Fatal("no device id")
	}
	if _, ok := matchProbe(ProbePort{Kind: "hdhr-compatible"}, []byte(`{`)); ok {
		t.Fatal("broken json")
	}
	playlists := []struct{ kind, name string }{
		{"tvheadend", "tvheadend"},
		{"channels-dvr", "channels-dvr"},
		{"fastchannels", "FastChannels"},
		{"pluto", "Pluto"},
		{"samsung", "Samsung TV Plus"},
	}
	body := []byte("#EXTM3U\n#EXTINF:-1,News\nhttp://127.0.0.1/a\n")
	for _, tc := range playlists {
		got, ok := matchProbe(ProbePort{Kind: tc.kind}, body)
		if !ok || got.Kind != tc.kind || got.Name != tc.name || got.ID != "" {
			t.Fatalf("%s %+v %v", tc.kind, got, ok)
		}
		if _, ok := matchProbe(ProbePort{Kind: tc.kind}, []byte("not a playlist")); ok {
			t.Fatalf("%s accepted a page", tc.kind)
		}
	}
	got, ok = matchProbe(ProbePort{Kind: "plex"}, []byte(`{"MediaContainer":"Plex"}`))
	if !ok || got.Kind != "plex" || got.Name != "plex" {
		t.Fatalf("%+v %v", got, ok)
	}
	got, ok = matchProbe(ProbePort{Kind: "tablo"}, []byte("TABLO"))
	if !ok || got.Kind != "tablo" || got.Name != "tablo" {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok := matchProbe(ProbePort{Kind: "jellyfin"}, []byte("hello")); ok {
		t.Fatal("unrelated body")
	}
	if _, ok := matchProbe(ProbePort{Kind: "tablo"}, nil); ok {
		t.Fatal("empty body")
	}
}

func TestProbeHostSkipsFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := ProbeHost(ctx, "127.0.0.1", []ProbePort{{Port: 9, Path: "/", Kind: "plex"}}); ok {
		t.Fatal("cancelled context probed")
	}
	if _, ok := ProbeHost(context.Background(), "127.0.0.1", []ProbePort{{Port: 9, Path: "/\n", Kind: "plex"}}); ok {
		t.Fatal("bad url")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()
	host, portText, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ProbeHost(context.Background(), host, []ProbePort{{Port: port, Path: "/", Kind: "plex"}}); ok {
		t.Fatal("404 is not a server")
	}
}

func TestScanHomeStaysEmptyUnderTheHarness(t *testing.T) {
	t.Setenv("BROADWAVE_E2E", "1")
	if got := ScanHome(context.Background()); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestLookWithNoHostsIsEmpty(t *testing.T) {
	if Look(context.Background(), nil) != nil {
		t.Fatal("nil hosts")
	}
	if got := Look(context.Background(), []string{}); got != nil {
		t.Fatalf("%v", got)
	}
}

func TestLocalHostsSkipsThisMachine(t *testing.T) {
	mine := map[string]bool{}
	for _, ip := range localIPs() {
		mine[ip] = true
	}
	seen := map[string]bool{}
	for _, host := range LocalHosts() {
		parsed := net.ParseIP(host)
		if host == "" || parsed == nil || parsed.To4() == nil || parsed.IsLoopback() || mine[host] || seen[host] {
			t.Fatal("a listed host was empty, this machine, or a loopback")
		}
		seen[host] = true
	}
}

func TestHostsInKeepsToASmallBlock(t *testing.T) {
	if HostsIn(&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)}) != nil {
		t.Fatal("ipv6")
	}
	if HostsIn(&net.IPNet{IP: net.IPv4(203, 0, 113, 1), Mask: net.CIDRMask(64, 128)}) != nil {
		t.Fatal("mask")
	}
	_, small, err := net.ParseCIDR("203.0.113.0/30")
	if err != nil {
		t.Fatal(err)
	}
	got := HostsIn(small)
	if len(got) != 2 || got[0] != "203.0.113.1" || got[1] != "203.0.113.2" {
		t.Fatalf("%v", got)
	}
	_, wide, err := net.ParseCIDR("203.0.113.0/31")
	if err != nil {
		t.Fatal(err)
	}
	if HostsIn(wide) != nil {
		t.Fatal("/31")
	}
}

func TestKeepLocalCloudDropsBlanksAndRenames(t *testing.T) {
	if got := KeepLocalCloud(context.Background(), []byte(`[]`), nil); got != nil {
		t.Fatalf("nil probe: %+v", got)
	}
	if got := KeepLocalCloud(context.Background(), []byte(``), func(context.Context, string) (string, bool) {
		return "ABC123", true
	}); got != nil {
		t.Fatalf("empty: %+v", got)
	}
	if got := KeepLocalCloud(context.Background(), []byte(`{`), func(context.Context, string) (string, bool) {
		return "ABC123", true
	}); got != nil {
		t.Fatalf("broken: %+v", got)
	}
	raw := []byte(`[
		{"DeviceID":"ABC123","LocalIP":"","FriendlyName":"Nope"},
		{"DeviceID":"","LocalIP":"203.0.113.10","FriendlyName":"Nope"},
		{"DeviceID":"DEAD","LocalIP":"203.0.113.11","FriendlyName":"Stranger"},
		{"DeviceID":"QUIET","LocalIP":"203.0.113.12","FriendlyName":"Quiet"},
		{"DeviceID":"BLANK","LocalIP":"203.0.113.13","FriendlyName":""},
		{"DeviceID":"KEPT","LocalIP":"203.0.113.14","FriendlyName":"Lounge"}
	]`)
	got := KeepLocalCloud(context.Background(), raw, func(_ context.Context, host string) (string, bool) {
		if host == "" {
			t.Fatal("probed a blank address")
		}
		switch host {
		case "203.0.113.11":
			return "NOTDEAD", true
		case "203.0.113.12":
			return "", false
		case "203.0.113.13":
			return "BLANK", true
		case "203.0.113.14":
			return "KEPT", true
		default:
			t.Fatalf("probed %s", host)
			return "", false
		}
	})
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0] != (Found{Kind: "hdhomerun", Name: "HDHomeRun", Addr: "203.0.113.13", ID: "BLANK"}) {
		t.Fatalf("%+v", got[0])
	}
	if got[1] != (Found{Kind: "hdhomerun", Name: "Lounge", Addr: "203.0.113.14", ID: "KEPT"}) {
		t.Fatalf("%+v", got[1])
	}
}

func TestScreenNounNamesEachKind(t *testing.T) {
	nouns := []struct{ kind, noun string }{
		{"appletv", "Apple TV"},
		{"iphone", "iPhone"},
		{"ipad", "iPad"},
		{"chromecast", "Chromecast"},
		{"airplay", "AirPlay device"},
		{"firetv", "Fire TV"},
		{"androidtv", "Android TV"},
		{"tv", "TV"},
		{"web", "This browser"},
		{"satellite", "satellite"},
	}
	for _, tc := range nouns {
		if got := screenNoun(tc.kind); got != tc.noun {
			t.Fatalf("%s: %q", tc.kind, got)
		}
	}
}

func TestNoticeStaysQuietForTheWrongGroup(t *testing.T) {
	if _, ok := notice(Place{Group: "screen", Kind: "tv", Name: "\x00", Action: "found"}); ok {
		t.Fatal("blank name")
	}
	if _, ok := notice(Place{Group: "server", Kind: "hdhomerun", Name: "Lounge", Action: "add"}); ok {
		t.Fatal("add is only a tuner")
	}
	if _, ok := notice(Place{Group: "screen", Kind: "plex", Name: "Lounge", Action: "use"}); ok {
		t.Fatal("use is only a server")
	}
	if _, ok := notice(Place{Group: "tuner", Kind: "iphone", Name: "Den", Action: "here"}); ok {
		t.Fatal("here is only a screen")
	}
	msg, ok := notice(Place{Group: "screen", Kind: "firetv", Name: "Den TV", Action: "found"})
	if !ok || msg != "New Fire TV found: Den TV." {
		t.Fatalf("%q %v", msg, ok)
	}
	msg, ok = notice(Place{Group: "screen", Kind: "airplay", Name: "Lounge", Action: "here"})
	if !ok || msg != "New AirPlay device found: Lounge." {
		t.Fatalf("%q %v", msg, ok)
	}
}

func TestFriendlyNamePrefersTheCastLabel(t *testing.T) {
	plain := &zeroconf.ServiceEntry{ServiceRecord: zeroconf.ServiceRecord{Instance: "Lounge"}}
	if friendlyName(plain) != "Lounge" {
		t.Fatal(friendlyName(plain))
	}
	cast := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: `Den\ TV`},
		Text:          []string{"noequals", "id=abc", "fn=Lounge"},
	}
	if friendlyName(cast) != "Lounge" {
		t.Fatal(friendlyName(cast))
	}
	blank := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "Den TV"},
		Text:          []string{"fn="},
	}
	if friendlyName(blank) != "Den TV" {
		t.Fatal(friendlyName(blank))
	}
	escaped := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "x"},
		Text:          []string{`fn=Den\ TV`},
	}
	if friendlyName(escaped) != "Den TV" {
		t.Fatal(friendlyName(escaped))
	}
	if unescapeDNS("Lounge") != "Lounge" {
		t.Fatal("plain")
	}
}

func TestParseSSDPNamesChannelsAndTvheadend(t *testing.T) {
	channels, ok := parseSSDP("HTTP/1.1 200 OK\r\nST: channels:1\r\nLOCATION: http://203.0.113.23/\r\nUSN: uuid:c\r\n\r\n", "203.0.113.23:1900")
	if !ok || channels.Kind != "channels-dvr" || channels.Name != "channels-dvr" || channels.Addr != "203.0.113.23" || channels.ID != "uuid:c" {
		t.Fatalf("%+v %v", channels, ok)
	}
	tvh, ok := parseSSDP("HTTP/1.1 200 OK\r\nSERVER: tvheadend\r\nUSN: uuid:t\r\n\r\n", "203.0.113.24:1900")
	if !ok || tvh.Kind != "tvheadend" || tvh.Name != "tvheadend" || tvh.Addr != "203.0.113.24" {
		t.Fatalf("%+v %v", tvh, ok)
	}
	if _, ok := parseSSDP("HTTP/1.1 200 OK\r\nSERVER: Gateway\r\n\r\n", "203.0.113.1:1900"); ok {
		t.Fatal("router")
	}
	bare, ok := parseSSDP("HTTP/1.1 200 OK\r\nSERVER: HDHomeRun\r\n\r\n", "203.0.113.10")
	if !ok || bare.Kind != "hdhomerun" || bare.Addr != "203.0.113.10" || bare.Name != "HDHomeRun" {
		t.Fatalf("%+v %v", bare, ok)
	}
}

func TestBlankServerNamesFallBack(t *testing.T) {
	plex, ok := parsePlex("HTTP/1.0 200 OK\r\nContent-Type: plex/media-server\r\nResource-Identifier: lounge\r\n\r\n", "203.0.113.20:32414")
	if !ok || plex.Kind != "plex" || plex.Name != "Plex" || plex.Addr != "203.0.113.20" || plex.ID != "lounge" {
		t.Fatalf("%+v %v", plex, ok)
	}
	if _, ok := parseMedia([]byte(`not json`), "jellyfin"); ok {
		t.Fatal("garbage")
	}
	if _, ok := parseMedia([]byte(`{"Id":"x"}`), "jellyfin"); ok {
		t.Fatal("no address or name")
	}
	jelly, ok := parseMedia([]byte(`{"Address":"http://203.0.113.21:8096","Id":"j1"}`), "jellyfin")
	if !ok || jelly.Kind != "jellyfin" || jelly.Name != "Jellyfin" || jelly.Addr != "203.0.113.21" || jelly.ID != "j1" {
		t.Fatalf("%+v %v", jelly, ok)
	}
	named, ok := parseMedia([]byte(`{"Address":"http://203.0.113.21:8096","Id":"j2","Name":"Jellyfin Lounge"}`), "emby")
	if !ok || named.Kind != "jellyfin" || named.Name != "Jellyfin Lounge" || named.Addr != "203.0.113.21" {
		t.Fatalf("%+v %v", named, ok)
	}
	films, ok := parseMedia([]byte(`{"Address":"http://","Id":"j3","Name":"Films"}`), "jellyfin")
	if !ok || films.Kind != "jellyfin" || films.Name != "Films" || films.Addr != "" {
		t.Fatalf("%+v %v", films, ok)
	}
}

func TestFinderCloseStopsAnswers(t *testing.T) {
	if err := (*Finder)(nil).Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := listenFinder(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &Finder{conn: conn, cancel: cancel}
	defer func() { _ = f.Close() }()
	go serveFinder(ctx, conn, 8477, "abc", "Lounge", nil)

	client, err := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte(finderAsk)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n < len(finderYes) || string(buf[:len(finderYes)]) != finderYes {
		t.Fatalf("reply %q", buf[:n])
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte(finderAsk)); err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	if _, err := client.Read(buf); err == nil {
		t.Fatal("closed finder answered")
	}
}

func TestPublicKeyStringMatchesTheLoadedKey(t *testing.T) {
	key, err := LoadKey(filepath.Join(t.TempDir(), "discovery.key"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(PublicKeyString(key))
	if err != nil {
		t.Fatal(err)
	}
	pub := key.Public().(ed25519.PublicKey)
	if string(raw) != string(pub) {
		t.Fatal("published key bytes")
	}
	msg := []byte("lounge")
	if !ed25519.Verify(ed25519.PublicKey(raw), msg, ed25519.Sign(key, msg)) {
		t.Fatal("published key did not verify")
	}
	if PublicKeyString(nil) != "" || PublicKeyString(ed25519.PrivateKey{1, 2, 3}) != "" {
		t.Fatal("short key")
	}
	if _, err := LoadKey(filepath.Join(t.TempDir(), "missing", "discovery.key")); err == nil {
		t.Fatal("wrote a key without a directory")
	}
}

func TestAssembleNamesTheRestOfTheHouse(t *testing.T) {
	_, n, err := net.ParseCIDR("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	found := []Found{
		{Kind: "mystery"},
		{Kind: "fridge", Name: "Fridge", Addr: "203.0.113.9", ID: "fr"},
		{Kind: "plex", Name: "Far", Addr: "127.0.0.1", ID: "far"},
		{Kind: "tv", Name: "Den TV", Addr: "203.0.113.40"},
		{Kind: "firetv", Name: "Fire TV", Addr: "203.0.113.40"},
		{Kind: "chromecast", Name: "Lounge", ID: "cc1"},
		{Kind: "airplay", Addr: "203.0.113.41"},
		{Kind: "plex", Name: "Plex", Addr: "203.0.113.2", ID: "p1"},
		{Kind: "plex", Name: "Plex", Addr: "203.0.113.3", ID: "p2"},
		{Kind: "jellyfin", Name: "Jellyfin Films", Addr: "203.0.113.4", ID: "j1"},
		{Kind: "emby", Name: "Den TV", Addr: "http://203.0.113.8:8096/web", ID: "e2"},
		{Kind: "emby", ID: "e1"},
		{Kind: "channels", Name: "Lounge", Addr: "203.0.113.5", ID: "c1"},
		{Kind: "channels", Name: "Lounge", Addr: "203.0.113.6", ID: "c2"},
		{Kind: "tvheadend", Name: "Guide", Addr: "203.0.113.15", ID: "th1"},
		{Kind: "hdhr-compatible", Name: "Lounge", Addr: "203.0.113.16", ID: "uuid:ABCD"},
		{Kind: "hdhomerun", Addr: "203.0.113.17"},
		{Kind: "hdhomerun", Addr: "203.0.113.18", ID: "NEW1"},
	}
	known := []Known{
		{ID: "ABCD", Addr: "203.0.113.16", Tuners: 2},
		{ID: "", Name: "Spare", Addr: "203.0.113.17", Tuners: 1},
		{Addr: "203.0.113.30"},
		{ID: "OLD", Name: "Attic", Addr: "203.0.113.31"},
		{ID: "ABCD", Name: "Duplicate", Addr: "203.0.113.99", Tuners: 4},
	}
	clipped := strings.Repeat("n", 64)
	screens := []Screen{
		{Kind: "iphone", Addr: "203.0.113.70"},
		{Name: "Den TV", Kind: "ipad", Addr: "http://203.0.113.71:9/app"},
		{Name: "  \x00Den\x7f  ", Kind: "appletv", Addr: "203.0.113.72"},
		{Name: "Nope", Kind: "fridge", Addr: "203.0.113.73"},
		{Name: strings.Repeat("n", 80), Kind: "web", Addr: "203.0.113.74"},
	}
	places := Assemble(found, known, screens, []*net.IPNet{n})
	want := []Place{
		{ID: "tuner|OLD", Group: "tuner", Kind: "hdhomerun", Name: "Attic", Addr: "203.0.113.31", Action: "added"},
		{ID: "tuner|th1", Group: "tuner", Kind: "tvheadend", Name: "Guide", Addr: "203.0.113.15", Action: "add"},
		{ID: "tuner|NEW1", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun", Addr: "203.0.113.18", Action: "add"},
		{ID: "tuner|203.0.113.30", Group: "tuner", Kind: "hdhomerun", Name: "HDHomeRun", Addr: "203.0.113.30", Action: "added"},
		{ID: "tuner|ABCD", Group: "tuner", Kind: "hdhomerun", Name: "Lounge", Addr: "203.0.113.16", Action: "added", Detail: "2 tuners"},
		{ID: "tuner|203.0.113.17", Group: "tuner", Kind: "hdhomerun", Name: "Spare", Addr: "203.0.113.17", Action: "added", Detail: "1 tuner"},
		{ID: "emby|den tv", Group: "server", Kind: "emby", Name: "Den TV", Addr: "203.0.113.8", Action: "use", Detail: "Emby"},
		{ID: "emby|e1", Group: "server", Kind: "emby", Name: "Emby", Action: "use"},
		{ID: "jellyfin|jellyfin films", Group: "server", Kind: "jellyfin", Name: "Jellyfin Films", Addr: "203.0.113.4", Action: "use"},
		{ID: "channels|lounge", Group: "server", Kind: "channels", Name: "Lounge", Addr: "203.0.113.5", Action: "use", Detail: "Channels"},
		{ID: "plex|203.0.113.2", Group: "server", Kind: "plex", Name: "Plex", Addr: "203.0.113.2", Action: "use"},
		{ID: "plex|203.0.113.3", Group: "server", Kind: "plex", Name: "Plex", Addr: "203.0.113.3", Action: "use"},
		{ID: "screen|airplay|203.0.113.41", Group: "screen", Kind: "airplay", Name: "AirPlay", Addr: "203.0.113.41", Action: "found"},
		{ID: "here|appletv|Den", Group: "screen", Kind: "appletv", Name: "Den", Addr: "203.0.113.72", Action: "here"},
		{ID: "here|ipad|Den TV", Group: "screen", Kind: "ipad", Name: "Den TV", Addr: "203.0.113.71", Action: "here"},
		{ID: "screen|firetv|203.0.113.40", Group: "screen", Kind: "firetv", Name: "Fire TV", Addr: "203.0.113.40", Action: "found"},
		{ID: "screen|chromecast|chromecast|Lounge", Group: "screen", Kind: "chromecast", Name: "Lounge", Action: "found"},
		{ID: "here|iphone|iPhone", Group: "screen", Kind: "iphone", Name: "iPhone", Addr: "203.0.113.70", Action: "here"},
		{ID: "here|web|" + clipped, Group: "screen", Kind: "web", Name: clipped, Addr: "203.0.113.74", Action: "here"},
	}
	if len(places) != len(want) {
		t.Fatalf("got %d places, want %d: %+v", len(places), len(want), places)
	}
	for i := range want {
		if places[i] != want[i] {
			t.Fatalf("%d\n got %+v\nwant %+v", i, places[i], want[i])
		}
	}
}

func ssdpReply(server, st, location, usn string) string {
	return "HTTP/1.1 200 OK\r\n" +
		"SERVER: " + server + "\r\n" +
		"ST: " + st + "\r\n" +
		"LOCATION: " + location + "\r\n" +
		"USN: " + usn + "\r\n" +
		"\r\n"
}

func headerValue(raw, key string) string {
	for _, line := range strings.Split(raw, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
