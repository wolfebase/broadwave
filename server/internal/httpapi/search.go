package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"broadwave/internal/store"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	airings, recordings, err := s.Store.Search(r.Context(), query, s.now().Add(-2*time.Hour), 40)
	if err != nil {
		writeError(w, err)
		return
	}
	for i := range recordings {
		recordings[i].Missing = fileGone(recordings[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "airings": airings, "recordings": recordings})
}

// fileGone is a finished recording whose file was moved or deleted outside Broadwave.
func fileGone(rec store.Recording) bool {
	if rec.Path == "" || rec.Status == "recording" {
		return false
	}
	_, err := os.Stat(rec.Path)
	return errors.Is(err, fs.ErrNotExist)
}
