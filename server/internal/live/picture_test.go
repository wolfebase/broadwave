package live

import (
	"bytes"
	"strings"
	"testing"
)

func TestBroadcastMPEG2IsFieldRate(t *testing.T) {
	args := PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "h264_nvenc", Profile: "transparent", Mode: "broadcast", Live: true})
	line := strings.Join(args, " ")
	if !strings.Contains(line, "bwdif=mode=send_field") {
		t.Fatalf("mpeg-2 should keep both fields: %s", line)
	}
	if strings.Contains(line, "send_frame") {
		t.Fatalf("field-rate deinterlace must not drop to one frame per pair: %s", line)
	}
	if !strings.Contains(line, "fps=60000/1001") || !strings.Contains(line, "-b:v 14M") {
		t.Fatalf("1080p60 rate: %s", line)
	}
	if !strings.Contains(line, "aresample=async=1000") || !strings.Contains(line, "expr:gte(t,n_forced*2)") {
		t.Fatalf("clock and keyframes: %s", line)
	}
	if !strings.Contains(line, "-hls_list_size 2700") {
		t.Fatalf("live buffer: %s", line)
	}
	if strings.Contains(line, "-tune ull") || strings.Contains(line, "zerolatency") {
		t.Fatalf("latency tune fights a steady picture: %s", line)
	}
}

func TestProgressiveSkipsDeinterlace(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "H264", Encoder: "libx264", Mode: "broadcast"}), " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "deinterlace_vaapi") {
		t.Fatalf("progressive picture should not be bobbed: %s", line)
	}
	if strings.Contains(line, "fps=") {
		t.Fatalf("a progressive picture keeps its own frame rate: %s", line)
	}
}

// A 720p60 MPEG-2 broadcast (most ABC and FOX stations) is progressive. Bobbing it
// as fields halves its detail, and a 29.97 cap throws away every other frame.
func TestProgressive720pKeepsEveryFrame(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3", Progressive: true}
	for _, enc := range []string{"h264_vaapi", "libx264", "h264_videotoolbox"} {
		line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, enc, "motion_adaptive"), " ")
		if strings.Contains(line, "deinterlace_vaapi") || strings.Contains(line, "bwdif") {
			t.Fatalf("%s: progressive broadcast was deinterlaced: %s", enc, line)
		}
		if strings.Contains(line, "fps=") {
			t.Fatalf("%s: progressive broadcast lost its frame rate: %s", enc, line)
		}
	}
	line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive"), " ")
	if !strings.Contains(line, "-hwaccel_output_format vaapi") || !strings.Contains(line, "scale_vaapi=w='min(1920,iw)'") || strings.Contains(line, "hwupload") {
		t.Fatalf("vaapi should decode and scale on the GPU and never upscale 720p: %s", line)
	}
}

func TestInterlacedStaysFieldRateOnTheGPU(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive"), " ")
	if !strings.Contains(line, "deinterlace_vaapi=mode=motion_adaptive:rate=field,scale_vaapi") {
		t.Fatalf("1080i should be bobbed to 59.94 on the GPU: %s", line)
	}
	tile := strings.Join(RenditionArgs(0, src, Rendition{Video: "360", Audio: "none"}, "h264_vaapi", "motion_adaptive"), " ")
	if !strings.Contains(tile, "fps=30000/1001,format=nv12,hwupload,deinterlace_vaapi=mode=motion_adaptive:rate=frame") {
		t.Fatalf("tiles stay at frame rate: %s", tile)
	}
}

func TestSaverStaysFrameRate(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Profile: "saver", Encoder: "libx264"}), " ")
	if !strings.Contains(line, "bwdif=mode=send_frame") || !strings.Contains(line, "fps=30000/1001") {
		t.Fatalf("saver is the bandwidth mode: %s", line)
	}
	if strings.Contains(line, "14M") {
		t.Fatalf("saver bitrate: %s", line)
	}
}

func TestVAAPIUsesMotionAdaptive(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "h264_vaapi", Deint: "motion_adaptive", Mode: "broadcast"}), " ")
	if !strings.Contains(line, "deinterlace_vaapi=mode=motion_adaptive:rate=field") {
		t.Fatalf("gpu deinterlace: %s", line)
	}
	if strings.Contains(line, "bwdif") {
		t.Fatalf("gpu path should not also run software deinterlace: %s", line)
	}
}

func TestSmoothCanUseMotionCompensated(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "h264_vaapi", Deint: "motion_compensated", Mode: "smooth"}), " ")
	if !strings.Contains(line, "mode=motion_compensated:rate=field") {
		t.Fatalf("smooth: %s", line)
	}
}

func TestScanTypeMatrix(t *testing.T) {
	type row struct {
		name    string
		src     Source
		encoder string
		want    []string
		forbid  []string
	}
	rows := []row{
		{"1080i vaapi", Source{VideoCodec: "MPEG2"}, "h264_vaapi", []string{"-hwaccel_output_format vaapi", "deinterlace_vaapi=mode=motion_adaptive:rate=field", "-rc_mode VBR", "-profile:v high", "-bf 2"}, []string{"bwdif", "pullup", "fps=", "hwupload"}},
		{"1080i libx264", Source{VideoCodec: "MPEG2"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001"}, []string{"deinterlace_vaapi", "pullup"}},
		{"1080i videotoolbox", Source{VideoCodec: "MPEG2"}, "h264_videotoolbox", []string{"bwdif=mode=send_field", "fps=60000/1001", "-a53cc 0"}, []string{"pullup"}},
		{"480i libx264", Source{VideoCodec: "MPEG2"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001", "min(1920,iw)"}, []string{"pullup"}},
		{"720p vaapi", Source{VideoCodec: "MPEG2", Progressive: true}, "h264_vaapi", []string{"-hwaccel_output_format vaapi", "scale_vaapi=w='min(1920,iw)'"}, []string{"deinterlace_vaapi", "bwdif", "fps=", "pullup", "hwupload"}},
		{"720p libx264", Source{VideoCodec: "MPEG2", Progressive: true}, "libx264", []string{"scale='min(1920,iw)'"}, []string{"bwdif", "fps=", "pullup"}},
		{"720p videotoolbox", Source{VideoCodec: "MPEG2", Progressive: true}, "h264_videotoolbox", []string{"scale='min(1920,iw)'", "-a53cc 0"}, []string{"bwdif", "fps="}},
		{"h264 paff vaapi", Source{VideoCodec: "H264"}, "h264_vaapi", []string{"-hwaccel_output_format vaapi", "deinterlace_vaapi=mode=motion_adaptive:rate=field"}, []string{"bwdif", "pullup", "hwupload"}},
		{"h264 mbaff libx264", Source{VideoCodec: "H264"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001"}, []string{"pullup"}},
		{"h264 mbaff videotoolbox", Source{VideoCodec: "H264"}, "h264_videotoolbox", []string{"bwdif=mode=send_field", "-a53cc 0"}, []string{"pullup"}},
		{"film vaapi", Source{VideoCodec: "MPEG2", Film: true}, "h264_vaapi", []string{"pullup,fps=24000/1001,format=nv12,hwupload,scale_vaapi"}, []string{"bwdif", "deinterlace_vaapi", "-hwaccel_output_format"}},
		{"film libx264", Source{VideoCodec: "MPEG2", Film: true}, "libx264", []string{"pullup", "fps=24000/1001"}, []string{"bwdif", "hwupload"}},
		{"film videotoolbox", Source{VideoCodec: "MPEG2", Film: true}, "h264_videotoolbox", []string{"pullup", "fps=24000/1001", "-a53cc 0"}, []string{"bwdif"}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			line := strings.Join(RenditionArgs(0, row.src, Rendition{Video: "1080", Audio: "aac2"}, row.encoder, "motion_adaptive"), " ")
			for _, want := range row.want {
				if !strings.Contains(line, want) {
					t.Fatalf("missing %q in %s", want, line)
				}
			}
			for _, bad := range row.forbid {
				if strings.Contains(line, bad) {
					t.Fatalf("unexpected %q in %s", bad, line)
				}
			}
		})
	}
}

func TestFilmRecovers24p(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "libx264", Mode: "film", Live: false}), " ")
	if !strings.Contains(line, "pullup") || !strings.Contains(line, "fps=24000/1001") {
		t.Fatalf("film: %s", line)
	}
	if strings.Contains(line, "bwdif") {
		t.Fatalf("film should not also bob: %s", line)
	}
	if !strings.Contains(line, "-hls_playlist_type event") {
		t.Fatalf("recording playlist: %s", line)
	}
}

func TestSmoothDoesNotInventFrames(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "H264", Mode: "smooth", Encoder: "libx264"}), " ")
	if strings.Contains(line, "minterpolate") || strings.Contains(line, "framerate=") {
		t.Fatalf("smooth must not interpolate: %s", line)
	}
	live := strings.Join(RenditionArgs(0, Source{VideoCodec: "H264", Progressive: true}, Rendition{Video: "1080", Audio: "aac2", Mode: "smooth"}, "h264_vaapi", ""), " ")
	if strings.Contains(live, "minterpolate") || strings.Contains(live, "fps=") {
		t.Fatalf("progressive smooth stays at the source rate: %s", live)
	}
}

func TestHEVCRenditionUsesHVC1(t *testing.T) {
	line := strings.Join(RenditionArgs(0, Source{VideoCodec: "MPEG2"}, Rendition{Video: "1080", Audio: "aac2", Codec: "hevc"}, "h264_vaapi", "motion_adaptive"), " ")
	for _, want := range []string{"-hwaccel_output_format vaapi", "-c:v hevc_vaapi -sei 0", "-tag:v hvc1", "-profile:v main", "deinterlace_vaapi"} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "hwupload") || strings.Contains(line, "h264_vaapi") {
		t.Fatalf("hevc gpu path: %s", line)
	}
}

func TestStreamFacts(t *testing.T) {
	noted := notedPicture{Width: 1280, Height: 720, FPS: "59.94"}
	prog := streamFacts(Source{VideoCodec: "MPEG2", Progressive: true}, "progressive", Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive", false, noted)
	if prog.Scan != "progressive" || prog.OutputWidth != 1280 || prog.OutputHeight != 720 || prog.OutputFPS != "59.94" || prog.Bitrate != "10M" || prog.Decode != "gpu" {
		t.Fatalf("720p: %+v", prog)
	}
	lace := streamFacts(Source{VideoCodec: "MPEG2"}, "tt", Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive", false, notedPicture{Width: 1920, Height: 1080, FPS: "29.97"})
	if lace.Scan != "interlaced" || lace.OutputFPS != "59.94" || lace.Bitrate != "14M" || lace.Decode != "gpu" || lace.OutputWidth != 1920 {
		t.Fatalf("1080i: %+v", lace)
	}
	soft := streamFacts(Source{VideoCodec: "MPEG2"}, "tt", Rendition{Video: "1080", Audio: "aac2"}, "libx264", "", false, notedPicture{Width: 1920, Height: 1080, FPS: "29.97"})
	if soft.Decode != "cpu" || soft.OutputFPS != "59.94" {
		t.Fatalf("software: %+v", soft)
	}
	fell := streamFacts(Source{VideoCodec: "MPEG2"}, "tt", Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive", true, notedPicture{Width: 1920, Height: 1080, FPS: "29.97"})
	if fell.Decode != "cpu" || fell.Encoder != "libx264" {
		t.Fatalf("fallback stays on the CPU: %+v", fell)
	}
	hevcFell := streamFacts(Source{VideoCodec: "MPEG2"}, "tt", Rendition{Video: "1080", Audio: "aac2", Codec: "hevc"}, "h264_vaapi", "motion_adaptive", true, notedPicture{Width: 1920, Height: 1080, FPS: "29.97"})
	if hevcFell.Decode != "cpu" || hevcFell.Encoder != "libx265" {
		t.Fatalf("hevc fallback is libx265: %+v", hevcFell)
	}
	direct := streamFacts(Source{VideoCodec: "MPEG2", Progressive: true}, "progressive", Rendition{Video: "copy", Audio: "copy"}, "h264_vaapi", "", false, noted)
	if direct.Decode != "" || direct.Bitrate != "" || direct.OutputWidth != 1280 || direct.OutputFPS != "59.94" {
		t.Fatalf("copy: %+v", direct)
	}
	unknown := streamFacts(Source{VideoCodec: "MPEG2", Progressive: true}, "progressive", Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "", false, notedPicture{})
	if unknown.OutputWidth != 0 || unknown.SourceWidth != 0 || unknown.Decode != "gpu" {
		t.Fatalf("unknown size must stay blank: %+v", unknown)
	}
}

func TestMuxPictureKeepsTheWindowOpen(t *testing.T) {
	seq := []byte{0x00, 0x00, 0x01, 0xB3, 0x50, 0x02, 0xD0, 0x37}
	ts := programTS(1, streamMPEG2, 0x100, seq)
	m := &mux{}
	m.observePicture(ts)
	if _, ok := m.picture(1); ok {
		t.Fatal("a picture waits until its program is on the mux")
	}
	m.noteProgram(1)
	got, ok := m.picture(1)
	if !ok || got.Width != 1280 || got.Height != 720 || got.FPS != "59.94" {
		t.Fatalf("picture: %+v ok=%v", got, ok)
	}
	if m.picDone {
		t.Fatal("one program must not close the window; a later subchannel still needs it")
	}
	// program 0 is the only program in a filtered stream.
	m.noteProgram(0)
	zero, ok := m.picture(0)
	if !ok || zero.Width != 1280 || zero.Height != 720 {
		t.Fatalf("program 0: %+v ok=%v", zero, ok)
	}

	blank := &mux{}
	blank.noteProgram(1)
	blank.observePicture(bytes.Repeat([]byte{0x47}, 4<<20))
	if !blank.picDone {
		t.Fatal("a full capture window must stop the scan")
	}
	if _, ok := blank.picture(1); ok {
		t.Fatal("padding is not a picture")
	}
}

func TestVideoToolboxIsRealtime(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "h264_videotoolbox", Mode: "broadcast"}), " ")
	if !strings.Contains(line, "-c:v h264_videotoolbox -realtime 1 -a53cc 0") {
		t.Fatalf("videotoolbox args: %s", line)
	}
}
