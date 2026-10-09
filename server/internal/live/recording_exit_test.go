package live

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// An ffmpeg copy that exits on its own used to leave the row recording and
// the tuner held. Nothing waited on that process until somebody stopped it.
func TestRecordingFFmpegExitFailsAndReleasesTheTune(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "ffmpeg.pid")
	bin := filepath.Join(dir, "exit-ffmpeg")
	script := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexit 1\n", pidPath)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h, st, city := labTune(t, dir, bin)

	// Long enough that the end timer cannot mark the row complete during the test.
	rec, err := h.RecordMeta(ctx, 60, store.Recording{ChannelID: city, Title: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.StopRecord(rec.ID) })
	pid := waitPID(t, pidPath)

	waitUntil(t, 2*time.Second, "the recording to fail", func() bool {
		got, err := st.Recording(ctx, rec.ID)
		return err == nil && got.Status == "failed"
	})
	got, err := st.Recording(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Error != "The recording stopped early." {
		t.Fatalf("status %s error %q", got.Status, got.Error)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("recording ffmpeg %d is still alive", pid)
	}
	if _, err := os.Stat(filepath.Join(h.Dir, "pids", fmt.Sprint(pid))); !os.IsNotExist(err) {
		t.Fatalf("pid file still recorded: %v", err)
	}
	h.mu.Lock()
	tuned := h.channels[city] != nil
	left := len(h.muxes)
	h.mu.Unlock()
	if tuned || left != 0 {
		t.Fatalf("early exit left the tune up: channel %v muxes %d", tuned, left)
	}
}

// Stopping a recording has to reap the copy that is still reading. The stop
// holds the hub lock while it waits, and the watcher closes that wait before
// it takes the lock.
func TestStopRecordReapsTheCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "ffmpeg.pid")
	bin := filepath.Join(dir, "copy-ffmpeg")
	script := fmt.Sprintf("#!/bin/sh\necho $$ > %q\ncat >/dev/null\nexit 0\n", pidPath)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h, st, city := labTune(t, dir, bin)
	rec, err := h.RecordMeta(ctx, 60, store.Recording{ChannelID: city, Title: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	pid := waitPID(t, pidPath)
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	go func() {
		h.StopRecord(rec.ID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return")
	}
	got, err := st.Recording(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "complete" || got.Error != "" {
		t.Fatalf("status %s error %q", got.Status, got.Error)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("recording ffmpeg %d is still alive", pid)
	}
	h.mu.Lock()
	tuned := h.channels[city] != nil
	left := len(h.muxes)
	h.mu.Unlock()
	if tuned || left != 0 {
		t.Fatalf("stop left the tune up: channel %v muxes %d", tuned, left)
	}
}

// labTune is City 8.1 already on a mux this process never dials. bin is the
// ffmpeg stand-in. tuner -1 keeps releasing the feed off the control port.
func labTune(t *testing.T, dir, bin string) (*Hub, *store.Store, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "lab", FriendlyName: "Lab", BaseURL: "http://tuner.example", TunerCount: 1,
	}, []hdhr.Channel{
		{GuideNumber: "8.1", GuideName: "City"},
		{GuideNumber: "8.2", GuideName: "Elsewhere"},
	}); err != nil {
		t.Fatal(err)
	}
	city := idOf(t, st, "8.1")
	const freq = 210000000
	h := &Hub{
		Store: st, Dir: filepath.Join(dir, "hub"), FFmpeg: bin,
		channels: map[int64]*feed{}, muxes: map[int]*mux{},
	}
	m := &mux{freq: freq, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}}
	f := &feed{
		channel: store.SourceChannel{
			Channel:     store.Channel{ID: city, GuideNumber: "8.1", GuideName: "City", DisplayName: "City"},
			FrequencyHz: freq,
		},
		renditions: map[string]*rendition{},
	}
	m.feeds["8.1"] = f
	h.muxes[freq] = m
	h.channels[city] = f
	return h, st, city
}

func waitPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	waitUntil(t, 2*time.Second, "ffmpeg to write its pid", func() bool {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		_, err = fmt.Sscanf(string(raw), "%d", &pid)
		return err == nil && pid > 0
	})
	return pid
}
