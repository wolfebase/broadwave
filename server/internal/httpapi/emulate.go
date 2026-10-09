package httpapi

import (
	"context"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// emulatedTuners is what other apps see. Every "tuner" rides the shared tune, so
// the real limit is distinct frequencies, which the hub enforces.
const emulatedTuners = 8

// Emulator lets Plex, Jellyfin, or Channels add this server as an HDHomeRun by address.
// It does not answer UDP discovery, so the real tuner stays the one on the network.
type Emulator struct {
	mu  sync.Mutex
	srv *http.Server
}

var lineEmulator Emulator

func SyncEmulator(st *store.Store, hub *live.Hub) {
	if st == nil {
		return
	}
	values, err := st.Settings(context.Background())
	want := err == nil && values["hdhrEmulate"] == "1"
	lineEmulator.mu.Lock()
	defer lineEmulator.mu.Unlock()
	if !want {
		if lineEmulator.srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = lineEmulator.srv.Shutdown(ctx)
			cancel()
			lineEmulator.srv = nil
		}
		return
	}
	if lineEmulator.srv != nil {
		return
	}
	mux := http.NewServeMux()
	h := &emuHandler{store: st, hub: hub}
	mux.HandleFunc("GET /discover.json", h.discover)
	mux.HandleFunc("GET /lineup.json", h.lineup)
	mux.HandleFunc("GET /lineup_status.json", h.lineupStatus)
	mux.HandleFunc("GET /auto/", h.stream)
	srv := &http.Server{Addr: ":8478", Handler: mux}
	lineEmulator.srv = srv
	go func() { _ = srv.ListenAndServe() }()
}

type emuHandler struct {
	store *store.Store
	hub   *live.Hub
	// now overrides the clock in tests. Nil is the wall clock.
	now func() time.Time
}

func (h *emuHandler) discover(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	id, _ := h.store.Identity(r.Context(), DefaultServerName())
	deviceID := "WAVEGD01"
	if len(id.ID) >= 8 {
		deviceID = strings.ToUpper(id.ID[:8])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"FriendlyName":    id.Name,
		"ModelNumber":     "HDTC-2US",
		"FirmwareName":    "hdhomeruntc_atsc",
		"FirmwareVersion": "20260101",
		"DeviceID":        deviceID,
		"DeviceAuth":      "broadwave",
		"TunerCount":      emulatedTuners,
		"BaseURL":         "http://" + host,
		"LineupURL":       "http://" + host + "/lineup.json",
	})
}

func (h *emuHandler) lineupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ScanInProgress": 0, "ScanPossible": 0, "Source": "Antenna", "SourceList": []string{"Antenna"}})
}

func (h *emuHandler) lineup(w http.ResponseWriter, r *http.Request) {
	rows := []map[string]any{}
	if channels, err := h.store.Channels(r.Context(), true); err == nil {
		for _, ch := range channels {
			row := map[string]any{
				"GuideNumber": ch.DisplayNumber,
				"GuideName":   ch.DisplayName,
				"URL":         "http://" + r.Host + "/auto/c" + strconv.FormatInt(ch.ID, 10),
			}
			if ch.HD {
				row["HD"] = 1
			}
			rows = append(rows, row)
		}
	}
	for _, mo := range sharedMosaics(r.Context(), h.store) {
		rows = append(rows, map[string]any{
			"GuideNumber": mo.number,
			"GuideName":   mo.name,
			"URL":         "http://" + r.Host + "/auto/m" + mo.key,
			"HD":          1,
		})
	}
	if list, err := h.store.Virtuals(r.Context()); err == nil {
		for _, item := range list {
			rows = append(rows, map[string]any{
				"GuideNumber": item.Number,
				"GuideName":   item.Name,
				"URL":         "http://" + r.Host + "/auto/v" + item.Number,
			})
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *emuHandler) stream(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/auto/")
	if id, ok := strings.CutPrefix(rest, "c"); ok {
		channelID, err := strconv.ParseInt(id, 10, 64)
		if err != nil || h.hub == nil {
			http.NotFound(w, r)
			return
		}
		exportChannel(w, r, h.hub, channelID)
		return
	}
	if key, ok := strings.CutPrefix(rest, "m"); ok {
		ids, err := live.ParseMosaicKey(key)
		if err != nil || h.hub == nil || !isShared(r.Context(), h.store, key) {
			http.NotFound(w, r)
			return
		}
		exportMosaicTo(w, r, h.hub, ids)
		return
	}
	h.streamVirtual(w, r, strings.TrimPrefix(rest, "v"))
}

func exportChannel(w http.ResponseWriter, r *http.Request, hub *live.Hub, channelID int64) {
	w.Header().Set("Content-Type", "video/mp2t")
	if err := hub.Export(r.Context(), channelID, flushWriter{w}); err != nil {
		var busy *live.BusyError
		if asBusy(err, &busy) {
			w.Header().Set("X-HDHomeRun-Error", "805 All Tuners In Use")
			http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		}
	}
}

func (h *emuHandler) streamVirtual(w http.ResponseWriter, r *http.Request, number string) {
	list, err := h.store.Virtuals(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var chosen store.VirtualChannel
	for _, item := range list {
		if item.Number == number {
			chosen = item
			break
		}
	}
	if chosen.ID == 0 || len(chosen.Recordings) == 0 {
		http.NotFound(w, r)
		return
	}
	recs, _ := h.store.Recordings(r.Context())
	now := time.Now()
	if h.now != nil {
		now = h.now()
	}
	id, offset, ok := dvr.Join(chosen.OrderMode, chosen.Recordings, recs, now)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var rec store.Recording
	found := false
	for _, item := range recs {
		if item.ID == id {
			rec = item
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	path, inside := recordingInside(mediaRoots(r.Context(), h.store, h.hub), rec.Path)
	if !inside {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	ffmpeg := ""
	if h.hub != nil {
		ffmpeg = h.hub.FFmpeg
	}
	if ffmpeg == "" {
		http.ServeFile(w, r, path)
		return
	}
	if rec.Status == "recording" {
		// The schedule point can sit past the bytes written so far. -ss there
		// makes ffmpeg exit with an empty response, so the copy starts at the
		// later of the schedule and the media that is actually on disk.
		written := live.MediaWritten(path, rec.StartedAt, now)
		h.followVirtual(w, r, ffmpeg, path, rec.ID, live.ExportResume(offset, written, true))
		return
	}
	playable := rec.Duration
	if playable == 0 {
		playable = recordingDuration(h.hub, rec)
	}
	if playable < 0 {
		playable = 0
	}
	at := live.ExportResume(offset, playable, false)
	args := []string{"-hide_banner", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at, 'f', 3, 64))
	}
	args = append(args, "-re", "-i", path, "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd := exec.CommandContext(r.Context(), ffmpeg, args...)
	cmd.Stdout = flushWriter{w}
	_ = cmd.Run()
}

// followVirtual copies a recording that is still being written, starting at
// seconds into its clock. The pipe cannot be seeked, so the bytes before
// that point are dropped. A point past the end of what has been written is
// not passed here: ffmpeg would exit before sending a packet.
func (h *emuHandler) followVirtual(w http.ResponseWriter, r *http.Request, ffmpeg, path string, id int64, at float64) {
	cmd := exec.CommandContext(r.Context(), ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-i", "pipe:0", "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd.Stdout = flushWriter{w}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return
	}
	go live.FollowFileAt(path, stdin, func() bool {
		if r.Context().Err() != nil {
			return false
		}
		cur, err := h.store.Recording(context.Background(), id)
		return err == nil && cur.Status == "recording"
	}, at)
	_ = cmd.Wait()
}

// flushWriter pushes each chunk to the client so live video is not held in buffers.
type flushWriter struct{ w http.ResponseWriter }

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}
