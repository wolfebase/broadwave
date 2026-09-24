package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/discovery"
	"waveguide/internal/hdhr"
	"waveguide/internal/live"
	"waveguide/internal/realtime"
	"waveguide/internal/source"
	"waveguide/internal/sports"
	"waveguide/internal/store"
)

type Server struct {
	Store   *store.Store
	HDHR    *hdhr.Client
	Hub     *live.Hub
	Assets  fs.FS
	Dev     bool
	Version string
	Bus     *realtime.Bus
	Sports  sports.Provider

	routes []string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		s.routes = append(s.routes, pattern)
		mux.HandleFunc(method+" /api/v1"+path, h)
		mux.HandleFunc(method+" /api"+path, h)
	}
	api("GET /health", s.health)
	api("GET /server", s.serverInfo)
	api("PATCH /server", s.renameServer)
	api("GET /clock", s.clock)
	api("GET /diagnostics", s.diagnostics)
	api("GET /ws", s.socket)
	api("GET /profile", s.profile)
	api("GET /devices", s.devices)
	api("POST /devices/{id}/scan", s.startScan)
	api("GET /devices/{id}/scan", s.scanStatus)
	api("POST /sources/discover", s.discover)
	api("POST /sources/look", s.look)
	api("GET /sources", s.listSources)
	api("POST /sources", s.addSource)
	api("GET /channels", s.channels)
	api("PATCH /channels/{id}", s.patchChannel)
	api("GET /channels/{id}/frame", s.frame)
	api("GET /settings", s.getSettings)
	api("PUT /settings", s.putSettings)
	api("POST /watch", s.watch)
	api("POST /multiview/plan", s.multiviewPlan)
	api("POST /watch/{id}/stop", s.release)
	api("GET /tuners", s.tuners)
	api("POST /recordings", s.startRecording)
	api("POST /recordings/{id}/stop", s.stopRecording)
	api("GET /recordings", s.recordings)
	api("DELETE /recordings/{id}", s.deleteRecording)
	api("GET /recordings/{id}/file", s.downloadRecording)
	api("GET /airings", s.airings)
	api("GET /schedule", s.schedule)
	api("POST /schedule/skip", s.skipAiring)
	api("GET /events", s.events)
	api("POST /guide/refresh", s.refreshGuide)
	api("GET /search", s.search)
	api("GET /sports/scoreboard", s.scoreboard)
	api("GET /teams", s.teams)
	api("PUT /teams", s.teams)
	api("DELETE /teams/{id}", s.unfollowTeam)
	api("GET /passes", s.passes)
	api("POST /passes", s.addPass)
	api("PATCH /passes/{id}", s.updatePass)
	api("DELETE /passes/{id}", s.deletePass)
	api("POST /recordings/{id}/play", s.playRecording)
	api("PUT /recordings/{id}/progress", s.saveProgress)
	api("PUT /recordings/{id}/watched", s.setWatched)
	api("GET /recordings/{id}/markers", s.markers)
	api("POST /recordings/{id}/markers", s.addMarker)
	api("DELETE /markers/{id}", s.deleteMarker)
	api("POST /recordings/{id}/detect", s.detectBreaks)
	api("GET /virtuals", s.virtuals)
	api("POST /virtuals", s.createVirtual)
	api("PATCH /virtuals/{id}", s.updateVirtual)
	api("GET /virtuals/schedule", s.virtualSchedule)
	api("POST /virtuals/{id}/play", s.playVirtual)
	api("GET /storage", s.storage)
	api("GET /backup", s.backup)
	api("POST /backup", s.restore)
	mux.HandleFunc("GET /export/lineup.m3u", s.exportLineup)
	mux.HandleFunc("GET /export/guide.xml", s.exportGuide)
	mux.HandleFunc("GET /export/stream/{id}", s.exportStream)
	mux.HandleFunc("GET /media/live/", s.media)
	mux.HandleFunc("GET /media/file/", s.fileMedia)
	mux.HandleFunc("GET /media/poster/{id}", s.poster)
	mux.HandleFunc("GET /media/art/{kind}/{id}", s.art)
	mux.HandleFunc("GET /", s.ui)
	return s.withDevCORS(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "waveguide"})
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
	type noted struct {
		store.Device
		Note string `json:"note,omitempty"`
	}
	out := make([]noted, 0, len(devices))
	for _, device := range devices {
		out = append(out, noted{Device: device, Note: hdhr.ModelNote(device.ModelNumber)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *Server) deviceBase(r *http.Request) (string, error) {
	id := r.PathValue("id")
	devices, err := s.Store.Devices(r.Context())
	if err != nil {
		return "", err
	}
	for _, device := range devices {
		if device.DeviceID == id && device.BaseURL != "" {
			return device.BaseURL, nil
		}
	}
	return "", fmt.Errorf("no tuner named %s", id)
}

func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	base, err := s.deviceBase(r)
	if err != nil {
		httpError(w, "That tuner is not in the lineup.", http.StatusNotFound)
		return
	}
	client := s.HDHR
	if client == nil {
		client = &hdhr.Client{}
	}
	if err := client.StartScan(r.Context(), base); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scanning": true})
}

func (s *Server) scanStatus(w http.ResponseWriter, r *http.Request) {
	base, err := s.deviceBase(r)
	if err != nil {
		httpError(w, "That tuner is not in the lineup.", http.StatusNotFound)
		return
	}
	client := s.HDHR
	if client == nil {
		client = &hdhr.Client{}
	}
	prog, err := client.ScanProgress(r.Context(), base)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scanning": prog.Scan, "found": prog.Found})
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP string `json:"ip"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			httpError(w, "invalid json", http.StatusBadRequest)
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

func (s *Server) look(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	found := discovery.Look(ctx, discovery.LocalHosts())
	browseCtx, stopBrowse := context.WithTimeout(ctx, 2*time.Second)
	for _, service := range []string{"_channels_dvr._tcp", "_htsp._tcp"} {
		extra, _ := discovery.Browse(browseCtx, service)
		found = append(found, extra...)
	}
	stopBrowse()
	if len(found) == 0 {
		found = discovery.ConfirmCloud(ctx)
	}
	if found == nil {
		found = []discovery.Found{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": found})
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
	writeCachedJSON(w, r, http.StatusOK, map[string]any{
		"channels": channels,
		"listings": "empty",
		"message":  "Listings turn on when you add an XMLTV file. Live picture arrives with the player.",
	})
}

func (s *Server) patchChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		Favorite     *bool   `json:"favorite"`
		Enabled      *bool   `json:"enabled"`
		Hidden       *bool   `json:"hidden"`
		CustomName   *string `json:"customName"`
		CustomNumber *string `json:"customNumber"`
		GuideKey     *string `json:"guideKey"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	ch, err := s.Store.PatchChannel(r.Context(), id, store.ChannelPatch{
		Favorite: body.Favorite, Enabled: body.Enabled, Hidden: body.Hidden,
		CustomName: body.CustomName, CustomNumber: body.CustomNumber, GuideKey: body.GuideKey,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpError(w, "channel not found", http.StatusNotFound)
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
	if values["hideScores"] == "" {
		values["hideScores"] = "0"
	}
	if strings.TrimSpace(values["sdPassword"]) != "" {
		values["sdPasswordSet"] = "1"
	} else {
		values["sdPasswordSet"] = "0"
	}
	delete(values, "sdPassword")
	if strings.TrimSpace(values["tmdbKey"]) != "" {
		values["tmdbKeySet"] = "1"
	} else {
		values["tmdbKeySet"] = "0"
	}
	delete(values, "tmdbKey")
	needs, err := s.Store.ApplySetupDefault(r.Context(), time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	if needs {
		values["needsSetup"] = "1"
	} else {
		values["needsSetup"] = "0"
		values["setupComplete"] = "1"
	}
	writeCachedJSON(w, r, http.StatusOK, values)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.Store.PutSettings(r.Context(), body); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.getSettings(w, r)
}

func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if s.Assets == nil {
		httpError(w, "site is not built", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	// Bundles are content-hashed, so they never change; the page that names them always can.
	if strings.HasPrefix(path, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if serveFile(w, r, s.Assets, path) {
		return
	}
	if strings.Contains(path, ".") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if !serveFile(w, r, s.Assets, "index.html") {
		http.NotFound(w, r)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, assets fs.FS, path string) bool {
	if enc, ext := compressedEncoding(r); ext != "" {
		if body, err := fs.ReadFile(assets, path+"."+ext); err == nil {
			w.Header().Set("Content-Encoding", enc)
			w.Header().Set("Vary", "Accept-Encoding")
			w.Header().Set("Content-Type", contentType(path))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return true
		}
	}
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

// compressedEncoding prefers brotli, then gzip, when the browser accepts it.
func compressedEncoding(r *http.Request) (encoding, ext string) {
	ae := r.Header.Get("Accept-Encoding")
	if strings.Contains(ae, "br") {
		return "br", "br"
	}
	if strings.Contains(ae, "gzip") {
		return "gzip", "gz"
	}
	return "", ""
}

func contentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

func (s *Server) withDevCORS(next http.Handler) http.Handler {
	if !s.Dev {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, If-None-Match")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "ETag")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := marshalJSON(v)
	if err != nil {
		http.Error(w, "json", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeCachedJSON sets an ETag and gzips when the client accepts it.
// A matching If-None-Match returns 304. The tag is the body, so gzip and plain share it.
func writeCachedJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := marshalJSON(v)
	if err != nil {
		http.Error(w, "json", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var buf bytes.Buffer
		gz, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
		_, _ = gz.Write(body)
		_ = gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(buf.Bytes())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func marshalJSON(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
