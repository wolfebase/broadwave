package live

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// sleepPlayer is an ffmpeg stand-in. It records its pid, optionally writes
// the playlist the player is waiting for, then sleeps. exec keeps that pid.
func sleepPlayer(writePlaylist bool) string {
	write := ""
	if writePlaylist {
		// Only the player writes the playlist. The caption pass has no
		// working directory, so a write here would land in the test package.
		write = "if [ \"$kind\" = \"player\" ]; then\n  printf '#EXTM3U\\n' > index.m3u8\nfi\n"
	}
	return "#!/bin/sh\n" +
		"kind=player\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"webvtt\" ]; then\n" +
		"    kind=caption\n" +
		"  fi\n" +
		"done\n" +
		"echo \"$kind $$\" >> LOG\n" +
		write +
		"exec sleep 60\n"
}

func playFixture(t *testing.T, writePlaylist bool) (h *Hub, rec, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "pids.txt")
	bin := filepath.Join(dir, "ffmpeg")
	script := strings.ReplaceAll(sleepPlayer(writePlaylist), "LOG", log)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	rec = filepath.Join(dir, "rec.ts")
	if err := os.WriteFile(rec, []byte("not a broadcast"), 0o644); err != nil {
		t.Fatal(err)
	}
	h = &Hub{Dir: filepath.Join(dir, "hub"), FFmpeg: bin}
	t.Cleanup(func() { killLogged(log) })
	return h, rec, log
}

func shortenPlayStart(t *testing.T, d time.Duration) {
	t.Helper()
	prev := playStart
	playStart = d
	t.Cleanup(func() { playStart = prev })
}

func loggedRoles(log string) (players, captions []int) {
	b, err := os.ReadFile(log)
	if err != nil {
		return nil, nil
	}
	for _, line := range strings.Split(string(b), "\n") {
		role, pidText, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		switch role {
		case "player":
			players = append(players, pid)
		case "caption":
			captions = append(captions, pid)
		}
	}
	return players, captions
}

func awaitRoles(t *testing.T, log string, wantPlayers, wantCaptions int) (players, captions []int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		players, captions = loggedRoles(log)
		if len(players) >= wantPlayers && len(captions) >= wantCaptions {
			return players, captions
		}
		if time.Now().After(deadline) {
			t.Fatalf("log has %d players and %d captions, want %d and %d", len(players), len(captions), wantPlayers, wantCaptions)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func processLive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processLive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("process %d was still running", pid)
}

func killLogged(log string) {
	players, captions := loggedRoles(log)
	for _, pid := range append(players, captions...) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		// Reap when this test started the process and nobody else Waited.
		if p, err := os.FindProcess(pid); err == nil {
			_, _ = p.Wait()
		}
	}
}

func TestPlayFileStopsWhenThePlaylistNeverStarts(t *testing.T) {
	shortenPlayStart(t, 300*time.Millisecond)
	h, rec, log := playFixture(t, false)
	_, err := h.PlayFile(4, rec, "hevc", "broadcast", "")
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatal(err)
	}
	players, captions := awaitRoles(t, log, 1, 1)
	for _, pid := range append(players, captions...) {
		waitDead(t, pid)
	}

	// A retry must not pile another encode on top of the first.
	_, err = h.PlayFile(4, rec, "hevc", "broadcast", "")
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatal(err)
	}
	players, captions = awaitRoles(t, log, 2, 2)
	for _, pid := range append(players, captions...) {
		waitDead(t, pid)
	}
}

func TestPlayFileLeavesAStartedPlayerRunning(t *testing.T) {
	shortenPlayStart(t, 2*time.Second)
	h, rec, log := playFixture(t, true)
	got, err := h.PlayFile(4, rec, "hevc", "broadcast", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/media/file/4/index.m3u8" {
		t.Fatalf("playlist %s", got)
	}
	players, _ := awaitRoles(t, log, 1, 0)
	if !processLive(players[0]) {
		t.Fatal("a playlist that started was stopped")
	}
}

func TestPlayFollowStopsWhenThePlaylistNeverStarts(t *testing.T) {
	shortenPlayStart(t, 300*time.Millisecond)
	h, rec, log := playFixture(t, false)
	_, err := h.PlayFollow(9, rec, "hevc", "broadcast", "", func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatal(err)
	}
	players, captions := awaitRoles(t, log, 1, 0)
	if len(captions) != 0 {
		t.Fatalf("follow started captions %v", captions)
	}
	waitDead(t, players[0])
	h.playMu.Lock()
	_, held := h.plays[9]
	h.playMu.Unlock()
	if held {
		t.Fatal("a player that never started still holds the recording")
	}

	_, err = h.PlayFollow(9, rec, "hevc", "broadcast", "", func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatal(err)
	}
	players, _ = awaitRoles(t, log, 2, 0)
	for _, pid := range players {
		waitDead(t, pid)
	}
}

func TestPlayFollowLeavesAStartedPlayerRunning(t *testing.T) {
	shortenPlayStart(t, 2*time.Second)
	h, rec, log := playFixture(t, true)
	got, err := h.PlayFollow(9, rec, "hevc", "broadcast", "", func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if got != "/media/file/9/index.m3u8" {
		t.Fatalf("playlist %s", got)
	}
	players, _ := awaitRoles(t, log, 1, 0)
	if !processLive(players[0]) {
		t.Fatal("a playlist that started was stopped")
	}
	h.playMu.Lock()
	_, held := h.plays[9]
	h.playMu.Unlock()
	if !held {
		t.Fatal("a running follow was dropped")
	}
}
