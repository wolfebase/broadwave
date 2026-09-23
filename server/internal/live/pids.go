package live

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// NotePID remembers an ffmpeg process so a later start can stop it if this
// process was killed before it could.
func NotePID(dir string, pid int) {
	if dir == "" || pid <= 0 {
		return
	}
	path := filepath.Join(dir, "pids")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(path, strconv.Itoa(pid)), nil, 0o644)
}

// ForgetPID drops a process that exited on its own.
func ForgetPID(dir string, pid int) {
	if dir == "" || pid <= 0 {
		return
	}
	_ = os.Remove(filepath.Join(dir, "pids", strconv.Itoa(pid)))
}

// Reap stops ffmpeg processes left behind by a crash and deletes the stale
// live buffer. A pid is killed only when it is still ffmpeg, so a reused pid
// is left alone.
func Reap(dir string) {
	reap(dir, func(pid int) {
		if !isFFmpeg(pid) {
			return
		}
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
}

func reap(dir string, kill func(pid int)) {
	matches, _ := filepath.Glob(filepath.Join(dir, "pids", "*"))
	for _, path := range matches {
		pid, err := strconv.Atoi(filepath.Base(path))
		if err != nil || pid <= 0 {
			continue
		}
		kill(pid)
		_ = os.Remove(path)
	}
	_ = os.RemoveAll(filepath.Join(dir, "live"))
}

func isFFmpeg(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "linux" {
		buf, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
		return err == nil && bytes.Contains(buf, []byte("ffmpeg"))
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	return err == nil && bytes.Contains(out, []byte("ffmpeg"))
}
