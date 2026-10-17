package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
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
	list := frameList{NoSignal: []int64{}}
	list.Frames = s.freshFrames(time.Now())
	list.Channels = make([]int64, len(list.Frames))
	for i, f := range list.Frames {
		list.Channels[i] = f.ID
	}
	if s.Hub != nil {
		list.NoSignal = s.Hub.NoSignalChannels()
	}
	writeJSON(w, http.StatusOK, list)
}

// frameList is the channels with a fresh picture, and the ones whose last
// tune found no signal, for the guide to mark.
type frameList struct {
	Channels []int64      `json:"channels"`
	Frames   []frameStamp `json:"frames"`
	NoSignal []int64      `json:"noSignal"`
}

// frameStamp is when a channel's stills were last written, in Unix seconds.
// A client that adds it to the still's address (?at=) gets a picture it may
// cache, since a newer grab is a new address.
type frameStamp struct {
	ID int64 `json:"id"`
	At int64 `json:"at"`
}

func (s *Server) freshFrames(now time.Time) []frameStamp {
	out := []frameStamp{}
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
		out = append(out, frameStamp{ID: id, At: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
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
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	// A grab from before a size existed still answers with the next one down.
	var path string
	var info os.FileInfo
	for _, size := range slices.Backward(live.FrameWidths) {
		if size > live.FrameWidth(width) {
			continue
		}
		path = live.FramePath(s.Hub.Dir, id, size)
		if info, err = os.Stat(path); err == nil && info.Mode().IsRegular() {
			break
		}
		info = nil
	}
	if info == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	// An address that names the grab it wants (at, from /frames) is that
	// picture for good; a newer grab is another address. A plain one is
	// asked again each time.
	cache := "no-cache"
	if at := r.URL.Query().Get("at"); at != "" && at == strconv.FormatInt(info.ModTime().Unix(), 10) {
		cache = "private, max-age=86400, immutable"
	}
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	if live.FrameStale(info.ModTime(), time.Now()) {
		w.Header().Set("X-Frame-Stale", "1")
	}
	http.ServeFile(w, r, path)
}
