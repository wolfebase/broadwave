package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCastPageIsTheReceiver(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	page := get(t, h, "/cast")
	body := page.Body.String()
	if ct := page.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("type %q", ct)
	}
	if page.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("cache %q", page.Header().Get("Cache-Control"))
	}
	if !strings.Contains(body, "https://www.gstatic.com/cast/sdk/libs/caf_receiver/v3/cast_receiver_framework.js") {
		t.Fatalf("missing receiver SDK: %s", body)
	}
	if !strings.Contains(body, `src="/cast/receiver.js"`) {
		t.Fatalf("missing receiver script: %s", body)
	}
	if strings.Contains(body, "cast_sender") || strings.Contains(body, "CastReceiverContext") {
		t.Fatal("receiver page vendors the SDK")
	}
	if page.Body.Len() > 8000 {
		t.Fatalf("page is %d bytes", page.Body.Len())
	}
	if !strings.Contains(body, "This page plays on a Cast device.") {
		t.Fatal(body)
	}
}

func TestCastPageIsNotTheWebApp(t *testing.T) {
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("SPA")}}
	h := (&Server{Store: testStore(t), Assets: assets}).Handler()
	page := get(t, h, "/cast")
	if strings.Contains(page.Body.String(), "SPA") {
		t.Fatal("cast served the web app")
	}
	home := get(t, h, "/")
	if !strings.Contains(home.Body.String(), "SPA") {
		t.Fatal("home did not serve the web app")
	}
}

func TestCastPageStaysOpenWhenDeviceSignInIsOn(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	on := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	if on.Code != http.StatusOK {
		t.Fatalf("enable %d %s", on.Code, on.Body.String())
	}
	page := call(t, h, http.MethodGet, "/cast", "", "")
	if page.Code != http.StatusOK {
		t.Fatalf("cast %d %s", page.Code, page.Body.String())
	}
	channels := call(t, h, http.MethodGet, "/api/v1/channels", "", "")
	if channels.Code != http.StatusUnauthorized {
		t.Fatalf("channels %d %s", channels.Code, channels.Body.String())
	}
}

func TestCastAppIDPublishedWhenValid(t *testing.T) {
	st := testStore(t)
	empty := get(t, (&Server{Store: st}).Handler(), "/api/v1/server")
	if strings.Contains(empty.Body.String(), "castAppId") {
		t.Fatalf("empty id published: %s", empty.Body.String())
	}
	set := get(t, (&Server{Store: st, CastAppID: "  4F8C2A1B  "}).Handler(), "/api/v1/server")
	if !strings.Contains(set.Body.String(), `"castAppId":"4F8C2A1B"`) {
		t.Fatalf("trimmed id missing: %s", set.Body.String())
	}
	if strings.Contains(set.Body.String(), `"cast"`) {
		t.Fatalf("cast is not a feature flag: %s", set.Body.String())
	}
	for _, bad := range []string{" bad ", "ab", "has space", "../../x", "CC1AD845!"} {
		body := get(t, (&Server{Store: st, CastAppID: bad}).Handler(), "/api/v1/server").Body.String()
		if strings.Contains(body, "castAppId") {
			t.Fatalf("%q published: %s", bad, body)
		}
	}
	for _, good := range []string{"CC1AD845", "abcd1234"} {
		body := get(t, (&Server{Store: st, CastAppID: good}).Handler(), "/api/v1/server").Body.String()
		if !strings.Contains(body, `"castAppId":"`+good+`"`) {
			t.Fatalf("%q omitted: %s", good, body)
		}
	}
}
