package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/realtime"
	"github.com/coder/websocket"
)

func TestAScreenPlaysWhatAnotherSendsIt(t *testing.T) {
	st := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dev := hdhr.Device{DeviceID: "1234ABCD", FriendlyName: "Tuner", BaseURL: "http://tuner.invalid", TunerCount: 2}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV", StreamURL: "http://tuner.invalid/auto/v4.1"}}); err != nil {
		t.Fatal(err)
	}
	channel := guideID(t, st, "4.1")
	bus := realtime.NewBus()
	ws := httptest.NewServer(bus)
	defer ws.Close()
	dial := func(here string) *websocket.Conn {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ws.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		raw, _ := json.Marshal(realtime.Message{Type: "here", Data: json.RawMessage(here)})
		if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	tv := dial(`{"id":"den-tv-1","name":"Den TV","kind":"appletv"}`)
	phone := dial(`{"id":"phone-1","name":"Sam's iPhone","kind":"iphone"}`)
	join, _ := json.Marshal(realtime.Message{Type: "sync.join", Data: json.RawMessage(`{"room":"channel:` + strconv.FormatInt(channel, 10) + `","channelId":` + strconv.FormatInt(channel, 10) + `}`)})
	if err := phone.Write(ctx, websocket.MessageText, join); err != nil {
		t.Fatal(err)
	}
	// An app from before screen ids is listed nowhere a sender can pick it.
	dial(`{"name":"Old TV","kind":"appletv"}`)
	h := (&Server{Store: st, Bus: bus}).Handler()

	var list struct {
		Screens []Screen `json:"screens"`
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/screens", nil))
		list.Screens = nil
		_ = json.Unmarshal(rec.Body.Bytes(), &list)
		if len(list.Screens) == 2 && (list.Screens[0].ChannelID != 0 || list.Screens[1].ChannelID != 0) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	byID := map[string]Screen{}
	for _, s := range list.Screens {
		byID[s.ID] = s
	}
	if len(list.Screens) != 2 || byID["den-tv-1"].Name != "Den TV" || byID["phone-1"].ChannelID != channel || byID["den-tv-1"].ChannelID != 0 {
		t.Fatalf("screens: %+v", list.Screens)
	}

	send := func(id, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/screens/"+id+"/watch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send("den-tv-1", `{"channelId":`+strconv.FormatInt(channel, 10)+`,"from":"Sam's iPhone\u0007"}`); code != http.StatusAccepted {
		t.Fatalf("send: %d", code)
	}
	for {
		_, raw, err := tv.Read(ctx)
		if err != nil {
			t.Fatalf("the TV never heard it: %v", err)
		}
		var m realtime.Message
		_ = json.Unmarshal(raw, &m)
		if m.Type != "screen.watch" {
			continue
		}
		var got struct {
			ChannelID int64  `json:"channelId"`
			From      string `json:"from"`
		}
		if json.Unmarshal(m.Data, &got) != nil || got.ChannelID != channel || got.From != "Sam's iPhone" {
			t.Fatalf("screen.watch: %s", m.Data)
		}
		break
	}
	if code := send("gone-tv", `{"channelId":`+strconv.FormatInt(channel, 10)+`}`); code != http.StatusNotFound {
		t.Fatalf("a screen that isn't open: %d", code)
	}
	if code := send("den-tv-1", `{"channelId":999999}`); code != http.StatusNotFound {
		t.Fatalf("a channel that doesn't exist: %d", code)
	}
	if code := send("den-tv-1", `{}`); code != http.StatusBadRequest {
		t.Fatalf("no channel: %d", code)
	}
}
