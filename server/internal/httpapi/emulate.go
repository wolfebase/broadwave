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
	slots := dvr.Slots(chosen.OrderMode, chosen.Recordings, recs, now.Add(-6*time.Hour), now.Add(time.Hour))
	var path string
	offset := 0.0
	for _, slot := range slots {
		if !now.Before(slot.Start) && now.Before(slot.End) {
			for _, rec := range recs {
				if rec.ID == slot.RecordingID {
					path = rec.Path
					offset = now.Sub(slot.Start).Seconds()
				}
			}
		}
	}
	if path == "" {
		for _, rec := range recs {
			if rec.ID == chosen.Recordings[0] {
				path = rec.Path
			}
		}
	}
	if path == "" {
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
	args := []string{"-hide_banner", "-loglevel", "error"}
	if offset > 1 {
		args = append(args, "-ss", strconv.FormatFloat(offset, 'f', 1, 64))
	}
	args = append(args, "-re", "-i", path, "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd := exec.CommandContext(r.Context(), ffmpeg, args...)
	cmd.Stdout = flushWriter{w}
	_ = cmd.Run()
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
