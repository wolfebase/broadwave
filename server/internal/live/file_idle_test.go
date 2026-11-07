package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/disk"
)

// resumeFFmpeg is an ffmpeg stand-in that notes its arguments and writes a
// playlist with two segments when it is the first encode, then waits.
func resumeFFmpeg(t *testing.T, dir string) (bin, argsLog string) {
	t.Helper()
	bin = filepath.Join(dir, "ffmpeg")
	argsLog = filepath.Join(dir, "args.log")
	pids := filepath.Join(dir, "pids.log")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\n" +
		"echo $$ >> " + pids + "\n" +
		"echo \"$*\" >> " + argsLog + "\n" +
		"if [ ! -f index.m3u8 ]; then\n" +
		"printf '#EXTM3U\\n#EXT-X-VERSION:6\\n#EXT-X-TARGETDURATION:2\\n#EXT-X-MEDIA-SEQUENCE:0\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:2.000,\\nseg00000.ts\\n#EXTINF:2.000,\\nseg00001.ts\\n' > index.m3u8.tmp\n" +
		"echo x > seg00000.ts\necho x > seg00001.ts\nmv index.m3u8.tmp index.m3u8\nfi\n" +
		"exec sleep 60\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killPids(pids) })
	return bin, argsLog
}

// clockFile writes a recording with one PCR a second, at the given seconds.
func clockFile(t *testing.T, path string, seconds ...uint64) {
	t.Helper()
	var raw []byte
	for _, s := range seconds {
		raw = append(raw, pcrTS(0x100, s*90000)...)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func steadyFile(t *testing.T, path string, seconds int) {
	t.Helper()
	ticks := make([]uint64, seconds)
	for i := range ticks {
		ticks[i] = uint64(i)
	}
	clockFile(t, path, ticks...)
}

func killPids(path string) {
	raw, _ := os.ReadFile(path)
	for _, line := range strings.Fields(string(raw)) {
		if pid, _ := strconv.Atoi(line); pid > 0 {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
	}
}

func encodeRunning(h *Hub, id int64) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	return h.fileEnc[id] != nil
}

func awaitEncode(t *testing.T, h *Hub, id int64, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if encodeRunning(h, id) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("encode running is not %v", want)
}

func argLines(t *testing.T, path string, want int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, _ := os.ReadFile(path)
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(raw) > 0 && len(lines) >= want {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d encodes started, want %d", len(lines), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A moment of playback used to transcode the whole recording. Nobody asking
// for it stops the encode, and what it already wrote stays for the next play.
func TestAnAbandonedRecordingStopsEncoding(t *testing.T) {
	dir := t.TempDir()
	bin, _ := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 300 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	if _, err := os.Stat(filepath.Join(dir, "file", "7", "index.m3u8")); err != nil {
		t.Fatalf("the copy went with the encode: %v", err)
	}
}

// A slow client and a second viewer both count: any request keeps it going.
func TestRequestsKeepARecordingEncoding(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 400 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(until) {
		h.FileRequested(7)
		time.Sleep(100 * time.Millisecond)
		if !encodeRunning(h, 7) {
			t.Fatal("an encode with a viewer stopped")
		}
	}
	if n := len(argLines(t, argsLog, 1)); n != 1 {
		t.Fatalf("%d encodes for one steady viewer", n)
	}
}

// The next request carries on after the last whole segment: the numbering and
// the clock continue, so a list that already names later segments holds.
func TestTheNextRequestCarriesOnWhereTheEncodeStopped(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	h.FileRequested(7)
	lines := argLines(t, argsLog, 2)
	again := lines[1]
	for _, want := range []string{"-ss 4.000", "-output_ts_offset 4.000", "append_list"} {
		if !strings.Contains(again, want) {
			t.Fatalf("missing %q in %s", want, again)
		}
	}
	if strings.Contains(lines[0], "append_list") || strings.Contains(lines[0], "output_ts_offset") {
		t.Fatalf("a first encode has nothing to carry on from: %s", lines[0])
	}
}

// Opening the recording again after a stop carries on the copy. It used to serve
// the partial playlist with nothing writing it, and the player stalled.
func TestPlayingAPartialCopyCarriesOn(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	if !encodeRunning(h, 7) {
		t.Fatal("nothing is writing the rest of the recording")
	}
	lines := argLines(t, argsLog, 2)
	if !strings.Contains(lines[1], "-ss 4.000") {
		t.Fatalf("second encode: %s", lines[1])
	}
}

// After a restart nothing knows this copy's clock. A new viewer starts again.
func TestAPartialCopyFromAnEarlierRunStartsAgain(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: bin}
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	h.Shutdown()
	restarted := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: bin}
	t.Cleanup(restarted.stopFileEncodes)
	if _, err := restarted.PlayFile(7, rec, "mpeg2video", "broadwave", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	if !encodeRunning(restarted, 7) {
		t.Fatal("nothing is writing the recording")
	}
	lines := argLines(t, argsLog, 2)
	if strings.Contains(lines[1], "append_list") {
		t.Fatalf("carried on with a clock nobody read: %s", lines[1])
	}
}

// A kill can leave a list cut off. It is not carried on, or served as it is.
func TestATornPlaylistIsNotReused(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	torn := "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXTINF:2.000,\nseg00000.ts\n#EXTINF:2.000,\nseg000"
	playlist := filepath.Join(dir, "file", "7", "index.m3u8")
	if err := os.WriteFile(playlist, []byte(torn), 0o644); err != nil {
		t.Fatal(err)
	}
	h.FileRequested(7)
	time.Sleep(300 * time.Millisecond)
	if n := len(argLines(t, argsLog, 1)); n != 1 {
		t.Fatalf("%d encodes: carried on from a torn list", n)
	}
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	lines := argLines(t, argsLog, 2)
	if strings.Contains(lines[1], "append_list") {
		t.Fatalf("appended to a torn list: %s", lines[1])
	}
}

func TestEveryPlaybackEncodeUsesTempFiles(t *testing.T) {
	h := &Hub{Encoder: "libx264"}
	line := strings.Join(PictureArgs(h.fileGraph("MPEG2", "broadcast", "progressive")), " ")
	if !strings.Contains(line, "temp_file") {
		t.Fatalf("args: %s", line)
	}
}

func encodeKeepsRunning(t *testing.T, h *Hub, id int64, d time.Duration) {
	t.Helper()
	until := time.Now().Add(d)
	for time.Now().Before(until) {
		if !encodeRunning(h, id) {
			t.Fatal("the encode stopped with a clock that can't be trusted")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A clock reset makes the sum of segment times name the wrong place to seek
// to, so a join would repeat or skip show. That encode runs to the end.
func TestAnEncodeWithAnUntrustedClockIsNotStopped(t *testing.T) {
	for name, seconds := range map[string][]uint64{
		"went back to zero": {0, 1, 2, 3, 4, 5, 0, 1, 2, 3},
		"jumped ahead":      {0, 1, 2, 3, 4, 5, 40, 41, 42, 43},
		"no clock at all":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			bin, _ := resumeFFmpeg(t, dir)
			rec := filepath.Join(dir, "show.ts")
			clockFile(t, rec, seconds...)
			if len(seconds) == 0 {
				if err := os.WriteFile(rec, make([]byte, 188*64), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 100 * time.Millisecond, FFmpeg: bin}
			t.Cleanup(h.stopFileEncodes)
			if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
				t.Fatal(err)
			}
			encodeKeepsRunning(t, h, 7, 500*time.Millisecond)
		})
	}
}

func TestADiscontinuityFlagIsAReset(t *testing.T) {
	c := newFileClock(0)
	flagged := pcrTS(0x100, 2*90000)
	flagged[5] |= 0x80
	raw := append(append(pcrTS(0x100, 0), pcrTS(0x100, 90000)...), flagged...)
	c.feed(0, raw)
	if c.safe(true) {
		t.Fatal("a discontinuity reads as straight")
	}
}

// The same for a recording still being written, seen as the copy reads it.
func TestAFollowThatSeesAResetIsNotStopped(t *testing.T) {
	dir := t.TempDir()
	bin, _ := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	clockFile(t, rec, 0, 1, 2, 3, 0, 1, 2)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 100 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFollow(8, rec, "mpeg2video", "broadcast", "progressive", 0, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	encodeKeepsRunning(t, h, 8, 500*time.Millisecond)
}

// ffmpeg dies on a file with a reset. mediaSpan can't be believed there, so the
// list is not closed with show left, and requests don't start it over and over.
func TestAnEncodeThatDiesOnAResetLeavesTheListOpen(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg")
	starts := filepath.Join(dir, "starts")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\necho x >> " + starts + "\n" +
		"printf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:6.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nsleep 0.3\nexit 1\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(dir, "show.ts")
	clockFile(t, rec, 0, 1, 2, 3, 0, 1, 2)
	h := &Hub{Dir: dir, Encoder: "libx264", FFmpeg: bin}
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	time.Sleep(50 * time.Millisecond)
	for range 10 {
		h.FileRequested(7)
		time.Sleep(20 * time.Millisecond)
	}
	if playlistEnded(filepath.Join(dir, "file", "7", "index.m3u8")) {
		t.Fatal("the list was closed with show left")
	}
	raw, _ := os.ReadFile(starts)
	if n := strings.Count(string(raw), "x"); n != 1 {
		t.Fatalf("%d starts", n)
	}
}

// A carry-on of a growing recording doesn't read what came before: after any
// 30 s pause, on a share, that could take minutes.
func TestACarryOnSeeksInsteadOfReadingTheFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg")
	argsLog := filepath.Join(dir, "args.log")
	pids := filepath.Join(dir, "pids.log")
	second := filepath.Join(dir, "second.bin")
	list := "#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:100.000,\\nseg00000.ts\\n"
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\necho $$ >> " + pids + "\necho \"$*\" >> " + argsLog + "\n" +
		"if [ -f index.m3u8 ]; then\nexec cat > " + second + "\nfi\n" +
		"printf '" + list + "' > index.m3u8\necho x > seg00000.ts\nexec cat > /dev/null\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killPids(pids) })
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 120)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 500 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFollow(8, rec, "mpeg2video", "broadcast", "progressive", 0, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 8, false)
	// What's before the last mark can't be read any more, and doesn't need to be.
	raw, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 80 * 188 {
		raw[i] = 0xff
	}
	if err := os.WriteFile(rec, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	h.FileRequested(8)
	want := int64(20 * 188)
	deadline := time.Now().Add(3 * time.Second)
	for {
		info, err := os.Stat(second)
		if err == nil && info.Size() == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("second copy: %v %v, want %d bytes from the clock", info, err, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if pcr, ok := readPCR(got[:188]); !ok || pcr != 100*90000 {
		t.Fatalf("first packet clock %d %v", pcr, ok)
	}
}

// The tick that starts over for a resume the clock never reached starts its
// clock over too.
func TestAClockThatStartsOverCanBeUsedAgain(t *testing.T) {
	c := newFileClock(30)
	c.feed(0, pcrTS(0x100, 0))
	c.restartAt(0)
	raw := append(pcrTS(0x100, 0), pcrTS(0x100, 90000)...)
	c.feed(0, append(raw, pcrTS(0x100, 2*90000)...))
	if !c.safe(true) || c.end() != 2 {
		t.Fatalf("safe %v end %v", c.safe(true), c.end())
	}
}

// Shutdown stops the encodes before the HTTP server, and a player still asking
// must not start one that outlives the process.
func TestARequestDuringShutdownStartsNoEncode(t *testing.T) {
	dir := t.TempDir()
	bin, argsLog := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: bin}
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	h.Shutdown()
	h.FileRequested(7)
	time.Sleep(200 * time.Millisecond)
	if encodeRunning(h, 7) {
		t.Fatal("a request started an encode during shutdown")
	}
	if n := len(argLines(t, argsLog, 1)); n != 1 {
		t.Fatalf("%d encodes", n)
	}
}

// ffmpeg that cannot go on must not be started again by every player request.
func TestAFailingResumeIsNotRetriedOnEveryRequest(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg")
	starts := filepath.Join(dir, "starts")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\necho x >> " + starts + "\n" +
		"if [ ! -f index.m3u8 ]; then\nprintf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:2.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nexec sleep 60\nfi\nexit 1\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(dir, "show.ts")
	// A clock far past the two seconds written: this is not the end of the file.
	steadyFile(t, rec, 600)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	for range 20 {
		h.FileRequested(7)
		time.Sleep(50 * time.Millisecond)
	}
	got, _ := os.ReadFile(starts)
	if n := strings.Count(string(got), "x"); n != 2 {
		t.Fatalf("%d starts, want the first and one failed resume", n)
	}
	if playlistEnded(filepath.Join(dir, "file", "7", "index.m3u8")) {
		t.Fatal("a failed encode ended the list ten minutes early")
	}
	// Pressing play is a person asking again.
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, _ = os.ReadFile(starts)
		if strings.Count(string(got), "x") == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("play did not try again: %q", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Stopped on the last seconds of the file, the seek finds no picture and
// ffmpeg writes nothing. The list still ends, or the player waits forever.
func TestAResumeAtTheEndClosesTheList(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\n" +
		"if [ ! -f index.m3u8 ]; then\nprintf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:6.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nexec sleep 60\nfi\nexit 1\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 8)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 7, false)
	h.FileRequested(7)
	playlist := filepath.Join(dir, "file", "7", "index.m3u8")
	deadline := time.Now().Add(3 * time.Second)
	for !playlistEnded(playlist) {
		if time.Now().After(deadline) {
			t.Fatal("the list never ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A recording still being written restarts its copy at the clock of the last
// whole segment, and the bytes before it are not sent again.
func TestAStoppedFollowCarriesOnAtTheClock(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg")
	argsLog := filepath.Join(dir, "args.log")
	pids := filepath.Join(dir, "pids.log")
	second := filepath.Join(dir, "second.bin")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\necho $$ >> " + pids + "\necho \"$*\" >> " + argsLog + "\n" +
		"if [ -f index.m3u8 ]; then\nexec cat > " + second + "\nfi\n" +
		"printf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:4.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nexec cat > /dev/null\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killPids(pids) })
	rec := filepath.Join(dir, "show.ts")
	var raw []byte
	for _, tick := range []uint64{0, 2 * 90000, 4 * 90000, 6 * 90000} {
		raw = append(raw, pcrTS(0x100, tick)...)
	}
	if err := os.WriteFile(rec, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: 200 * time.Millisecond, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	still := func() bool { return true }
	if _, err := h.PlayFollow(8, rec, "mpeg2video", "broadcast", "progressive", 0, still); err != nil {
		t.Fatal(err)
	}
	awaitEncode(t, h, 8, false)
	h.FileRequested(8)
	lines := argLines(t, argsLog, 2)
	if !strings.Contains(lines[1], "-i pipe:0") || !strings.Contains(lines[1], "-output_ts_offset 4.000") {
		t.Fatalf("resume: %s", lines[1])
	}
	want := len(pcrTS(0x100, 0)) * 2
	deadline := time.Now().Add(3 * time.Second)
	for {
		info, err := os.Stat(second)
		if err == nil && info.Size() == int64(want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("second copy: %v %v, want %d bytes from the clock", info, err, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Real ffmpeg: a stop part way through and a carry-on make the same list the
// player already has, longer, with a clock that runs straight across the join.
func TestAResumedEncodeJoinsTheOneBeforeIt(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH")
	}
	dir := t.TempDir()
	rec := filepath.Join(dir, "show.ts")
	build := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=s=160x90:r=30000/1001:d=20",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=20",
		"-c:v", "mpeg2video", "-b:v", "800k", "-g", "15",
		"-c:a", "ac3", "-b:a", "96k", "-shortest", "-f", "mpegts", rec)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: ffmpeg}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(1, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	play := filepath.Join(dir, "file", "1")
	playlist := filepath.Join(play, "index.m3u8")
	awaitEnded(t, playlist)

	// What a stop leaves: the first segments listed, a torn one after them.
	body, err := os.ReadFile(playlist)
	if err != nil {
		t.Fatal(err)
	}
	pl, ok := parseMediaPlaylist(body)
	if !ok || len(pl.segs) < 6 {
		t.Fatalf("first encode: %s", body)
	}
	keep := 3
	lines := strings.Split(string(body), "\n")
	var partial []string
	seen := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-ENDLIST") {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME") || strings.HasPrefix(line, "#EXTINF") {
			if seen == keep {
				break
			}
		}
		partial = append(partial, line)
		if strings.HasPrefix(line, "seg") {
			seen++
		}
	}
	if err := os.WriteFile(playlist, []byte(strings.Join(partial, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := keep + 1; i < len(pl.segs); i++ {
		_ = os.Remove(filepath.Join(play, fmt.Sprintf("seg%05d.ts", i)))
	}
	if err := os.Truncate(filepath.Join(play, fmt.Sprintf("seg%05d.ts", keep)), 100); err != nil {
		t.Fatal(err)
	}
	before := h.StableRecordingPlaylist(play, mustRead(t, playlist))
	if len(before) == 0 {
		t.Fatal("no playlist before the stop")
	}
	abs, err := filepath.Abs(rec)
	if err != nil {
		t.Fatal(err)
	}
	job := &fileJob{path: abs, graph: h.fileGraphFor(rec, "mpeg2video", "broadcast", "progressive"), clock: newFileClock(0)}
	job.clock.scan(abs, func() bool { return false })
	if !job.clock.safe(false) {
		t.Fatal("a generated file's clock reads as unsteady")
	}
	h.keepFileJob(1, job)

	if _, err := h.PlayFile(1, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	awaitEnded(t, playlist)
	after := h.StableRecordingPlaylist(play, mustRead(t, playlist))
	if issues := CheckRecordingPlaylist(after, play); len(issues) > 0 {
		t.Fatalf("issues %v\n%s", issues, after)
	}
	if issues := CheckRecordingUpdate(before, after); len(issues) > 0 {
		t.Fatalf("update %v\n%s\n%s", issues, before, after)
	}
	if strings.Contains(string(after), "DISCONTINUITY") {
		t.Fatalf("the clock runs on, so nothing is discontinuous:\n%s", after)
	}
	done, _ := parseMediaPlaylist(after)
	var total float64
	numbered := regexp.MustCompile(`^seg(\d+)\.ts$`)
	for i, s := range done.segs {
		total += s.dur
		m := numbered.FindStringSubmatch(s.uri)
		if m == nil {
			t.Fatalf("segment %q", s.uri)
		}
		if n, _ := strconv.Atoi(m[1]); n != i {
			t.Fatalf("segment %d is named %s", i, s.uri)
		}
	}
	if total < 19 || total > 21 {
		t.Fatalf("the list covers %.2f s of a 20 s recording", total)
	}
	lastBefore := lastVideoPTS(t, filepath.Join(play, fmt.Sprintf("seg%05d.ts", keep-1)))
	firstAfter := firstVideoPTS(t, filepath.Join(play, fmt.Sprintf("seg%05d.ts", keep)))
	if step := firstAfter - lastBefore; step < 0 || step > 0.1 {
		t.Fatalf("the clock jumps %.3f s across the join (%.3f to %.3f)", step, lastBefore, firstAfter)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func awaitEnded(t *testing.T, playlist string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !playlistEnded(playlist) {
		if time.Now().After(deadline) {
			t.Fatal("the encode never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func videoPTS(t *testing.T, path string) []float64 {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Packets []struct {
			PTS string `json:"pts_time"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	var times []float64
	for _, p := range doc.Packets {
		v, err := strconv.ParseFloat(p.PTS, 64)
		if err == nil {
			times = append(times, v)
		}
	}
	if len(times) == 0 {
		t.Fatalf("no pictures in %s", path)
	}
	return times
}

func firstVideoPTS(t *testing.T, path string) float64 {
	t.Helper()
	times := videoPTS(t, path)
	first := times[0]
	for _, v := range times {
		if v < first {
			first = v
		}
	}
	return first
}

func lastVideoPTS(t *testing.T, path string) float64 {
	t.Helper()
	times := videoPTS(t, path)
	last := times[0]
	for _, v := range times {
		if v > last {
			last = v
		}
	}
	return last
}

func copyDir(t *testing.T, h *Hub, id int64, size int, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(h.Dir, "file", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seg00000.ts"), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-age)
	if err := os.Chtimes(dir, at, at); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestTheSweepRemovesCopiesNobodyHasPlayedLately(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	old := copyDir(t, h, 1, 10, fileKeepFor+time.Hour)
	recent := copyDir(t, h, 2, 10, fileKeepFor-time.Hour)
	other := filepath.Join(h.Dir, "file", "not-a-recording")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	h.SweepFileCopies(time.Now())
	if exists(old) {
		t.Fatal("a copy a day old stayed")
	}
	if !exists(recent) {
		t.Fatal("a recent copy went")
	}
	if !exists(other) {
		t.Fatal("the sweep removed something it did not make")
	}
}

func TestTheSweepKeepsCopiesUnderTheLimit(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	prev := fileKeepBytes
	fileKeepBytes = 100
	t.Cleanup(func() { fileKeepBytes = prev })
	oldest := copyDir(t, h, 1, 60, fileLimitIdle+5*time.Hour)
	middle := copyDir(t, h, 2, 60, fileLimitIdle+4*time.Hour)
	newest := copyDir(t, h, 3, 30, fileLimitIdle+3*time.Hour)
	h.SweepFileCopies(time.Now())
	if exists(oldest) {
		t.Fatal("over the limit, the oldest copy stayed")
	}
	if !exists(middle) || !exists(newest) {
		t.Fatal("the sweep removed more than it had to")
	}
}

// A paused viewer asks for nothing. Being over the limit does not take the
// copy out from under them while it is this recent.
func TestTheLimitLeavesARecentlyPlayedCopy(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	prev := fileKeepBytes
	fileKeepBytes = 10
	t.Cleanup(func() { fileKeepBytes = prev })
	paused := copyDir(t, h, 1, 60, fileLimitIdle-time.Minute)
	seen := copyDir(t, h, 2, 60, fileLimitIdle+5*time.Hour)
	h.FileRequested(2)
	h.SweepFileCopies(time.Now())
	if !exists(paused) {
		t.Fatal("a copy written minutes ago went")
	}
	if !exists(seen) {
		t.Fatal("a copy a viewer just asked for went")
	}
}

func TestTheSweepLeavesARunningEncode(t *testing.T) {
	dir := t.TempDir()
	bin, _ := resumeFFmpeg(t, dir)
	rec := filepath.Join(dir, "show.ts")
	steadyFile(t, rec, 60)
	h := &Hub{Dir: dir, Encoder: "libx264", FileIdle: time.Minute, FFmpeg: bin}
	t.Cleanup(h.stopFileEncodes)
	if _, err := h.PlayFile(7, rec, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	play := filepath.Join(dir, "file", "7")
	at := time.Now().Add(-2 * fileKeepFor)
	if err := os.Chtimes(play, at, at); err != nil {
		t.Fatal(err)
	}
	h.SweepFileCopies(time.Now())
	if !exists(play) {
		t.Fatal("the sweep removed a copy being written")
	}
}

// One game's copy passes the limit by itself. A viewer who paused it for the
// evening gets their picture back: a 404 makes the player give up.
func TestAPausedGameKeepsItsCopyPastTheLimit(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	prev := fileKeepBytes
	fileKeepBytes = 10
	t.Cleanup(func() { fileKeepBytes = prev })
	game := copyDir(t, h, 1, 60, 8*time.Hour)
	h.SweepFileCopies(time.Now())
	if !exists(game) {
		t.Fatal("a copy paused for 8 hours went")
	}
}

func standInDisk(t *testing.T, free uint64) {
	t.Helper()
	prev := diskStat
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: free, Total: 100 << 30}, nil }
	t.Cleanup(func() { diskStat = prev })
}

// A disk about to fill comes before a viewer who may be gone.
func TestANearlyFullDiskTakesAQuietCopy(t *testing.T) {
	standInDisk(t, 1<<30)
	h := &Hub{Dir: t.TempDir()}
	quiet := copyDir(t, h, 1, 60, fileRoomIdle+time.Minute)
	played := copyDir(t, h, 2, 60, fileRoomIdle-time.Minute)
	h.SweepFileCopies(time.Now())
	if exists(quiet) {
		t.Fatal("a nearly full disk kept a copy nobody asked for")
	}
	if !exists(played) {
		t.Fatal("a copy asked for a minute ago went")
	}
}

// The first pass is at startup: nothing else cleans what an earlier run left.
func TestTheSweepRunsAtStartup(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	old := copyDir(t, h, 1, 10, fileKeepFor+time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.RunFileSweep(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for exists(old) {
		if time.Now().After(deadline) {
			t.Fatal("startup left the old copy")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
