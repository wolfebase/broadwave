package guide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestSchedulesDirectMapsChannelNumbers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"token":"abc"}`))
		case "/lineups/USA-TEST":
			_, _ = w.Write([]byte(`{"map":[{"stationID":"1","channel":"14.1"}]}`))
		case "/schedules":
			body, _ := io.ReadAll(r.Body)
			if strings.Count(string(body), "20") > 8 && !strings.Contains(r.Header.Get("X-Test"), "wide") {
				http.Error(w, "one day", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`[{"stationID":"1","programs":[{"programID":"SH1","airDateTime":"2026-09-22T20:00:00Z","duration":1800}]}]`))
		case "/programs":
			_, _ = w.Write([]byte(`[{"programID":"SH1","titles":[{"title120":"Diginet"}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	got, err := newSD(srv.Client(), srv.URL, "user", "secret").Fetch(t.Context(), "USA-TEST", []SDChannel{{Channel: store.Channel{ID: 9, GuideNumber: "14.1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if rows := got.Rows; len(rows) != 1 || rows[0].Title != "Diginet" || rows[0].ChannelID != 9 || len(got.Channels) != 1 {
		t.Fatalf("%+v ids %v", rows, got.Channels)
	}
}

// A large lineup is asked for in batches, each response under the cap.
func TestSchedulesDirectAsksInBatches(t *testing.T) {
	var schedules, programs atomic.Int32
	const stations = 45
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"token":"abc"}`))
		case "/lineups/USA-TEST":
			var rows []string
			for i := range stations {
				rows = append(rows, fmt.Sprintf(`{"stationID":"%d","channel":"%d.1"}`, i, i+2))
			}
			_, _ = w.Write([]byte(`{"map":[` + strings.Join(rows, ",") + `]}`))
		case "/schedules":
			schedules.Add(1)
			var req []struct {
				StationID string `json:"stationID"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if len(req) > sdStationsPerRequest {
				http.Error(w, "too many", http.StatusBadRequest)
				return
			}
			var out []string
			for _, st := range req {
				var progs []string
				for p := range 30 {
					progs = append(progs, fmt.Sprintf(`{"programID":"EP%s-%d","airDateTime":"2026-09-22T%02d:00:00Z","duration":1800}`, st.StationID, p, p%24))
				}
				out = append(out, fmt.Sprintf(`{"stationID":%q,"programs":[%s]}`, st.StationID, strings.Join(progs, ",")))
			}
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
		case "/programs":
			programs.Add(1)
			var ids []string
			_ = json.NewDecoder(r.Body).Decode(&ids)
			if len(ids) > sdProgramsPerRequest {
				http.Error(w, "too many", http.StatusBadRequest)
				return
			}
			var out []string
			for _, id := range ids {
				out = append(out, fmt.Sprintf(`{"programID":%q,"titles":[{"title120":"Show %s"}]}`, id, id))
			}
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	var lineup []SDChannel
	for i := range stations {
		lineup = append(lineup, SDChannel{Channel: store.Channel{ID: int64(i + 1), GuideNumber: fmt.Sprintf("%d.1", i+2)}})
	}
	got, err := newSD(srv.Client(), srv.URL, "batch-user", "secret").Fetch(t.Context(), "USA-TEST", lineup)
	if err != nil {
		t.Fatal(err)
	}
	rows, ids := got.Rows, got.Channels
	if len(rows) != stations*30 || len(ids) != stations {
		t.Fatalf("%d rows on %d channels", len(rows), len(ids))
	}
	if !strings.HasPrefix(rows[0].Title, "Show EP") {
		t.Fatalf("title %q", rows[0].Title)
	}
	if schedules.Load() != 3 || programs.Load() != 3 {
		t.Fatalf("%d schedule and %d program requests, want 3 and 3", schedules.Load(), programs.Load())
	}
}

func TestSchedulesDirectNamesAResponseOverTheCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, sdMaxResponse+10))
	}))
	defer srv.Close()
	_, err := newSD(srv.Client(), srv.URL, "u", "p").do(t.Context(), http.MethodGet, "/x", "abc", nil)
	if err == nil || !strings.Contains(err.Error(), "more than 8 MB") {
		t.Fatalf("err %v", err)
	}
}

func sdFixtureServer(t *testing.T, logins *atomic.Int32) *httptest.Server {
	t.Helper()
	read := func(name string) []byte {
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" && r.Header.Get("token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":4006,"message":"Token has expired. Request new token."}`))
			return
		}
		switch r.URL.Path {
		case "/token":
			logins.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"message":"OK","token":"tok"}`))
		case "/lineups/USA-OTA80000":
			_, _ = w.Write(read("sd-lineup-antenna.json"))
		case "/schedules":
			_, _ = w.Write(read("sd-schedules.json"))
		case "/programs":
			_, _ = w.Write(read("sd-programs.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// An antenna lineup has no channel numbers, only ATSC major and minor. Two
// stations on 4.1 (two markets in reach) are told apart by the RF channel.
func TestSchedulesDirectReadsAnAntennaLineup(t *testing.T) {
	var logins atomic.Int32
	srv := sdFixtureServer(t, &logins)
	lineup := []SDChannel{
		{Channel: store.Channel{ID: 1, GuideNumber: "4.1", GuideName: "KBWV-DT"}, RF: 20},
		{Channel: store.Channel{ID: 2, GuideNumber: "4.2", GuideName: "Diginet"}},
		{Channel: store.Channel{ID: 3, GuideNumber: "5.1", GuideName: "WTST-HD"}},
		{Channel: store.Channel{ID: 4, GuideNumber: "4.1", GuideName: "KFAR"}, RF: 44},
	}
	got, err := newSD(srv.Client(), srv.URL, "antenna", "secret").Fetch(t.Context(), "USA-OTA80000", lineup)
	if err != nil {
		t.Fatal(err)
	}
	by := map[int64][]store.Airing{}
	for _, row := range got.Rows {
		by[row.ChannelID] = append(by[row.ChannelID], row)
	}
	if len(by[1]) != 2 || len(by[3]) != 1 || len(by[4]) != 1 || len(by[2]) != 0 {
		t.Fatalf("rows per channel: %v", map[int64]int{1: len(by[1]), 2: len(by[2]), 3: len(by[3]), 4: len(by[4])})
	}
	ep := by[1][0]
	if ep.Title != "Harbor Lights" || ep.Subtitle != "The Long Tide" || ep.Description != "A storm keeps the crew at sea." ||
		ep.Season != 3 || ep.Episode != 1 || ep.EpisodeLabel != "S3E1" || ep.OriginalAir != "2026-10-20" ||
		!ep.New || !ep.Premiere || ep.Finale || ep.Live || ep.Rating != "TV-14" || ep.Cast != "Sam Ortega, Rowan Ellis" ||
		ep.ProgramID != "EP01234567.0003" || ep.SeriesID != "SH01234567" || ep.Category != "Series, Drama" {
		t.Fatalf("episode %+v", ep)
	}
	if !ep.End.Equal(ep.Start.Add(time.Hour)) {
		t.Fatalf("times %v %v", ep.Start, ep.End)
	}
	game := by[1][1]
	if !game.Live || game.Category != "Sports, Football" || game.Subtitle != "Plainsview at Ridgecrest" {
		t.Fatalf("game %+v", game)
	}
	movie := by[3][0]
	if movie.Category != "Movie, Drama" || movie.OriginalAir != "1987" || movie.Rating != "PG" || movie.SeriesID != "" {
		t.Fatalf("movie %+v", movie)
	}
	if by[4][0].Title != "EP055555550001" {
		t.Fatalf("a program with no details keeps its id: %+v", by[4][0])
	}
	if got.Art[1] != "https://logos.example/kbwv.png" || got.Art[2] != "https://logos.example/kbwv2.png" || got.Art[3] != "" {
		t.Fatalf("art %v", got.Art)
	}
	if got.Networks[1] != "NBC" || got.Networks[3] != "CBS" || got.Networks[4] != "ABC" || got.Networks[2] != "" {
		t.Fatalf("networks %v", got.Networks)
	}
	// The second pull reuses the token.
	if _, err := newSD(srv.Client(), srv.URL, "antenna", "secret").Fetch(t.Context(), "USA-OTA80000", lineup); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 1 {
		t.Fatalf("%d sign-ins, want 1", logins.Load())
	}
}

// A token Schedules Direct no longer honors is replaced once.
func TestSchedulesDirectSignsInAgainWhenTheTokenExpires(t *testing.T) {
	var logins atomic.Int32
	srv := sdFixtureServer(t, &logins)
	c := newSD(srv.Client(), srv.URL, "expired", "secret")
	sdTokens.Lock()
	sdTokens.m[c.tokenKey()] = sdToken{value: "stale", until: time.Now().Add(time.Hour)}
	sdTokens.Unlock()
	if _, err := c.call(t.Context(), http.MethodGet, "/lineups/USA-OTA80000", nil); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 1 {
		t.Fatalf("%d sign-ins", logins.Load())
	}
}

func TestSchedulesDirectErrorsUseItsOwnWords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"response":"INVALID_USER","code":4003,"message":"Invalid username or password."}`))
	}))
	defer srv.Close()
	_, err := newSD(srv.Client(), srv.URL, "nobody", "wrong").token(t.Context())
	if err == nil || err.Error() != "Schedules Direct: Invalid username or password." {
		t.Fatalf("err %v", err)
	}
}

func TestRFChannel(t *testing.T) {
	for hz, want := range map[int]int{57_000_000: 2, 79_000_000: 5, 177_000_000: 7, 213_000_000: 13, 473_000_000: 14, 605_000_000: 36, 0: 0, 700_000_000: 0} {
		if got := RFChannel(hz); got != want {
			t.Errorf("%d: %d, want %d", hz, got, want)
		}
	}
}
