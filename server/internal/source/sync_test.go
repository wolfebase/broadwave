package source

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestSyncAddsACompatibleDeviceByAddress(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			_, _ = w.Write([]byte(`{"FriendlyName":"ErsatzTV","ModelNumber":"HDHR","DeviceID":"emu1","BaseURL":"` + srv.URL + `","LineupURL":"` + srv.URL + `/lineup.json","TunerCount":1}`))
		case "/lineup.json":
			_, _ = w.Write([]byte(`[{"GuideNumber":"1","GuideName":"News","URL":"` + srv.URL + `/auto/v1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	host := strings.TrimPrefix(srv.URL, "http://")
	if _, err := Sync(t.Context(), st, nil, host); err != nil {
		t.Fatal(err)
	}
	devices, err := st.Devices(t.Context())
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "EMU1" {
		t.Fatalf("%+v %v", devices, err)
	}
	channels, err := st.Channels(t.Context(), false)
	if err != nil || len(channels) != 1 || channels[0].GuideName != "News" {
		t.Fatalf("%+v %v", channels, err)
	}
}

func TestBroadcastBasesIgnoresThePacketURL(t *testing.T) {
	_, nets, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	got := broadcastBases([]hdhr.Reply{{
		Addr: "192.168.1.252", BaseURL: "http://169.254.169.254/", DeviceID: "ABC",
	}}, []*net.IPNet{nets})
	if len(got) != 1 || got[0] != "http://192.168.1.252" {
		t.Fatalf("%v", got)
	}
	if off := broadcastBases([]hdhr.Reply{{Addr: "1.2.3.4", BaseURL: "http://1.2.3.4"}}, []*net.IPNet{nets}); len(off) != 0 {
		t.Fatalf("public %v", off)
	}
	if loop := broadcastBases([]hdhr.Reply{{Addr: "127.0.0.1", BaseURL: "http://evil.example"}}, nil); len(loop) != 1 || loop[0] != "http://127.0.0.1" {
		t.Fatalf("loop %v", loop)
	}
}

func TestMaintainAdoptsOnlyTheFirstTuner(t *testing.T) {
	a := tunerServer(t, "AAAA1111", "First")
	b := tunerServer(t, "BBBB2222", "Second")
	st := openStore(t)
	n, err := writeDevices(t.Context(), st, &hdhr.Client{}, []string{a, b}, false)
	if err != nil || n != 1 {
		t.Fatalf("empty catalog stored %d: %v", n, err)
	}
	n, err = writeDevices(t.Context(), st, &hdhr.Client{}, []string{a, b}, false)
	if err != nil || n != 1 {
		t.Fatalf("second pass stored %d: %v", n, err)
	}
	devices, err := st.Devices(t.Context())
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "AAAA1111" {
		t.Fatalf("%+v %v", devices, err)
	}
}

func TestSyncStillAddsTheTunerYouAskedFor(t *testing.T) {
	a := tunerServer(t, "AAAA1111", "First")
	b := tunerServer(t, "BBBB2222", "Second")
	st := openStore(t)
	if _, err := writeDevices(t.Context(), st, &hdhr.Client{}, []string{a}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDevices(t.Context(), st, &hdhr.Client{}, []string{b}, true); err != nil {
		t.Fatal(err)
	}
	devices, err := st.Devices(t.Context())
	if err != nil || len(devices) != 2 {
		t.Fatalf("%+v %v", devices, err)
	}
}

func TestMaintainSkipsABoxWithNoTuner(t *testing.T) {
	zero := tunerServerCount(t, "ZERO0000", "Empty", 0)
	real := tunerServerCount(t, "REAL2222", "Duo", 2)
	st := openStore(t)
	n, err := writeDevices(t.Context(), st, &hdhr.Client{}, []string{zero, real}, false)
	if err != nil || n != 1 {
		t.Fatalf("stored %d: %v", n, err)
	}
	devices, err := st.Devices(t.Context())
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "REAL2222" {
		t.Fatalf("%+v %v", devices, err)
	}
}

func tunerServer(t *testing.T, id, name string) string {
	t.Helper()
	return tunerServerCount(t, id, name, 2)
}

func tunerServerCount(t *testing.T, id, name string, tuners int) string {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			_, _ = w.Write([]byte(`{"FriendlyName":"` + name + `","DeviceID":"` + id + `","BaseURL":"` + srv.URL + `","LineupURL":"` + srv.URL + `/lineup.json","TunerCount":` + strconv.Itoa(tuners) + `}`))
		case "/lineup.json":
			_, _ = w.Write([]byte(`[{"GuideNumber":"4.1","GuideName":"WDAF"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
