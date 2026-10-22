package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"broadwave/internal/store"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	// Listings that already ended are left out: what is on now comes first,
	// then what starts next, across every listing the guide has. Recordings
	// cover what aired.
	airings, recordings, err := s.Store.Search(r.Context(), query, s.now(), 60)
	if err != nil {
		writeError(w, err)
		return
	}
	// Channels come from the lineup, not the listings, so 5.1 or "pbs" is
	// found before the guide has anything.
	channels, err := s.Store.SearchChannels(r.Context(), query, s.now())
	if err != nil {
		writeError(w, err)
		return
	}
	for i := range recordings {
		recordings[i].Missing = fileGone(recordings[i])
	}
	writeCachedJSON(w, r, map[string]any{"query": query, "channels": channels, "airings": airings, "recordings": recordings})
}

// fileGone is a finished recording whose file was moved or deleted outside Broadwave.
func fileGone(rec store.Recording) bool {
	if rec.Path == "" || rec.Status == "recording" {
		return false
	}
	_, err := os.Stat(rec.Path)
	return errors.Is(err, fs.ErrNotExist)
}
