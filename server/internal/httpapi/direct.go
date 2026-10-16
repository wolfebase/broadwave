package httpapi

import (
	"net/http"

	"broadwave/internal/direct"
)

// Direct is the secure name on the home network. Nil in tests and when
// BROADWAVE_DIRECT=0.
type Direct interface {
	Host() string
	URL() string
	Status() direct.Status
	SetEnabled(on bool) error
}

// directHost reports the server's own secure name. Only its own: another
// server's name under the same domain could point at this address too.
func (s *Server) directHost(name string) bool {
	if s.Direct == nil || name == "" {
		return false
	}
	return name == s.Direct.Host()
}

func (s *Server) directStatus(w http.ResponseWriter, r *http.Request) {
	if s.Direct == nil {
		writeJSON(w, http.StatusOK, direct.Status{State: "unavailable", Addresses: []string{}})
		return
	}
	writeJSON(w, http.StatusOK, s.Direct.Status())
}

func (s *Server) putDirect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Enabled == nil {
		httpError(w, "Send enabled: true or false.", http.StatusBadRequest)
		return
	}
	if s.Direct == nil {
		httpError(w, "This server was started with BROADWAVE_DIRECT=0.", http.StatusConflict)
		return
	}
	if err := s.Direct.SetEnabled(*body.Enabled); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.Direct.Status())
}
