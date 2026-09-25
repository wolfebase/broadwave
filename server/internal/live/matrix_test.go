package live

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestScanMatrixPicture builds the scan-type fixtures and measures the software
// and VideoToolbox graphs. VAAPI is measured on TUS by the picture lab.
func TestScanMatrixPicture(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	encoders := []string{"libx264"}
	if enc, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output(); err == nil && strings.Contains(string(enc), "h264_videotoolbox") {
		encoders = append(encoders, "h264_videotoolbox")
	}
	// Moving horizontal bars comb when interlaced and come apart after a field bob.
	bars := func(size, rate, dur string) string {
		return "nullsrc=s=" + size + ":r=" + rate + ":d=" + dur + ",format=yuv420p,geq=r='clip(128+100*sin((Y+N*6)/5),0,255)':g=128:b=128"
	}
	cases := []struct {
		name   string
		src    string
		build  []string
		source Source
		fps    float64
		frames int
	}{
		{"720p", "p720.ts", []string{"-f", "lavfi", "-i", bars("640x360", "60000/1001", "1"), "-c:v", "mpeg2video", "-b:v", "2M", "-f", "mpegts"}, Source{VideoCodec: "MPEG2", Progressive: true}, 59.94, 60},
		{"1080i", "i1080.ts", []string{"-f", "lavfi", "-i", bars("640x360", "60000/1001", "1"), "-vf", "tinterlace=mode=interleave_top", "-c:v", "mpeg2video", "-b:v", "2M", "-flags", "+ildct+ilme", "-top", "1", "-f", "mpegts"}, Source{VideoCodec: "MPEG2"}, 59.94, 60},
		{"480i", "i480.ts", []string{"-f", "lavfi", "-i", bars("320x240", "60000/1001", "1"), "-vf", "tinterlace=mode=interleave_top", "-c:v", "mpeg2video", "-b:v", "1M", "-flags", "+ildct+ilme", "-top", "1", "-f", "mpegts"}, Source{VideoCodec: "MPEG2"}, 59.94, 60},
		{"telecine", "film.ts", []string{"-f", "lavfi", "-i", "nullsrc=s=640x360:r=24000/1001:d=1,format=yuv420p,geq=r='if(lt(abs(X-mod(N*12\\,640)),30),240,20)':g=128:b=40", "-vf", "telecine=pattern=23", "-c:v", "mpeg2video", "-b:v", "4M", "-flags", "+ildct+ilme", "-top", "1", "-f", "mpegts"}, Source{VideoCodec: "MPEG2", Film: true}, 23.976, 24},
		{"paff", "paff.ts", []string{"-f", "lavfi", "-i", bars("640x360", "60000/1001", "1"), "-vf", "tinterlace=mode=interleave_top", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-x264-params", "tff=1", "-flags", "+ildct+ilme", "-f", "mpegts"}, Source{VideoCodec: "H264", Lace: true}, 59.94, 60},
		{"mbaff", "mbaff.ts", []string{"-f", "lavfi", "-i", bars("640x360", "60000/1001", "1"), "-vf", "tinterlace=mode=interleave_top", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-x264-params", "tff=1:interlaced=1", "-flags", "+ildct+ilme", "-f", "mpegts"}, Source{VideoCodec: "H264", Lace: true}, 59.94, 60},
	}
	for _, tc := range cases {
		in := filepath.Join(dir, tc.src)
		args := append([]string{"-hide_banner", "-loglevel", "error"}, tc.build...)
		args = append(args, in)
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("fixture %s: %v %s", tc.name, err, out)
		}
		for _, enc := range encoders {
			t.Run(tc.name+"/"+enc, func(t *testing.T) {
				out := filepath.Join(dir, tc.name+"-"+enc+".mp4")
				vf := filterOf(RenditionArgs(0, tc.source, Rendition{Video: "1080", Audio: "none"}, enc, "motion_adaptive"))
				cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", in, "-an", "-vf", vf, "-c:v", enc, "-t", "1", out)
				if enc == "h264_videotoolbox" {
					cmd = exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", in, "-an", "-vf", vf, "-c:v", enc, "-a53cc", "0", "-t", "1", out)
				}
				if msg, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("encode: %v %s", err, msg)
				}
				fps, frames := measure(t, out)
				if frames < tc.frames-8 || frames > tc.frames+12 {
					t.Fatalf("frames %d, want about %d", frames, tc.frames)
				}
				if fps < tc.fps-2 || fps > tc.fps+2 {
					t.Fatalf("fps %.3f, want about %.3f", fps, tc.fps)
				}
				interlaced, progressive := idet(t, out)
				t.Logf("fps=%.3f frames=%d idet interlaced=%d progressive=%d", fps, frames, interlaced, progressive)
				if interlaced > 2 && interlaced*4 > progressive {
					t.Fatalf("idet interlaced=%d progressive=%d", interlaced, progressive)
				}
				if tc.fps > 50 {
					kept := mpdecimate(t, out)
					if kept < frames-4 {
						t.Fatalf("mpdecimate kept %d of %d (duplicate frames)", kept, frames)
					}
				}
			})
		}
	}
}

// A progressive H.264 playlist has an empty field order. Field bob would
// turn 30p into 60 and 60p into 120, and an HLS source never sees the packet scan.
func TestUnscannedH264KeepsItsRate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	src := Source{VideoCodec: "H264"}
	vf := filterOf(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "none"}, "libx264", "motion_adaptive"))
	if strings.Contains(vf, "bwdif") || strings.Contains(vf, "fps=") {
		t.Fatalf("unscanned h264 filter doubles: %s", vf)
	}
	for _, tc := range []struct {
		name   string
		rate   string
		want   float64
		frames int
	}{
		{"30p", "30", 30, 30},
		{"60p", "60", 60, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := filepath.Join(dir, tc.name+".ts")
			build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
				"-f", "lavfi", "-i", "nullsrc=s=320x180:r="+tc.rate+":d=1,format=yuv420p",
				"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "30", "-f", "mpegts", in)
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, out)
			}
			outPath := filepath.Join(dir, tc.name+"-out.mp4")
			cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", in, "-an", "-vf", vf, "-c:v", "libx264", "-t", "1", outPath)
			if msg, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("encode: %v %s", err, msg)
			}
			fps, frames := measure(t, outPath)
			t.Logf("%s %.3f fps %d frames", tc.name, fps, frames)
			if frames < tc.frames-5 || frames > tc.frames+8 {
				t.Fatalf("frames %d, want about %d (doubled would be %d)", frames, tc.frames, tc.frames*2)
			}
			if fps < tc.want-2 || fps > tc.want+2 {
				t.Fatalf("fps %.3f, want about %.3f", fps, tc.want)
			}
		})
	}
}

func filterOf(args []string) string {
	for i, a := range args {
		if a == "-vf" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func measure(t *testing.T, path string) (float64, int) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_entries", "stream=nb_read_frames,r_frame_rate", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 2 {
		t.Fatalf("probe %q", out)
	}
	rate := strings.Split(parts[0], "/")
	num, _ := strconv.ParseFloat(rate[0], 64)
	den := 1.0
	if len(rate) == 2 {
		den, _ = strconv.ParseFloat(rate[1], 64)
	}
	frames, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return num / den, frames
}

func idet(t *testing.T, path string) (interlaced, progressive int) {
	t.Helper()
	out, _ := exec.Command("ffmpeg", "-hide_banner", "-i", path, "-vf", "idet", "-f", "null", "-").CombinedOutput()
	line := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "Multi frame detection:") {
			line = l
		}
	}
	interlaced = idetField(line, "TFF:") + idetField(line, "BFF:")
	progressive = idetField(line, "Progressive:")
	return interlaced, progressive
}

func idetField(line, key string) int {
	i := strings.Index(line, key)
	if i < 0 {
		return 0
	}
	rest := strings.TrimSpace(line[i+len(key):])
	n, _, _ := strings.Cut(rest, " ")
	v, _ := strconv.Atoi(n)
	return v
}

func mpdecimate(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-i", path, "-vf", "mpdecimate", "-f", "null", "-").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "frame=") {
		t.Fatalf("mpdecimate: %v %s", err, out)
	}
	frames := 0
	for _, line := range strings.Split(string(out), "\r") {
		if i := strings.Index(line, "frame="); i >= 0 {
			rest := strings.TrimSpace(line[i+len("frame="):])
			n, _, _ := strings.Cut(rest, " ")
			if v, err := strconv.Atoi(n); err == nil {
				frames = v
			}
		}
	}
	return frames
}
