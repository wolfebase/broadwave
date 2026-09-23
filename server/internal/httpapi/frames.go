package httpapi

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"waveguide/internal/live"
)

func (s *Server) frame(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || s.Hub.Dir == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	width := 480
	if r.URL.Query().Get("w") == "1280" {
		width = 1280
	}
	path := live.FramePath(s.Hub.Dir, id, width)
	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	if live.FrameStale(info.ModTime(), time.Now()) {
		w.Header().Set("X-Frame-Stale", "1")
	}
	http.ServeFile(w, r, path)
}
