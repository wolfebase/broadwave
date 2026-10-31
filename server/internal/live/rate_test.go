package live

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A 25 Hz broadcast (DVB, most of the world) keeps its clock: 50 fields
// stretched to 59.94 repeat every fifth picture.
func TestFiftyHzKeepsItsOwnClock(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "MPEG", Rate: "25"}
	full := strings.Join(RenditionArgs(1, src, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(full, "bwdif=mode=send_field") || !strings.Contains(full, "fps=50,") || strings.Contains(full, "60000/1001") {
		t.Fatalf("1080 of 25i: %s", full)
	}
	small := strings.Join(RenditionArgs(1, src, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(small, "fps=25") || strings.Contains(small, "30000/1001") {
		t.Fatalf("540 of 25i: %s", small)
	}
	film := strings.Join(RenditionArgs(1, src, Rendition{Video: "1080", Audio: "aac2", Mode: "film"}, "libx264", ""), " ")
	if strings.Contains(film, "24000/1001") {
		t.Fatalf("film on 25i drops pictures: %s", film)
	}
	p50 := Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true, Rate: "50"}
	if line := strings.Join(RenditionArgs(1, p50, Rendition{Video: "720", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " "); strings.Contains(line, "fps=") {
		t.Fatalf("720p50 keeps every frame: %s", line)
	}
	if line := strings.Join(RenditionArgs(1, p50, Rendition{Video: "360", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " "); !strings.Contains(line, "fps=25") {
		t.Fatalf("a 50p tile halves to 25: %s", line)
	}
	us := strings.Join(RenditionArgs(1, Source{VideoCodec: "MPEG2", AudioCodec: "AC3", Rate: "29.97"}, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(us, "fps=60000/1001") {
		t.Fatalf("29.97i stays 59.94: %s", us)
	}
	unknown := strings.Join(RenditionArgs(1, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(unknown, "fps=60000/1001") {
		t.Fatalf("an unread rate stays 59.94: %s", unknown)
	}
	if facts := streamFacts(src, "tt", Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", "", false, notedPicture{Width: 720, Height: 576, FPS: "25"}); facts.OutputFPS != "50" {
		t.Fatalf("stream panel output %q", facts.OutputFPS)
	}
	if line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "libx264", Rate: "25"}), " "); !strings.Contains(line, "fps=50,") {
		t.Fatalf("recording playback of 25i: %s", line)
	}
}

func TestFiftyHzLadderAndMosaic(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "MPEG", Rate: "25"}
	line := strings.Join(LadderArgs(1, src, nil, "broadcast", "libx264", ""), " ")
	if !strings.Contains(line, "fps=50") || strings.Contains(line, "60000/1001") {
		t.Fatalf("ladder of 25i: %s", line)
	}
	if got := ladderFrameRate(src, "broadcast"); got != "50" {
		t.Fatalf("ladder FRAME-RATE %q", got)
	}
	if got := ladderFrameRate(Source{VideoCodec: "MPEG2"}, "broadcast"); got != "59.94" {
		t.Fatalf("ladder FRAME-RATE %q", got)
	}
	both := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3", rate: "25"}, {input: "pipe:4", rate: "50"}}, "libx264"), " ")
	if !strings.Contains(both, "fps=50,") || strings.Contains(both, "60000/1001") {
		t.Fatalf("mosaic of 50 Hz tiles: %s", both)
	}
	mixed := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3", rate: "25"}, {input: "pipe:4"}}, "libx264"), " ")
	if !strings.Contains(mixed, "fps=60000/1001") {
		t.Fatalf("mosaic with an unknown tile: %s", mixed)
	}
}

// The H.264 frame rate is read from the SPS timing: 1080i25 is the usual DVB
// HD picture, and its SPS counts 50 ticks a second.
func TestH264PictureRate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	for _, c := range []struct {
		name, rate, want string
		lace             bool
	}{
		{"i25", "50", "25", true},
		{"p50", "50", "50", false},
		{"p2997", "30000/1001", "29.97", false},
	} {
		out := filepath.Join(dir, c.name+".ts")
		vf := "format=yuv420p"
		x264 := "keyint=15"
		if c.lace {
			vf = "tinterlace=interleave_top,fieldorder=tff,format=yuv420p"
			x264 += ":tff=1"
		}
		gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate="+c.rate,
			"-t", "1", "-vf", vf, "-c:v", "libx264", "-preset", "ultrafast", "-x264-params", x264, "-f", "mpegts", out)
		if b, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", c.name, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := pictureFacts(data, 0)
		if !ok || p.FPS != c.want {
			t.Fatalf("%s: %+v %v, want %s", c.name, p, ok, c.want)
		}
		if got := fileRate(out); got != c.want {
			t.Fatalf("%s: file rate %q", c.name, got)
		}
	}
}
