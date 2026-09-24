package discovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestProbeHostReadsDiscoverJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"FriendlyName":"HDHomeRun CONNECT DUO","DeviceID":"10611B4C"}`))
	}))
	defer srv.Close()
	u := srv.Listener.Addr().String()
	host, portText, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	found, ok := ProbeHost(context.Background(), host, []ProbePort{{Port: port, Path: "/discover.json", Kind: "hdhomerun"}})
	if !ok || found.ID != "10611B4C" || found.Name != "HDHomeRun CONNECT DUO" {
		t.Fatalf("%+v ok=%v", found, ok)
	}
}

func TestProbeHostIgnoresAWebPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not a tuner</html>"))
	}))
	defer srv.Close()
	host, portText, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	_, ok := ProbeHost(context.Background(), host, []ProbePort{{Port: port, Path: "/discover.json", Kind: "hdhomerun"}})
	if ok {
		t.Fatal("an ordinary page is not a tuner")
	}
}

func TestLookFindsOneHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"FriendlyName":"Desk tuner","DeviceID":"ABCDEF01"}`))
	}))
	defer srv.Close()
	host, portText, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	prev := KnownPorts
	KnownPorts = []ProbePort{{Port: port, Path: "/", Kind: "hdhomerun"}}
	defer func() { KnownPorts = prev }()
	found := Look(context.Background(), []string{host, "127.0.0.1"})
	if len(found) == 0 || found[0].ID != "ABCDEF01" {
		t.Fatalf("%+v", found)
	}
}

func TestHostsInSkipsNetworkAndBroadcast(t *testing.T) {
	_, n, err := net.ParseCIDR("192.168.9.0/30")
	if err != nil {
		t.Fatal(err)
	}
	got := HostsIn(n)
	if len(got) != 2 || got[0] != "192.168.9.1" || got[1] != "192.168.9.2" {
		t.Fatalf("%v", got)
	}
	_, wide, _ := net.ParseCIDR("10.0.0.0/16")
	if HostsIn(wide) != nil {
		t.Fatal("a network wider than /24 is not probed")
	}
}
