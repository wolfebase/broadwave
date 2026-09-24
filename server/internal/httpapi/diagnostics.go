package httpapi

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"waveguide/internal/disk"
	"waveguide/internal/doctor"
	"waveguide/internal/live"
	"waveguide/internal/store"
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
	out["doctor"] = s.doctorNotes(devices)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) doctorNotes(devices []store.Device) []doctor.Note {
	var ips []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip.IsLoopback() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	path := ""
	var free int64
	if s.Hub != nil {
		path = filepath.Join(s.Hub.Dir, "recordings")
		if space, err := disk.Stat(path); err == nil {
			free = int64(space.Free)
		}
	}
	mounts, _ := os.ReadFile("/proc/mounts")
	quiet := false
	for _, d := range devices {
		seen, err := time.Parse(time.RFC3339, d.LastSeen)
		if d.TunerCount > 0 && err == nil && time.Since(seen) > 3*time.Minute {
			quiet = true
		}
	}
	heard := false
	for _, d := range devices {
		if d.TunerCount > 0 {
			heard = true
		}
	}
	return doctor.Notes(doctor.Facts{
		IPs: ips, BroadcastOK: heard, HostHasGPU: hostGPU(), DevDri: driPresent(),
		RecordingsPath: path, Mounts: string(mounts), FreeBytes: free,
		Timezone: os.Getenv("TZ"), Now: time.Now(), UID: os.Getuid(),
		PUID: os.Getenv("PUID"), PGID: os.Getenv("PGID"), TunerQuiet: quiet,
	})
}

func driPresent() bool {
	_, err := os.Stat("/dev/dri")
	return err == nil
}

func hostGPU() bool {
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "card") {
			return true
		}
	}
	return false
}
