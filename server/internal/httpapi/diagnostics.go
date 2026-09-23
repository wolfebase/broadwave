package httpapi

import (
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"waveguide/internal/disk"
	"waveguide/internal/live"
)

// diagnostics is everything a support conversation needs, in one call.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{
		"version":  s.Version,
		"os":       runtime.GOOS + "/" + runtime.GOARCH,
		"checked":  time.Now().UTC(),
		"features": s.features(),
	}
	if id, err := s.Store.Identity(ctx, DefaultServerName()); err == nil {
		out["server"] = id
	}
	devices, _ := s.Store.Devices(ctx)
	out["devices"] = devices
	if s.Hub != nil {
		tuners, err := s.Hub.Tuners(ctx)
		if err != nil {
			out["tunerError"] = err.Error()
		}
		if tuners == nil {
			tuners = []live.Tuner{}
		}
		out["tuners"] = tuners
		out["encoder"] = map[string]any{
			"name":        s.Hub.Encoder,
			"hardware":    s.Hub.Encoder != "libx264",
			"deinterlace": s.Hub.DeintBroadcast,
			"ffmpeg":      s.Hub.FFmpegVersion(),
		}
		out["relay"] = s.Hub.Status()
		if space, err := disk.Stat(filepath.Join(s.Hub.Dir, "recordings")); err == nil {
			out["storage"] = space
		}
	}
	channels, _ := s.Store.Channels(ctx, true)
	airings, _ := s.Store.Airings(ctx, time.Now(), time.Now().Add(14*24*time.Hour))
	listed := map[int64]bool{}
	var last time.Time
	for _, a := range airings {
		listed[a.ChannelID] = true
		if a.End.After(last) {
			last = a.End
		}
	}
	guideInfo := map[string]any{"channels": len(channels), "channelsWithListings": len(listed), "airings": len(airings)}
	if !last.IsZero() {
		guideInfo["listingsUntil"] = last
	}
	if lastPull, next, _, err := s.Store.GuideSchedule(ctx); err == nil {
		if !lastPull.IsZero() {
			guideInfo["lastRefresh"] = lastPull
		}
		if !next.IsZero() {
			guideInfo["nextRefresh"] = next
		}
	}
	out["guide"] = guideInfo
	if s.Bus != nil {
		out["connectedApps"] = s.Bus.Clients()
	}
	events, _ := s.Store.Events(ctx, 20)
	out["recentActivity"] = events
	writeJSON(w, http.StatusOK, out)
}
