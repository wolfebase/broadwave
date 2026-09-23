package live

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReapClearsPIDFilesAndLiveDir(t *testing.T) {
	dir := t.TempDir()
	NotePID(dir, 42)
	NotePID(dir, 99)
	if err := os.MkdirAll(filepath.Join(dir, "live", "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "live", "1", "index.m3u8"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var killed []int
	reap(dir, func(pid int) { killed = append(killed, pid) })
	if len(killed) != 2 || killed[0]+killed[1] != 141 {
		t.Fatalf("killed %v", killed)
	}
	if _, err := os.Stat(filepath.Join(dir, "pids")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(dir, "pids"))
		if len(entries) != 0 {
			t.Fatalf("pid files left: %d", len(entries))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "live")); !os.IsNotExist(err) {
		t.Fatal("stale live buffer should be removed")
	}
}

func TestShutdownIdleHub(t *testing.T) {
	h := &Hub{channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	h.Shutdown()
}

func TestReapKillsALiveFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "30", "-f", "null", "-")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	NotePID(dir, cmd.Process.Pid)
	Reap(dir)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("ffmpeg was still running after reap")
	}
}

func TestReapIgnoresAReusedPID(t *testing.T) {
	if isFFmpeg(os.Getpid()) {
		t.Fatal("this test process is not ffmpeg")
	}
}
