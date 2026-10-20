package discovery

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

func TestBrowseReadsALocalService(t *testing.T) {
	if reason := noMulticastRoute(); reason != "" {
		t.Skip(reason)
	}
	server, err := zeroconf.Register("BroadwaveFake", "_wgfind._tcp", "local.", 9, []string{"id=fake"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	found, err := Browse(ctx, "_wgfind._tcp")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range found {
		if item.Name == "BroadwaveFake" {
			return
		}
	}
	t.Fatalf("missing local service: %+v", found)
}

// noMulticastRoute explains why this host cannot loop mDNS back to itself,
// or returns "". A VPN that claims 224.0.0.251 sends the query down a
// point-to-point tunnel, so the local responder never hears it.
func noMulticastRoute() string {
	conn, err := net.Dial("udp4", "224.0.0.251:5353")
	if err != nil {
		return "no route to 224.0.0.251: " + err.Error()
	}
	local := conn.LocalAddr().(*net.UDPAddr).IP
	conn.Close()
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifi := range ifaces {
		addrs, _ := ifi.Addrs()
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || !ipnet.IP.Equal(local) {
				continue
			}
			if ifi.Flags&net.FlagPointToPoint != 0 {
				return "224.0.0.251 routes through point-to-point " + ifi.Name + " (a VPN?), so mDNS cannot reach this host"
			}
			if ifi.Flags&net.FlagMulticast == 0 {
				return "224.0.0.251 routes through " + ifi.Name + ", which has no multicast"
			}
			return ""
		}
	}
	return ""
}

func TestUnescapeDNSDecodesDecimalEscapes(t *testing.T) {
	cases := map[string]string{
		`Plain Name`:                    "Plain Name",
		`Living\032Room`:                "Living Room",
		`Alex\226\128\153s MacBook Air`: "Alex’s MacBook Air",
		`Den\.TV`:                       "Den.TV",
		`Back\\slash`:                   `Back\slash`,
		`Caf\195\169`:                   "Café",
		`Odd\999`:                       "Odd999",
		`Short\12`:                      "Short12",
		`Broken\255Byte`:                "Broken�Byte",
		`Trailing\`:                     `Trailing\`,
	}
	for in, want := range cases {
		if got := unescapeDNS(in); got != want {
			t.Errorf("unescapeDNS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFriendlyNameUnescapesInstanceAndLabel(t *testing.T) {
	entry := &zeroconf.ServiceEntry{}
	entry.Instance = `Alex\226\128\153s iPad`
	if got := friendlyName(entry); got != "Alex’s iPad" {
		t.Fatalf("instance: %q", got)
	}
	entry.Text = []string{`fn=Kitchen\032TV`}
	if got := friendlyName(entry); got != "Kitchen TV" {
		t.Fatalf("fn label: %q", got)
	}
}
