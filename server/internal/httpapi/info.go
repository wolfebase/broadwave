package httpapi

import (
	"net/http"
	"os"
	"strings"
)

const apiVersion = 1

func defaultServerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "OTA Viewer"
	}
	host = strings.TrimSuffix(host, ".local")
	return "OTA Viewer on " + host
}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	id, err := s.Store.Identity(r.Context(), defaultServerName())
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
	if _, err := s.Store.Identity(r.Context(), defaultServerName()); err != nil {
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
	return []string{"live", "dvr", "passes", "virtualChannels", "commercialDetection", "hdhrEmulation"}
}
