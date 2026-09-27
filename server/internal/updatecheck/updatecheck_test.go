package updatecheck

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func parse[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 9, 27, 8, 30, 0, 0, time.UTC)
	free := parse[Tuners](t, `{"tuners":[{"index":0,"ours":false},{"index":1,"ours":false}]}`)
	held := parse[Tuners](t, `{"tuners":[{"index":0,"ours":false},{"index":1,"ours":true}]}`)
	none := Schedule{}
	idle := parse[Recordings](t, `{"recordings":[{"title":"Jeopardy!","status":"complete"}]}`)
	busy := parse[Recordings](t, `{"recordings":[{"title":"Jeopardy!","status":"recording"}]}`)
	soon := parse[Schedule](t, `{"items":[{"airing":{"title":"Game","start":"2026-09-27T10:25:00Z","end":"2026-09-27T13:00:00Z"},"padBefore":1,"skipped":false}]}`)
	later := parse[Schedule](t, `{"items":[{"airing":{"title":"Game","start":"2026-09-27T10:32:00Z","end":"2026-09-27T13:00:00Z"},"padBefore":1,"skipped":false}]}`)
	skipped := parse[Schedule](t, `{"items":[{"airing":{"title":"Game","start":"2026-09-27T09:00:00Z","end":"2026-09-27T10:00:00Z"},"skipped":true}]}`)
	for _, c := range []struct {
		name string
		t    Tuners
		s    Schedule
		r    Recordings
		busy bool
	}{
		{"idle", free, none, idle, false},
		{"someone watching", held, none, idle, true},
		{"recording", free, none, busy, true},
		{"due inside two hours with padding", free, soon, idle, true},
		{"due after two hours", free, later, idle, false},
		{"a skipped airing", free, skipped, idle, false},
	} {
		if got := Decide(c.t, c.s, c.r, now, Window); (got != "") != c.busy {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

func TestRunExitCodes(t *testing.T) {
	tuners := `{"tuners":[{"index":0,"ours":true}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/tuners":
			_, _ = w.Write([]byte(tuners))
		case "/api/v1/schedule":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/api/v1/recordings":
			_, _ = w.Write([]byte(`{"recordings":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	_, portText, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	if code, why := Run(port, time.Now()); code != Skip {
		t.Fatalf("a held tuner must skip: %d %s", code, why)
	}
	tuners = `{"tuners":[{"index":0,"ours":false}]}`
	if code, why := Run(port, time.Now()); code != 0 {
		t.Fatalf("an idle server updates: %d %s", code, why)
	}
	srv.Close()
	if code, _ := Run(port, time.Now()); code != 0 {
		t.Fatal("a server that does not answer has nothing to cut off")
	}
}
