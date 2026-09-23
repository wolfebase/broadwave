package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"waveguide/internal/disk"
	"waveguide/internal/live"
)

// apiError writes the error envelope every client reads: {"code", "message", ...details}.
func apiError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	body := map[string]any{"code": code, "message": message}
	for k, v := range details {
		body[k] = v
	}
	writeJSON(w, status, body)
}

func httpError(w http.ResponseWriter, message string, status int) {
	apiError(w, status, codeFor(status), message, nil)
}

func writeError(w http.ResponseWriter, err error) {
	var busy *live.BusyError
	var low *disk.LowError
	switch {
	case errors.As(err, &busy):
		apiError(w, http.StatusConflict, "tuners_busy", "Every tuner is busy. Stop a recording or watch something already on.", map[string]any{"tuners": busy.Tuners})
	case errors.As(err, &low):
		apiError(w, http.StatusInsufficientStorage, "disk_low", err.Error(), map[string]any{"freeBytes": low.Free, "needBytes": low.Need})
	case errors.Is(err, sql.ErrNoRows):
		apiError(w, http.StatusNotFound, "not_found", "Not found.", nil)
	default:
		apiError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
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
