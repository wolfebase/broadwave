package httpapi

import (
	"net/http"
	"os"
	"strings"
	"time"
)

const apiVersion = 1

func DefaultServerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "Broadwave"
	}
	host = strings.TrimSuffix(host, ".local")
	return "Broadwave on " + host
}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	id, err := s.Store.Identity(r.Context(), DefaultServerName())
	if err != nil {
		writeError(w, err)
		return
	}
	tuners := 0
	if devices, err := s.Store.Devices(r.Context()); err == nil {
		for _, d := range devices {
			tuners += d.TunerCount
		}
	}
	encoder := ""
	if s.Hub != nil {
		encoder = s.Hub.Encoder
	}
	version := s.Version
	if version == "" {
		version = "dev"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id.ID,
		"name":       id.Name,
		"version":    version,
		"apiVersion": apiVersion,
		"encoder":    encoder,
		"tunerCount": tuners,
		"features":   s.features(),
	})
}

func (s *Server) renameServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if _, err := s.Store.Identity(r.Context(), DefaultServerName()); err != nil {
		writeError(w, err)
		return
	}
	if err := s.Store.RenameServer(r.Context(), body.Name); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.serverInfo(w, r)
}

// features lets clients light up UI only for what this server build supports.
func (s *Server) features() []string {
	return []string{"live", "renditions", "wholeHomeSync", "events", "dvr", "passes", "virtualChannels", "commercialDetection", "hdhrEmulation", "export"}
}

// clock gives clients the server time (Unix ms) for a first offset estimate;
// the socket's clock messages refine it.
func (s *Server) clock(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]float64{"serverTime": float64(s.now().UnixNano()) / 1e6})
}

func (s *Server) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Server) socket(w http.ResponseWriter, r *http.Request) {
	if s.Bus == nil {
		httpError(w, "Live updates are not available.", http.StatusServiceUnavailable)
		return
	}
	s.Bus.ServeHTTP(w, r)
}
