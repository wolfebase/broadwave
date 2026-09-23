package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ota-viewer/internal/hdhr"
	"ota-viewer/internal/live"
	"ota-viewer/internal/source"
	"ota-viewer/internal/store"
)

type Server struct {
	Store  *store.Store
	HDHR   *hdhr.Client
	Hub    *live.Hub
	Assets fs.FS
	Dev    bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/profile", s.profile)
	mux.HandleFunc("GET /api/devices", s.devices)
	mux.HandleFunc("POST /api/sources/discover", s.discover)
	mux.HandleFunc("GET /api/sources", s.listSources)
	mux.HandleFunc("POST /api/sources", s.addSource)
	mux.HandleFunc("GET /api/channels", s.channels)
	mux.HandleFunc("PATCH /api/channels/{id}", s.patchChannel)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/watch", s.watch)
	mux.HandleFunc("POST /api/watch/{id}/stop", s.release)
	mux.HandleFunc("GET /api/tuners", s.tuners)
	mux.HandleFunc("POST /api/recordings", s.startRecording)
	mux.HandleFunc("POST /api/recordings/{id}/stop", s.stopRecording)
	mux.HandleFunc("GET /api/recordings", s.recordings)
	mux.HandleFunc("DELETE /api/recordings/{id}", s.deleteRecording)
	mux.HandleFunc("GET /api/recordings/{id}/file", s.downloadRecording)
	mux.HandleFunc("GET /api/airings", s.airings)
	mux.HandleFunc("GET /api/schedule", s.schedule)
	mux.HandleFunc("POST /api/schedule/skip", s.skipAiring)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/guide/refresh", s.refreshGuide)
	mux.HandleFunc("GET /api/passes", s.passes)
	mux.HandleFunc("POST /api/passes", s.addPass)
	mux.HandleFunc("PATCH /api/passes/{id}", s.updatePass)
	mux.HandleFunc("DELETE /api/passes/{id}", s.deletePass)
	mux.HandleFunc("POST /api/recordings/{id}/play", s.playRecording)
	mux.HandleFunc("PUT /api/recordings/{id}/progress", s.saveProgress)
	mux.HandleFunc("PUT /api/recordings/{id}/watched", s.setWatched)
	mux.HandleFunc("GET /api/recordings/{id}/markers", s.markers)
	mux.HandleFunc("POST /api/recordings/{id}/markers", s.addMarker)
	mux.HandleFunc("DELETE /api/markers/{id}", s.deleteMarker)
	mux.HandleFunc("POST /api/recordings/{id}/detect", s.detectBreaks)
	mux.HandleFunc("GET /api/virtuals", s.virtuals)
	mux.HandleFunc("POST /api/virtuals", s.createVirtual)
	mux.HandleFunc("PATCH /api/virtuals/{id}", s.updateVirtual)
	mux.HandleFunc("GET /api/virtuals/schedule", s.virtualSchedule)
	mux.HandleFunc("POST /api/virtuals/{id}/play", s.playVirtual)
	mux.HandleFunc("GET /api/storage", s.storage)
	mux.HandleFunc("GET /api/backup", s.backup)
	mux.HandleFunc("POST /api/backup", s.restore)
	mux.HandleFunc("GET /media/live/", s.media)
	mux.HandleFunc("GET /media/file/", s.fileMedia)
	mux.HandleFunc("GET /media/poster/{id}", s.poster)
	mux.HandleFunc("GET /", s.ui)
	return s.withDevCORS(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "ota-viewer"})
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":   1,
		"name": "Home",
		"auth": "local-open",
		"note": "A password is required before this server is opened beyond your home network. That arrives with accounts.",
	})
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.Store.Devices(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if devices == nil {
		devices = []store.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP string `json:"ip"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	n, err := source.Sync(ctx, s.Store, s.HDHR, body.IP)
	if err != nil {
		writeError(w, err)
		return
	}
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	if devices == nil {
		devices = []store.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices, "found": n})
}

func (s *Server) channels(w http.ResponseWriter, r *http.Request) {
	guideOnly := r.URL.Query().Get("guide") == "1"
	channels, err := s.Store.Channels(r.Context(), guideOnly)
	if err != nil {
		writeError(w, err)
		return
	}
	if channels == nil {
		channels = []store.Channel{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channels": channels,
		"listings": "empty",
		"message":  "Listings turn on when you add an XMLTV file. Live picture arrives with the player.",
	})
}

func (s *Server) patchChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		Favorite     *bool   `json:"favorite"`
		Enabled      *bool   `json:"enabled"`
		Hidden       *bool   `json:"hidden"`
		CustomName   *string `json:"customName"`
		CustomNumber *string `json:"customNumber"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	ch, err := s.Store.PatchChannel(r.Context(), id, store.ChannelPatch{
		Favorite: body.Favorite, Enabled: body.Enabled, Hidden: body.Hidden,
		CustomName: body.CustomName, CustomNumber: body.CustomNumber,
	})
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	values, err := s.Store.Settings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if values["layout"] == "" {
		values["layout"] = "auto"
	}
	if _, ok := values["recordingsPath"]; !ok {
		values["recordingsPath"] = ""
	}
	if values["profile"] == "" {
		values["profile"] = "transparent"
	}
	if values["audio"] == "" {
		values["audio"] = "stereo"
	}
	if strings.TrimSpace(values["watermarkGB"]) == "" {
		values["watermarkGB"] = "10"
	}
	if values["pictureMode"] == "" {
		values["pictureMode"] = "broadcast"
	}
	if values["autoplay"] == "" {
		values["autoplay"] = "1"
	}
	if values["hdhrEmulate"] == "" {
		values["hdhrEmulate"] = "0"
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.Store.PutSettings(r.Context(), body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.getSettings(w, r)
}

func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if s.Assets == nil {
		http.Error(w, "site is not built", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if serveFile(w, r, s.Assets, path) {
		return
	}
	if strings.Contains(path, ".") {
		http.NotFound(w, r)
		return
	}
	if !serveFile(w, r, s.Assets, "index.html") {
		http.NotFound(w, r)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, assets fs.FS, path string) bool {
	f, err := assets.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, path, stat.ModTime(), rs)
		return true
	}
	body, err := io.ReadAll(f)
	if err != nil {
		return false
	}
	http.ServeContent(w, r, path, stat.ModTime(), bytes.NewReader(body))
	return true
}

func (s *Server) withDevCORS(next http.Handler) http.Handler {
	if !s.Dev {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadGateway)
}
