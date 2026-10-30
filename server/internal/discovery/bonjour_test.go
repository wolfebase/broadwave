package discovery

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestServiceInstanceStopsOnACharacter(t *testing.T) {
	if got := serviceInstance(strings.Repeat("a", 70)); len(got) != 63 || got != strings.Repeat("a", 63) {
		t.Fatalf("ascii %q", got)
	}
	got := serviceInstance(strings.Repeat("é", 63))
	if len(got) > 63 || !utf8.ValidString(got) || utf8.RuneCountInString(got) != 31 {
		t.Fatalf("cut %q (%d bytes, %d runes)", got, len(got), utf8.RuneCountInString(got))
	}
	// A character that does not fit in the last bytes is left off.
	name := strings.Repeat("a", 61) + string(rune(0x1F309))
	if got = serviceInstance(name); got != strings.Repeat("a", 61) || !utf8.ValidString(got) {
		t.Fatalf("wide character %q", got)
	}
}

// Container and VM bridges are not advertised: their addresses reach nothing
// from a TV. The house network's interfaces are.
func TestBridgesAreNotAdvertised(t *testing.T) {
	for _, name := range []string{"docker0", "br-3f2a9c", "veth12ab", "virbr0", "vnet3", "cni0", "tailscale0", "wg0"} {
		if !bridgeInterface(name) {
			t.Errorf("%s is advertised", name)
		}
	}
	for _, name := range []string{"eth0", "br0", "bond0", "en0", "enp3s0", "wlan0", "shim-br0"} {
		if bridgeInterface(name) {
			t.Errorf("%s is left out", name)
		}
	}
}

// The addresses go out under the server's own name, never the machine's,
// which the machine's own responder also answers.
func TestTheHostLabelIsTheServers(t *testing.T) {
	if got := hostLabel("38760bb4994efb0f58a5865e66b056e5"); got != "broadwave-38760bb4" {
		t.Fatalf("label %q", got)
	}
	if got := hostLabel(""); got != "broadwave" {
		t.Fatalf("empty id gave %q", got)
	}
}
