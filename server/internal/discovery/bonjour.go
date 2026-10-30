// Package discovery advertises the server on the local network so the iPhone
// and Apple TV apps find it without an address.
package discovery

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/libp2p/zeroconf/v2"
)

// bridgePrefixes name the interfaces of containers and virtual machines on
// the server itself. Their addresses (Docker's 172.17.0.1, libvirt's
// 192.168.122.1) reach nothing from another device, and a TV that tried one
// first stalled before it found the server.
var bridgePrefixes = []string{"docker", "br-", "veth", "virbr", "vnet", "cni", "flannel", "cali", "kube", "lxc", "lxd", "podman", "zt", "tailscale", "wg"}

func bridgeInterface(name string) bool {
	for _, prefix := range bridgePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || bridgeInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLinkLocalUnicast() && ipnet.IP.To4() == nil {
				continue
			}
			if ipnet.IP.IsGlobalUnicast() || ipnet.IP.IsPrivate() {
				out = append(out, ipnet.IP.String())
			}
		}
	}
	return out
}

const Service = "_broadwave._tcp"

type Advert struct {
	ID      string
	Name    string
	Version string
	Port    int
}

func (a Advert) txt() []string {
	return []string{
		"id=" + a.ID,
		"name=" + a.Name,
		"version=" + a.Version,
		"api=1",
		"port=" + strconv.Itoa(a.Port),
	}
}

// serviceInstance is the Bonjour name. A DNS-SD instance is one label, 63
// bytes, and the cut stays on a character.
func serviceInstance(name string) string {
	if len(name) <= 63 {
		return name
	}
	cut := 0
	for i := range name {
		if i > 63 {
			break
		}
		cut = i
	}
	return name[:cut]
}

// hostLabel is the name the advert's addresses are published under. It is
// the server's own, not the machine's: the machine's name is answered by its
// own responder too (avahi on a NAS), with every address it has, Docker's
// bridge included, and the two answers were mixed.
func hostLabel(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() == 8 {
			break
		}
	}
	if b.Len() == 0 {
		return "broadwave"
	}
	return "broadwave-" + b.String()
}

// Announce registers the service. Call Shutdown on the result to withdraw it.
func Announce(a Advert) (*zeroconf.Server, error) {
	instance := serviceInstance(a.Name)
	host := hostLabel(a.ID)
	ips := localIPs()
	if len(ips) == 0 {
		return nil, errors.New("no network address to advertise")
	}
	srv, err := zeroconf.RegisterProxy(instance, Service, "local.", a.Port, host, ips, a.txt(), nil)
	if err != nil {
		return nil, err
	}
	slog.Info(fmt.Sprintf("bonjour: %s on port %d", Service, a.Port))
	return srv, nil
}
