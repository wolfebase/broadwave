package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"waveguide/internal/store"
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
