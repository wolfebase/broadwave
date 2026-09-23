// Package discovery advertises the server on the local network so the iPhone
// and Apple TV apps find it without an address.
package discovery

import (
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/libp2p/zeroconf/v2"
)

func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
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

const Service = "_otaviewer._tcp"

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

// Announce registers the service. Call Shutdown on the result to withdraw it.
func Announce(a Advert) (*zeroconf.Server, error) {
	instance := a.Name
	if len(instance) > 63 {
		instance = instance[:63]
	}
	host, _ := os.Hostname()
	host = strings.TrimSuffix(strings.TrimSuffix(host, "."), ".local")
	if host == "" {
		host = "ota-viewer"
	}
	ips := localIPs()
	if len(ips) == 0 {
		return nil, errors.New("no network address to advertise")
	}
	srv, err := zeroconf.RegisterProxy(instance, Service, "local.", a.Port, host, ips, a.txt(), nil)
	if err != nil {
		return nil, err
	}
	log.Printf("bonjour: %s on port %d", Service, a.Port)
	return srv, nil
}
