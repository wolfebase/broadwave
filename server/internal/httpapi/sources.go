package httpapi

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/dvr"
	"waveguide/internal/guide"
	"waveguide/internal/source"
	"waveguide/internal/store"
)

func (s *Server) addSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		URL   string `json:"url"`
		XMLTV string `json:"xmltvUrl"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	switch kind {
	case "m3u":
		raw, err := source.FetchText(ctx, strings.TrimSpace(body.URL))
		if err != nil {
			writeError(w, err)
			return
		}
		entries := source.ParseM3U(strings.NewReader(string(raw)))
		item, err := s.Store.AddSource(ctx, "m3u", strings.TrimSpace(body.Name), body.URL, body.XMLTV)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := source.Install(ctx, s.Store, item.ID, item.Name, "Playlist", entries); err != nil {
			writeError(w, err)
			return
		}
		s.attachXMLTV(ctx, item.ID, body.XMLTV)
		writeJSON(w, http.StatusOK, item)
	case "link":
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "Stream"
		}
		item, err := s.Store.AddSource(ctx, "link", name, body.URL, "")
		if err != nil {
			writeError(w, err)
			return
		}
		err = source.Install(ctx, s.Store, item.ID, name, "Link", []source.Entry{{Number: "801", Name: name, URL: strings.TrimSpace(body.URL)}})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "folder":
		found, err := source.ScanMedia(strings.TrimSpace(body.URL))
		if err != nil {
			writeError(w, err)
			return
		}
		added := 0
		existing, _ := s.Store.Recordings(ctx)
		have := map[string]bool{}
		for _, rec := range existing {
			have[rec.Path] = true
		}
		for _, rec := range found {
			if have[rec.Path] {
				continue
			}
			info, err := os.Stat(rec.Path)
			if err != nil {
				continue
			}
			rec.StartedAt = info.ModTime()
			if _, err := s.Store.CreateRecording(ctx, rec); err == nil {
				added++
			}
		}
		item, err := s.Store.AddSource(ctx, "folder", strings.TrimSpace(body.Name), body.URL, "")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"source": item, "added": added})
	default:
		httpError(w, "kind must be m3u, link, or folder", http.StatusBadRequest)
	}
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Sources(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": list})
}

func (s *Server) attachXMLTV(ctx context.Context, sourceID int64, rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	body, err := source.FetchText(ctx, rawURL)
	if err != nil {
		return
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return
	}
	prefix := "src-"
	var mine []store.Channel
	var ids []int64
	for _, ch := range channels {
		if strings.HasPrefix(ch.DeviceID, prefix) {
			mine = append(mine, ch)
			ids = append(ids, ch.ID)
		}
	}
	rows, err := guide.Parse(body, mine)
	if err != nil {
		return
	}
	_ = s.Store.ReplaceAiringsFor(ctx, ids, rows)
	_ = sourceID
}

func (s *Server) setWatched(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	var body struct {
		Watched bool `json:"watched"`
	}
	if err := decodeJSON(r, &body); err != nil && err != io.EOF {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	flag := 2
	if body.Watched {
		flag = 1
	}
	if err := s.Store.SetWatched(r.Context(), id, flag); err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "watched": body.Watched})
}

func (s *Server) skipAiring(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProgramID string `json:"programId"`
		Title     string `json:"title"`
		Subtitle  string `json:"subtitle"`
		ChannelID int64  `json:"channelId"`
		Start     string `json:"start"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := store.EpisodeKey(body.ProgramID, body.Title, body.Subtitle, body.ChannelID)
	if key == "" {
		key = store.EpisodeKey("once", body.Title, body.Start, body.ChannelID)
	}
	if err := s.Store.SkipAiring(r.Context(), key, body.Start); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) updateVirtual(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		OrderMode  string  `json:"orderMode"`
		RuleTitle  string  `json:"ruleTitle"`
		Recordings []int64 `json:"recordings"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.RuleTitle != "" && body.Recordings == nil {
		recs, _ := s.Store.Recordings(r.Context())
		for _, rec := range recs {
			if rec.Status != "failed" && strings.EqualFold(rec.Title, body.RuleTitle) {
				body.Recordings = append(body.Recordings, rec.ID)
			}
		}
	}
	updated, err := s.Store.UpdateVirtual(r.Context(), id, body.OrderMode, body.RuleTitle, body.Recordings)
	if err != nil {
		httpError(w, "channel not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) virtualSchedule(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Virtuals(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	recs, err := s.Store.Recordings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	now := time.Now()
	type row struct {
		store.VirtualChannel
		Slots []any `json:"slots"`
	}
	out := make([]row, 0, len(list))
	for _, v := range list {
		slots := virtualSlots(v, recs, now, now.Add(48*time.Hour))
		out = append(out, row{VirtualChannel: v, Slots: slots})
	}
	writeJSON(w, http.StatusOK, map[string]any{"virtuals": out})
}

func virtualSlots(v store.VirtualChannel, recs []store.Recording, from, to time.Time) []any {
	slots := dvr.Slots(v.OrderMode, v.Recordings, recs, from, to)
	out := make([]any, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot)
	}
	return out
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
