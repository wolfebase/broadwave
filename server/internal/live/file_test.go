package live

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
		followFile(path, &buf, still.Load)
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
