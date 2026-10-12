package httpapi

import (
	"net/http"
	"runtime"
	"time"
)

// procStart is when this process began. A soak compares counters against it.
var procStart = time.Now()

// runtime reports goroutines, memory, and open files for a long run.
// gc=1 collects first so a quiet reading is not inflated by freeable cache.
func (s *Server) runtime(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("gc") == "1" {
		runtime.GC()
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeJSON(w, http.StatusOK, map[string]any{
		"goroutines": runtime.NumGoroutine(),
		"heapAlloc":  mem.HeapAlloc,
		"heapInuse":  mem.HeapInuse,
		"heapSys":    mem.HeapSys,
		"sys":        mem.Sys,
		"numGC":      mem.NumGC,
		"maxRss":     maxRSS(),
		"openFDs":    openFDs(),
		"uptimeSec":  int(time.Since(procStart).Seconds()),
	})
}
