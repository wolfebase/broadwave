package httpapi

import (
	"context"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"ota-viewer/internal/dvr"
	"ota-viewer/internal/store"
)

// Emulator lets another app add this server as an HDHomeRun by address.
// It does not answer UDP discovery, so the real tuner stays the one on the network.
type Emulator struct {
	mu  sync.Mutex
	srv *http.Server
}

var lineEmulator Emulator

func SyncEmulator(st *store.Store, ffmpeg string) {
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
	h := &emuHandler{store: st, ffmpeg: ffmpeg}
	mux.HandleFunc("GET /discover.json", h.discover)
	mux.HandleFunc("GET /lineup.json", h.lineup)
	mux.HandleFunc("GET /auto/", h.stream)
	srv := &http.Server{Addr: ":8478", Handler: mux}
	lineEmulator.srv = srv
	go func() { _ = srv.ListenAndServe() }()
}

type emuHandler struct {
	store  *store.Store
	ffmpeg string
}

func (h *emuHandler) discover(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	writeJSON(w, http.StatusOK, map[string]any{
		"FriendlyName": "OTA Viewer",
		"ModelNumber":  "OTA-VIEWER",
		"FirmwareName": "ota-viewer",
		"DeviceID":     "OTAVIEW01",
		"TunerCount":   1,
		"BaseURL":      "http://" + host,
		"LineupURL":    "http://" + host + "/lineup.json",
	})
}

func (h *emuHandler) lineup(w http.ResponseWriter, r *http.Request) {
	list, _ := h.store.Virtuals(r.Context())
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, map[string]any{
			"GuideNumber": item.Number,
			"GuideName":   item.Name,
			"URL":         "http://" + r.Host + "/auto/v" + item.Number,
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *emuHandler) stream(w http.ResponseWriter, r *http.Request) {
	number := strings.TrimPrefix(r.URL.Path, "/auto/v")
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
	if h.ffmpeg == "" {
		http.ServeFile(w, r, path)
		return
	}
	args := []string{"-hide_banner", "-loglevel", "error"}
	if offset > 1 {
		args = append(args, "-ss", strconv.FormatFloat(offset, 'f', 1, 64))
	}
	args = append(args, "-i", path, "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd := exec.CommandContext(r.Context(), h.ffmpeg, args...)
	cmd.Stdout = w
	_ = cmd.Run()
}
