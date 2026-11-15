package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// frames lists channels whose 480 preview exists and is still fresh, its
// own or that of another row of the same broadcast (stillPartners). The wide
// JPEGs are written beside it. A card asks for /frame only for these ids, so
// a channel on an open mux is included and one that was never sampled is not.
func (s *Server) frames(w http.ResponseWriter, r *http.Request) {
	// A grab lands a minute after the tune. A cached empty list would hide it.
	w.Header().Set("Cache-Control", "no-cache")
	list := frameList{NoSignal: []int64{}}
	list.Frames = s.freshFrames(r.Context(), s.now())
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
	// Sweep is a still the idle sweep made, not a tune someone is on.
	Sweep bool `json:"sweep,omitempty"`
	// Earlier is a still from before the program on now, or its first
	// minutes (live.StillSettle): it shows the program before.
	Earlier bool `json:"earlier,omitempty"`
}

// still is one channel's own 480 preview on disk.
type still struct {
	id    int64
	at    time.Time
	swept bool
}

func (s *Server) freshFrames(ctx context.Context, now time.Time) []frameStamp {
	out := []frameStamp{}
	if s.Hub == nil || s.Hub.Dir == "" {
		return out
	}
	entries, err := os.ReadDir(filepath.Join(s.Hub.Dir, "frames"))
	if err != nil {
		return out
	}
	own := map[int64]still{}
	for _, entry := range entries {
		id, ok := previewChannel(entry.Name())
		if !ok {
			continue
		}
		// A still is listed as long as a swept one, whoever made it: one
		// from a tune that ended was dropped at ten minutes while the sweep
		// did not come back to it for thirty.
		if st, ok := s.ownStill(id); ok && now.Sub(st.at) <= live.SweepStaleAfter {
			own[id] = st
		}
	}
	best := map[int64]still{}
	for id, st := range own {
		best[id] = st
	}
	starts := map[int64]time.Time{}
	if s.Store != nil {
		if chs, err := s.Store.Channels(ctx, false); err == nil {
			partners := stillPartners(chs)
			for _, ch := range chs {
				for _, p := range partners[ch.ID] {
					if st, ok := own[p]; ok && st.at.After(best[ch.ID].at) {
						best[ch.ID] = st
					}
				}
			}
			if airings, err := s.Store.Airings(ctx, now, now.Add(time.Second)); err == nil {
				for _, a := range airings {
					starts[a.ChannelID] = a.Start
				}
				// A row with no listing of its own goes by its partner's.
				for _, ch := range chs {
					if _, ok := starts[ch.ID]; ok {
						continue
					}
					for _, p := range partners[ch.ID] {
						if start, ok := starts[p]; ok {
							starts[ch.ID] = start
							break
						}
					}
				}
			}
		}
	}
	for id, st := range best {
		start, on := starts[id]
		out = append(out, frameStamp{ID: id, At: st.at.Unix(), Sweep: st.swept, Earlier: on && st.at.Before(start.Add(live.StillSettle))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ownStill is channel id's own 480 preview.
func (s *Server) ownStill(id int64) (still, bool) {
	info, err := os.Stat(live.FramePath(s.Hub.Dir, id, 480))
	if err != nil || !info.Mode().IsRegular() {
		return still{}, false
	}
	_, swept := s.Hub.SweptAt(id)
	return still{id: id, at: info.ModTime(), swept: swept}, true
}

// stillPartners are, for each channel, the other rows of the same broadcast:
// the row a SameAs row stands for and back, the other half of a 1.0/3.0 pair,
// and the clear channel an encrypted one plays as. A still of one is the
// picture of all.
func stillPartners(chs []store.Channel) map[int64][]int64 {
	out := map[int64][]int64{}
	link := func(a, b int64) {
		if a == 0 || b == 0 || a == b {
			return
		}
		if !slices.Contains(out[a], b) {
			out[a] = append(out[a], b)
		}
		if !slices.Contains(out[b], a) {
			out[b] = append(out[b], a)
		}
	}
	for _, ch := range chs {
		link(ch.ID, ch.SameAs)
		link(ch.ID, ch.TwinID)
		link(ch.ID, ch.PlaysAs)
	}
	// A SameAs row of a pair's half shares the pair's picture too.
	for _, ch := range chs {
		if ch.SameAs == 0 {
			continue
		}
		for _, p := range out[ch.SameAs] {
			link(ch.ID, p)
		}
	}
	return out
}

// frameSource is the channel whose stills answer for id: its own, or a
// partner's newer one, the same choice freshFrames lists.
func (s *Server) frameSource(ctx context.Context, id int64, at string) int64 {
	st, ok := s.ownStill(id)
	if ok && at == strconv.FormatInt(st.at.Unix(), 10) {
		return id
	}
	if s.Store == nil {
		return id
	}
	chs, err := s.Store.Channels(ctx, false)
	if err != nil {
		return id
	}
	src := id
	for _, p := range stillPartners(chs)[id] {
		if other, found := s.ownStill(p); found && (!ok || other.at.After(st.at)) {
			src, st, ok = p, other, true
		}
	}
	return src
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
	id = s.frameSource(r.Context(), id, r.URL.Query().Get("at"))
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
