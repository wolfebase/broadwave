package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
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

// exportMosaic is a mosaic the exports list, as MPEG-TS, for apps that take
// a stream URL.
func (s *Server) exportMosaic(w http.ResponseWriter, r *http.Request) {
	ids, err := live.ParseMosaicKey(r.PathValue("key"))
	if err != nil || s.Hub == nil || !isShared(r.Context(), s.Store, r.PathValue("key")) {
		http.NotFound(w, r)
		return
	}
	exportMosaicTo(w, r, s.Hub, ids)
}

func exportMosaicTo(w http.ResponseWriter, r *http.Request, hub *live.Hub, ids []int64) {
	w.Header().Set("Content-Type", "video/mp2t")
	out := &wroteWriter{w: flushWriter{w}}
	err := hub.ExportMosaic(r.Context(), ids, out)
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

// mosaicShareMax is how many mosaics the exports can list.
const mosaicShareMax = 8

// mosaicList cleans the exportMosaics setting: mosaic keys joined by commas,
// each once. A slot's place is its channel number (990.1 is the first), so a
// removed mosaic leaves an empty slot and the ones after it keep their
// numbers in Plex and Channels. Empty slots at the end are dropped.
func mosaicList(raw string) (string, error) {
	var keys []string
	for _, key := range strings.Split(raw, ",") {
		key = strings.TrimSpace(key)
		if key != "" {
			if _, err := live.ParseMosaicKey(key); err != nil {
				return "", fmt.Errorf("%q is not a mosaic: %w", key, err)
			}
			if slices.Contains(keys, key) {
				key = ""
			}
		}
		keys = append(keys, key)
	}
	for len(keys) > 0 && keys[len(keys)-1] == "" {
		keys = keys[:len(keys)-1]
	}
	if len(keys) > mosaicShareMax {
		return "", fmt.Errorf("other apps can list at most %d mosaics", mosaicShareMax)
	}
	return strings.Join(keys, ","), nil
}

// sharedMosaic is a mosaic the exports list as a channel of its own.
type sharedMosaic struct {
	key    string
	number string
	name   string
	about  string
}

// sharedMosaics are the mosaics in the exportMosaics setting whose channels
// are all still in the lineup, numbered 990 dot their slot.
func sharedMosaics(ctx context.Context, st *store.Store) []sharedMosaic {
	settings, err := st.Settings(ctx)
	if err != nil || settings["exportMosaics"] == "" {
		return nil
	}
	channels, err := st.Channels(ctx, true)
	if err != nil {
		return nil
	}
	byID := map[int64]store.Channel{}
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	var out []sharedMosaic
	for slot, key := range strings.Split(settings["exportMosaics"], ",") {
		ids, err := live.ParseMosaicKey(key)
		if err != nil {
			continue
		}
		var names, about []string
		for _, id := range ids {
			ch, ok := byID[id]
			if !ok {
				names = nil
				break
			}
			names = append(names, ch.DisplayName)
			about = append(about, ch.DisplayNumber+" "+ch.DisplayName)
		}
		if names == nil {
			continue
		}
		out = append(out, sharedMosaic{
			key:    key,
			number: "990." + strconv.Itoa(slot+1),
			name:   "Multiview: " + strings.Join(names, " + "),
			about:  strings.Join(about, ", ") + ", side by side. The sound is " + about[0] + ".",
		})
	}
	return out
}

// isShared is whether the exports list this mosaic. The export routes play
// only those, so an app on the network cannot start any mix it likes.
func isShared(ctx context.Context, st *store.Store, key string) bool {
	for _, mo := range sharedMosaics(ctx, st) {
		if mo.key == key {
			return true
		}
	}
	return false
}

// m3uText keeps a name inside its M3U attribute and line: players read
// tvg-name up to the next double quote and know no escapes.
func m3uText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '"':
			return '\''
		case r < ' ':
			return -1
		}
		return r
	}, s)
}
