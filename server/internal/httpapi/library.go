package httpapi

import (
	"broadwave/internal/dvr"
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/nfo"
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
	if _, ok := recordingInside(s.downloadRoots(r.Context()), rec.Path); !ok {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	codec, mode, order := s.playbackChoice(r.Context(), rec.ChannelID, body.Picture)
	position, _ := s.Store.Progress(r.Context(), id)
	var playlist string
	if rec.Status == "recording" {
		if rec.Duration <= 0 && !rec.StartedAt.IsZero() {
			if elapsed := time.Since(rec.StartedAt).Seconds(); elapsed > 0 {
				rec.Duration = elapsed
			}
		}
		playlist, err = s.Hub.PlayFollow(id, rec.Path, codec, mode, order, live.ResumeAt(position, rec.Duration), func() bool {
			cur, curErr := s.Store.Recording(context.Background(), id)
			return curErr == nil && cur.Status == "recording"
		})
	} else {
		if rec.Duration <= 0 {
			if dur := recordingDuration(s.Hub, rec); dur > 0 {
				rec.Duration = dur
				_ = s.Store.SetDuration(r.Context(), id, dur)
			}
		}
		playlist, err = s.Hub.PlayFile(id, rec.Path, codec, mode, order, live.ResumeAt(position, rec.Duration))
	}
	if err != nil {
		writeError(w, err)
		return
	}
	markers, _ := s.Store.Markers(r.Context(), id)
	if markers == nil {
		markers = []store.Marker{}
	}
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

// recordAgain names the next airing of a recording's episode in the guide,
// for Record it again on a damaged copy. Airing is null when the guide has
// none, or the episode has no id or name to match.
func (s *Server) recordAgain(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"airing": s.nextAiring(r.Context(), rec, time.Now())})
}

func (s *Server) nextAiring(ctx context.Context, rec store.Recording, now time.Time) *store.Airing {
	key := store.EpisodeKey(rec.ProgramID, rec.Title, rec.Subtitle, rec.ChannelID)
	if key == "" {
		return nil
	}
	airings, err := s.Store.RecordingAirings(ctx, now, now.Add(15*24*time.Hour))
	if err != nil {
		return nil
	}
	chs, err := s.Store.Channels(ctx, false)
	if err != nil {
		return nil
	}
	off := map[int64]bool{}
	for _, ch := range chs {
		off[ch.ID] = !ch.Enabled
	}
	// A simulcast copy records on its other channel; that row is in the list too.
	for i := range airings {
		air := airings[i]
		if air.Start.After(now) && air.Simulcast == 0 && !off[air.ChannelID] && store.EpisodeKey(air.ProgramID, air.Title, air.Subtitle, air.ChannelID) == key {
			return &air
		}
	}
	return nil
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
	list, err := dvr.IndexBreaks(r.Context(), s.Store, s.Hub.FFmpeg, rec, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Marker{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"markers": list})
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
	// rec.Path is stored data. A ".." or a symlink must not leave the
	// recordings folder or a library folder the owner added.
	path, ok := recordingInside(s.downloadRoots(r.Context()), rec.Path)
	if !ok {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", downloadTypes[strings.ToLower(filepath.Ext(path))])
	w.Header().Set("Content-Disposition", attachmentDisposition(filepath.Base(path)))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

// downloadTypes are the files a recording or a library folder item can be.
var downloadTypes = map[string]string{
	".ts":  "video/mp2t",
	".mp4": "video/mp4",
	".m4v": "video/x-m4v",
	".mkv": "video/x-matroska",
	".mov": "video/quicktime",
}

func (s *Server) downloadRoots(ctx context.Context) []string {
	return mediaRoots(ctx, s.Store, s.Hub)
}

// mediaRoots are the folders a recording's file may be opened from: the
// recordings folder, the default one recordings made before it was moved
// stay in, and each library folder source.
func mediaRoots(ctx context.Context, st *store.Store, hub *live.Hub) []string {
	var roots []string
	if hub != nil && hub.Dir != "" {
		roots = append(roots, hub.Recordings())
		if old := filepath.Join(hub.Dir, "recordings"); old != hub.Recordings() {
			roots = append(roots, old)
		}
	}
	sources, _ := st.Sources(ctx)
	for _, src := range sources {
		if src.Kind == "folder" && strings.TrimSpace(src.URL) != "" {
			roots = append(roots, strings.TrimSpace(src.URL))
		}
	}
	return roots
}

// recordingInside is the cleaned path when it is a media file inside one of
// roots. A missing file still counts, so the caller can answer 404. A symlink
// that resolves outside every root does not.
func recordingInside(roots []string, stored string) (string, bool) {
	if stored == "" {
		return "", false
	}
	clean := filepath.Clean(stored)
	if downloadTypes[strings.ToLower(filepath.Ext(clean))] == "" {
		return "", false
	}
	for _, root := range roots {
		if pathInside(root, clean) {
			return clean, true
		}
	}
	return "", false
}

func pathInside(root, candidate string) bool {
	return nfo.Inside(root, candidate)
}

// attachmentDisposition is one header field. A quote or a newline in the
// file name is encoded, so it cannot start a second line or close the name.
func attachmentDisposition(name string) string {
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." {
		name = "recording.ts"
	}
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
		return v
	}
	return `attachment; filename="recording.ts"`
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
	if _, ok := recordingInside(s.downloadRoots(r.Context()), rec.Path); !ok {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	codec, mode, order := s.playbackChoice(r.Context(), rec.ChannelID, body.Picture)
	playlist, err := s.Hub.PlayFile(rec.ID, rec.Path, codec, mode, order, 0)
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
	dir := s.Hub.Recordings()
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
		"path":        dir,
		"freeBytes":   space.Free,
		"totalBytes":  space.Total,
		"watermarkGB": disk.WatermarkGB(raw),
	})
}

// recordingDuration is the file's length in seconds. A stored length wins.
// Zero means unknown, and a resume then starts at the beginning rather than
// seeking past the end of a short file.
func recordingDuration(hub *live.Hub, rec store.Recording) float64 {
	if rec.Duration > 0 {
		return rec.Duration
	}
	if hub == nil || rec.Path == "" || hub.FFmpeg == "" {
		return 0
	}
	tool := live.FFProbePath(hub.FFmpeg)
	if tool == "" {
		return 0
	}
	got, err := live.ProbeDuration(tool, rec.Path)
	if err != nil || got <= 0 {
		return 0
	}
	return got
}

// nullPackets is a short MPEG-TS of nothing. Gap segments are not encoded;
// this answers a player that asks for one anyway.
func nullPackets() []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = 0x1F
	pkt[2] = 0xFF
	pkt[3] = 0x10
	out := make([]byte, 0, len(pkt)*8)
	for range 8 {
		out = append(out, pkt...)
	}
	return out
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
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, ok := recordingInside(s.downloadRoots(r.Context()), rec.Path); !ok {
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
	gap := strings.HasPrefix(name, "gap") && strings.HasSuffix(name, ".ts")
	if name != "index.m3u8" && name != "captions.vtt" && !gap && !(strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts")) {
		http.NotFound(w, r)
		return
	}
	if s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Hub.Dir, "file", id, filepath.Base(name))
	switch {
	case gap:
		// The resume point is covered by gap segments the encode never wrote.
		// A player that still asks for one gets empty transport packets, not a 404.
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(nullPackets())
		return
	case strings.HasSuffix(name, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		body, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		dir := filepath.Dir(path)
		body = live.OffsetPlaylist(body, live.FileOffset(dir))
		if recID, err := strconv.ParseInt(id, 10, 64); err == nil && s.Store != nil {
			if rec, err := s.Store.Recording(r.Context(), recID); err == nil {
				body = stampRecordingPlaylist(body, rec.StartedAt)
			}
		}
		body = s.Hub.StableRecordingPlaylist(dir, body)
		_, _ = w.Write(body)
		return
	case strings.HasSuffix(name, ".vtt"):
		w.Header().Set("Content-Type", "text/vtt")
	}
	if strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts") {
		// A new resume replaces seg00000. A cached copy would be the previous picture.
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, path)
}

// stampRecordingPlaylist sets EXT-X-PROGRAM-DATE-TIME from the recording's
// start plus each segment's offset. ffmpeg writes the wall clock of the
// transcode (when play was pressed) because PictureArgs asks for
// program_date_time; AVKit reads those tags as the on-screen date.
func stampRecordingPlaylist(body []byte, start time.Time) []byte {
	if start.IsZero() || !strings.Contains(string(body), "#EXTINF:") {
		return body
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	var out strings.Builder
	at := start.UTC()
	var pending []string
	var dur float64
	writePending := func() {
		for _, l := range pending {
			out.WriteString(l)
			out.WriteByte('\n')
		}
		pending = pending[:0]
		dur = 0
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#EXT-X-PROGRAM-DATE-TIME:") {
			continue
		}
		if trim == "#EXT-X-GAP" {
			pending = append(pending, line)
			continue
		}
		if strings.HasPrefix(trim, "#EXTINF:") {
			pending = append(pending, line)
			if v, ok := strings.CutPrefix(trim, "#EXTINF:"); ok {
				sec, err := strconv.ParseFloat(strings.SplitN(v, ",", 2)[0], 64)
				if err == nil && sec > 0 {
					dur += sec
				}
			}
			continue
		}
		if trim != "" && !strings.HasPrefix(trim, "#") && len(pending) > 0 {
			out.WriteString("#EXT-X-PROGRAM-DATE-TIME:")
			out.WriteString(at.Format("2006-01-02T15:04:05.000Z"))
			out.WriteByte('\n')
			seg := dur
			writePending()
			out.WriteString(line)
			out.WriteByte('\n')
			at = at.Add(time.Duration(seg * float64(time.Second)))
			continue
		}
		writePending()
		out.WriteString(line)
		out.WriteByte('\n')
	}
	writePending()
	return []byte(out.String())
}
