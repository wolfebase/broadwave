package live

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type bufferCloser struct{ bytes.Buffer }

func (bufferCloser) Close() error { return nil }

func TestFollowFileReadsBytesWrittenLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "show.ts")
	if err := os.WriteFile(path, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bufferCloser
	var still atomic.Bool
	still.Store(true)
	done := make(chan struct{})
	go func() {
		followFile(path, &buf, still.Load, 0)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("bbb")); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	time.Sleep(400 * time.Millisecond)
	still.Store(false)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("follower did not finish")
	}
	if buf.String() != "aaabbb" {
		t.Fatalf("read %q", buf.String())
	}
}

// A resume into a file that is still growing starts at that clock, then
// keeps the bytes that arrive after it. The packets before it are not sent.
func TestFollowFileStartsAtTheResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "show.ts")
	var raw []byte
	for _, tick := range []uint64{0, 5 * 90000, 10 * 90000, 15 * 90000} {
		raw = append(raw, pcrTS(0x100, tick)...)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bufferCloser
	var still atomic.Bool
	still.Store(true)
	done := make(chan struct{})
	go func() {
		followFile(path, &buf, still.Load, 10)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	later := pcrTS(0x100, 20*90000)
	if _, err := f.Write(later); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	time.Sleep(400 * time.Millisecond)
	still.Store(false)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("follower did not finish")
	}
	want := append(append(pcrTS(0x100, 10*90000), pcrTS(0x100, 15*90000)...), later...)
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("copied %d bytes, want the resume and what was appended (%d)", buf.Len(), len(want))
	}
}

func pcrTS(pid uint16, ticks uint64) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid >> 8)
	pkt[2] = byte(pid)
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

// Replacing an encode deletes its segments. The process that was writing
// them has to be gone first, or the new seg00000 is the old picture.
func TestPlayFileStopsTheEncodeItReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "show.ts")
	if err := os.WriteFile(path, mpeg2TS(1, true), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ffmpeg")
	pidPath := filepath.Join(dir, "pid")
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\necho $$ > " + pidPath + "\nprintf '%s\\n' \"$@\" >> " + argsPath + "\ncat > index.m3u8 << 'EOF'\n#EXTM3U\n#EXTINF:2.000,\nseg00000.ts\nEOF\necho x > seg00000.ts\nexec sleep 60\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		body, err := os.ReadFile(pidPath)
		if err != nil {
			return
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	h := &Hub{Dir: dir, Encoder: "libx264", FFmpeg: bin}
	if _, err := h.PlayFile(4, path, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil || first <= 0 {
		t.Fatalf("pid %q", body)
	}
	if _, err := h.PlayFile(4, path, "mpeg2video", "broadcast", "progressive", 40); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(first, 0); err == nil {
		t.Fatalf("first encode %d still running", first)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !inputSeek(string(args), "40.000") {
		t.Fatalf("args:\n%s", args)
	}
}

func TestRecordingOrderPrefersTheFile(t *testing.T) {
	if recordingOrder("progressive", true, "tt") != "progressive" {
		t.Fatal("the recording's headers win over the channel")
	}
	if recordingOrder("film", true, "tt") != "film" {
		t.Fatal("soft telecine in the file is this recording's scan")
	}
	if recordingOrder("tt", true, "progressive") != "tt" {
		t.Fatal("an interlaced recording must not inherit a progressive channel")
	}
	if recordingOrder("", false, "progressive") != "progressive" {
		t.Fatal("a file with no header yet uses the channel scan")
	}
	if recordingOrder("", false, "film") != "" || recordingOrder("", false, "tt") != "" {
		t.Fatal("a stored film flag and an interlaced channel stay on the field graph")
	}
}

func TestFileScanReadsProgressiveHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.ts")
	if err := os.WriteFile(path, mpeg2TS(1, true), 0o644); err != nil {
		t.Fatal(err)
	}
	order, ok := fileScanOrder(path)
	if !ok || order != "progressive" {
		t.Fatalf("scan %q ok=%v", order, ok)
	}
	if _, ok := fileScanOrder(filepath.Join(t.TempDir(), "missing.ts")); ok {
		t.Fatal("a missing recording has no scan")
	}
}

// A 720p60 MPEG-2 recording used to take the interlaced graph: field bob, then
// a 59.94 cap. The file's sequence header keeps every frame at the source size.
func TestProgressiveRecordingKeepsNativeRate(t *testing.T) {
	h := &Hub{Encoder: "libx264", DeintBroadcast: "motion_adaptive"}
	prog := h.fileGraph("MPEG2", "broadcast", "progressive")
	if !prog.Progressive || prog.Mode != "broadcast" {
		t.Fatalf("progressive graph: %+v", prog)
	}
	line := strings.Join(PictureArgs(prog), " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "deinterlace_vaapi") || strings.Contains(line, "fps=") {
		t.Fatalf("720p recording was field-bobbed: %s", line)
	}
	if !strings.Contains(line, "min(1920,iw)") || !strings.Contains(line, "min(1080,ih)") {
		t.Fatalf("scale should not upscale 720p: %s", line)
	}
	bobbed := strings.Join(PictureArgs(h.fileGraph("MPEG2", "broadcast", "")), " ")
	if !strings.Contains(bobbed, "bwdif=mode=send_field") || !strings.Contains(bobbed, "fps=60000/1001") {
		t.Fatalf("an interlaced recording still plays at field rate: %s", bobbed)
	}
	if graphStamp(prog) == graphStamp(h.fileGraph("MPEG2", "broadcast", "tt")) {
		t.Fatal("a bobbed playlist must not be reused for a progressive recording")
	}

	va := &Hub{Encoder: "h264_vaapi", DeintBroadcast: "motion_adaptive"}
	vaLine := strings.Join(PictureArgs(va.fileGraph("MPEG2", "broadcast", "progressive")), " ")
	for _, want := range []string{"-hwaccel_output_format vaapi", "scale_vaapi=w='min(1920,iw)'"} {
		if !strings.Contains(vaLine, want) {
			t.Fatalf("missing %q in %s", want, vaLine)
		}
	}
	for _, bad := range []string{"deinterlace_vaapi", "bwdif", "fps=", "hwupload"} {
		if strings.Contains(vaLine, bad) {
			t.Fatalf("unexpected %q in %s", bad, vaLine)
		}
	}
	tb := strings.Join(PictureArgs((&Hub{Encoder: "h264_videotoolbox"}).fileGraph("MPEG2", "broadcast", "progressive")), " ")
	if strings.Contains(tb, "bwdif") || strings.Contains(tb, "fps=") || !strings.Contains(tb, "-a53cc 0") {
		t.Fatalf("videotoolbox 720p: %s", tb)
	}

	film := h.fileGraph("MPEG2", "broadcast", "film")
	if film.Progressive || film.Mode != "film" {
		t.Fatalf("film graph: %+v", film)
	}
	filmLine := strings.Join(PictureArgs(film), " ")
	if !strings.Contains(filmLine, "pullup") || !strings.Contains(filmLine, "fps=24000/1001") || strings.Contains(filmLine, "bwdif") {
		t.Fatalf("film recording: %s", filmLine)
	}
	smooth := h.fileGraph("MPEG2", "smooth", "film")
	if smooth.Mode != "smooth" {
		t.Fatalf("an explicit smooth choice stays: %+v", smooth)
	}
}

func TestProgressiveRecordingStays720p60(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "rec.ts")
	// Horizontal bars comb if a field bob invents lines. A progressive encode stays clean.
	bars := "nullsrc=s=1280x720:r=60000/1001:d=1,format=yuv420p,geq=r='clip(128+100*sin((Y+N*6)/5),0,255)':g=128:b=128"
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", bars,
		"-c:v", "mpeg2video", "-b:v", "8M", "-f", "mpegts", in)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	order, ok := fileScanOrder(in)
	if !ok || order != "progressive" {
		t.Fatalf("scan %q ok=%v", order, ok)
	}
	h := &Hub{Encoder: "libx264"}
	args := PictureArgs(h.fileGraph("MPEG2", "broadcast", order))
	line := strings.Join(args, " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "fps=") {
		t.Fatalf("graph bobbed a progressive recording: %s", line)
	}
	out := filepath.Join(dir, "out.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", in, "-an", "-vf", filterOf(args), "-c:v", "libx264", "-t", "1", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("encode: %v %s", err, msg)
	}
	width, height, fps, frames := probePicture(t, out)
	t.Logf("progressive recording %dx%d %.3f fps %d frames", width, height, fps, frames)
	if width != 1280 || height != 720 {
		t.Fatalf("size %dx%d, want 1280x720", width, height)
	}
	if fps < 58 || fps > 61 {
		t.Fatalf("fps %.3f, want about 59.94", fps)
	}
	if frames < 52 || frames > 68 {
		t.Fatalf("frames %d, want about 60", frames)
	}
	interlaced, progressive := idet(t, out)
	if interlaced > 2 && interlaced*4 > progressive {
		t.Fatalf("idet interlaced=%d progressive=%d", interlaced, progressive)
	}
	kept := mpdecimate(t, out)
	if kept < frames-4 {
		t.Fatalf("mpdecimate kept %d of %d (duplicate frames)", kept, frames)
	}
}

// A resume deep into a recording that has never been played must encode from
// that point. Starting at the beginning makes the player wait out the prefix.
func TestPlayFileResumeMatchesTheRecording(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "show.ts")
	// Red climbs with time, and a keyframe lands on the resume point.
	build := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "nullsrc=s=160x90:r=30:d=8,format=yuv420p,geq=r='clip(40*T,0,255)':g=16:b=16",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=8",
		"-force_key_frames", "expr:gte(t,n_forced*2)",
		"-c:v", "mpeg2video", "-b:v", "800k", "-g", "15",
		"-c:a", "ac3", "-b:a", "96k", "-shortest", "-f", "mpegts", in)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	h := &Hub{Dir: dir, Encoder: "libx264", FFmpeg: ffmpeg}
	if _, err := h.PlayFile(1, in, "mpeg2video", "broadcast", "progressive", 4); err != nil {
		t.Fatal(err)
	}
	play := filepath.Join(dir, "file", "1")
	seg := filepath.Join(play, "seg00000.ts")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		info, err := os.Stat(seg)
		if err == nil && info.Size() > 1000 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	got := meanRed(t, ffmpeg, seg, 0)
	atStart := meanRed(t, ffmpeg, in, 0)
	atResume := meanRed(t, ffmpeg, in, 4)
	t.Logf("red segment %.1f start %.1f resume %.1f", got, atStart, atResume)
	if abs(got-atResume) >= abs(got-atStart) {
		t.Fatalf("first segment red %.1f is closer to the start (%.1f) than to 4s (%.1f)", got, atStart, atResume)
	}
	if got < 100 {
		t.Fatalf("first segment red %.1f, want the picture from about 4s", got)
	}
	off := FileOffset(play)
	if off < 3.9 || off > 4.1 {
		t.Fatalf("offset %v", off)
	}
}

func meanRed(t *testing.T, ffmpeg, path string, at float64) float64 {
	t.Helper()
	args := []string{"-hide_banner", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at, 'f', 3, 64))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	out, err := exec.Command(ffmpeg, args...).Output()
	if err != nil || len(out) < 3 {
		t.Fatalf("frame %s at %v: %v (%d bytes)", path, at, err, len(out))
	}
	var sum float64
	n := 0
	for i := 0; i+2 < len(out); i += 3 {
		sum += float64(out[i])
		n++
	}
	return sum / float64(n)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func probePicture(t *testing.T, path string) (width, height int, fps float64, frames int) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-count_frames",
		"-show_entries", "stream=width,height,r_frame_rate,nb_read_frames", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 4 {
		t.Fatalf("probe %q", out)
	}
	width, _ = strconv.Atoi(parts[0])
	height, _ = strconv.Atoi(parts[1])
	rate := strings.Split(parts[2], "/")
	num, _ := strconv.ParseFloat(rate[0], 64)
	den := 1.0
	if len(rate) == 2 {
		den, _ = strconv.ParseFloat(rate[1], 64)
	}
	frames, _ = strconv.Atoi(strings.TrimSpace(parts[3]))
	return width, height, num / den, frames
}
