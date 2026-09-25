package httpapi

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"broadwave/internal/discovery"
)

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	found := s.cachedHome(ctx, r.URL.Query().Get("fresh") == "1")
	var known []discovery.Known
	if s.Store != nil {
		devices, err := s.Store.Devices(ctx)
		if err != nil {
			writeError(w, err)
			return
		}
		for _, device := range devices {
			known = append(known, discovery.Known{
				ID: device.DeviceID, Name: device.FriendlyName, Addr: hostOf(device.BaseURL), Tuners: device.TunerCount,
			})
		}
	}
	var screens []discovery.Screen
	if s.Bus != nil {
		for _, screen := range s.Bus.Screens() {
			screens = append(screens, discovery.Screen{Name: screen.Name, Kind: screen.Kind, Addr: screen.Addr})
		}
	}
	places := discovery.Assemble(found, known, screens, discovery.LocalNets())
	if places == nil {
		places = []discovery.Place{}
	}
	sharing := false
	if !s.Staging && s.Store != nil {
		if values, err := s.Store.Settings(ctx); err == nil && values["hdhrEmulate"] == "1" {
			sharing = true
		}
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"places":       places,
		"tunerAddress": host + ":8478",
		"sharing":      sharing,
	})
}

func (s *Server) cachedHome(ctx context.Context, fresh bool) []discovery.Found {
	s.homeMu.Lock()
	defer s.homeMu.Unlock()
	if !fresh && time.Since(s.homeAt) < 20*time.Second && s.homeAt != (time.Time{}) {
		return s.homeFound
	}
	var found []discovery.Found
	if s.HomeScan != nil {
		found = s.HomeScan(ctx)
	} else {
		found = discovery.ScanHome(ctx)
	}
	if found == nil {
		found = []discovery.Found{}
	}
	parts := make([]string, 0, len(found))
	for _, item := range found {
		parts = append(parts, item.Kind+" "+item.Name+" "+item.Addr)
	}
	if len(parts) == 0 {
		log.Printf("home scan: nothing")
	} else {
		log.Printf("home scan: %s", strings.Join(parts, "; "))
	}
	s.homeFound = found
	s.homeAt = time.Now()
	return found
}

func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
