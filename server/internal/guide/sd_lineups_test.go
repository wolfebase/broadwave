package guide

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSDPostal(t *testing.T) {
	for raw, want := range map[string][2]string{
		"80000":       {"80000", "USA"},
		" 80000-1234": {"80000", "USA"},
		"k1a 0b1":     {"K1A0B1", "CAN"},
		"8000":        {"8000", ""},
		"hello":       {"HELLO", ""},
	} {
		code, country := SDPostal(raw)
		if code != want[0] || country != want[1] {
			t.Errorf("%q: %q %q", raw, code, country)
		}
	}
}

func sdAccountServer(t *testing.T, lineups string, max int, puts *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"code":0,"token":"tok"}`))
		case r.URL.Path == "/headends":
			if r.URL.Query().Get("country") != "USA" || r.URL.Query().Get("postalcode") != "80000" {
				http.Error(w, `{"code":2102,"message":"Unknown postal code."}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`[
				{"headend":"CO00001","transport":"Cable","location":"Plainsview","lineups":[{"name":"Valley Cable - Digital","lineup":"USA-CO00001-X"}]},
				{"headend":"80000","transport":"Antenna","location":"80000","lineups":[{"name":"Antenna","lineup":"USA-OTA80000"}]},
				{"headend":"DISH1","transport":"Satellite","location":"Plainsview","lineups":[{"name":"Sky Dish","lineup":"USA-DISH1-DEFAULT"}]}]`))
		case r.URL.Path == "/status":
			_, _ = w.Write([]byte(`{"account":{"expires":"2027-01-01T00:00:00Z","maxLineups":` + strings.TrimSpace(string(rune('0'+max))) + `},"lineups":` + lineups + `}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/lineups/"):
			*puts = append(*puts, strings.TrimPrefix(r.URL.Path, "/lineups/"))
			_, _ = w.Write([]byte(`{"code":0,"changesRemaining":5}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSDHeadendsListAntennaFirst(t *testing.T) {
	var puts []string
	srv := sdAccountServer(t, `[]`, 4, &puts)
	got, err := NewSDWith(srv.Client(), srv.URL, "lineups", "pw").Headends(t.Context(), "USA", "80000")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "USA-OTA80000" || got[1].Transport != "Cable" || got[2].Transport != "Satellite" {
		t.Fatalf("%+v", got)
	}
	_, err = NewSDWith(srv.Client(), srv.URL, "lineups", "pw").Headends(t.Context(), "USA", "99999")
	if err == nil || err.Error() != "Schedules Direct: Unknown postal code." {
		t.Fatalf("err %v", err)
	}
}

func TestSDAddLineupRespectsTheLimit(t *testing.T) {
	var puts []string
	srv := sdAccountServer(t, `[{"lineup":"USA-OTA80000"}]`, 2, &puts)
	c := NewSDWith(srv.Client(), srv.URL, "limits", "pw")
	if err := c.AddLineup(t.Context(), "USA-OTA80000"); err != nil || len(puts) != 0 {
		t.Fatalf("a lineup already on the account: %v %v", err, puts)
	}
	if err := c.AddLineup(t.Context(), "USA-CO00001-X"); err != nil || len(puts) != 1 {
		t.Fatalf("add: %v %v", err, puts)
	}
	full := sdAccountServer(t, `[{"lineup":"A"},{"lineup":"B"}]`, 2, &puts)
	err := NewSDWith(full.Client(), full.URL, "full", "pw").AddLineup(t.Context(), "USA-OTA80000")
	if err == nil || !strings.Contains(err.Error(), "already has 2 of its 2 lineups") {
		t.Fatalf("err %v", err)
	}
}
