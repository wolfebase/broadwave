package live

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fillFFmpeg stands in for ffmpeg. A copy writes its first segment and waits;
// a fill writes a segment every 0.2 s from its -start_number. quirk runs
// first in a fill: refuseGPU, firstDies, or firstHangs.
func fillFFmpeg(t *testing.T, dir, quirk string) (bin, fills string) {
	t.Helper()
	bin = filepath.Join(dir, "ffmpeg")
	fills = filepath.Join(dir, "fills.log")
	pids := filepath.Join(dir, "pids.log")
	script := "#!/bin/sh\necho $$ >> " + pids + "\n" +
		"case \" $* \" in\n*fill%05d.ts*)\necho \"$*\" >> " + fills + "\n" + quirk +
		"n=0; prev=''; list=''\nfor a in \"$@\"; do\n[ \"$prev\" = -start_number ] && n=$a\nprev=$a; list=$a\ndone\n" +
		"while :; do\nf=$(printf 'fill%05d.ts' $n)\necho x > $f.tmp && mv $f.tmp $f\nprintf '#EXTINF:2.000,\\n%s\\n' $f >> $list\nn=$((n+1)); sleep 0.2\ndone\n;;\n" +
		"*\" -f hls \"*)\nprintf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:2.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nexec sleep 60\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killPids(pids) })
	return bin, fills
}

// fillHub plays a recording from 201 s, so 100 fill segments come before its copy.
// The quirks a fill stand-in can have: a GPU it can't start on, and a first
// run that exits or hangs before writing anything.
const (
	refuseGPU  = "case \" $* \" in\n*h264_vaapi*) exit 1 ;;\nesac\n"
	firstDies  = "if [ ! -f once ]; then touch once; exit 1; fi\n"
	firstHangs = "if [ ! -f once ]; then touch once; exec sleep 60; fi\n"
	alwaysDies = "exit 1\n"
	// stallsLater writes its first two segments, then nothing, the first time.
	stallsLater = "if [ ! -f once ]; then touch once\nn=0; prev=''; list=''\nfor a in \"$@\"; do\n[ \"$prev\" = -start_number ] && n=$a\nprev=$a; list=$a\ndone\n" +
		"for i in 1 2; do\nf=$(printf 'fill%05d.ts' $n)\necho x > $f.tmp && mv $f.tmp $f\nprintf '#EXTINF:2.000,\\n%s\\n' $f >> $list\nn=$((n+1))\ndone\nexec sleep 60\nfi\n"
	// slowStart writes its first segment's .tmp a little at a time for 1.5 s.
	slowStart = "f=$(printf 'fill%05d.ts' $(echo \"$*\" | sed 's/.*-start_number \\([0-9]*\\).*/\\1/'))\n" +
		"for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do echo x >> $f.tmp; sleep 0.1; done\n"
)

func fillHub(t *testing.T, encoder, quirk string) (*Hub, string, string) {
	t.Helper()
	dir := t.TempDir()
	bin, fills := fillFFmpeg(t, dir, quirk)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 300)
	h := &Hub{Dir: dir, Encoder: encoder, DeintBroadcast: "motion_adaptive", FileIdle: time.Minute, FFmpeg: bin}
	t.Cleanup(h.Shutdown)
	if _, err := h.PlayFile(1, rec, "mpeg2video", "broadcast", "progressive", 201); err != nil {
		t.Fatal(err)
	}
	if got := FileOffset(filepath.Join(dir, "file", "1")); got != 200 {
		t.Fatalf("copy from %v", got)
	}
	return h, dir, fills
}

func fillStarts(path string) []string {
	raw, _ := os.ReadFile(path)
	return strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(raw)), " ", "_"))
}

func fill(t *testing.T, h *Hub, n int) string {
	path, _ := h.FillSegment(t.Context(), 1, n)
	return path
}

func getFill(t *testing.T, h *Hub, n int) {
	t.Helper()
	began := time.Now()
	if got := fill(t, h, n); got == "" {
		t.Fatalf("fill %d did not come", n)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("fill %d took %v", n, took)
	}
}

// Two screens going back in one recording at different spots used to kill
// each other's fill encode on every request.
func TestTwoViewersKeepTheirOwnFill(t *testing.T) {
	h, _, fills := fillHub(t, "libx264", "")
	for i := range 4 {
		getFill(t, h, 10+i)
		getFill(t, h, 60+i)
	}
	if n := len(fillStarts(fills)); n != 2 {
		t.Fatalf("%d fill encodes for two viewers, want 2", n)
	}
}

// What a fill needs was only in memory: after a restart a player holding the
// list got empty packets for every fill segment.
func TestAFillIsMadeAfterARestart(t *testing.T) {
	h, dir, _ := fillHub(t, "libx264", "")
	h.Shutdown()
	again := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: h.FFmpeg}
	t.Cleanup(again.Shutdown)
	getFill(t, again, 5)
}

// A fill the GPU can't start moves to the processor, as the copy does, and
// stays there.
func TestAFillTheGPUCantStartUsesTheProcessor(t *testing.T) {
	prev := fillWait
	fillWait = 5 * time.Second
	t.Cleanup(func() { fillWait = prev })
	h, _, fills := fillHub(t, "h264_vaapi", refuseGPU)
	getFill(t, h, 3)
	getFill(t, h, 50)
	starts := fillStarts(fills)
	if len(starts) != 3 || !strings.Contains(starts[0], "h264_vaapi") {
		t.Fatalf("fill encodes:\n%s", strings.Join(starts, "\n"))
	}
	for _, s := range starts[1:] {
		if strings.Contains(s, "vaapi") || !strings.Contains(s, "libx264") {
			t.Fatalf("not on the processor: %s", s)
		}
	}
}

// A fill that died on the processor, for any reason, turned every fill of
// that copy into a 404 until the next restart. It is tried again after a pause.
func TestAFillThatDiedIsTriedAgain(t *testing.T) {
	prev := fillRetryAfter
	fillRetryAfter = 300 * time.Millisecond
	t.Cleanup(func() { fillRetryAfter = prev })
	h, _, fills := fillHub(t, "libx264", firstDies)
	// Resting is late, not gone: a player that gets a 404 may give up.
	if path, err := h.FillSegment(t.Context(), 1, 4); !errors.Is(err, ErrFillLate) {
		t.Fatalf("a fill that died: %q %v", path, err)
	}
	time.Sleep(400 * time.Millisecond)
	getFill(t, h, 4)
	if n := len(fillStarts(fills)); n != 2 {
		t.Fatalf("%d fill encodes, want 2", n)
	}
}

// A fill encode that runs and never writes was waited on by every request,
// each of which kept it from going idle: a 503 every 12 s for good. It is
// stopped when its first segment is late, and the next request starts afresh.
func TestAFillThatHangsIsReplaced(t *testing.T) {
	prevWait, prevFirst := fillWait, fillFirstWithin
	fillWait, fillFirstWithin = 3*time.Second, 500*time.Millisecond
	t.Cleanup(func() { fillWait, fillFirstWithin = prevWait, prevFirst })
	h, _, fills := fillHub(t, "libx264", firstHangs)
	getFill(t, h, 4)
	if n := len(fillStarts(fills)); n != 2 {
		t.Fatalf("%d fill encodes, want 2", n)
	}
}

// A slow start that is writing its first segment is not a hang: one on a busy
// processor or a slow disk was stopped and started again for good.
func TestASlowFillIsNotStopped(t *testing.T) {
	prevWait, prevFirst := fillWait, fillFirstWithin
	fillWait, fillFirstWithin = 5*time.Second, 500*time.Millisecond
	t.Cleanup(func() { fillWait, fillFirstWithin = prevWait, prevFirst })
	h, _, fills := fillHub(t, "libx264", slowStart)
	getFill(t, h, 4)
	if n := len(fillStarts(fills)); n != 1 {
		t.Fatalf("%d fill encodes for one slow start, want 1", n)
	}
}

// A fill that died on the processor every time answered a 503 every 30 s for
// good. Three in a row give the copy's fills up: a 404.
func TestAFillThatKeepsDyingIsGivenUp(t *testing.T) {
	prev := fillRetryAfter
	fillRetryAfter = 50 * time.Millisecond
	t.Cleanup(func() { fillRetryAfter = prev })
	h, _, fills := fillHub(t, "libx264", alwaysDies)
	var errs []error
	for range 5 {
		_, err := h.FillSegment(t.Context(), 1, 4)
		errs = append(errs, err)
		time.Sleep(100 * time.Millisecond)
	}
	for i, err := range errs {
		want := ErrFillLate
		if i >= fillDeathsMax-1 {
			want = ErrFillGone
		}
		if !errors.Is(err, want) {
			t.Fatalf("request %d: %v, want %v (all: %v)", i+1, err, want, errs)
		}
	}
	if n := len(fillStarts(fills)); n != fillDeathsMax {
		t.Fatalf("%d fill encodes, want %d", n, fillDeathsMax)
	}
}

// A request gave up after 12 s while a slow fill was still writing its first
// segment, and a hung one was only stopped after 20 s. A request waits while
// its encode writes.
func TestARequestWaitsWhileItsFillWrites(t *testing.T) {
	prevWait, prevFirst := fillWait, fillFirstWithin
	fillWait, fillFirstWithin = time.Second, 500*time.Millisecond
	t.Cleanup(func() { fillWait, fillFirstWithin = prevWait, prevFirst })
	h, _, fills := fillHub(t, "libx264", slowStart)
	getFill(t, h, 4)
	if n := len(fillStarts(fills)); n != 1 {
		t.Fatalf("%d fill encodes for one slow start, want 1", n)
	}
}

// Only a fill's first segment was watched: one that stalled after it was
// kept alive by the requests waiting on it. Every segment is watched.
func TestAFillThatStallsLaterIsReplaced(t *testing.T) {
	prevWait, prevFirst := fillWait, fillFirstWithin
	fillWait, fillFirstWithin = 3*time.Second, 500*time.Millisecond
	t.Cleanup(func() { fillWait, fillFirstWithin = prevWait, prevFirst })
	h, _, fills := fillHub(t, "libx264", stallsLater)
	getFill(t, h, 4)
	getFill(t, h, 6)
	if n := len(fillStarts(fills)); n != 2 {
		t.Fatalf("%d fill encodes, want 2", n)
	}
}
