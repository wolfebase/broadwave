package doctor

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Note is one thing to fix, in a single line.
type Note struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// Facts is what the checks can see. Callers fill it; tests set it directly.
type Facts struct {
	IPs            []string
	TunerStored    bool
	HostHasGPU     bool
	DevDri         bool
	RecordingsPath string
	Mounts         string
	FreeBytes      int64
	Timezone       string
	Now            time.Time
	UID            int
	PUID           string
	PGID           string
	TunerQuiet     bool
}

// Notes returns every check that failed.
func Notes(f Facts) []Note {
	var out []Note
	if inDockerBridge(f.IPs) {
		if f.TunerStored {
			out = append(out, Note{ID: "bonjour", Message: "Phones and TVs can't find this server on their own from inside Docker. Switch the container to host networking."})
		} else {
			out = append(out, Note{ID: "bridge", Message: "Broadwave can't see your tuner from inside Docker. Switch the container to host networking."})
		}
	}
	if f.HostHasGPU && !f.DevDri {
		out = append(out, Note{ID: "dri", Message: "This server has graphics, but the container can't use them. Pass /dev/dri through."})
	}
	if unmounted(f) {
		out = append(out, Note{ID: "volume", Message: "Recordings are on the container disk. Map a folder for them so they survive a rebuild."})
	}
	if f.FreeBytes > 0 && f.FreeBytes < 20*1e9 {
		out = append(out, Note{ID: "disk", Message: "Less than 20 GB is free. Free some space before a long recording."})
	}
	if strings.TrimSpace(f.Timezone) == "" {
		out = append(out, Note{ID: "tz", Message: "The time zone is not set. Set TZ so the guide uses your local time."})
	}
	if !f.Now.IsZero() && (f.Now.Year() < 2024 || f.Now.Year() > 2036) {
		out = append(out, Note{ID: "clock", Message: "This server's clock is off. Whole-home sync needs the right time. Turn on network time."})
	}
	if f.UID == 0 && (strings.TrimSpace(f.PUID) == "" || strings.TrimSpace(f.PGID) == "") {
		out = append(out, Note{ID: "owner", Message: "Recordings are owned by root. Set PUID and PGID to the user who should own them (99 and 100 on Unraid)."})
	}
	if f.TunerQuiet {
		out = append(out, Note{ID: "tuner", Message: "Your tuner stopped answering. Check that it is plugged in."})
	}
	if out == nil {
		out = []Note{}
	}
	return out
}

// inDockerBridge is a container on Docker's bridge network: every IPv4
// address it has is in 172.17-31. Host networking shows docker0 there too,
// beside the home network's address.
func inDockerBridge(ips []string) bool {
	bridge := false
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil || ip.To4() == nil || ip.IsLinkLocalUnicast() {
			continue
		}
		v := ip.To4()
		if v[0] != 172 || v[1] < 17 || v[1] > 31 {
			return false
		}
		bridge = true
	}
	return bridge
}

func unmounted(f Facts) bool {
	path := filepath.Clean(strings.TrimSpace(f.RecordingsPath))
	if path == "" || path == "." || strings.TrimSpace(f.Mounts) == "" {
		return false
	}
	best := "/"
	for _, line := range strings.Split(f.Mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		point := filepath.Clean(fields[1])
		if point == "/" {
			continue
		}
		if path == point || strings.HasPrefix(path, point+string(os.PathSeparator)) {
			if len(point) > len(best) {
				best = point
			}
		}
	}
	return best == "/"
}

// ApplyIdentity honors PUID, PGID, and UMASK. It only changes the user when this process is root.
func ApplyIdentity(recordings string) {
	if raw := strings.TrimSpace(os.Getenv("UMASK")); raw != "" {
		if n, err := strconv.ParseUint(raw, 8, 32); err == nil {
			syscall.Umask(int(n))
		}
	}
	if os.Getuid() != 0 {
		return
	}
	uid, uerr := strconv.Atoi(strings.TrimSpace(os.Getenv("PUID")))
	gid, gerr := strconv.Atoi(strings.TrimSpace(os.Getenv("PGID")))
	if uerr != nil || gerr != nil {
		return
	}
	if recordings != "" {
		_ = os.Chown(recordings, uid, gid)
	}
	_ = syscall.Setgid(gid)
	_ = syscall.Setuid(uid)
}
