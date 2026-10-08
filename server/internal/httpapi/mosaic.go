package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"broadwave/internal/live"
)

// mosaicReady is how long a mosaic watch waits for the first segment, so a
// player that loads the playlist at once finds media in it.
const mosaicReady = 20 * time.Second

// watchMosaic starts or joins one stream of two to four channels side by
// side, with the first channel's sound.
func (s *Server) watchMosaic(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		httpError(w, "Live TV is not set up on this server.", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ChannelIDs []int64 `json:"channelIds"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if _, err := live.MosaicKey(body.ChannelIDs); err != nil {
		apiError(w, http.StatusBadRequest, "bad_request", "Pick 2 to 4 different channels.", nil)
		return
	}
	session, err := s.Hub.WatchMosaic(r.Context(), body.ChannelIDs)
	if err != nil {
		watchError(w, err)
		return
	}
	deadline := time.Now().Add(mosaicReady)
	for {
		if body, err := s.Hub.MosaicPlaylist(session.Key); err == nil && strings.Contains(string(body), "#EXTINF") {
			break
		}
		if r.Context().Err() != nil {
			s.Hub.ReleaseMosaic(session.Key)
			return
		}
		if time.Now().After(deadline) {
			s.Hub.ReleaseMosaic(session.Key)
			apiError(w, http.StatusServiceUnavailable, "stream_down", "The mosaic did not start. The server log says why.", nil)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) releaseMosaic(w http.ResponseWriter, r *http.Request) {
	if s.Hub != nil {
		if _, err := live.ParseMosaicKey(r.PathValue("key")); err == nil {
			s.Hub.ReleaseMosaic(r.PathValue("key"))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// mosaicMedia serves a running mosaic's playlist and media. It starts
// nothing: a watch does.
func (s *Server) mosaicMedia(w http.ResponseWriter, r *http.Request) {
	key, name := r.PathValue("key"), r.PathValue("name")
	if s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	if _, err := live.ParseMosaicKey(key); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if name == "index.m3u8" {
		if msn, part, ok := blockReload(r); ok {
			s.Hub.WaitMosaic(key, msn, part)
		}
		body, err := s.Hub.MosaicPlaylist(key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("_HLS_skip") {
		case "YES", "v2":
			body = live.DeltaPlaylist(body)
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	var contentType string
	switch {
	case name == "init.mp4":
		contentType = "video/mp4"
	case (strings.HasPrefix(name, "seg") || strings.HasPrefix(name, "part")) && strings.HasSuffix(name, ".m4s"):
		contentType = "video/iso.segment"
	}
	path, ok := s.Hub.MosaicFile(key, name)
	if contentType == "" || !ok {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(path); err != nil {
		body, ok := live.PartFromSegment(path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, path)
}

// exportMosaic is the mosaic as MPEG-TS, for apps that take a stream URL.
func (s *Server) exportMosaic(w http.ResponseWriter, r *http.Request) {
	ids, err := live.ParseMosaicKey(r.PathValue("key"))
	if err != nil || s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	out := &wroteWriter{w: flushWriter{w}}
	err = s.Hub.ExportMosaic(r.Context(), ids, out)
	if err == nil {
		return
	}
	slog.Warn("mosaic export: " + err.Error())
	if out.wrote {
		return
	}
	var busy *live.BusyError
	if errors.As(err, &busy) {
		w.Header().Set("X-HDHomeRun-Error", "805 All Tuners In Use")
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	http.Error(w, "The mosaic did not start", http.StatusServiceUnavailable)
}

// wroteWriter remembers whether any bytes went out, after which a status can
// no longer be sent.
type wroteWriter struct {
	w     flushWriter
	wrote bool
}

func (o *wroteWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		o.wrote = true
	}
	return o.w.Write(p)
}
