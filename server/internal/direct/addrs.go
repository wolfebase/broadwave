package direct

import (
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
)

// Addr is an IP address.
type Addr = netip.Addr

var (
	cgnat = netip.MustParsePrefix("100.64.0.0/10")
	ula   = netip.MustParsePrefix("fd00::/8")
	// Docker gives bridge networks 172.17.0.0/16 through 172.31.0.0/16;
	// Podman's default network is 10.88.0.0/16.
	dockerPools = netip.MustParsePrefix("172.16.0.0/12")
	dockerSpare = netip.MustParsePrefix("172.16.0.0/16")
	podmanNet   = netip.MustParsePrefix("10.88.0.0/16")
)

// Publishable is an address the service accepts: a home network range. The
// service refuses public addresses, so a name under it is never a trusted name
// for a site on the internet.
func Publishable(a netip.Addr) bool {
	a = a.Unmap()
	if a.Is4() {
		return a.IsPrivate() || cgnat.Contains(a)
	}
	return ula.Contains(a)
}

// iface is one network interface's name and addresses.
type iface struct {
	Name  string
	Addrs []netip.Addr
}

// virtual names interfaces that a phone on the home network cannot reach:
// container bridges, VM bridges, and VPN tunnels.
func virtual(name string) bool {
	for _, p := range []string{"docker", "br-", "veth", "virbr", "vnet", "cni", "flannel", "cali", "podman", "lxc", "tun", "utun", "wg", "zt", "tailscale", "lo"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// pick chooses the addresses to publish. The interface that holds primary
// (the address the server uses for its default route) wins, with its unique
// local IPv6 addresses. A name with an address the phone cannot reach makes
// every connection wait on it first, so other interfaces are used only when
// primary is not a home address or sits on a bridge or tunnel.
func pick(primary netip.Addr, ifaces []iface) []netip.Addr {
	var out []netip.Addr
	add := func(a netip.Addr) {
		a = a.Unmap()
		if Publishable(a) && !slices.Contains(out, a) && len(out) < 4 {
			out = append(out, a)
		}
	}
	primary = primary.Unmap()
	if Publishable(primary) && primary.Is4() && !onVirtual(primary, ifaces) {
		add(primary)
		for _, f := range ifaces {
			if slices.Contains(f.Addrs, primary) {
				for _, a := range f.Addrs {
					if a.Is6() && !a.Is4In6() {
						add(a)
					}
				}
			}
		}
		return out
	}
	for _, f := range ifaces {
		if virtual(f.Name) {
			continue
		}
		for _, a := range f.Addrs {
			if a.Is4() {
				add(a)
			}
		}
	}
	return out
}

// onVirtual reports that a holds an address on a bridge or tunnel interface,
// such as a full-tunnel VPN that owns the default route.
func onVirtual(a netip.Addr, ifaces []iface) bool {
	for _, f := range ifaces {
		if slices.Contains(f.Addrs, a) {
			return virtual(f.Name)
		}
	}
	return false
}

// LocalAddrs is the home network address of this machine. On Docker host
// networking it is the host's.
func LocalAddrs() []netip.Addr {
	return pick(localInterfaces())
}

// localInterfaces is the address of the default route and every interface
// that is up, without loopback.
func localInterfaces() (netip.Addr, []iface) {
	var primary netip.Addr
	// A UDP connect sends nothing; it only picks the route.
	if c, err := net.Dial("udp4", "1.1.1.1:53"); err == nil {
		if ua, ok := c.LocalAddr().(*net.UDPAddr); ok {
			primary, _ = netip.AddrFromSlice(ua.IP)
		}
		_ = c.Close()
	}
	var list []iface
	ifs, _ := net.Interfaces()
	for _, f := range ifs {
		if f.Flags&net.FlagUp == 0 || f.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := f.Addrs()
		if err != nil {
			continue
		}
		one := iface{Name: f.Name}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				if ip, ok := netip.AddrFromSlice(n.IP); ok {
					one.Addrs = append(one.Addrs, ip.Unmap())
				}
			}
		}
		list = append(list, one)
	}
	return primary.Unmap(), list
}

// containerNet is an address a container gets on a bridge network, which
// only the host can reach.
func containerNet(a netip.Addr) bool {
	a = a.Unmap()
	return (dockerPools.Contains(a) && !dockerSpare.Contains(a)) || podmanNet.Contains(a)
}

// bridgedReason explains why the secure name cannot work here, or returns "".
// In a container on a bridge network the server only sees its own bridge
// address: phones cannot reach it, and the HTTPS port is the host's to
// publish, so a certificate would be wasted. Host networking shows the host's
// interfaces, bridges included, so a visible bridge means it is not bridged.
// A macvlan address on the home network is not a bridge address and passes.
func bridgedReason(inContainer bool, ifaces []iface, picked []netip.Addr) string {
	if !inContainer || len(picked) == 0 {
		return ""
	}
	for _, f := range ifaces {
		if strings.HasPrefix(f.Name, "docker") || strings.HasPrefix(f.Name, "br-") || strings.HasPrefix(f.Name, "veth") || strings.HasPrefix(f.Name, "podman") || strings.HasPrefix(f.Name, "cni") {
			return ""
		}
	}
	for _, a := range picked {
		if !containerNet(a) {
			return ""
		}
	}
	return "the container is on a bridge network (" + picked[0].String() + "), which phones and TVs cannot reach. Use host networking, or set BROADWAVE_DIRECT_ADDRS to the host's home address and publish port 8443"
}

// inContainer reports a Docker or Podman container.
func inContainer() bool {
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Bridged explains why this server should not ask for a secure name: it runs
// in a container on a bridge network. Empty when the name can work.
func Bridged() string {
	primary, list := localInterfaces()
	return bridgedReason(inContainer(), list, pick(primary, list))
}

// ParseAddrs reads a comma-separated override, keeping only publishable addresses.
func ParseAddrs(raw string) []netip.Addr {
	var out []netip.Addr
	for _, part := range strings.Split(raw, ",") {
		if a, err := netip.ParseAddr(strings.TrimSpace(part)); err == nil && Publishable(a) && !slices.Contains(out, a.Unmap()) {
			out = append(out, a.Unmap())
		}
	}
	return out
}
