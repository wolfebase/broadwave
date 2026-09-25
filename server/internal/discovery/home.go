package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"broadwave/internal/hdhr"
)

// Place is one tuner, screen, or server on the local network.
// Action is the single thing a person can do with it: add, added, use, here, or found.
type Place struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Addr   string `json:"addr,omitempty"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// Known is a tuner already in the catalog.
type Known struct {
	ID     string
	Name   string
	Addr   string
	Tuners int
}

// Screen is a client that announced itself on the event socket.
type Screen struct {
	Name string
	Kind string
	Addr string
}

// ScanHome asks the local network for tuners, screens, and servers.
// It does not open a stream and it does not add anything.
func ScanHome(ctx context.Context) []Found {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
	}
	var mu sync.Mutex
	var found []Found
	add := func(items []Found) {
		if len(items) == 0 {
			return
		}
		mu.Lock()
		found = append(found, items...)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	run(func() { add(discoverTuners(ctx)) })
	run(func() {
		items, _ := searchSSDPKinds(ctx, "", homeSearchTargets)
		add(items)
	})
	for _, service := range []struct{ name, kind string }{
		{"_googlecast._tcp", "chromecast"},
		{"_airplay._tcp", "airplay"},
		{"_channels_dvr._tcp", "channels"},
	} {
		run(func() {
			items, _ := Browse(ctx, service.name)
			for i := range items {
				items[i].Kind = service.kind
				if items[i].Name == "" {
					items[i].Name = kindLabel(service.kind)
				}
			}
			add(items)
		})
	}
	run(func() {
		items, _ := searchPlex(ctx)
		add(items)
	})
	run(func() {
		items, _ := searchMedia(ctx, "who is JellyfinServer?", "jellyfin")
		add(items)
	})
	run(func() {
		items, _ := searchMedia(ctx, "who is EmbyServer?", "emby")
		add(items)
	})
	wg.Wait()
	return found
}

func refuseHomeRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// tunerLabel reads the name the tuner calls itself. A miss stays "HDHomeRun".
// Device auth in discover.json is not mapped, so it is not returned.
func tunerLabel(ctx context.Context, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return "HDHomeRun"
	}
	dev, err := (&hdhr.Client{HTTP: &http.Client{Timeout: 400 * time.Millisecond, CheckRedirect: refuseHomeRedirect}}).FetchDevice(ctx, base)
	if err != nil {
		return "HDHomeRun"
	}
	return fallbackName(dev.FriendlyName, "hdhomerun")
}

func discoverTuners(ctx context.Context) []Found {
	timeout := 2 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left > 0 && left < timeout {
			timeout = left
		}
	}
	replies, err := hdhr.Discover(timeout)
	if err != nil {
		return nil
	}
	var out []Found
	nets := LocalNets()
	for _, reply := range replies {
		addr := hostOnly(reply.Addr)
		ip := net.ParseIP(addr)
		if reply.DeviceID == "" && addr == "" {
			continue
		}
		name := "HDHomeRun"
		// The packet's BaseURL is not fetched. Only the sender, and only on this LAN.
		if ip != nil && ip.To4() != nil && (ip.IsLoopback() || OnLAN(addr, nets)) {
			nameCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
			name = tunerLabel(nameCtx, "http://"+addr)
			cancel()
		}
		out = append(out, Found{Kind: "hdhomerun", Name: name, Addr: addr, ID: reply.DeviceID})
	}
	return out
}

var homeSearchTargets = []string{
	"upnp:rootdevice",
	"urn:dial-multiscreen-org:service:dial:1",
	"urn:schemas-upnp-org:device:MediaRenderer:1",
}

func searchSSDPKinds(ctx context.Context, dest string, targets []string) ([]Found, error) {
	if dest == "" {
		dest = "239.255.255.250:1900"
	}
	if !strings.Contains(dest, ":") {
		dest += ":1900"
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("ssdp socket is not udp")
	}
	remote, err := net.ResolveUDPAddr("udp4", dest)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = udp.SetDeadline(deadline)
	for _, st := range targets {
		packet := "M-SEARCH * HTTP/1.1\r\n" +
			"HOST: 239.255.255.250:1900\r\n" +
			"MAN: \"ssdp:discover\"\r\n" +
			"MX: 1\r\n" +
			"ST: " + st + "\r\n" +
			"\r\n"
		if _, err := udp.WriteToUDP([]byte(packet), remote); err != nil {
			return nil, err
		}
	}
	seen := map[string]Found{}
	buf := make([]byte, 4096)
	for {
		n, addr, err := udp.ReadFrom(buf)
		if err != nil {
			break
		}
		item, ok := classifySSDP(string(buf[:n]), addr.String())
		if !ok {
			continue
		}
		seen[item.Kind+"|"+item.Addr+"|"+item.ID] = item
	}
	out := make([]Found, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	return out, nil
}

// classifySSDP keeps tuners, servers, and screens. Printers and routers are ignored.
func classifySSDP(raw, from string) (Found, bool) {
	header := ssdpHeaders(raw)
	server := header["server"]
	st := header["st"]
	location := header["location"]
	blob := strings.ToLower(server + " " + st + " " + location + " " + header["usn"])
	kind := ""
	switch {
	case strings.Contains(blob, "hdhomerun"):
		kind = "hdhomerun"
	case strings.Contains(blob, "plex"):
		kind = "plex"
	case strings.Contains(blob, "jellyfin"):
		kind = "jellyfin"
	case strings.Contains(blob, "emby"):
		kind = "emby"
	case strings.Contains(blob, "channels"):
		kind = "channels"
	case strings.Contains(blob, "tvheadend"):
		kind = "tvheadend"
	case strings.Contains(blob, "dial"):
		if strings.Contains(blob, "fire") || strings.Contains(blob, "amazon") {
			kind = "firetv"
		} else {
			kind = "androidtv"
		}
	case strings.Contains(strings.ToLower(st), "mediarenderer"):
		kind = "tv"
	default:
		return Found{}, false
	}
	addr := hostOnly(from)
	if location != "" {
		if u, err := url.Parse(location); err == nil && u.Hostname() != "" {
			addr = u.Hostname()
		}
	}
	return Found{Kind: kind, Name: displayName(kind, server), Addr: addr, ID: header["usn"]}, true
}

func ssdpHeaders(raw string) map[string]string {
	header := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		header[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
	}
	return header
}

func searchPlex(ctx context.Context) ([]Found, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("plex socket is not udp")
	}
	remote, err := net.ResolveUDPAddr("udp4", "255.255.255.255:32414")
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = udp.SetDeadline(deadline)
	enableBroadcast(udp)
	if _, err := udp.WriteToUDP([]byte("M-SEARCH * HTTP/1.1\r\n\r\n"), remote); err != nil {
		return nil, err
	}
	seen := map[string]Found{}
	buf := make([]byte, 4096)
	for {
		n, addr, err := udp.ReadFrom(buf)
		if err != nil {
			break
		}
		item, ok := parsePlex(string(buf[:n]), addr.String())
		if !ok {
			continue
		}
		seen[item.ID+"|"+item.Addr] = item
	}
	out := make([]Found, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	return out, nil
}

func parsePlex(raw, from string) (Found, bool) {
	header := ssdpHeaders(raw)
	if !strings.Contains(strings.ToLower(header["content-type"]+" "+raw), "plex") {
		return Found{}, false
	}
	name := header["name"]
	if name == "" {
		name = "Plex"
	}
	return Found{Kind: "plex", Name: cleanLabel(name), Addr: hostOnly(from), ID: header["resource-identifier"]}, true
}

func searchMedia(ctx context.Context, payload, kind string) ([]Found, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("media socket is not udp")
	}
	remote, err := net.ResolveUDPAddr("udp4", "255.255.255.255:7359")
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = udp.SetDeadline(deadline)
	enableBroadcast(udp)
	if _, err := udp.WriteToUDP([]byte(payload), remote); err != nil {
		return nil, err
	}
	seen := map[string]Found{}
	buf := make([]byte, 4096)
	for {
		n, _, err := udp.ReadFrom(buf)
		if err != nil {
			break
		}
		item, ok := parseMedia(buf[:n], kind)
		if !ok {
			continue
		}
		seen[item.ID+"|"+item.Addr] = item
	}
	out := make([]Found, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	return out, nil
}

func parseMedia(body []byte, kind string) (Found, bool) {
	var doc struct {
		Address string `json:"Address"`
		ID      string `json:"Id"`
		Name    string `json:"Name"`
	}
	if json.Unmarshal(body, &doc) != nil || (doc.Address == "" && doc.Name == "") {
		return Found{}, false
	}
	addr := ""
	if u, err := url.Parse(doc.Address); err == nil {
		addr = u.Hostname()
	}
	name := doc.Name
	if name == "" {
		name = kindLabel(kind)
	}
	got := kind
	if strings.Contains(strings.ToLower(name), "emby") {
		got = "emby"
	} else if strings.Contains(strings.ToLower(name), "jellyfin") {
		got = "jellyfin"
	}
	return Found{Kind: got, Name: cleanLabel(name), Addr: addr, ID: doc.ID}, true
}

// LocalNets returns the IPv4 subnets on interfaces that are up, skipping loopback.
func LocalNets() []*net.IPNet {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var nets []*net.IPNet
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			n := *ipnet
			n.IP = ipnet.IP.Mask(ipnet.Mask)
			nets = append(nets, &n)
		}
	}
	return nets
}

// OnLAN reports whether host is an IPv4 address inside one of the given subnets.
func OnLAN(host string, nets []*net.IPNet) bool {
	ip := net.ParseIP(hostOnly(host))
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Assemble builds the home list. Network results stay on the given subnets.
// Announced screens are whoever connected to this server; the probes never leave the LAN.
func Assemble(found []Found, known []Known, screens []Screen, nets []*net.IPNet) []Place {
	seenTuner := map[string]bool{}
	var places []Place
	bestScreen := map[string]Found{}
	for _, item := range found {
		if item.Addr != "" && !OnLAN(item.Addr, nets) {
			continue
		}
		if item.Addr == "" && item.ID == "" {
			continue
		}
		switch groupOf(item.Kind) {
		case "screen":
			key := item.Addr
			if key == "" {
				key = item.Kind + "|" + item.Name
			}
			prev, ok := bestScreen[key]
			if !ok || screenRank(item.Kind) > screenRank(prev.Kind) {
				bestScreen[key] = item
			}
		case "tuner":
			place := tunerPlace(item, known)
			if place.ID == "" || seenTuner[place.ID] || (place.Addr != "" && seenTuner["addr|"+place.Kind+"|"+place.Addr]) {
				continue
			}
			seenTuner[place.ID] = true
			if place.Addr != "" {
				seenTuner["addr|"+place.Kind+"|"+place.Addr] = true
			}
			places = append(places, place)
		case "server":
			addr := hostOnly(item.Addr)
			name := fallbackName(item.Name, item.Kind)
			// The same server can answer on the LAN and on a bridge address.
			// A distinctive name is one device; a generic name stays per address.
			id := item.Kind + "|" + addr
			if addr == "" {
				id = item.Kind + "|" + item.ID
			}
			if !strings.EqualFold(name, kindLabel(item.Kind)) {
				id = item.Kind + "|" + strings.ToLower(name)
			}
			detail := ""
			if !strings.Contains(strings.ToLower(name), strings.ToLower(kindLabel(item.Kind))) {
				detail = kindLabel(item.Kind)
			}
			places = append(places, Place{
				ID: id, Group: "server", Kind: item.Kind, Name: name, Addr: addr, Action: "use", Detail: detail,
			})
		}
	}
	for _, item := range bestScreen {
		key := item.Addr
		if key == "" {
			key = item.Kind + "|" + item.Name
		}
		places = append(places, Place{
			ID: "screen|" + item.Kind + "|" + key, Group: "screen", Kind: item.Kind,
			Name: fallbackName(item.Name, item.Kind), Addr: item.Addr, Action: "found",
		})
	}
	for _, item := range known {
		id := "tuner|" + item.ID
		if item.ID == "" {
			id = "tuner|" + hostOnly(item.Addr)
		}
		if seenTuner[id] || (item.Addr != "" && seenTuner["addr|hdhomerun|"+hostOnly(item.Addr)]) {
			continue
		}
		name := item.Name
		if name == "" {
			name = "HDHomeRun"
		}
		detail := ""
		if item.Tuners > 0 {
			detail = tunerDetail(item.Tuners)
		}
		places = append(places, Place{
			ID: id, Group: "tuner", Kind: "hdhomerun", Name: name, Addr: hostOnly(item.Addr), Action: "added", Detail: detail,
		})
		seenTuner[id] = true
	}
	for _, screen := range screens {
		kind := strings.ToLower(strings.TrimSpace(screen.Kind))
		switch kind {
		case "iphone", "ipad", "appletv", "web":
		default:
			continue
		}
		name := cleanLabel(screen.Name)
		if name == "" {
			name = kindLabel(kind)
		}
		places = append(places, Place{
			ID: "here|" + kind + "|" + name, Group: "screen", Kind: kind, Name: name, Addr: hostOnly(screen.Addr), Action: "here",
		})
	}
	places = dedupe(places)
	sort.Slice(places, func(i, j int) bool {
		if groupOrder(places[i].Group) != groupOrder(places[j].Group) {
			return groupOrder(places[i].Group) < groupOrder(places[j].Group)
		}
		if places[i].Name != places[j].Name {
			return places[i].Name < places[j].Name
		}
		return places[i].Addr < places[j].Addr
	})
	return places
}

func dedupe(places []Place) []Place {
	seen := map[string]bool{}
	var out []Place
	for _, place := range places {
		if seen[place.ID] {
			continue
		}
		seen[place.ID] = true
		out = append(out, place)
	}
	return out
}

func tunerPlace(item Found, known []Known) Place {
	for _, one := range known {
		if sameTuner(item.Kind, item.ID, item.Addr, one.ID, one.Addr) {
			name := one.Name
			if name == "" {
				name = fallbackName(item.Name, "hdhomerun")
			}
			id := "tuner|" + one.ID
			if one.ID == "" {
				id = "tuner|" + hostOnly(item.Addr)
			}
			detail := ""
			if one.Tuners > 0 {
				detail = tunerDetail(one.Tuners)
			}
			return Place{ID: id, Group: "tuner", Kind: "hdhomerun", Name: name, Addr: hostOnly(item.Addr), Action: "added", Detail: detail}
		}
	}
	id := "tuner|" + item.ID
	if item.ID == "" {
		id = "tuner|" + hostOnly(item.Addr)
	}
	return Place{ID: id, Group: "tuner", Kind: item.Kind, Name: fallbackName(item.Name, item.Kind), Addr: hostOnly(item.Addr), Action: "add"}
}

func sameTuner(kind, id, addr, knownID, knownAddr string) bool {
	if kind != "hdhomerun" && kind != "hdhr-compatible" {
		return false
	}
	id = strings.ToLower(id)
	knownID = strings.ToLower(knownID)
	if id != "" && knownID != "" && (id == knownID || strings.Contains(id, knownID) || strings.Contains(knownID, id)) {
		return true
	}
	a := hostOnly(addr)
	b := hostOnly(knownAddr)
	return a != "" && a == b
}

func groupOf(kind string) string {
	switch kind {
	case "hdhomerun", "hdhr-compatible", "tvheadend":
		return "tuner"
	case "plex", "jellyfin", "emby", "channels", "channels-dvr":
		return "server"
	case "chromecast", "airplay", "firetv", "androidtv", "tv", "iphone", "ipad", "appletv", "web":
		return "screen"
	default:
		return ""
	}
}

func screenRank(kind string) int {
	switch kind {
	case "firetv", "androidtv", "chromecast":
		return 3
	case "airplay":
		return 2
	default:
		return 1
	}
}

func groupOrder(group string) int {
	switch group {
	case "tuner":
		return 0
	case "server":
		return 1
	default:
		return 2
	}
}

func tunerDetail(n int) string {
	if n == 1 {
		return "1 tuner"
	}
	return strconv.Itoa(n) + " tuners"
}

func displayName(kind, server string) string {
	name := cleanLabel(server)
	lower := strings.ToLower(name)
	if name == "" || strings.Contains(lower, "linux") || strings.Contains(lower, "upnp/") || strings.Contains(lower, "dial") {
		return kindLabel(kind)
	}
	return name
}

func fallbackName(name, kind string) string {
	name = cleanLabel(name)
	if name == "" {
		return kindLabel(kind)
	}
	return name
}

func kindLabel(kind string) string {
	switch kind {
	case "hdhomerun":
		return "HDHomeRun"
	case "plex":
		return "Plex"
	case "jellyfin":
		return "Jellyfin"
	case "emby":
		return "Emby"
	case "channels", "channels-dvr":
		return "Channels"
	case "tvheadend":
		return "tvheadend"
	case "chromecast":
		return "Chromecast"
	case "airplay":
		return "AirPlay"
	case "firetv":
		return "Fire TV"
	case "androidtv":
		return "Android TV"
	case "tv":
		return "TV"
	case "iphone":
		return "iPhone"
	case "ipad":
		return "iPad"
	case "appletv":
		return "Apple TV"
	case "web":
		return "This browser"
	default:
		return kind
	}
}

func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
		n++
		if n >= 64 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func enableBroadcast(udp *net.UDPConn) {
	raw, err := udp.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
}

func hostOnly(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if u, err := url.Parse(addr); err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Hostname()
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
