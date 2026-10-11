package httpapi

import (
	"net/http"
	"time"

	"broadwave/internal/dvr"
)

// storagePlan is the clean-up the rules would run, and the recordings with
// lost time that can be recorded again. It does not delete anything.
func (s *Server) storagePlan(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || s.Hub.Dir == "" {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	plan, err := dvr.Preview(r.Context(), s.Store, s.Hub, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
