package guide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
	rows, ids, err := fetchSchedules(t.Context(), srv.Client(), srv.URL, "user", "secret", "USA-TEST", []store.Channel{{ID: 9, GuideNumber: "14.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Diginet" || rows[0].ChannelID != 9 || len(ids) != 1 {
		t.Fatalf("%+v ids %v", rows, ids)
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
	var lineup []store.Channel
	for i := range stations {
		lineup = append(lineup, store.Channel{ID: int64(i + 1), GuideNumber: fmt.Sprintf("%d.1", i+2)})
	}
	rows, ids, err := fetchSchedules(t.Context(), srv.Client(), srv.URL, "user", "secret", "USA-TEST", lineup)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != stations*30 || len(ids) != stations {
		t.Fatalf("%d rows on %d channels", len(rows), len(ids))
	}
	if rows[0].Title != "Show "+rows[0].ProgramID {
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
	_, err := sdGet(t.Context(), srv.Client(), srv.URL, "abc")
	if err == nil || !strings.Contains(err.Error(), "more than 8 MB") {
		t.Fatalf("err %v", err)
	}
}
