package httpapi

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"broadwave/internal/disk"
	"broadwave/internal/live"
)

// apiError writes the error envelope every client reads: {"code", "message", ...details}.
func apiError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	body := map[string]any{"code": code, "message": message}
	for k, v := range details {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// jsonRequest rejects a browser form post. An empty type is still accepted
// so a client that sends no header keeps working. The apps send JSON.
func jsonRequest(r *http.Request) bool {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	return ct == "" || strings.Contains(ct, "application/json")
}

func httpError(w http.ResponseWriter, message string, status int) {
	apiError(w, status, codeFor(status), message, nil)
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message, details := errorFor(err)
	apiError(w, status, code, message, details)
}

// watchError is writeError for a channel start. A viewer never reads Go or
// ffmpeg text; the log keeps it.
func watchError(w http.ResponseWriter, err error) {
	status, code, message, details := errorFor(err)
	switch code {
	case "not_found":
		message = "That channel is not in the lineup."
	case "internal":
		slog.Warn("watch: " + err.Error())
		message = "This channel did not start. The server log says why."
	}
	apiError(w, status, code, message, details)
}

func errorFor(err error) (int, string, string, map[string]any) {
	var busy *live.BusyError
	var full *live.PictureError
	var low *disk.LowError
	var blocked *disk.WriteError
	var streams *live.StreamLimitError
	switch {
	case errors.As(err, &busy):
		return http.StatusConflict, "tuners_busy", "Every tuner is busy. Stop a recording or watch something already on.", map[string]any{"tuners": busy.Tuners}
	case errors.As(err, &full):
		return http.StatusConflict, "pictures_full", full.Error(), map[string]any{"tiles": full.Tiles}
	case errors.As(err, &low):
		return http.StatusInsufficientStorage, "disk_low", err.Error(), map[string]any{"freeBytes": low.Free, "needBytes": low.Need}
	case errors.As(err, &blocked):
		return http.StatusInsufficientStorage, "disk_low", blocked.Error(), nil
	case errors.Is(err, live.ErrNoSignal):
		return http.StatusServiceUnavailable, "no_signal", live.ErrNoSignal.Error(), nil
	case errors.Is(err, live.ErrStreamDown):
		return http.StatusServiceUnavailable, "stream_down", live.ErrStreamDown.Error(), nil
	case errors.As(err, &streams):
		return http.StatusConflict, "streams_full", streams.Error(), map[string]any{"limit": streams.Limit}
	case errors.Is(err, live.ErrNoSource):
		return http.StatusNotFound, "no_source", live.ErrNoSource.Error(), nil
	case errors.Is(err, live.ErrTunerSilent):
		return http.StatusServiceUnavailable, "tuner_silent", live.ErrTunerSilent.Error(), nil
	case errors.Is(err, live.ErrTunerRefused):
		slog.Warn("tuner: " + err.Error())
		return http.StatusServiceUnavailable, "tuner_refused", live.ErrTunerRefused.Error(), nil
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound, "not_found", "Not found.", nil
	default:
		return http.StatusInternalServerError, "internal", err.Error(), nil
	}
}

func codeFor(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusInsufficientStorage:
		return "disk_low"
	case http.StatusServiceUnavailable:
		return "unavailable"
	default:
		if status >= 500 {
			return "internal"
		}
		return "error"
	}
}
