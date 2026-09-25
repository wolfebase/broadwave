package live

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

// writeFakeFFmpeg is an ffmpeg stand-in. The first run writes a broken init.mp4
// and exits. Later runs record whether that file was still there, then either
// stay up or exit again.
func writeFakeFFmpeg(t *testing.T, mode string) (bin, mark string) {
	t.Helper()
	dir := t.TempDir()
	mark = filepath.Join(dir, "mark")
	bin = filepath.Join(dir, "ffmpeg")
	stay := "exec sleep 60"
	if mode == "always" || mode == "clean" {
		stay = "exit 0"
	}
	if mode == "always" {
		stay = "exit 1"
	}
	body := fmtScript(mark, mode, stay)
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, mark
}

func fmtScript(mark, mode, stay string) string {
	first := `
echo broken > init.mp4
exit 1
`
	if mode == "hold" {
		first = "exec sleep 60\n"
	}
	if mode == "clean" {
		first = "exit 0\n"
	}
	return "#!/bin/sh\nexec >/dev/null 2>&1\nmark=" + strconv.Quote(mark) + `
n=0
if [ -f "$mark.count" ]; then n=$(cat "$mark.count"); fi
n=$((n+1))
echo "$n" > "$mark.count"
if [ -f init.mp4 ]; then echo saw-init >> "$mark.log"; fi
if [ "$n" -eq 1 ]; then
` + first + `fi
echo fresh > init.mp4
printf '%s\n' "$@" > "$mark.args.$n"
` + stay + "\n"
}

func restartHub(t *testing.T, window time.Duration, mode string) (*Hub, *feed, string) {
	t.Helper()
	bin, mark := writeFakeFFmpeg(t, mode)
	h, m := testHub(t)
	h.FFmpeg = bin
	h.Encoder = "h264_vaapi"
	h.FallbackWindow = window
	m.input = "sample.ts"
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{
		Channel:    store.Channel{ID: 1, GuideNumber: "4.1"},
		FieldOrder: "progressive",
	})
	f.source = Source{VideoCodec: "MPEG2", Progressive: true}
	h.mu.Unlock()
	t.Cleanup(func() {
		h.mu.Lock()
		if live := h.channels[1]; live != nil {
			h.stopFeedLocked(live)
		}
		h.mu.Unlock()
	})
	return h, f, mark
}

func startRendition(t *testing.T, h *Hub, f *feed, key string) {
	t.Helper()
	spec, ok := ParseRenditionKey(key)
	if !ok {
		t.Fatalf("bad key %s", key)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	r.viewers = 1
}

func waitMark(t *testing.T, mark, name string) string {
	t.Helper()
	path := mark + name
	deadline := time.Now().Add(3 * time.Second)
	var prev string
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		// printf creates the file before it finishes writing. Two identical
		// reads mean the args are complete.
		if err == nil && len(body) > 0 {
			got := string(body)
			if got == prev {
				return got
			}
			prev = got
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
}

func markCount(t *testing.T, mark string) int {
	t.Helper()
	body, err := os.ReadFile(mark + ".count")
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEarlyVAAPIFallbackReplacesInit(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "once")
	startRendition(t, h, f, "1080.aac2.broadcast")
	args := waitMark(t, mark, ".args.2")
	if strings.Contains(args, "h264_vaapi") || !strings.Contains(args, "libx264") {
		t.Fatalf("software fallback args:\n%s", args)
	}
	logBody, _ := os.ReadFile(mark + ".log")
	if strings.Contains(string(logBody), "saw-init") {
		t.Fatal("software fallback saw the broken init.mp4")
	}
	init, err := os.ReadFile(filepath.Join(h.Dir, "live", "1", "1080.aac2.broadcast", "init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(init)) != "fresh" {
		t.Fatalf("init.mp4 is %q", init)
	}
	h.mu.Lock()
	r := f.renditions["1080.aac2.broadcast"]
	held := h.channels[1] != nil && r != nil && r.fallback
	h.mu.Unlock()
	if !held {
		t.Fatal("an early GPU failure should keep the channel on the software encode")
	}
	if markCount(t, mark) != 2 {
		t.Fatalf("starts = %d", markCount(t, mark))
	}
}

func TestEarlyHEVCFallbackUsesLibx265(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "once")
	h.HEVC = true
	startRendition(t, h, f, "1080.aac2.broadcast.hevc")
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "libx265") || strings.Contains(args, "hevc_vaapi") {
		t.Fatalf("hevc fallback args:\n%s", args)
	}
}

func TestLateVAAPIDeathRebuildsWithoutTheOldInit(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "once")
	startRendition(t, h, f, "1080.aac2.broadcast")
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "h264_vaapi") {
		t.Fatalf("a late failure should rebuild the same encoder:\n%s", args)
	}
	logBody, _ := os.ReadFile(mark + ".log")
	if strings.Contains(string(logBody), "saw-init") {
		t.Fatal("rebuild saw the broken init.mp4")
	}
	init, err := os.ReadFile(filepath.Join(h.Dir, "live", "1", "1080.aac2.broadcast", "init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(init)) != "fresh" {
		t.Fatalf("init.mp4 is %q", init)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[1] == nil || h.muxes[575000000] == nil {
		t.Fatal("a rebuilt rendition should keep the tuner")
	}
	if f.renditions["1080.aac2.broadcast"] == nil || f.renditions["1080.aac2.broadcast"].fallback {
		t.Fatal("a late rebuild stays on the GPU encoder")
	}
}

func TestSecondDeathReleasesTheTuner(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "always")
	startRendition(t, h, f, "1080.aac2.broadcast")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		gone := h.channels[1] == nil && h.muxes[575000000] == nil
		h.mu.Unlock()
		if gone {
			if markCount(t, mark) != 2 {
				t.Fatalf("released without one rebuild, starts = %d", markCount(t, mark))
			}
			logBody, _ := os.ReadFile(mark + ".log")
			if strings.Contains(string(logBody), "saw-init") {
				t.Fatal("the rebuild reused init.mp4")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a second death should release the tuner")
	_ = f
}

func TestDeathDuringARecordingReleasesTheRenditionOnly(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "always")
	h.mu.Lock()
	f.recording = &recording{id: 7}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		renditions := len(f.renditions)
		held := h.channels[1] != nil && h.muxes[575000000] != nil
		h.mu.Unlock()
		if renditions == 0 && held {
			if markCount(t, mark) != 2 {
				t.Fatalf("starts = %d", markCount(t, mark))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the rendition should stop and the recording should keep the tuner")
}

func TestCleanExitReleasesTheTuner(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "clean")
	startRendition(t, h, f, "1080.aac2.broadcast")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		gone := h.channels[1] == nil && h.muxes[575000000] == nil
		h.mu.Unlock()
		if gone {
			if markCount(t, mark) != 1 {
				t.Fatalf("a clean exit should not restart, starts = %d", markCount(t, mark))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a finished encode should release the tuner")
}

func TestRestartDropsStaleSegmentTimes(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "hold")
	startRendition(t, h, f, "1080.aac2.broadcast")
	waitMark(t, mark, ".count")
	h.mu.Lock()
	r := f.renditions["1080.aac2.broadcast"]
	r.stamper.cache = map[string]int64{"seg00001.m4s": 999}
	pid := r.cmd.Process.Pid
	h.mu.Unlock()
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	waitMark(t, mark, ".args.2")
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.stamper.cache["seg00001.m4s"] == 999 {
		t.Fatal("a restart kept the previous segment's timestamp")
	}
}

func TestStopSkipsAProcessThatAlreadyExited(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "4.1")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	spec, _ := ParseRenditionKey("1080.aac2.broadcast")
	r := &rendition{spec: spec, cmd: cmd, dir: t.TempDir()}
	r.waited.Store(true)
	f.renditions[spec.Key()] = r
	h.stopRenditionLocked(f, spec.Key())
	out, err := exec.Command("ps", "-p", strconv.Itoa(cmd.Process.Pid), "-o", "state=").Output()
	state := strings.TrimSpace(string(out))
	if err != nil || state == "" || state[0] == 'Z' {
		t.Fatalf("stop signaled a waited pid, state %q err %v", state, err)
	}
}

func TestStoppedRenditionDoesNotRestart(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "hold")
	startRendition(t, h, f, "1080.aac2.broadcast")
	waitMark(t, mark, ".count")
	h.mu.Lock()
	h.stopRenditionLocked(f, "1080.aac2.broadcast")
	h.dropIfUnusedLocked(f)
	h.mu.Unlock()
	time.Sleep(150 * time.Millisecond)
	if markCount(t, mark) != 1 {
		t.Fatalf("stopping a rendition restarted it, starts = %d", markCount(t, mark))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[1] != nil || h.muxes[575000000] != nil {
		t.Fatal("stopping the only rendition should release the tuner")
	}
}
