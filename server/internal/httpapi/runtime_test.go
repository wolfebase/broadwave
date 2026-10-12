package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"broadwave/internal/store"
)

func TestRuntimeReportsTheProcess(t *testing.T) {
	h := (&Server{}).Handler()
	res := get(t, h, "/api/v1/runtime")
	body := decodeRuntime(t, res.Body.Bytes())
	if body.Goroutines < 2 {
		t.Fatalf("goroutines %d", body.Goroutines)
	}
	if body.HeapAlloc == 0 || body.HeapInuse == 0 || body.Sys == 0 {
		t.Fatalf("memory %+v", body)
	}
	if body.UptimeSec < 0 {
		t.Fatalf("uptime %d", body.UptimeSec)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if body.OpenFDs < 3 {
			t.Fatalf("openFDs %d", body.OpenFDs)
		}
		if body.MaxRss == 0 {
			t.Fatalf("maxRss %d", body.MaxRss)
		}
	}
}

func TestRuntimeGCStillReports(t *testing.T) {
	h := (&Server{}).Handler()
	res := get(t, h, "/api/v1/runtime?gc=1")
	body := decodeRuntime(t, res.Body.Bytes())
	if body.Goroutines < 2 || body.HeapAlloc == 0 {
		t.Fatalf("%+v", body)
	}
}

func decodeRuntime(t *testing.T, raw []byte) runtimeBody {
	t.Helper()
	var body runtimeBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

type runtimeBody struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heapAlloc"`
	HeapInuse  uint64 `json:"heapInuse"`
	HeapSys    uint64 `json:"heapSys"`
	Sys        uint64 `json:"sys"`
	NumGC      uint32 `json:"numGC"`
	MaxRss     uint64 `json:"maxRss"`
	OpenFDs    int    `json:"openFDs"`
	UptimeSec  int    `json:"uptimeSec"`
}

// A paired server keeps the process counters for admin devices.
func TestRuntimeNeedsAdmin(t *testing.T) {
	if got := requiredScope(http.MethodGet, "/api/v1/runtime"); got != store.ScopeAdmin {
		t.Fatalf("scope %q", got)
	}
}

// The legacy /api prefix serves the same handler.
func TestRuntimeOnTheShortPrefix(t *testing.T) {
	h := (&Server{}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
