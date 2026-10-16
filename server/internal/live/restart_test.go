package live

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	again := ""
	if mode == "sound" {
		first = `
echo '[mp4 @ 0x1] sample rate not set' >&3
echo '[out#0/mp4 @ 0x1] Could not write header (incorrect codec parameters ?): Invalid argument' >&3
exit 1
`
	}
	if mode == "twice" {
		again = `if [ "$n" -eq 2 ]; then echo fresh > init.mp4; printf '%s\n' "$@" > "$mark.args.$n"; exit 1; fi`
	}
	return "#!/bin/sh\nexec 3>&2 >/dev/null 2>&1\nmark=" + strconv.Quote(mark) + `
n=0
if [ -f "$mark.count" ]; then n=$(cat "$mark.count"); fi
n=$((n+1))
echo "$n" > "$mark.count"
if [ -f init.mp4 ]; then echo saw-init >> "$mark.log"; fi
if [ "$n" -eq 1 ]; then
` + first + `fi
` + again + `
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

// A guess nobody joined is stopped by its idle timer. A rebuild cancels that
// timer with the old process. The replacement has to keep the guess and arm
// the timer on the key it runs under now, or the tuner stays on that channel.
func TestRebuiltGuessReleasesItsTuner(t *testing.T) {
	h, f, _ := restartHub(t, time.Hour, "hold")
	h.mu.Lock()
	h.NoAC4 = true
	h.RenditionIdle = 200 * time.Millisecond
	want := Rendition{Video: "1080", Audio: "aac2"}
	before := want.Key()
	r, err := h.ensureRenditionLocked(f, want)
	if err != nil {
		h.mu.Unlock()
		t.Fatal(err)
	}
	r.guess = true
	r.viewers = 0
	// Progressive is already true. Setting it true again does not change the graph.
	f.source.Progressive = false
	f.channel.AudioCodec = "AC4"
	f.source.AudioCodec = "AC4"
	after := Unvoiced(f.channel.AudioCodec, want).Key()
	if after == before {
		h.mu.Unlock()
		t.Fatalf("audio choice did not move the key: %s", before)
	}
	h.rebuildRenditionsLocked(f)
	nr := f.renditions[after]
	guess, idle, viewers := false, false, -1
	if nr != nil {
		guess, idle, viewers = nr.guess, nr.idle != nil, nr.viewers
	}
	oldLeft := f.renditions[before] != nil
	moved := f.moved[before]
	h.mu.Unlock()
	if oldLeft || nr == nil || moved != after || !guess || !idle || viewers != 0 {
		t.Fatalf("rebuilt guess %s -> %s (moved %s): old left %v guess %v idle %v viewers %d", before, after, moved, oldLeft, guess, idle, viewers)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		gone := h.channels[1] == nil && len(h.muxes) == 0
		h.mu.Unlock()
		if gone {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a rebuilt guess kept the tuner")
}

// The old encode is already gone when the replacement fails to start. Nothing
// is watching, so the tuner has to be released with it.
func TestFailedRebuildReleasesAnEmptyTune(t *testing.T) {
	h, f, _ := restartHub(t, time.Hour, "hold")
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2"}); err != nil {
		t.Fatal(err)
	}
	h.FFmpeg = filepath.Join(t.TempDir(), "no-ffmpeg")
	f.source.Progressive = false
	h.rebuildRenditionsLocked(f)
	if len(f.renditions) != 0 || h.channels[1] != nil || len(h.muxes) != 0 {
		t.Fatalf("tuner stayed up: renditions %d channel %v muxes %d", len(f.renditions), h.channels[1] != nil, len(h.muxes))
	}
}

// The ladder is the only thing holding the tuner. A rebuild that cannot start
// the new encode has to let the tuner go, the same as one rendition.
func TestFailedLadderRebuildReleasesAnEmptyTune(t *testing.T) {
	h, f, _ := restartHub(t, time.Hour, "hold")
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.ensureLadderLocked(f, ladderRung("broadcast", "1080"), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if len(f.renditions) < 2 {
		t.Fatalf("ladder rungs: %d", len(f.renditions))
	}
	h.FFmpeg = filepath.Join(t.TempDir(), "no-ffmpeg")
	f.source.Progressive = false
	h.rebuildLaddersLocked(f)
	if len(f.renditions) != 0 || h.channels[1] != nil || len(h.muxes) != 0 {
		t.Fatalf("tuner stayed up: renditions %d channel %v muxes %d", len(f.renditions), h.channels[1] != nil, len(h.muxes))
	}
}

// libx265 runs a 1080 picture far slower than real time: on 29.4 it never
// wrote a fragment and the Apple TV waited 12 s for nothing. A GPU HEVC
// encode that fails at its start moves to the processor as H.264.
func TestEarlyHEVCFallbackIsH264(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "once")
	h.HEVC = true
	startRendition(t, h, f, "1080.aac2.broadcast.hevc")
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "libx264") || strings.Contains(args, "libx265") || strings.Contains(args, "hevc_vaapi") {
		t.Fatalf("hevc fallback args:\n%s", args)
	}
}

// A copied AC-3 track the probe saw no rate for stops the muxer's header.
// That is the sound, not the GPU: the encode starts again on the GPU with
// the sound converted.
func TestASoundHeaderFailureConvertsTheSound(t *testing.T) {
	bin, mark := writeFakeFFmpeg(t, "sound")
	h, f, _ := restartHub(t, time.Hour, "hold")
	h.FFmpeg = bin
	h.HEVC = true
	startRendition(t, h, f, "1080.copy.broadcast.hevc")
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "hevc_vaapi") || strings.Contains(args, "libx26") {
		t.Fatalf("a sound failure moved the picture:\n%s", args)
	}
	if !strings.Contains(args, "-c:a\naac\n") || strings.Contains(args, "-c:a\ncopy\n") {
		t.Fatalf("the restart still copies the sound:\n%s", args)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := f.renditions["1080.copy.broadcast.hevc"]; r == nil || !r.aac || r.fallback {
		t.Fatalf("rendition %+v", r)
	}
}

// A late death runs the same command line into the same packager, so the
// players' playlist keeps its numbers and its gate.
func TestLateVAAPIDeathContinuesTheSamePlaylist(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "once")
	h.mu.Lock()
	r := f.renditions["1080.aac2.broadcast"]
	h.mu.Unlock()
	if r != nil {
		t.Fatal("started early")
	}
	startRendition(t, h, f, "1080.aac2.broadcast")
	h.mu.Lock()
	r = f.renditions["1080.aac2.broadcast"]
	gate, in := r.gate, r.input
	h.mu.Unlock()
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "h264_vaapi") {
		t.Fatalf("a late failure should rebuild the same encoder:\n%s", args)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.gate != gate || r.input != in || len(r.restarts) != 1 {
		t.Fatalf("the restart replaced the playlist: gate %v input %v restarts %d", r.gate == gate, r.input == in, len(r.restarts))
	}
	if h.channels[1] == nil || h.muxes[575000000] == nil {
		t.Fatal("a rebuilt rendition should keep the tuner")
	}
	if f.renditions["1080.aac2.broadcast"] == nil || f.renditions["1080.aac2.broadcast"].fallback {
		t.Fatal("a late rebuild stays on the GPU encoder")
	}
}

// An encode that dies at every start is started again restartBudget times,
// then gives the tuner back.
func TestDeathsPastTheBudgetReleaseTheTuner(t *testing.T) {
	h, f, mark := restartHub(t, time.Nanosecond, "always")
	startRendition(t, h, f, "1080.aac2.broadcast")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		gone := h.channels[1] == nil && h.muxes[575000000] == nil
		h.mu.Unlock()
		if gone {
			if n := markCount(t, mark); n != restartBudget+1 {
				t.Fatalf("released after %d starts, want %d", n, restartBudget+1)
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
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		renditions := len(f.renditions)
		held := h.channels[1] != nil && h.muxes[575000000] != nil
		h.mu.Unlock()
		if renditions == 0 && held {
			if markCount(t, mark) != restartBudget+1 {
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

// A restart in a new directory (here the software fallback) reuses
// seg00001.m4s for a new file; the old time must not date it.
func TestRestartDropsStaleSegmentTimes(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "hold")
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

// VideoToolbox and QSV fall back like VAAPI when they fail at the start.
func TestEarlyFallbackCoversEveryGPU(t *testing.T) {
	for _, encoder := range []string{"h264_videotoolbox", "h264_qsv"} {
		t.Run(encoder, func(t *testing.T) {
			h, f, mark := restartHub(t, time.Hour, "once")
			h.Encoder = encoder
			startRendition(t, h, f, "1080.aac2.broadcast")
			args := waitMark(t, mark, ".args.2")
			if strings.Contains(args, encoder) || !strings.Contains(args, "libx264") {
				t.Fatalf("software fallback args:\n%s", args)
			}
		})
	}
}

// An HEVC encode on VideoToolbox that fails at its start moves to the
// processor as H.264, never to libx265, which cannot keep up with a 1080
// picture on most CPUs.
func TestAnHEVCGPUStartFailureStaysOffLibx265(t *testing.T) {
	h, f, mark := restartHub(t, time.Hour, "once")
	h.HEVC = true
	h.Encoder = "h264_videotoolbox"
	startRendition(t, h, f, "1080.aac2.broadcast.hevc")
	args := waitMark(t, mark, ".args.2")
	if strings.Contains(args, "libx265") || strings.Contains(args, "hevc_videotoolbox") || !strings.Contains(args, "libx264") {
		t.Fatalf("restart args:\n%s", args)
	}
}

func TestPlanRestart(t *testing.T) {
	now := time.Now()
	spent := []time.Time{now.Add(-time.Minute), now.Add(-2 * time.Minute), now.Add(-3 * time.Minute)}
	old := []time.Time{now.Add(-6 * time.Minute), now.Add(-7 * time.Minute), now.Add(-8 * time.Minute)}
	died := errors.New("exit status 1")
	copySpec, _ := ParseRenditionKey("1080.copy.broadcast.hevc")
	aacSpec, _ := ParseRenditionKey("1080.aac2.broadcast")
	cases := []struct {
		name    string
		r       *rendition
		err     error
		cause   exitCause
		early   bool
		stalled bool
		silent  bool
		encoder string
		want    restartPlan
	}{
		{"clean exit", &rendition{spec: aacSpec}, nil, causeUnknown, false, false, false, "h264_vaapi", planRelease},
		{"budget spent", &rendition{spec: aacSpec, restarts: spent}, died, causeUnknown, false, false, false, "h264_vaapi", planRelease},
		{"old restarts do not count", &rendition{spec: aacSpec, restarts: old}, died, causeUnknown, false, false, false, "h264_vaapi", planSame},
		{"late death", &rendition{spec: aacSpec}, died, causeGPU, false, false, false, "h264_vaapi", planSame},
		{"sound header on a copy", &rendition{spec: copySpec}, died, causeParams, true, false, false, "hevc_vaapi", planAAC},
		{"sound header after converting", &rendition{spec: copySpec, aac: true}, died, causeParams, true, false, false, "hevc_vaapi", planSame},
		{"sound header is not the GPU", &rendition{spec: aacSpec}, died, causeParams, true, false, false, "h264_vaapi", planSame},
		{"gpu at its start", &rendition{spec: copySpec}, died, causeGPU, true, false, false, "hevc_vaapi", planSoftware},
		{"unknown at its start", &rendition{spec: aacSpec}, died, causeUnknown, true, false, false, "h264_vaapi", planSoftware},
		{"already on the processor", &rendition{spec: aacSpec, fallback: true}, died, causeUnknown, true, false, false, "libx264", planSame},
		{"extras first", &rendition{spec: aacSpec, extras: []AudioTrack{{PID: 258}}}, died, causeUnknown, true, false, false, "h264_vaapi", planPlain},
		{"extras and the gpu", &rendition{spec: aacSpec, extras: []AudioTrack{{PID: 258}}}, died, causeGPU, true, false, false, "h264_vaapi", planSoftware},
		{"one stalled start", &rendition{spec: aacSpec, stalls: 1}, died, causeUnknown, true, true, false, "h264_vaapi", planSame},
		{"two stalled starts", &rendition{spec: aacSpec, stalls: 2}, died, causeUnknown, true, true, false, "h264_vaapi", planSoftware},
		{"a copied sound that sends nothing", &rendition{spec: copySpec}, died, causeParams, true, false, true, "hevc_vaapi", planSilent},
		{"converted sound waiting on nothing", &rendition{spec: copySpec, aac: true, stalls: 1}, died, causeUnknown, true, true, true, "hevc_vaapi", planSilent},
		{"already silent", &rendition{spec: copySpec, mute: true}, died, causeUnknown, true, true, true, "hevc_vaapi", planSame},
		{"silent sound and a gpu death", &rendition{spec: aacSpec}, died, causeGPU, true, false, true, "h264_vaapi", planSoftware},
	}
	for _, c := range cases {
		if got := planRestart(c.r, c.err, c.cause, c.early, c.stalled, c.silent, c.encoder, now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// A channel whose sound sent nothing starts its next encode without sound,
// instead of failing once more first.
func TestAChannelWithoutSoundStartsSilent(t *testing.T) {
	h, f, _ := restartHub(t, time.Hour, "hold")
	h.mu.Lock()
	h.mutes = map[int64]time.Time{1: time.Now().Add(time.Minute)}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.copy.broadcast")
	h.mu.Lock()
	defer h.mu.Unlock()
	r := f.renditions["1080.copy.broadcast"]
	if r == nil || !r.mute || !slices.Contains(r.args, "-an") {
		t.Fatalf("the encode starts with sound: %v", r)
	}
}
