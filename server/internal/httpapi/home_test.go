package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/discovery"
	"broadwave/internal/hdhr"
	"broadwave/internal/realtime"
	"github.com/coder/websocket"
)

func TestHomeListsTunersServersAndScreens(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "CONNECT DUO", BaseURL: "http://192.168.1.252", TunerCount: 2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bus := realtime.NewBus()
	srv := httptest.NewServer(bus)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	raw := []byte(`{"type":"here","data":{"name":"Broadwave Staging TV","kind":"appletv"}}`)
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(bus.Screens()) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(bus.Screens()) != 1 {
		t.Fatal("screen did not announce")
	}
	s := &Server{
		Store: st,
		Bus:   bus,
		HomeScan: func(context.Context) []discovery.Found {
			return []discovery.Found{
				{Kind: "hdhomerun", Name: "HDHomeRun", Addr: "192.168.1.252", ID: "10611B4C"},
				{Kind: "plex", Name: "Far away", Addr: "8.8.8.8"},
			}
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/home?fresh=1", nil)
	rec := httptest.NewRecorder()
	s.home(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Places       []discovery.Place `json:"places"`
		TunerAddress string            `json:"tunerAddress"`
		Sharing      bool              `json:"sharing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Sharing || !strings.HasSuffix(body.TunerAddress, ":8478") {
		t.Fatalf("%+v", body)
	}
	var duo, screen bool
	for _, place := range body.Places {
		if place.Addr == "8.8.8.8" {
			t.Fatal("public address was listed")
		}
		if place.ID == "tuner|10611B4C" && place.Action == "added" && place.Name == "CONNECT DUO" {
			duo = true
		}
		if place.Action == "here" && place.Kind == "appletv" && place.Name == "Broadwave Staging TV" {
			screen = true
		}
	}
	if !duo || !screen {
		t.Fatalf("missing duo or screen: %+v", body.Places)
	}
	before, _ := st.Devices(context.Background())
	req = httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	rec = httptest.NewRecorder()
	s.home(rec, req)
	after, _ := st.Devices(context.Background())
	if len(after) != len(before) {
		t.Fatal("the scan added a device")
	}
}
