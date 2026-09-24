package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
	"broadwave/internal/realtime"
	"broadwave/internal/sports"
	"broadwave/internal/store"
)

// contractNow is the clock every golden response is recorded against.
var contractNow = time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)

func TestContractFixtures(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := testStore(t)
	fakeTuner := &fake.Server{}
	base, _, err := fakeTuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fakeTuner.Close)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIdentity(ctx, "contract-server", "Home", contractNow); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"setupComplete": "1"}); err != nil {
		t.Fatal(err)
	}
	start := contractNow
	end := start.Add(time.Hour)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: 1, Title: "Jeopardy!", Subtitle: "Semifinals", Category: "Game show", Start: start, End: end, GuideSource: "broadcast"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Jeopardy!", 1, 1, 2); err != nil {
		t.Fatal(err)
	}
	recID, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 1, GuideNumber: "4.1", Title: "Jeopardy!",
		Path: filepath.Join(dir, "jeopardy.ts"), Status: "complete",
		StartedAt: start, EndedAt: &end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMarker(ctx, recID, 12, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVirtual(ctx, "9000", "Jeopardy", []int64{recID}); err != nil {
		t.Fatal(err)
	}
	if err := st.FollowTeam(ctx, store.TeamFollow{Name: "Chiefs", Abbr: "KC", League: "nfl", Record: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddEventAt(ctx, contractNow, "guide", "Listings are in."); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFrequencySignal(ctx, 593000000, true, 90, 88, 100, contractNow); err != nil {
		t.Fatal(err)
	}

	bus := realtime.NewBus()
	bus.SetClock(func() time.Time { return contractNow })
	api := &Server{
		Store:   st,
		HDHR:    client,
		Hub:     &live.Hub{Store: st, Dir: dir, Encoder: "libx264"},
		Bus:     bus,
		Sports:  contractSports{},
		Version: "dev",
		Clock:   func() time.Time { return contractNow },
	}
	h := api.Handler()
	root := fixtureDir(t)

	cases := []struct {
		name, method, path, body string
	}{
		{"health", "GET", "/api/v1/health", ""},
		{"server", "GET", "/api/v1/server", ""},
		{"clock", "GET", "/api/v1/clock", ""},
		{"profile", "GET", "/api/v1/profile", ""},
		{"devices", "GET", "/api/v1/devices", ""},
		{"sources", "GET", "/api/v1/sources", ""},
		{"channels", "GET", "/api/v1/channels?guide=1", ""},
		{"airings", "GET", "/api/v1/airings?from=2026-09-24T14:00:00Z&to=2026-09-24T18:00:00Z", ""},
		{"schedule", "GET", "/api/v1/schedule", ""},
		{"search", "GET", "/api/v1/search?q=Jeopardy", ""},
		{"scoreboard", "GET", "/api/v1/sports/scoreboard?date=2026-09-24", ""},
		{"signals", "GET", "/api/v1/signals", ""},
		{"tuners", "GET", "/api/v1/tuners", ""},
		{"recordings", "GET", "/api/v1/recordings", ""},
		{"markers", "GET", "/api/v1/recordings/1/markers", ""},
		{"passes", "GET", "/api/v1/passes", ""},
		{"teams", "GET", "/api/v1/teams", ""},
		{"events", "GET", "/api/v1/events", ""},
		{"virtuals", "GET", "/api/v1/virtuals", ""},
		{"virtual-schedule", "GET", "/api/v1/virtuals/schedule", ""},
		{"settings", "GET", "/api/v1/settings", ""},
		{"storage", "GET", "/api/v1/storage", ""},
		{"diagnostics", "GET", "/api/v1/diagnostics", ""},
		{"multiview", "POST", "/api/v1/multiview/plan", `{"channelIds":[1,2]}`},
		{"channel", "PATCH", "/api/v1/channels/1", `{"favorite":true,"customName":"Fox 4"}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		matchFixture(t, root, tc.name, rec.Body.Bytes())
	}
	matchSocket(t, h, root)
}

func matchSocket(t *testing.T, h http.Handler, root string) {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	read := func(kind string) []byte {
		t.Helper()
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for %s: %v", kind, err)
			}
			var m realtime.Message
			if json.Unmarshal(raw, &m) != nil || m.Type != kind {
				continue
			}
			return raw
		}
	}
	send := func(kind string, v any) {
		t.Helper()
		data, _ := json.Marshal(v)
		raw, _ := json.Marshal(realtime.Message{Type: kind, Data: data})
		if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatal(err)
		}
	}
	matchFixture(t, root, "ws-hello", read("hello"))
	send("clock", map[string]float64{"t0": 1000})
	matchFixture(t, root, "ws-clock", read("clock"))
	send("sync.join", map[string]any{"room": "channel:1", "channelId": 1})
	matchFixture(t, root, "ws-sync", read("sync.state"))
}

func matchFixture(t *testing.T, root, name string, raw []byte) {
	t.Helper()
	got := canonical(t, raw)
	path := filepath.Join(root, name+".json")
	if os.Getenv("CONTRACT_UPDATE") == "1" {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing %s (CONTRACT_UPDATE=1 to record): %v", path, err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("%s drifted\n got %s\nwant %s", name, got, want)
	}
}

func canonical(t *testing.T, raw []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not json: %v %s", err, raw)
	}
	scrub(v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n')
}

var loopbackPort = regexp.MustCompile(`127\.0\.0\.1:\d+`)

func scrub(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch k {
			case "lastSeen":
				t[k] = "2026-09-24T15:00:00Z"
			case "doctor":
				t[k] = []any{}
			case "os":
				t[k] = "test"
			case "freeBytes", "totalBytes", "Free", "Total":
				t[k] = float64(0)
			case "ffmpeg":
				t[k] = "test"
			default:
				if s, ok := val.(string); ok {
					t[k] = loopbackPort.ReplaceAllString(s, "127.0.0.1:9")
				} else {
					scrub(val)
				}
			}
		}
	case []any:
		for i := range t {
			scrub(t[i])
		}
	}
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "api", "fixtures"))
}

type contractSports struct{}

func (contractSports) Scoreboard(context.Context, string, time.Time) ([]sports.Game, error) {
	return contractGames(), nil
}

func (contractSports) Boards(context.Context, time.Time) ([]sports.Game, error) {
	return contractGames(), nil
}

func contractGames() []sports.Game {
	return []sports.Game{{
		ID: "nfl-1", League: "nfl", Name: "Chiefs at Bills", ShortName: "KC @ BUF",
		Start: contractNow, State: "pre", Detail: "Sun 1:00 PM",
		Teams: []sports.Team{
			{Name: "Chiefs", Abbr: "KC", Home: true},
			{Name: "Bills", Abbr: "BUF"},
		},
	}}
}
