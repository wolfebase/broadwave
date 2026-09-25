package httpapi

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func (s *Server) playRecording(w http.ResponseWriter, r *http.Request) {
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
	if s.Hub == nil {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Picture string `json:"pictureMode"`
	}
	_ = decodeJSON(r, &body)
	codec, mode, order := s.playbackChoice(r.Context(), rec.ChannelID, body.Picture)
	var playlist string
	if rec.Status == "recording" {
		playlist, err = s.Hub.PlayFollow(id, rec.Path, codec, mode, order, func() bool {
			cur, curErr := s.Store.Recording(context.Background(), id)
			return curErr == nil && cur.Status == "recording"
		})
	} else {
		playlist, err = s.Hub.PlayFile(id, rec.Path, codec, mode, order)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	markers, _ := s.Store.Markers(r.Context(), id)
	if markers == nil {
		markers = []store.Marker{}
	}
	position, _ := s.Store.Progress(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"playlist":  playlist,
		"recording": rec,
		"markers":   markers,
		"position":  position,
		"growing":   rec.Status == "recording",
	})
}

func (s *Server) saveProgress(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	var body struct {
		Position float64 `json:"position"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Position < 0 {
		httpError(w, "position required", http.StatusBadRequest)
		return
	}
	if _, err := s.Store.Recording(r.Context(), id); err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	if err := s.Store.SaveProgress(r.Context(), id, body.Position); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"position": body.Position})
}

func (s *Server) markers(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	list, err := s.Store.Markers(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Marker{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"markers": list})
}

func (s *Server) addMarker(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	var body struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	if err := decodeJSON(r, &body); err != nil || body.End <= body.Start {
		httpError(w, "start and end required", http.StatusBadRequest)
		return
	}
	marker, err := s.Store.AddMarker(r.Context(), id, body.Start, body.End)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, marker)
	s.writeEDL(r.Context(), id)
}

func (s *Server) deleteMarker(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid marker", http.StatusBadRequest)
		return
	}
	recordingID, err := s.Store.DeleteMarker(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	s.writeEDL(r.Context(), recordingID)
}

func (s *Server) detectBreaks(w http.ResponseWriter, r *http.Request) {
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
	found, err := live.DetectBreaks(s.Hub.FFmpeg, rec.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	rows := make([]store.Marker, 0, len(found))
	for _, item := range found {
		rows = append(rows, store.Marker{Start: item.Start, End: item.End})
	}
	if err := s.Store.ReplaceMarkers(r.Context(), id, rows); err != nil {
		writeError(w, err)
		return
	}
	list, _ := s.Store.Markers(r.Context(), id)
	if list == nil {
		list = []store.Marker{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"markers": list})
	s.writeEDL(r.Context(), id)
}

func (s *Server) deletePass(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid pass", http.StatusBadRequest)
		return
	}
	if err := s.Store.DeletePass(r.Context(), id); err != nil {
		httpError(w, "pass not found", http.StatusNotFound)
		return
	}
	s.passes(w, r)
}

func (s *Server) downloadRecording(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	rec, err := s.Store.Recording(r.Context(), id)
	if err != nil || rec.Path == "" {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	clean := filepath.Clean(rec.Path)
	if !strings.EqualFold(filepath.Ext(clean), ".ts") {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(clean)+`"`)
	http.ServeFile(w, r, clean)
}

func (s *Server) playVirtual(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		Index   int    `json:"index"`
		Picture string `json:"pictureMode"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Index < 0 {
		httpError(w, "index required", http.StatusBadRequest)
		return
	}
	channel, err := s.Store.Virtual(r.Context(), id)
	if err != nil {
		httpError(w, "channel not found", http.StatusNotFound)
		return
	}
	if body.Index >= len(channel.Recordings) {
		httpError(w, "this channel has no recording at that position", http.StatusBadRequest)
		return
	}
	if s.Hub == nil {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	rec, err := s.Store.Recording(r.Context(), channel.Recordings[body.Index])
	if err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	codec, mode, order := s.playbackChoice(r.Context(), rec.ChannelID, body.Picture)
	playlist, err := s.Hub.PlayFile(rec.ID, rec.Path, codec, mode, order)
	if err != nil {
		writeError(w, err)
		return
	}
	markers, _ := s.Store.Markers(r.Context(), rec.ID)
	if markers == nil {
		markers = []store.Marker{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"usesTuner": false,
		"index":     body.Index,
		"count":     len(channel.Recordings),
		"playlist":  playlist,
		"recording": rec,
		"markers":   markers,
		"number":    channel.Number,
		"name":      channel.Name,
	})
}

func (s *Server) virtuals(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Virtuals(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.VirtualChannel{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"virtuals": list})
}

func (s *Server) createVirtual(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Number     string  `json:"number"`
		Name       string  `json:"name"`
		Recordings []int64 `json:"recordings"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpError(w, "name required", http.StatusBadRequest)
		return
	}
	if body.Number == "" {
		body.Number = "900"
	}
	created, err := s.Store.CreateVirtual(r.Context(), body.Number, strings.TrimSpace(body.Name), body.Recordings)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (s *Server) writeEDL(ctx context.Context, recordingID int64) {
	if recordingID == 0 {
		return
	}
	rec, err := s.Store.Recording(ctx, recordingID)
	if err != nil {
		return
	}
	markers, err := s.Store.Markers(ctx, recordingID)
	if err != nil {
		return
	}
	_ = live.WriteEDL(rec.Path, markers)
}

func (s *Server) storage(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || s.Hub.Dir == "" {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	dir := filepath.Join(s.Hub.Dir, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, err)
		return
	}
	space, err := disk.Stat(dir)
	if err != nil {
		writeError(w, err)
		return
	}
	raw := ""
	if values, err := s.Store.Settings(r.Context()); err == nil {
		raw = values["watermarkGB"]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"freeBytes":   space.Free,
		"totalBytes":  space.Total,
		"watermarkGB": disk.WatermarkGB(raw),
	})
}

func (s *Server) playbackChoice(ctx context.Context, channelID int64, requested string) (codec, mode, order string) {
	mode = live.NormalizeMode(requested)
	if strings.TrimSpace(requested) == "" {
		if values, err := s.Store.Settings(ctx); err == nil {
			mode = live.NormalizeMode(values["pictureMode"])
		}
	}
	if channelID != 0 {
		if ch, err := s.Store.SourceChannel(ctx, channelID); err == nil {
			codec = ch.VideoCodec
			order = ch.FieldOrder
		}
	}
	return codec, mode, order
}

func (s *Server) poster(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || s.Hub.FFmpeg == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rec, err := s.Store.Recording(r.Context(), id)
	if err != nil || rec.Path == "" {
		http.NotFound(w, r)
		return
	}
	dest := filepath.Join(s.Hub.Dir, "posters", strconv.FormatInt(id, 10)+".jpg")
	if err := live.EnsurePoster(s.Hub.FFmpeg, rec.Path, dest); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, dest)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.Hub.Dir, "backup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, err)
		return
	}
	path := filepath.Join(dir, "broadwave-backup.db")
	_ = os.Remove(path)
	if err := s.Store.BackupTo(r.Context(), path); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="broadwave-backup.db"`)
	http.ServeFile(w, r, path)
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	path := filepath.Join(s.Hub.Dir, "backup", "restore.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeError(w, err)
		return
	}
	_ = os.Remove(path)
	f, err := os.Create(path)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := io.Copy(f, io.LimitReader(r.Body, 64<<20)); err != nil {
		f.Close()
		writeError(w, err)
		return
	}
	f.Close()
	if err := s.Store.RestoreFrom(r.Context(), path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fileMedia(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/media/file/")
	id, name, ok := strings.Cut(rel, "/")
	if !ok || strings.Contains(id, "..") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	if name != "index.m3u8" && name != "captions.vtt" && !(strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts")) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Hub.Dir, "file", id, filepath.Base(name))
	switch {
	case strings.HasSuffix(name, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case strings.HasSuffix(name, ".vtt"):
		w.Header().Set("Content-Type", "text/vtt")
	}
	http.ServeFile(w, r, path)
}
