package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/discovery"
	"broadwave/internal/dvr"
	"broadwave/internal/guide"
	"broadwave/internal/source"
	"broadwave/internal/store"
)

func (s *Server) addSource(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		s.addPlaylistFile(w, r)
		return
	}
	var body struct {
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		XMLTV  string `json:"xmltvUrl"`
		User   string `json:"username"`
		Pass   string `json:"password"`
		Groups string `json:"groups"`
		Keep   string `json:"keep"`
		Start  int    `json:"start"`
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
		raw, err := source.ReadPlaylist(ctx, strings.TrimSpace(body.URL))
		if err != nil {
			writeError(w, err)
			return
		}
		s.installPlaylist(w, ctx, "m3u", body.Name, body.URL, body.Groups, body.Keep, body.XMLTV, body.Start, raw)
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
	case "xtream":
		playlist, _, err := source.Xtream(ctx, body.URL, body.User, body.Pass)
		if err != nil {
			writeError(w, err)
			return
		}
		stored := strings.TrimRight(strings.TrimSpace(body.URL), "/")
		if body.User != "" {
			u, err := url.Parse(stored)
			if err != nil || u.Host == "" {
				httpError(w, "The server address should start with http:// or https://.", http.StatusBadRequest)
				return
			}
			u.User = url.UserPassword(body.User, body.Pass)
			stored = u.String()
		}
		s.installPlaylist(w, ctx, "xtream", body.Name, stored, body.Groups, body.Keep, body.XMLTV, body.Start, playlist)
	case "tvheadend":
		playlist, loc, guide, err := source.TVHeadend(ctx, body.URL, body.User, body.Pass)
		if err != nil {
			writeError(w, err)
			return
		}
		if body.XMLTV != "" {
			guide = body.XMLTV
		}
		s.installPlaylist(w, ctx, "tvheadend", body.Name, loc, body.Groups, body.Keep, guide, body.Start, playlist)
	case "channels":
		playlist, loc, guide, err := source.ChannelsDVR(ctx, body.URL)
		if err != nil {
			writeError(w, err)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "Channels DVR"
		}
		s.installPlaylist(w, ctx, "channels", name, loc, body.Groups, body.Keep, guide, body.Start, playlist)
	case "threadfin", "xteve", "ersatztv", "dispatcharr":
		playlist, loc, guide, err := source.EmulatorM3U(ctx, kind, body.URL)
		if err != nil {
			writeError(w, err)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = kind
		}
		s.installPlaylist(w, ctx, kind, name, loc, body.Groups, body.Keep, guide, body.Start, playlist)
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
		httpError(w, "kind must be m3u, xtream, tvheadend, channels, threadfin, xteve, ersatztv, dispatcharr, link, or folder", http.StatusBadRequest)
	}
}

func (s *Server) addPlaylistFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpError(w, "Choose a playlist file.", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, "Choose a playlist file.", http.StatusBadRequest)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		writeError(w, err)
		return
	}
	raw, err = source.UnpackPlaylist(raw)
	if err != nil {
		writeError(w, err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" && header != nil {
		name = header.Filename
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	s.installPlaylist(w, ctx, "m3u", name, "file:"+header.Filename, r.FormValue("groups"), r.FormValue("keep"), r.FormValue("xmltvUrl"), 0, raw)
}

func (s *Server) installPlaylist(w http.ResponseWriter, ctx context.Context, kind, name, loc, groups, keep, xmltv string, start int, raw []byte) {
	parsed := source.ParseM3U(strings.NewReader(string(raw)))
	if strings.TrimSpace(groups) == "" && strings.TrimSpace(keep) == "" {
		if msg := source.BigPlaylistMessage(parsed); msg != "" {
			writeJSON(w, http.StatusOK, source.PlaylistPick(parsed, msg))
			return
		}
	}
	entries := source.Renumber(source.FilterKeep(source.FilterGroups(parsed, groups), keep), start)
	guideURL := source.GuideFromPlaylist(xmltv, entries)
	item, err := s.Store.AddSource(ctx, kind, strings.TrimSpace(name), loc, guideURL)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := source.Install(ctx, s.Store, item.ID, item.Name, "Playlist", entries); err != nil {
		writeError(w, err)
		return
	}
	if err := s.attachXMLTV(ctx, item.ID, guideURL); err != nil {
		_ = s.Store.AddEvent(ctx, "source", fmt.Sprintf("The guide for %s did not load. %v", item.Name, err))
	}
	_ = s.Store.RememberPlaylist(ctx, item.ID, groups, start, time.Now().Add(24*time.Hour))
	if len(entries) > 0 {
		if format := source.ProbeFormat(ctx, entries[0].URL); format != "" {
			_ = s.Store.SetStreamFormat(ctx, item.ID, format)
		}
	}
	writeJSON(w, http.StatusOK, item)
}

func playlistKind(kind string) bool {
	switch kind {
	case "m3u", "xtream", "tvheadend", "channels", "threadfin", "xteve", "ersatztv", "dispatcharr", "free":
		return true
	default:
		return false
	}
}

func (s *Server) freeSources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		found := source.FindFree(ctx, discovery.LocalHosts())
		if found == nil {
			found = []source.Feed{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"found": found, "guide": source.FreeGuide})
		return
	}
	var body struct {
		Kind     string `json:"kind"`
		Addr     string `json:"addr"`
		Playlist string `json:"playlist"`
		Guide    string `json:"guide"`
		Name     string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	feed := source.Feed{Playlist: strings.TrimSpace(body.Playlist), Guide: strings.TrimSpace(body.Guide), Name: strings.TrimSpace(body.Name)}
	if feed.Playlist == "" {
		resolved, err := source.FeedByKind(body.Kind, body.Addr)
		if err != nil {
			writeError(w, err)
			return
		}
		feed = resolved
	}
	raw, err := source.ReadPlaylist(ctx, feed.Playlist)
	if err != nil {
		writeError(w, err)
		return
	}
	kept, skipped := source.PrepareFree(source.ParseM3U(strings.NewReader(string(raw))))
	if len(kept) == 0 {
		msg := "That feed has no channels yet. Give it a minute, then add it again."
		if skipped > 0 {
			msg = "Every channel in that feed needs DRM."
		}
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	name := feed.Name
	if name == "" {
		name = source.FreeGroup
	}
	item, err := s.Store.AddSource(ctx, "free", name, feed.Playlist, feed.Guide)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := source.Install(ctx, s.Store, item.ID, item.Name, "Playlist", kept); err != nil {
		writeError(w, err)
		return
	}
	if err := s.attachXMLTV(ctx, item.ID, feed.Guide); err != nil {
		_ = s.Store.AddEvent(ctx, "source", fmt.Sprintf("The guide for %s did not load. %v", item.Name, err))
	}
	_ = s.Store.RememberPlaylist(ctx, item.ID, source.FreeGroup, 0, time.Now().Add(24*time.Hour))
	if format := source.ProbeFormat(ctx, kept[0].URL); format != "" {
		_ = s.Store.SetStreamFormat(ctx, item.ID, format)
	}
	note := ""
	if skipped > 0 {
		note = fmt.Sprintf("Skipped %d channels that need DRM.", skipped)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": item.ID, "kind": item.Kind, "name": item.Name, "url": item.URL, "xmltvUrl": item.XMLTV, "message": note,
	})
}

// RefreshSources reloads playlists whose next refresh time has passed.
// Channel ids stay put when the stream address or tvg-id still matches.
func (s *Server) RefreshSources(ctx context.Context, now time.Time) (int, error) {
	list, err := s.Store.Sources(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, item := range list {
		if !playlistKind(item.Kind) || !item.Enabled || strings.HasPrefix(item.URL, "file:") {
			continue
		}
		if item.Refresh != "" {
			at, err := time.Parse(time.RFC3339, item.Refresh)
			if err == nil && at.After(now) {
				continue
			}
		}
		loc := s.Store.FetchURL(ctx, item.ID, item.URL)
		var raw []byte
		if item.Kind == "xtream" {
			raw, _, err = source.XtreamFromStored(ctx, loc)
		} else {
			raw, err = source.ReadPlaylist(ctx, loc)
		}
		if err != nil {
			s.noteSourceDown(ctx, item, now, err)
			continue
		}
		parsed := source.ParseM3U(strings.NewReader(string(raw)))
		if item.Kind == "free" {
			parsed, _ = source.PrepareFree(parsed)
		}
		entries := source.Renumber(source.FilterGroups(parsed, item.Groups), item.NumberStart)
		if err := source.Install(ctx, s.Store, item.ID, item.Name, "Playlist", entries); err != nil {
			s.noteSourceDown(ctx, item, now, err)
			continue
		}
		if err := s.attachXMLTV(ctx, item.ID, s.Store.FetchURL(ctx, item.ID, item.XMLTV)); err != nil {
			_ = s.Store.AddEvent(ctx, "source", fmt.Sprintf("The guide for %s did not load. %v", item.Name, err))
		}
		if item.Health != "" {
			_ = s.Store.AddEvent(ctx, "source", item.Name+" is back.")
		}
		if err := s.Store.NoteRefresh(ctx, item.ID, now.Add(24*time.Hour), ""); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Server) noteSourceDown(ctx context.Context, item store.Source, now time.Time, cause error) {
	_ = s.Store.NoteRefresh(ctx, item.ID, now.Add(time.Hour), cause.Error())
	if item.Health == "" {
		_ = s.Store.AddEvent(ctx, "source", item.Name+" is offline.")
	}
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Sources(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if s.Hub != nil {
		for i := range list {
			list[i].StreamsInUse = s.Hub.StreamsInUse(list[i].DeviceID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": list})
}

func (s *Server) attachXMLTV(ctx context.Context, sourceID int64, rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	body, err := source.FetchText(ctx, rawURL)
	if err != nil {
		return err
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("src-%d", sourceID)
	var mine []store.Channel
	var ids []int64
	for _, ch := range channels {
		if ch.DeviceID == want {
			mine = append(mine, ch)
			ids = append(ids, ch.ID)
		}
	}
	rows, art, err := guide.Parse(body, mine)
	if err != nil {
		return err
	}
	if err := s.Store.SetChannelArt(ctx, art); err != nil {
		return err
	}
	return s.Store.ReplaceAiringsFor(ctx, ids, tagGuideSource(rows, "playlist"))
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
	now := s.now()
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
