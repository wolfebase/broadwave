package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func virtualFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, "recordings", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0x47, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ffmpegArgs(t *testing.T, dir string) (bin, argsPath string) {
	t.Helper()
	bin = filepath.Join(dir, "ffmpeg")
	argsPath = filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > '" + argsPath + "'\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsPath
}

func tuneVirtual(t *testing.T, status string, duration float64, now time.Time) string {
	t.Helper()
	st := testStore(t)
	dir := t.TempDir()
	path := virtualFile(t, dir, "night.ts")
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: status, Path: path, Duration: duration, StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duration != 0 {
		if err := st.SetDuration(t.Context(), id, duration); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateVirtual(t.Context(), "9001", "Night Shift", []int64{id}); err != nil {
		t.Fatal(err)
	}
	bin, argsPath := ffmpegArgs(t, dir)
	emu := &emuHandler{
		store: st,
		hub:   &live.Hub{Store: st, Dir: dir, FFmpeg: bin},
		now:   func() time.Time { return now },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := httptest.NewRecorder()
	emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil).WithContext(ctx), "9001")
	cancel()
	if status == "recording" {
		time.Sleep(800 * time.Millisecond)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestAFinishedVirtualTuneSeeksInsideTheFile(t *testing.T) {
	// 4:59 into a 5-minute file. The schedule offset is 299; playback stops
	// two seconds before the end.
	now := time.Date(2026, 10, 9, 0, 4, 59, 0, time.UTC)
	args := tuneVirtual(t, "complete", 5*60, now)
	if !strings.Contains(args, "-ss 298.000") {
		t.Fatalf("args %q", args)
	}
	if strings.Contains(args, "299.000") || strings.Contains(args, "1020") {
		t.Fatalf("args %q", args)
	}
	if strings.Index(args, "-ss") > strings.Index(args, " -i ") {
		t.Fatalf("seek follows the input %q", args)
	}
}

func TestAFinishedVirtualTuneSeeksToTheSchedule(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 17, 0, 0, time.UTC)
	args := tuneVirtual(t, "complete", 30*60, now)
	if !strings.Contains(args, "-ss 1020.000") {
		t.Fatalf("args %q", args)
	}
	if strings.Index(args, "-ss") > strings.Index(args, " -i ") {
		t.Fatalf("seek follows the input %q", args)
	}
	if !strings.Contains(args, "night.ts") {
		t.Fatalf("args %q", args)
	}
}

func TestAGrowingVirtualTuneFollowsTheFile(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 17, 0, 0, time.UTC)
	args := tuneVirtual(t, "recording", 30*60, now)
	if !strings.Contains(args, "pipe:0") {
		t.Fatalf("args %q", args)
	}
	if strings.Contains(args, "-ss") || strings.Contains(args, "night.ts") {
		t.Fatalf("args %q", args)
	}
}

func TestAVirtualTuneWithoutFFmpegSendsTheFile(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := virtualFile(t, dir, "night.ts")
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, 30*60); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVirtual(t.Context(), "9001", "Night Shift", []int64{id}); err != nil {
		t.Fatal(err)
	}
	emu := &emuHandler{store: st, hub: &live.Hub{Store: st, Dir: dir}}
	rec := httptest.NewRecorder()
	emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil), "9001")
	if rec.Code != http.StatusOK || rec.Body.String() != string([]byte{0x47, 0x00}) {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Fatalf("type %s", ct)
	}
}
