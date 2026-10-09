package httpapi

import (
	"context"
	"encoding/xml"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"broadwave/internal/dvr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// emulatedTuners is what other apps see. Every "tuner" rides the shared tune, so
// the real limit is distinct frequencies, which the hub enforces. Eight is high
// enough that an app does not refuse a second viewer of a channel already on.
const emulatedTuners = 8

// emulatorPort is not 5004. A real tuner owns 5004, and Channels assumes 5004
// unless the address the user types includes a port.
const emulatorPort = ":8478"

// emulatorModel is a CONNECT, which does not transcode. Jellyfin and Emby
// treat a model containing "hdtc" as a transcoding tuner and then request
// ?transcode=, which this server does not do.
const (
	emulatorModel    = "HDHR4-2US"
	emulatorFirmware = "hdhomerun4_atsc"
)

// Emulator lets Plex, Jellyfin, Emby, or Channels add this server as an HDHomeRun
// by address. It does not answer UDP discovery, so the real tuner stays the one
// on the network.
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
	srv := &http.Server{Addr: emulatorPort, Handler: newEmulatorMux(st, hub)}
	lineEmulator.srv = srv
	go func() { _ = srv.ListenAndServe() }()
}

func newEmulatorMux(st *store.Store, hub *live.Hub) http.Handler {
	mux := http.NewServeMux()
	h := &emuHandler{store: st, hub: hub}
	mux.HandleFunc("GET /discover.json", h.discover)
	mux.HandleFunc("GET /lineup.json", h.lineup)
	mux.HandleFunc("GET /lineup.xml", h.lineupXML)
	mux.HandleFunc("GET /lineup.m3u", h.lineupM3U)
	mux.HandleFunc("GET /lineup_status.json", h.lineupStatus)
	mux.HandleFunc("POST /lineup.post", h.lineupPost)
	mux.HandleFunc("GET /status.json", h.status)
	mux.HandleFunc("GET /tuners.html", h.tuners)
	mux.HandleFunc("GET /auto/{target}", h.stream)
	// /tuner0/v4.1 keeps the index in the same segment as "tuner", which a
	// ServeMux wildcard cannot match. /tuners.html is a different page.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && tunerStreamPath(r.URL.Path) {
			h.stream(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func tunerStreamPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/tuner")
	if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' {
		return false
	}
	return true
}

type emuHandler struct {
	store *store.Store
	hub   *live.Hub
}

// discover is the HDHomeRun device document. DeviceAuth is omitted on purpose:
// Channels and Jellyfin send that value to SiliconDust for guide data, and this
// server has no SiliconDust guide credential. The guide for other apps is
// /export/guide.xml on the main port. A DeviceAuth query on a later request is
// ignored, so a client that saved one still gets the lineup.
func (h *emuHandler) discover(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	name := "Broadwave"
	deviceID := "B0AD0001"
	if id, err := h.store.Identity(r.Context(), DefaultServerName()); err == nil {
		name = id.Name
		deviceID = emulatorDeviceID(id.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"FriendlyName":    name,
		"ModelNumber":     emulatorModel,
		"FirmwareName":    emulatorFirmware,
		"FirmwareVersion": "20260101",
		"DeviceID":        deviceID,
		"TunerCount":      emulatedTuners,
		"BaseURL":         "http://" + host,
		"LineupURL":       "http://" + host + "/lineup.json",
	})
}

// emulatorDeviceID is eight hexadecimal characters. Plex rejects anything else.
func emulatorDeviceID(id string) string {
	id = strings.ToUpper(strings.TrimSpace(id))
	if len(id) >= 8 && hexDeviceID(id[:8]) {
		return id[:8]
	}
	return "B0AD0001"
}

func hexDeviceID(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
}

func (h *emuHandler) lineupStatus(w http.ResponseWriter, r *http.Request) {
	// ScanPossible stays 0. A scan would take a real tuner, and the lineup is
	// already the one this server shows.
	writeJSON(w, http.StatusOK, struct {
		ScanInProgress int      `json:"ScanInProgress"`
		ScanPossible   int      `json:"ScanPossible"`
		Source         string   `json:"Source"`
		SourceList     []string `json:"SourceList"`
	}{
		Source:     "Antenna",
		SourceList: []string{"Antenna"},
	})
}

// lineupPost accepts the scan buttons Plex and Channels send. It does not scan.
func (h *emuHandler) lineupPost(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("scan") {
	case "start", "abort":
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "bad scan", http.StatusBadRequest)
	}
}

func (h *emuHandler) status(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Resource              string `json:"Resource"`
		VctNumber             string `json:"VctNumber"`
		VctName               string `json:"VctName"`
		TargetIP              string `json:"TargetIP"`
		SignalStrengthPercent int    `json:"SignalStrengthPercent"`
		SignalQualityPercent  int    `json:"SignalQualityPercent"`
		SymbolQualityPercent  int    `json:"SymbolQualityPercent"`
	}
	rows := make([]row, emulatedTuners)
	for i := range rows {
		rows[i].Resource = "tuner" + strconv.Itoa(i)
	}
	writeJSON(w, http.StatusOK, rows)
}

// tuners is the plain page Emby's tuner dashboard reads. It takes the characters
// immediately after the word Channel and compares them to "none", so there is
// no space there. Nothing is tuned from this page.
func (h *emuHandler) tuners(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><body><pre>\n")
	for i := 0; i < emulatedTuners; i++ {
		b.WriteString("Tuner ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" Channelnone\n")
	}
	b.WriteString("</pre></body></html>\n")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

type lineupRow struct {
	GuideNumber string `json:"GuideNumber" xml:"GuideNumber"`
	GuideName   string `json:"GuideName" xml:"GuideName"`
	HD          int    `json:"HD,omitempty" xml:"HD,omitempty"`
	Favorite    int    `json:"Favorite,omitempty" xml:"Favorite,omitempty"`
	DRM         int    `json:"DRM,omitempty" xml:"DRM,omitempty"`
	Tags        string `json:"Tags,omitempty" xml:"Tags,omitempty"`
	VideoCodec  string `json:"VideoCodec,omitempty" xml:"VideoCodec,omitempty"`
	AudioCodec  string `json:"AudioCodec,omitempty" xml:"AudioCodec,omitempty"`
	URL         string `json:"URL" xml:"URL"`
}

func (h *emuHandler) lineupRows(r *http.Request) []lineupRow {
	base := "http://" + r.Host
	rows := make([]lineupRow, 0)
	if h.store == nil {
		return rows
	}
	if channels, err := h.store.Channels(r.Context(), true); err == nil {
		for _, ch := range channels {
			rows = append(rows, channelLineupRow(base, ch))
		}
	}
	encoder := ""
	if h.hub != nil {
		encoder = h.hub.Encoder
	}
	video, audio := mosaicAdvertised(encoder)
	for _, mo := range sharedMosaics(r.Context(), h.store) {
		rows = append(rows, lineupRow{
			GuideNumber: mo.number,
			GuideName:   mo.name,
			HD:          1,
			VideoCodec:  video,
			AudioCodec:  audio,
			URL:         base + "/auto/v" + mo.number,
		})
	}
	if list, err := h.store.Virtuals(r.Context()); err == nil {
		for _, item := range list {
			rows = append(rows, lineupRow{
				GuideNumber: item.Number,
				GuideName:   item.Name,
				URL:         base + "/auto/v" + item.Number,
			})
		}
	}
	return rows
}

func channelLineupRow(base string, ch store.Channel) lineupRow {
	row := lineupRow{
		GuideNumber: ch.DisplayNumber,
		GuideName:   ch.DisplayName,
		VideoCodec:  ch.VideoCodec,
		AudioCodec:  ch.AudioCodec,
		URL:         base + "/auto/v" + ch.DisplayNumber,
	}
	var tags []string
	if ch.HD {
		row.HD = 1
	}
	if ch.Favorite {
		row.Favorite = 1
		tags = append(tags, "favorite")
	}
	if ch.Protected {
		row.DRM = 1
		tags = append(tags, "drm")
	}
	row.Tags = strings.Join(tags, ",")
	return row
}

func (h *emuHandler) lineup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.lineupRows(r))
}

func (h *emuHandler) lineupXML(w http.ResponseWriter, r *http.Request) {
	type program struct {
		XMLName xml.Name `xml:"Program"`
		lineupRow
	}
	type doc struct {
		XMLName  xml.Name  `xml:"Lineup"`
		Programs []program `xml:"Program"`
	}
	out := doc{}
	for _, row := range h.lineupRows(r) {
		out.Programs = append(out.Programs, program{lineupRow: row})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", " ")
	_ = enc.Encode(out)
}

func (h *emuHandler) lineupM3U(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, row := range h.lineupRows(r) {
		b.WriteString("#EXTINF:-1,")
		b.WriteString(m3uText(row.GuideNumber))
		b.WriteByte(' ')
		b.WriteString(m3uText(row.GuideName))
		b.WriteByte('\n')
		b.WriteString(row.URL)
		b.WriteByte('\n')
	}
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

type streamHit struct {
	kind      string
	id        int64
	mosaic    []int64
	virtual   string
	protected bool
}

func (h *emuHandler) stream(w http.ResponseWriter, r *http.Request) {
	// A transcode profile is an EXTEND feature. Answering with the original
	// broadcast would lie to an app that asked for AVC.
	if r.URL.Query().Get("transcode") != "" {
		writeHDHRError(w, http.StatusServiceUnavailable, "802 Unknown Transcode Profile")
		return
	}
	tuner, rest := splitStreamPath(r.URL.Path)
	if tuner >= emulatedTuners {
		writeHDHRError(w, http.StatusServiceUnavailable, "804 Tuner In Use")
		return
	}
	hit, ok := h.lookup(r.Context(), rest)
	if !ok {
		writeHDHRError(w, http.StatusNotFound, "801 Unknown Channel")
		return
	}
	if hit.protected {
		writeHDHRError(w, http.StatusServiceUnavailable, "811 Content Protection Required")
		return
	}
	switch hit.kind {
	case "channel":
		if h.hub == nil {
			writeHDHRError(w, http.StatusServiceUnavailable, "806 Tune Failed")
			return
		}
		exportChannel(w, r, h.hub, hit.id)
	case "mosaic":
		if h.hub == nil {
			writeHDHRError(w, http.StatusServiceUnavailable, "806 Tune Failed")
			return
		}
		exportMosaicTo(w, r, h.hub, hit.mosaic)
	default:
		h.streamVirtual(w, r, hit.virtual)
	}
}

// splitStreamPath reads /auto/v4.1 and /tuner0/v4.1. tuner is -1 for /auto.
func splitStreamPath(path string) (tuner int, rest string) {
	tuner = -1
	path = strings.TrimPrefix(path, "/")
	head, tail, ok := strings.Cut(path, "/")
	if !ok {
		return -1, ""
	}
	if n, ok := strings.CutPrefix(head, "tuner"); ok {
		i, err := strconv.Atoi(n)
		if err != nil {
			return emulatedTuners, ""
		}
		tuner = i
	}
	return tuner, tail
}

// lookup resolves the path the apps actually request. Channels and Plex ask
// for /auto/v and the guide number even when lineup.json names another URL.
// A real channel wins over a virtual channel or a mosaic with the same number.
func (h *emuHandler) lookup(ctx context.Context, rest string) (streamHit, bool) {
	if h.store == nil || rest == "" {
		return streamHit{}, false
	}
	if id, ok := strings.CutPrefix(rest, "c"); ok && !strings.Contains(id, ".") {
		channelID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return streamHit{}, false
		}
		channels, err := h.store.Channels(ctx, true)
		if err != nil {
			return streamHit{}, false
		}
		for _, ch := range channels {
			if ch.ID == channelID {
				return streamHit{kind: "channel", id: ch.ID, protected: ch.Protected}, true
			}
		}
		return streamHit{}, false
	}
	if key, ok := strings.CutPrefix(rest, "m"); ok && !strings.Contains(key, ".") {
		ids, err := live.ParseMosaicKey(key)
		if err != nil || !isShared(ctx, h.store, key) {
			return streamHit{}, false
		}
		return streamHit{kind: "mosaic", mosaic: ids}, true
	}
	number := strings.TrimPrefix(rest, "v")
	if number == "" || number == rest {
		return streamHit{}, false
	}
	if channels, err := h.store.Channels(ctx, true); err == nil {
		for _, ch := range channels {
			if ch.DisplayNumber == number {
				return streamHit{kind: "channel", id: ch.ID, protected: ch.Protected}, true
			}
		}
	}
	for _, mo := range sharedMosaics(ctx, h.store) {
		if mo.number == number {
			ids, err := live.ParseMosaicKey(mo.key)
			if err != nil {
				continue
			}
			return streamHit{kind: "mosaic", mosaic: ids}, true
		}
	}
	if list, err := h.store.Virtuals(ctx); err == nil {
		for _, item := range list {
			if item.Number == number {
				return streamHit{kind: "virtual", virtual: item.Number}, true
			}
		}
	}
	return streamHit{}, false
}

func exportChannel(w http.ResponseWriter, r *http.Request, hub *live.Hub, channelID int64) {
	w.Header().Set("Content-Type", "video/mp2t")
	out := &wroteWriter{w: flushWriter{w}}
	err := hub.Export(r.Context(), channelID, out)
	if err == nil || out.wrote {
		return
	}
	var busy *live.BusyError
	if asBusy(err, &busy) {
		writeHDHRError(w, http.StatusServiceUnavailable, "805 All Tuners In Use")
		return
	}
	writeHDHRError(w, http.StatusServiceUnavailable, "806 Tune Failed")
}

func writeHDHRError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("X-HDHomeRun-Error", msg)
	http.Error(w, msg, status)
}

func (h *emuHandler) streamVirtual(w http.ResponseWriter, r *http.Request, number string) {
	list, err := h.store.Virtuals(r.Context())
	if err != nil {
		writeHDHRError(w, http.StatusNotFound, "801 Unknown Channel")
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
		writeHDHRError(w, http.StatusNotFound, "801 Unknown Channel")
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
	if _, ok := recordingInside(mediaRoots(r.Context(), h.store, h.hub), path); !ok {
		writeHDHRError(w, http.StatusNotFound, "801 Unknown Channel")
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
