package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"broadwave/internal/discovery"
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
	base, control, err := fakeTuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fakeTuner.Close)
	// Probe the fake control port, not a tuner that might be on 65001.
	t.Setenv("HDHR_CONTROL_PORT", control)
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
	st.OnEvent = func(ev store.Event) { bus.Publish("activity", ev) }
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	feeds := httptest.NewServer(contractFeeds())
	t.Cleanup(feeds.Close)
	api := &Server{
		Store:   st,
		HDHR:    client,
		Hub:     &live.Hub{Store: st, Dir: dir, Encoder: "libx264"},
		Bus:     bus,
		Sports:  contractSports{},
		Version: "dev",
		Clock:   func() time.Time { return contractNow },
		// Setup must not start a broadcast dwell after it finishes.
		Staging: true,
		HomeScan: func(context.Context) []discovery.Found {
			return nil
		},
		LookAt: func(ctx context.Context) []discovery.Found {
			item, ok := discovery.ProbeHost(ctx, host, []discovery.ProbePort{{
				Port: port, Path: "/discover.json", Kind: "hdhomerun",
			}})
			if !ok {
				t.Errorf("fake tuner did not answer look")
				return nil
			}
			return []discovery.Found{item}
		},
		// No hosts, so this fixture does not probe the machine it runs on.
		FreeHosts: func() []string { return nil },
		SetupBench: func(context.Context, string, string) (float64, error) {
			return 4, nil
		},
		SetupSignal: func(context.Context) (int, int, int, int, error) {
			return 2, 0, 0, 0, nil
		},
		// Four is the stand-in count. The real pull talks to the public guide host.
		GuidePull: func(context.Context) (int, error) { return 4, nil },
	}
	h := api.Handler()
	root := fixtureDir(t)

	// GET /channels/{id}/frame is a JPEG preview. matchFrame checks it.
	// GET /backup is a SQLite file, not JSON. matchBackup checks status and content-type.
	// GET /recordings/{id}/file is MPEG-TS, not JSON. The case checks status, content-type, and bytes.
	// POST /watch records the error: the fake tuner serves no MPEG-TS, so ffmpeg never builds a playlist.
	// DELETE /virtuals does not exist.
	cases := []struct {
		name, method, path, body string
		status                   int
	}{
		{"health", "GET", "/api/v1/health", "", 0},
		{"server", "GET", "/api/v1/server", "", 0},
		{"clock", "GET", "/api/v1/clock", "", 0},
		{"profile", "GET", "/api/v1/profile", "", 0},
		{"devices", "GET", "/api/v1/devices", "", 0},
		{"sources", "GET", "/api/v1/sources", "", 0},
		{"channels", "GET", "/api/v1/channels?guide=1", "", 0},
		{"airings", "GET", "/api/v1/airings?from=2026-09-24T14:00:00Z&to=2026-09-24T18:00:00Z", "", 0},
		{"schedule", "GET", "/api/v1/schedule", "", 0},
		{"search", "GET", "/api/v1/search?q=Jeopardy", "", 0},
		{"scoreboard", "GET", "/api/v1/sports/scoreboard?date=2026-09-24", "", 0},
		{"signals", "GET", "/api/v1/signals", "", 0},
		{"tuners", "GET", "/api/v1/tuners", "", 0},
		{"recordings", "GET", "/api/v1/recordings", "", 0},
		{"markers", "GET", "/api/v1/recordings/1/markers", "", 0},
		{"passes", "GET", "/api/v1/passes", "", 0},
		{"teams", "GET", "/api/v1/teams", "", 0},
		{"events", "GET", "/api/v1/events", "", 0},
		{"virtuals", "GET", "/api/v1/virtuals", "", 0},
		{"virtual-schedule", "GET", "/api/v1/virtuals/schedule", "", 0},
		{"settings", "GET", "/api/v1/settings", "", 0},
		{"storage", "GET", "/api/v1/storage", "", 0},
		{"diagnostics", "GET", "/api/v1/diagnostics", "", 0},
		{"multiview", "POST", "/api/v1/multiview/plan", `{"channelIds":[1,2]}`, 0},
		{"channel", "PATCH", "/api/v1/channels/1", `{"favorite":true,"customName":"Fox 4"}`, 0},
		{"scan", "POST", "/api/v1/devices/FAKEHDHR/scan", "", 0},
		{"scan-status", "GET", "/api/v1/devices/FAKEHDHR/scan", "", 0},
		{"discover", "POST", "/api/v1/sources/discover", `{"ip":"` + host + ":" + portText + `"}`, 0},
		{"look", "POST", "/api/v1/sources/look", "", 0},
		{"home", "GET", "/api/v1/home?fresh=1", "", 0},
		{"affiliations", "GET", "/api/v1/affiliations", "", 0},
		{"star", "POST", "/api/v1/channels/star", "{}", 0},
		{"free", "GET", "/api/v1/sources/free", "", 0},
		{"free-add", "POST", "/api/v1/sources/free", `{"name":"FastChannels","playlist":"` + feeds.URL + `/free.m3u"}`, 0},
		{"xtream", "POST", "/api/v1/sources", `{"kind":"xtream","name":"Lab","url":"` + feeds.URL + `","username":"lab","password":"secret"}`, 0},
		{"watch", "POST", "/api/v1/watch", `{"channelId":1}`, http.StatusInternalServerError},
		{"watch-stop", "POST", "/api/v1/watch/1/stop", "{}", 0},
		{"setup-finish", "GET", "/api/v1/setup/finish", "", 0},
		{"server-rename", "PATCH", "/api/v1/server", `{"name":"Living Room"}`, 0},
		{"settings-save", "PUT", "/api/v1/settings", `{"hideScores":"1"}`, 0},
		{"guide-refresh", "POST", "/api/v1/guide/refresh", "", 0},
		{"schedule-skip", "POST", "/api/v1/schedule/skip", `{"programId":"EP1","title":"Jeopardy!","subtitle":"Semifinals","channelId":1,"start":"2026-09-24T15:00:00Z"}`, 0},
		{"recording-progress", "PUT", "/api/v1/recordings/1/progress", `{"position":12.5}`, 0},
		{"recording-watched", "PUT", "/api/v1/recordings/1/watched", `{"watched":true}`, 0},
		{"recording-play", "POST", "/api/v1/recordings/1/play", `{}`, 0},
		{"recording-file", "GET", "/api/v1/recordings/1/file", "", 0},
		{"marker-create", "POST", "/api/v1/recordings/1/markers", `{"start":90,"end":120}`, 0},
		{"marker-delete", "DELETE", "/api/v1/markers/2", "", 0},
		{"recording-detect", "POST", "/api/v1/recordings/1/detect", "", 0},
		{"recording-create", "POST", "/api/v1/recordings", `{"channelId":1,"minutes":30,"title":"Jeopardy!"}`, http.StatusInternalServerError},
		{"recording-stop", "POST", "/api/v1/recordings/1/stop", "", 0},
		{"pass-create", "POST", "/api/v1/passes", `{"title":"Wheel of Fortune","channelId":1,"padBefore":0,"padAfter":5}`, 0},
		{"pass-delete", "DELETE", "/api/v1/passes/3", "", 0},
		{"team-unfollow", "DELETE", "/api/v1/teams/1", "", 0},
		{"virtual-create", "POST", "/api/v1/virtuals", `{"number":"9001","name":"News","recordings":[1]}`, 0},
	}
	sample := filepath.Join(dir, "jeopardy.ts")
	for _, tc := range cases {
		if tc.name == "recording-play" || tc.name == "recording-file" || tc.name == "recording-detect" {
			contractSample(t, sample)
		}
		// PlayFile uses this binary. Set it after the recordings list so that fixture stays as it was.
		if tc.name == "recording-play" {
			api.Hub.FFmpeg = "ffmpeg"
		}
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.name == "home" {
			req.Host = "broadwave.local"
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		want := tc.status
		if want == 0 {
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Fatalf("%s %s %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if tc.name == "recording-file" {
			if ct := rec.Header().Get("Content-Type"); ct != "video/mp2t" {
				t.Fatalf("recording file type %s", ct)
			}
			wantFile, err := os.ReadFile(sample)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rec.Body.Bytes(), wantFile) {
				t.Fatal("recording file bytes drifted")
			}
			continue
		}
		matchFixture(t, root, tc.name, rec.Body.Bytes())
	}
	matchSetup(t, h, root)
	// A pass that would record in the next half hour depends on the wall clock.
	if err := st.DeletePass(ctx, 1); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/check", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signals check %d %s", rec.Code, rec.Body.String())
	}
	matchFixture(t, root, "signals-check", rec.Body.Bytes())
	waitSignal(t, api)
	matchSocket(t, h, bus, st, root)
	// Delete adds an activity row. It runs after the socket fixture, which records the next id.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/recordings/1", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete recording %d %s", rec.Code, rec.Body.String())
	}
	matchFixture(t, root, "recording-delete", rec.Body.Bytes())
	matchFrame(t, h, dir, root)
	matchBackup(t, h, root)
}

// contractSample is a short recording so play and commercial detection have a file.
// The bytes are not a fixture. The file route compares them in memory.
func contractSample(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		return
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "30",
		"-c:a", "aac", "-shortest", "-f", "mpegts", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
}

// matchBackup checks the catalog download. The body is a SQLite file, so it is
// not written under api/fixtures. Restore's JSON is the golden response.
func matchBackup(t *testing.T, h http.Handler, root string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("backup type %s", ct)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("SQLite format 3")) {
		t.Fatal("backup is not a sqlite file")
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backup", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore %d %s", rec.Code, rec.Body.String())
	}
	matchFixture(t, root, "backup-restore", rec.Body.Bytes())
}

// matchFrame checks the preview route. With no file it is a 404. With a file
// it returns that JPEG unchanged.
func matchFrame(t *testing.T, h http.Handler, dir, root string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/1/frame", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("frame before a preview %d", rec.Code)
	}
	body, err := os.ReadFile(filepath.Join(root, "frame.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	path := live.FramePath(dir, 1, 480)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("frame %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("frame type %s", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatal("frame bytes drifted")
	}
}

func contractFeeds() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/player_api.php", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			fmt.Fprint(w, `[{"category_id":1,"category_name":"News"}]`)
		case "get_live_streams":
			fmt.Fprint(w, `[{"name":"Local News","stream_id":7,"category_id":1,"num":1,"epg_channel_id":"news"}]`)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/xmltv.php", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><tv></tv>`)
	})
	mux.HandleFunc("/free.m3u", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Local News\nhttp://%s/news.ts\n", r.Host)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
	})
	return mux
}

func matchSetup(t *testing.T, h http.Handler, root string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/finish", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup post %d %s", rec.Code, rec.Body.String())
	}
	matchFixture(t, root, "setup-finish-post", rec.Body.Bytes())
	deadline := time.Now().Add(5 * time.Second)
	var body []byte
	for time.Now().Before(deadline) {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/setup/finish", nil)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("setup get %d %s", rec.Code, rec.Body.String())
		}
		body = append([]byte(nil), rec.Body.Bytes()...)
		if strings.Contains(rec.Body.String(), `"ready"`) && strings.Contains(rec.Body.String(), `"running":false`) {
			matchFixture(t, root, "setup-finish-done", body)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("setup did not finish: %s", body)
}

func waitSignal(t *testing.T, api *Server) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for api.signalRunning() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if api.signalRunning() {
		t.Fatal("signal check did not finish")
	}
}

func matchSocket(t *testing.T, h http.Handler, bus *realtime.Bus, st *store.Store, root string) {
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
	bus.Publish("sources.found", map[string]int{"found": 1})
	matchFixture(t, root, "ws-sources", read("sources.found"))
	bus.Publish("live.changed", nil)
	matchFixture(t, root, "ws-live", read("live.changed"))
	if err := st.AddEventAt(context.Background(), contractNow, "source", "Tuner found."); err != nil {
		t.Fatal(err)
	}
	matchFixture(t, root, "ws-activity", read("activity"))
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

func diskDetail(s string) bool {
	if strings.HasSuffix(s, " GB free.") || strings.HasSuffix(s, " TB free.") {
		return true
	}
	switch s {
	case "Less than 20 GB is free. Free some space before a long recording.",
		"Recordings are on the container disk. Map a folder for them so they survive a rebuild.",
		"Recordings save in the folder mapped for this server.":
		return true
	default:
		return false
	}
}

func scrub(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch k {
			case "lastSeen":
				t[k] = "2026-09-24T15:00:00Z"
			case "refresh", "lastRefresh":
				t[k] = "2026-09-24T15:00:00Z"
			case "doctor":
				t[k] = []any{}
			case "os":
				t[k] = "test"
			case "freeBytes", "totalBytes", "Free", "Total":
				t[k] = float64(0)
			case "ffmpeg":
				t[k] = "test"
			case "detail":
				// Free space and the low-disk notes depend on the machine.
				if s, ok := val.(string); ok && diskDetail(s) {
					t[k] = "0 GB free."
				}
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
