package httpapi

import (
	"context"
	"net/http"
	"path/filepath"

	"broadwave/internal/dvr"
	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

func (s *Server) writeNFOs(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Hub == nil || s.Hub.Dir == "" {
		httpError(w, "Recordings are not set up on this server.", http.StatusConflict)
		return
	}
	written, err := s.refreshNFOs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"written": written})
}

// refreshNFOs writes the .nfo beside every finished recording in the
// recordings folder.
func (s *Server) refreshNFOs(ctx context.Context) (int, error) {
	list, err := s.Store.Recordings(ctx)
	if err != nil {
		return 0, err
	}
	root := filepath.Join(s.Hub.Dir, "recordings")
	ready := make([]store.Recording, 0, len(list))
	for _, rec := range list {
		if rec.Status != "complete" || !pathInside(root, rec.Path) {
			continue
		}
		ready = append(ready, dvr.RecordingNFO(ctx, s.Store, rec))
	}
	return nfo.Refresh(root, ready)
}
