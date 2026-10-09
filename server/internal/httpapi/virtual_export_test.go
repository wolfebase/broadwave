package httpapi

import (
	"bytes"
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

func TestAShortFileDoesNotSeekToItsLastSeconds(t *testing.T) {
	// 12:10 into a slot. A 59-second file counts as half an hour on the
	// guide, so the offset is 600. Seeking to the tail would end at once.
	now := time.Date(2026, 10, 9, 12, 10, 0, 0, time.UTC)
	args := tuneVirtual(t, "complete", 59, now)
	if strings.Contains(args, "-ss") {
		t.Fatalf("args %q", args)
	}
}

func TestAMissingVirtualFileIsNotAnEmptyOK(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "gone.ts")
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
	bin, _ := ffmpegArgs(t, dir)
	emu := &emuHandler{store: st, hub: &live.Hub{Store: st, Dir: dir, FFmpeg: bin}}
	rec := httptest.NewRecorder()
	emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil), "9001")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
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

// clockTS is one transport packet carrying a PCR, in 90 kHz ticks.
func clockTS(ticks uint64) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = 0x01
	pkt[2] = 0x00
	pkt[3] = 0x20
	pkt[4] = 188 - 5
	pkt[5] = 0x10
	pkt[6] = byte(ticks >> 25)
	pkt[7] = byte(ticks >> 17)
	pkt[8] = byte(ticks >> 9)
	pkt[9] = byte(ticks >> 1)
	pkt[10] = byte(ticks<<7) | 0x7e
	for i := 11; i < len(pkt); i++ {
		pkt[i] = 0xff
	}
	return pkt
}

func TestAGrowingVirtualTuneStartsAtTheSchedule(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	// 00:17 into a 30-minute slot is 1020s. The file's clock already reaches it.
	now := time.Date(2026, 10, 9, 0, 17, 0, 0, time.UTC)
	var raw []byte
	for _, sec := range []uint64{0, 1020, 1022} {
		raw = append(raw, clockTS(sec*90000)...)
	}
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "recording", Path: path, Duration: 30 * 60, StartedAt: now,
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
	argsPath := filepath.Join(dir, "args")
	captured := filepath.Join(dir, "captured")
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > '" + argsPath + "'\ncat > '" + captured + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	emu := &emuHandler{
		store: st,
		hub:   &live.Hub{Store: st, Dir: dir, FFmpeg: bin},
		now:   func() time.Time { return now },
	}
	done := make(chan struct{})
	go func() {
		rec := httptest.NewRecorder()
		emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil), "9001")
		close(done)
	}()
	// The copy of what is already on disk is immediate. cat holds the bytes
	// until the pipe closes, which happens once the recording is finished.
	time.Sleep(300 * time.Millisecond)
	if err := st.FinishRecording(t.Context(), id, "complete", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("export did not finish")
	}
	want := append(clockTS(1020*90000), clockTS(1022*90000)...)
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "pipe:0") || strings.Contains(string(args), "-ss") {
		t.Fatalf("args %q", args)
	}
	got, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("copied %d bytes, want the schedule point (%d)", len(got), len(want))
	}
}

func TestAGrowingVirtualTuneDoesNotRunPastTheFile(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	now := time.Date(2026, 10, 9, 0, 17, 0, 0, time.UTC)
	// Thirty seconds on disk, and the schedule says seventeen minutes.
	var raw []byte
	for _, sec := range []uint64{0, 30} {
		raw = append(raw, clockTS(sec*90000)...)
	}
	// A third packet makes the alignment unambiguous and keeps the span at 30s.
	raw = append(raw, clockTS(30*90000)...)
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "recording", Path: path, Duration: 30 * 60, StartedAt: now,
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
	captured := filepath.Join(dir, "captured")
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncat > '" + captured + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	emu := &emuHandler{
		store: st,
		hub:   &live.Hub{Store: st, Dir: dir, FFmpeg: bin},
		now:   func() time.Time { return now },
	}
	done := make(chan struct{})
	go func() {
		rec := httptest.NewRecorder()
		emu.streamVirtual(rec, httptest.NewRequest(http.MethodGet, "/auto/v9001", nil), "9001")
		close(done)
	}()
	time.Sleep(300 * time.Millisecond)
	if err := st.FinishRecording(t.Context(), id, "complete", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("export did not finish")
	}
	// The live edge is the 30s packet. The packet at the start stays out.
	want := append(clockTS(30*90000), clockTS(30*90000)...)
	got, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("copied %d bytes, want the live edge (%d) and not the start", len(got), len(want))
	}
}
