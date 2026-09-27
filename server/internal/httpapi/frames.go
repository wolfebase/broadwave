package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/live"
)

// frames lists channels whose 480 preview exists and is still fresh.
// The wide JPEG is written beside it. A card asks for /frame only for these ids,
// so a channel on an open mux is included and one that was never sampled is not.
func (s *Server) frames(w http.ResponseWriter, r *http.Request) {
	// A grab lands a minute after the tune. A cached empty list would hide it.
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, frameList{Channels: s.freshFrames(time.Now())})
}

type frameList struct {
	Channels []int64 `json:"channels"`
}

func (s *Server) freshFrames(now time.Time) []int64 {
	out := []int64{}
	if s.Hub == nil || s.Hub.Dir == "" {
		return out
	}
	entries, err := os.ReadDir(filepath.Join(s.Hub.Dir, "frames"))
	if err != nil {
		return out
	}
	for _, entry := range entries {
		id, ok := previewChannel(entry.Name())
		if !ok {
			continue
		}
		info, err := os.Stat(live.FramePath(s.Hub.Dir, id, 480))
		if err != nil || !info.Mode().IsRegular() || live.FrameStale(info.ModTime(), now) {
			continue
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func previewChannel(name string) (int64, bool) {
	idText, ok := strings.CutSuffix(name, ".jpg")
	if !ok || idText == "" || strings.Contains(idText, "-") {
		return 0, false
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

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
