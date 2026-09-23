package guide

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"waveguide/internal/store"
)

func TestSchedulesDirectMapsChannelNumbers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"token":"abc"}`))
		case "/lineups/USA-TEST":
			_, _ = w.Write([]byte(`{"map":[{"stationID":"1","channel":"14.1"}]}`))
		case "/schedules":
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
