package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fillFFmpeg stands in for ffmpeg. A copy writes its first segment and waits;
// a fill writes a segment every 0.2 s from its -start_number. With refuseGPU a
// fill on the GPU exits at once, as one the GPU can't start does.
func fillFFmpeg(t *testing.T, dir string, refuseGPU bool) (bin, fills string) {
	t.Helper()
	bin = filepath.Join(dir, "ffmpeg")
	fills = filepath.Join(dir, "fills.log")
	pids := filepath.Join(dir, "pids.log")
	refuse := ""
	if refuseGPU {
		refuse = "case \" $* \" in\n*h264_vaapi*) exit 1 ;;\nesac\n"
	}
	script := "#!/bin/sh\necho $$ >> " + pids + "\n" +
		"case \" $* \" in\n*fill%05d.ts*)\necho \"$*\" >> " + fills + "\n" + refuse +
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
func fillHub(t *testing.T, encoder string, refuseGPU bool) (*Hub, string, string) {
	t.Helper()
	dir := t.TempDir()
	bin, fills := fillFFmpeg(t, dir, refuseGPU)
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
	h, _, fills := fillHub(t, "libx264", false)
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
	h, dir, _ := fillHub(t, "libx264", false)
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
	h, _, fills := fillHub(t, "h264_vaapi", true)
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
