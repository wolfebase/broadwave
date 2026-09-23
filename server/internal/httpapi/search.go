package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	airings, recordings, err := s.Store.Search(r.Context(), query, time.Now().Add(-2*time.Hour), 40)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "airings": airings, "recordings": recordings})
}
