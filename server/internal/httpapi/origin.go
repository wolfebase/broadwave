package httpapi

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/publicsuffix"
)

// homeDomains are names routers hand out under a public top-level domain.
// Their owners do not sell names under them, so nobody else can point one at a
// house.
var homeDomains = []string{"home.arpa", "fritz.box", "attlocal.net"}

// testHosts holds httptest's example.com, which tests use without a port.
var testHosts []string

var refused struct {
	sync.Mutex
	names map[string]bool
}

// withHostCheck refuses a request whose Host is a public name the server was
// not told about. A page on such a name that resolves to this server's address
// (DNS rebinding) would otherwise read and change the catalog as if it were
// the web app. A browser behind an HTTPS proxy passes; any other public name
// must be listed in Hosts.
func (s *Server) withHostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.hostAllowed(r) {
			next.ServeHTTP(w, r)
			return
		}
		name, _, _ := splitHost(r.Host)
		if len(name) > 253 {
			name = name[:253]
		}
		logRefused(name)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusMisdirectedRequest)
		fmt.Fprintf(w, "This server does not answer to %s. If you reach it by that name, set BROADWAVE_HOSTS=%s on the server.\n", name, name)
	})
}

// logRefused names each refused host once, and at most 16 of them, since
// anyone can send any Host.
func logRefused(name string) {
	refused.Lock()
	defer refused.Unlock()
	if refused.names == nil {
		refused.names = map[string]bool{}
	}
	if refused.names[name] || len(refused.names) >= 16 {
		return
	}
	refused.names[name] = true
	slog.Warn(fmt.Sprintf("refused a request for %q; set BROADWAVE_HOSTS if the server is reached by that name", name))
}

func (s *Server) hostAllowed(r *http.Request) bool {
	name, port, ok := splitHost(r.Host)
	if !ok {
		return false
	}
	if name == "" || localName(name) {
		return true
	}
	for _, h := range append(testHosts, s.Hosts...) {
		h = strings.ToLower(strings.TrimRight(strings.TrimSpace(h), "."))
		if h != "" && (name == h || strings.HasPrefix(h, ".") && strings.HasSuffix(name, h)) {
			return true
		}
	}
	// Browsers send Sec-Fetch-Site only over HTTPS, and a script cannot set
	// it. A rebinding page cannot do TLS for its name against this server, so
	// the header on a request for the standard port means a proxy with a real
	// certificate is in front.
	return (port == "" || port == "443") && r.Header.Get("Sec-Fetch-Site") != ""
}

// splitHost lowercases the name and drops a trailing dot. ok is false for a
// Host no browser sends, such as two ports or characters outside a name.
func splitHost(hostport string) (name, port string, ok bool) {
	name = hostport
	switch {
	case strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]"):
		name = hostport[1 : len(hostport)-1]
	case strings.Contains(hostport, ":"):
		h, p, err := net.SplitHostPort(hostport)
		if err != nil {
			return strings.ToLower(hostport), "", false
		}
		name, port = h, p
	}
	name = strings.ToLower(strings.TrimRight(name, "."))
	if _, err := netip.ParseAddr(name); err == nil {
		return name, port, true
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
			return name, port, false
		}
	}
	return name, port, true
}

// localName reports names that only a home network can resolve: addresses,
// single labels, mDNS, and top-level domains that are not on the public
// internet (.lan, .home, .internal).
func localName(name string) bool {
	if _, err := netip.ParseAddr(name); err == nil {
		return true
	}
	if !strings.Contains(name, ".") {
		return true
	}
	for _, d := range homeDomains {
		if name == d || strings.HasSuffix(name, "."+d) {
			return true
		}
	}
	// The whole name, not its last label: some country domains are listed only
	// as wildcards (*.np), so "np" alone reads as unlisted.
	suffix, icann := publicsuffix.PublicSuffix(name)
	return !icann && !strings.Contains(suffix, ".")
}

// crossSite reports a browser request made by a page on another site. The
// socket needs it because a WebSocket upgrade is a GET, which the cross-origin
// protection lets through. Apps and curl send neither header.
func crossSite(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "":
	default:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	return err != nil || !strings.EqualFold(u.Host, r.Host)
}
