package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/dvr"
	"waveguide/internal/guide"
	"waveguide/internal/live"
	"waveguide/internal/store"
)

func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		httpError(w, "Live TV is not set up on this server.", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ChannelID int64      `json:"channelId"`
		Caps      *live.Caps `json:"caps"`
		Prefs     live.Prefs `json:"prefs"`
		Rendition string     `json:"rendition"`
		Profile   string     `json:"profile"`
		Audio     string     `json:"audio"`
		Picture   string     `json:"pictureMode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	src, err := s.Hub.SourceOf(r.Context(), body.ChannelID)
	if err != nil {
		writeError(w, err)
		return
	}
	var decision live.Decision
	if forced, ok := live.ParseRenditionKey(body.Rendition); ok {
		decision = live.Decision{Rendition: forced, Reason: "Chosen in the player"}
	} else {
		caps, prefs := live.LegacyCaps(body.Profile, body.Audio, body.Picture)
		if body.Caps != nil {
			caps, prefs = *body.Caps, body.Prefs
		}
		if prefs.Picture == "" {
			_, prefs.Picture = s.playbackChoice(r.Context(), 0, "")
		}
		decision = live.Decide(src, caps, prefs)
	}
	session, err := s.Hub.Watch(r.Context(), body.ChannelID, decision.Rendition)
	if err != nil {
		writeError(w, err)
		return
	}
	session.Stream.Reason = decision.Reason
	waitPlaylist(session.File, 12*time.Second)
	if tuners, err := s.Hub.Tuners(r.Context()); err == nil {
		session.Tuners = tuners
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		Rendition string `json:"rendition"`
	}
	_ = decodeJSON(r, &body)
	if s.Hub != nil {
		s.Hub.Release(id, body.Rendition)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) tuners(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tuners": []live.Tuner{}})
		return
	}
	list, err := s.Hub.Tuners(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []live.Tuner{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tuners": list, "encoder": s.Hub.Encoder})
}

func (s *Server) startRecording(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChannelID int64  `json:"channelId"`
		Minutes   int    `json:"minutes"`
		Title     string `json:"title"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	meta := store.Recording{ChannelID: body.ChannelID, Title: strings.TrimSpace(body.Title)}
	minutes := body.Minutes
	if air, ok := s.listingFor(r.Context(), body.ChannelID, meta.Title); ok {
		meta.Title = air.Title
		meta.Subtitle = air.Subtitle
		meta.Description = air.Description
		meta.Category = air.Category
		meta.ProgramID = air.ProgramID
		pad := 2
		left := int(time.Until(air.End.Add(time.Duration(pad)*time.Minute)).Minutes()) + 1
		if left > minutes {
			minutes = left
		}
	}
	rec, err := s.Hub.RecordMeta(r.Context(), minutes, meta)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) stopRecording(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	s.Hub.StopRecord(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) recordings(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Recordings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Recording{}
	}
	for i := range list {
		if info, err := os.Stat(list[i].Path); err == nil {
			list[i].Bytes = info.Size()
		}
		if pos, err := s.Store.Progress(r.Context(), list[i].ID); err == nil && pos > 0 {
			list[i].Position = pos
		}
		if list[i].Duration == 0 && list[i].Status != "recording" && list[i].Path != "" && s.Hub != nil {
			if tool := live.FFProbePath(s.Hub.FFmpeg); tool != "" {
				seconds, err := live.ProbeDuration(tool, list[i].Path)
				if err != nil || seconds <= 0 {
					_ = s.Store.SetDuration(r.Context(), list[i].ID, -1)
				} else {
					_ = s.Store.SetDuration(r.Context(), list[i].ID, seconds)
					list[i].Duration = seconds
				}
			}
		}
		if list[i].Duration < 0 {
			list[i].Duration = 0
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recordings": list})
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	rec, err := s.Store.Recording(r.Context(), id)
	if err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	if rec.Status == "recording" {
		httpError(w, "stop the recording before deleting it", http.StatusConflict)
		return
	}
	if s.Hub != nil {
		s.Hub.StopRecord(id)
		removeRecordingFiles(s.Hub.Dir, rec)
	}
	if err := s.Store.DeleteRecording(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	_ = s.Store.AddEvent(r.Context(), "delete", "Deleted "+rec.Title)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Events(r.Context(), 40)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

func removeRecordingFiles(workDir string, rec store.Recording) {
	root := filepath.Join(workDir, "recordings")
	removeInside(root, rec.Path)
	base := strings.TrimSuffix(rec.Path, filepath.Ext(rec.Path))
	removeInside(root, base+".edl")
	removeInside(root, base+".json")
	_ = os.Remove(filepath.Join(workDir, "posters", strconv.FormatInt(rec.ID, 10)+".jpg"))
	_ = os.RemoveAll(filepath.Join(workDir, "file", strconv.FormatInt(rec.ID, 10)))
}

func removeInside(root, path string) {
	if path == "" {
		return
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(root), clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	_ = os.Remove(clean)
}

func (s *Server) airings(w http.ResponseWriter, r *http.Request) {
	from := time.Now().Add(-30 * time.Minute)
	to := time.Now().Add(48 * time.Hour)
	if raw := r.URL.Query().Get("hours"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 168 {
			to = time.Now().Add(time.Duration(n) * time.Hour)
		}
	}
	list, err := s.Store.Airings(r.Context(), from, to)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Airing{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"airings": list})
}

func (s *Server) refreshGuide(w http.ResponseWriter, r *http.Request) {
	_, _, lastManual, err := s.Store.GuideSchedule(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ok, retry := guide.ManualAllowed(lastManual, time.Now()); !ok {
		mins := int(time.Until(retry).Minutes()) + 1
		if mins < 1 {
			mins = 1
		}
		apiError(w, http.StatusTooManyRequests, "guide_rate_limited", fmt.Sprintf("Listings were just refreshed. Try again in %d minutes.", mins), map[string]any{"retryAt": retry.UTC()})
		return
	}
	if err := s.Store.SetManualGuidePull(r.Context(), time.Now()); err != nil {
		writeError(w, err)
		return
	}
	n, err := s.RefreshGuide(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"airings": n})
}

// GuideDelay is how long the automatic refresh should wait. Zero means pull now.
func (s *Server) GuideDelay(now time.Time) time.Duration {
	_, next, _, err := s.Store.GuideSchedule(context.Background())
	if err != nil {
		return 0
	}
	return guide.Delay(next, now)
}

// DeferGuide schedules another attempt after a failed pull without counting it as a success.
func (s *Server) DeferGuide(ctx context.Context, after time.Duration) {
	_ = s.Store.SetNextGuidePull(ctx, time.Now().Add(after))
}

func (s *Server) RefreshGuide(ctx context.Context) (int, error) {
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, errors.New("no tuner")
	}
	raw, err := guide.Pull(ctx, s.HDHR, devices[0].BaseURL)
	if err != nil {
		return 0, err
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return 0, err
	}
	var antenna []store.Channel
	var ids []int64
	for _, ch := range channels {
		if strings.HasPrefix(ch.DeviceID, "src-") {
			continue
		}
		antenna = append(antenna, ch)
		ids = append(ids, ch.ID)
	}
	rows, art, err := guide.Parse(raw, antenna)
	if err != nil {
		return 0, err
	}
	_ = s.Store.SetChannelArt(ctx, art)
	settings, _ := s.Store.Settings(ctx)
	if settings == nil {
		settings = map[string]string{}
	}
	tmdbKey := strings.TrimSpace(settings["tmdbKey"])
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}
	rows = guide.FillImages(ctx, tmdbKey, rows)
	if err := s.Store.ReplaceAiringsFor(ctx, ids, rows); err != nil {
		return 0, err
	}
	user := strings.TrimSpace(settings["sdUser"])
	pass := settings["sdPassword"]
	lineup := strings.TrimSpace(settings["sdLineup"])
	if user == "" {
		user = strings.TrimSpace(os.Getenv("SD_USERNAME"))
	}
	if pass == "" {
		pass = os.Getenv("SD_PASSWORD")
	}
	if lineup == "" {
		lineup = strings.TrimSpace(os.Getenv("SD_LINEUP"))
	}
	if extra, _, err := guide.SchedulesDirect(ctx, antenna, user, pass, lineup); err == nil && len(extra) > 0 {
		rows = s.fillUnlisted(ctx, rows, extra)
	}
	if rawURL := strings.TrimSpace(settings["guideUrl"]); rawURL != "" {
		if body, err := guide.PullURL(ctx, rawURL); err == nil {
			if extra, extraArt, err := guide.Parse(body, antenna); err == nil && len(extra) > 0 {
				_ = s.Store.SetChannelArt(ctx, extraArt)
				extra = guide.FillImages(ctx, tmdbKey, extra)
				rows = s.fillUnlisted(ctx, rows, extra)
			}
		}
	}
	now := time.Now().UTC()
	span := int64(guide.PullMax - guide.PullMin)
	jitter := time.Duration(rand.Int64N(span + 1))
	next := guide.NextPull(now, jitter)
	if err := s.Store.SetGuideSchedule(ctx, now, next); err != nil {
		return len(rows), err
	}
	listed := map[int64]struct{}{}
	for _, row := range rows {
		listed[row.ChannelID] = struct{}{}
	}
	log.Printf("guide: source=silicondust-xmltv airings=%d channels=%d next=%s", len(rows), len(listed), next.Format(time.RFC3339))
	_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", len(rows)))
	return len(rows), nil
}

// fillUnlisted keeps the listings a channel already has and adds rows only for channels that have none.
func (s *Server) fillUnlisted(ctx context.Context, rows, extra []store.Airing) []store.Airing {
	have := map[int64]bool{}
	for _, row := range rows {
		have[row.ChannelID] = true
	}
	var fill []store.Airing
	for _, row := range extra {
		if !have[row.ChannelID] {
			fill = append(fill, row)
		}
	}
	if len(fill) == 0 {
		return rows
	}
	var fillIDs []int64
	seen := map[int64]bool{}
	for _, row := range fill {
		if !seen[row.ChannelID] {
			seen[row.ChannelID] = true
			fillIDs = append(fillIDs, row.ChannelID)
		}
	}
	_ = s.Store.ReplaceAiringsFor(ctx, fillIDs, fill)
	return append(rows, fill...)
}

func (s *Server) schedule(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	end := now.Add(14 * 24 * time.Hour)
	passes, err := s.Store.Passes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	airings, err := s.Store.Airings(r.Context(), now.Add(-time.Minute), end)
	if err != nil {
		writeError(w, err)
		return
	}
	items := dvr.Plan(passes, airings, s.tunerCount(r.Context()), now, end)
	recs, _ := s.Store.Recordings(r.Context())
	seen, _ := s.Store.SeenDeleted(r.Context())
	skips, _ := s.Store.Skips(r.Context())
	items = dvr.ApplyLibrary(items, passes, recs, seen, skips)
	if items == nil {
		items = []dvr.Planned{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tunerCount": s.tunerCount(r.Context()), "items": items})
}

func (s *Server) tunerCount(ctx context.Context) int {
	devices, err := s.Store.Devices(ctx)
	if err != nil || len(devices) == 0 {
		return 2
	}
	n := 0
	for _, device := range devices {
		n += device.TunerCount
	}
	if n < 1 {
		return 2
	}
	return n
}

func (s *Server) passes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Passes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Pass{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"passes": list})
}

func (s *Server) addPass(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title     string `json:"title"`
		ChannelID int64  `json:"channelId"`
		PadBefore *int   `json:"padBefore"`
		PadAfter  *int   `json:"padAfter"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Title) == "" {
		httpError(w, "title required", http.StatusBadRequest)
		return
	}
	before, after := 1, 2
	if body.PadBefore != nil {
		before = clampPad(*body.PadBefore)
	}
	if body.PadAfter != nil {
		after = clampPad(*body.PadAfter)
	}
	if err := s.Store.AddPass(r.Context(), strings.TrimSpace(body.Title), body.ChannelID, before, after); err != nil {
		writeError(w, err)
		return
	}
	s.passes(w, r)
}

func (s *Server) updatePass(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid pass", http.StatusBadRequest)
		return
	}
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	current, err := s.passByID(r.Context(), id)
	if err != nil {
		httpError(w, "pass not found", http.StatusNotFound)
		return
	}
	if v, ok := body["padBefore"]; ok {
		current.PadBefore = clampPad(int(num(v)))
	}
	if v, ok := body["padAfter"]; ok {
		current.PadAfter = clampPad(int(num(v)))
	}
	if v, ok := body["priority"]; ok {
		current.Priority = clampPriority(int(num(v)))
	}
	if v, ok := body["episodes"].(string); ok && v != "" {
		current.Episodes = v
	}
	if v, ok := body["keepMode"].(string); ok && v != "" {
		current.KeepMode = v
	}
	if v, ok := body["keepCount"]; ok {
		current.KeepCount = int(num(v))
	}
	if v, ok := body["limitCount"]; ok {
		current.LimitCount = int(num(v))
	}
	if v, ok := body["rerecord"].(bool); ok {
		current.Rerecord = v
	}
	if v, ok := body["commercials"].(bool); ok {
		current.Commercials = v
	}
	if v, ok := body["timeStart"].(string); ok {
		current.TimeStart = v
	}
	if v, ok := body["timeEnd"].(string); ok {
		current.TimeEnd = v
	}
	if v, ok := body["matchKind"].(string); ok && v != "" {
		current.MatchKind = v
	}
	if v, ok := body["channelId"]; ok {
		current.ChannelID = int64(num(v))
	}
	current.ID = id
	if err := s.Store.UpdatePassRules(r.Context(), current); err != nil {
		httpError(w, "pass not found", http.StatusNotFound)
		return
	}
	s.passes(w, r)
}

func (s *Server) passByID(ctx context.Context, id int64) (store.Pass, error) {
	list, err := s.Store.Passes(ctx)
	if err != nil {
		return store.Pass{}, err
	}
	for _, pass := range list {
		if pass.ID == id {
			return pass, nil
		}
	}
	return store.Pass{}, sql.ErrNoRows
}

func (s *Server) listingFor(ctx context.Context, channelID int64, title string) (store.Airing, bool) {
	now := time.Now()
	rows, err := s.Store.Airings(ctx, now.Add(-3*time.Hour), now.Add(8*time.Hour))
	if err != nil {
		return store.Airing{}, false
	}
	var current, next *store.Airing
	for i := range rows {
		row := &rows[i]
		if row.ChannelID != channelID {
			continue
		}
		if title != "" && strings.EqualFold(row.Title, title) && row.End.After(now) {
			return *row, true
		}
		if !now.Before(row.Start) && now.Before(row.End) && current == nil {
			current = row
		}
		if row.Start.After(now) && (next == nil || row.Start.Before(next.Start)) {
			next = row
		}
	}
	if current != nil {
		return *current, true
	}
	if next != nil && next.Start.Before(now.Add(2*time.Minute)) {
		return *next, true
	}
	return store.Airing{}, false
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func clampPriority(priority int) int {
	if priority < 0 {
		return 0
	}
	if priority > 100 {
		return 100
	}
	return priority
}

func clampPad(minutes int) int {
	if minutes < 0 {
		return 0
	}
	if minutes > 30 {
		return 30
	}
	return minutes
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/live/"), "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	channelID, err := strconv.ParseInt(parts[0], 10, 64)
	key, name := parts[1], parts[2]
	if _, ok := live.ParseRenditionKey(key); err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	s.Hub.Touch(channelID, key)
	w.Header().Set("Cache-Control", "no-cache")
	if name == "index.m3u8" {
		body, err := s.Hub.Playlist(channelID, key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	var contentType string
	switch {
	case strings.Contains(name, ".."):
	case name == "init.mp4":
		contentType = "video/mp4"
	case strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".m4s"):
		contentType = "video/iso.segment"
	case strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts"):
		contentType = "video/mp2t"
	}
	if contentType == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Hub.Dir, "live", parts[0], key, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeFile(w, r, path)
}

func decodeJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dest)
}

// waitPlaylist returns once a live playlist has three segments; the first is
// withheld, so players still get two to start from.
func waitPlaylist(path string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if body, err := os.ReadFile(path); err == nil && strings.Count(string(body), "#EXTINF") >= 3 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
