package live

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStillWidthIsTheSmallestSizeThatCovers(t *testing.T) {
	for w, want := range map[int]int{0: 480, -3: 480, 100: 240, 240: 240, 241: 480, 700: 960, 1280: 1280, 1600: 1920, 4000: 1920} {
		if got := StillWidth(w); got != want {
			t.Errorf("StillWidth(%d) = %d, want %d", w, got, want)
		}
	}
	if StillPath("/w", 7, 12.34, 300) != filepath.Join("/w", "stills", "7", "123-480.jpg") {
		t.Fatal(StillPath("/w", 7, 12.34, 300))
	}
}

func TestARecordingStillIsMadeOnceAtItsTime(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "show.ts")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-t", "6",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-f", "mpegts", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	var wg sync.WaitGroup
	paths := make([]string, 6)
	errs := make([]error, 6)
	for i := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			paths[i], errs[i] = h.RecordingStill(ctx, 3, src, 2.5, 480)
		}()
	}
	wg.Wait()
	for i := range paths {
		if errs[i] != nil || paths[i] != StillPath(dir, 3, 2.5, 480) {
			t.Fatalf("request %d: %q %v", i, paths[i], errs[i])
		}
	}
	jpeg, err := os.ReadFile(paths[0])
	if err != nil || !bytes.HasPrefix(jpeg, []byte{0xff, 0xd8}) {
		t.Fatalf("not a JPEG: %v", err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=width", "-of", "csv=p=0", paths[0]).Output()
	if err != nil || strings.TrimSpace(string(out)) != "480" {
		t.Fatalf("width %q %v", out, err)
	}
	// Kept: a second ask does not need the file.
	if p, err := h.RecordingStill(ctx, 3, filepath.Join(dir, "gone.ts"), 2.5, 480); err != nil || p != paths[0] {
		t.Fatalf("kept still: %q %v", p, err)
	}
	if _, err := h.RecordingStill(ctx, 3, src, 60, 480); !errors.Is(err, ErrNoStill) {
		t.Fatalf("past the end: %v", err)
	}
	if _, err := h.RecordingStill(ctx, 3, src, -1, 480); !errors.Is(err, ErrNoStill) {
		t.Fatalf("before the start: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(paths[0])); len(entries) != 1 {
		t.Fatalf("left behind %v", entries)
	}
}
