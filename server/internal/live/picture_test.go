package live

import (
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
		line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, enc, "motion_adaptive", false), " ")
		if strings.Contains(line, "deinterlace_vaapi") || strings.Contains(line, "bwdif") {
			t.Fatalf("%s: progressive broadcast was deinterlaced: %s", enc, line)
		}
		if strings.Contains(line, "fps=") {
			t.Fatalf("%s: progressive broadcast lost its frame rate: %s", enc, line)
		}
	}
	line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive", false), " ")
	if !strings.Contains(line, "format=nv12,hwupload,scale_vaapi=w='min(1920,iw)'") {
		t.Fatalf("vaapi should scale on the GPU and never upscale 720p: %s", line)
	}
}

func TestInterlacedStaysFieldRateOnTheGPU(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	line := strings.Join(RenditionArgs(0, src, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", "motion_adaptive", false), " ")
	if !strings.Contains(line, "deinterlace_vaapi=mode=motion_adaptive:rate=field,scale_vaapi") {
		t.Fatalf("1080i should be bobbed to 59.94 on the GPU: %s", line)
	}
	tile := strings.Join(RenditionArgs(0, src, Rendition{Video: "360", Audio: "none"}, "h264_vaapi", "motion_adaptive", false), " ")
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
		{"1080i vaapi", Source{VideoCodec: "MPEG2"}, "h264_vaapi", []string{"deinterlace_vaapi=mode=motion_adaptive:rate=field"}, []string{"bwdif", "fieldmatch", "fps="}},
		{"1080i libx264", Source{VideoCodec: "MPEG2"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001"}, []string{"deinterlace_vaapi", "fieldmatch"}},
		{"1080i videotoolbox", Source{VideoCodec: "MPEG2"}, "h264_videotoolbox", []string{"bwdif=mode=send_field", "fps=60000/1001", "-a53cc 0"}, []string{"fieldmatch"}},
		{"480i libx264", Source{VideoCodec: "MPEG2"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001", "min(1920,iw)"}, []string{"fieldmatch"}},
		{"720p vaapi", Source{VideoCodec: "MPEG2", Progressive: true}, "h264_vaapi", []string{"scale_vaapi=w='min(1920,iw)'"}, []string{"deinterlace_vaapi", "bwdif", "fps=", "fieldmatch"}},
		{"720p libx264", Source{VideoCodec: "MPEG2", Progressive: true}, "libx264", []string{"scale='min(1920,iw)'"}, []string{"bwdif", "fps=", "fieldmatch"}},
		{"720p videotoolbox", Source{VideoCodec: "MPEG2", Progressive: true}, "h264_videotoolbox", []string{"scale='min(1920,iw)'", "-a53cc 0"}, []string{"bwdif", "fps="}},
		{"h264 paff vaapi", Source{VideoCodec: "H264"}, "h264_vaapi", []string{"deinterlace_vaapi=mode=motion_adaptive:rate=field"}, []string{"bwdif", "fieldmatch"}},
		{"h264 mbaff libx264", Source{VideoCodec: "H264"}, "libx264", []string{"bwdif=mode=send_field", "fps=60000/1001"}, []string{"fieldmatch"}},
		{"h264 mbaff videotoolbox", Source{VideoCodec: "H264"}, "h264_videotoolbox", []string{"bwdif=mode=send_field", "-a53cc 0"}, []string{"fieldmatch"}},
		{"film vaapi", Source{VideoCodec: "MPEG2", Film: true}, "h264_vaapi", []string{"fieldmatch,decimate,fps=24000/1001,format=nv12,hwupload,scale_vaapi"}, []string{"bwdif", "deinterlace_vaapi"}},
		{"film libx264", Source{VideoCodec: "MPEG2", Film: true}, "libx264", []string{"fieldmatch,decimate", "fps=24000/1001"}, []string{"bwdif", "hwupload"}},
		{"film videotoolbox", Source{VideoCodec: "MPEG2", Film: true}, "h264_videotoolbox", []string{"fieldmatch,decimate", "fps=24000/1001", "-a53cc 0"}, []string{"bwdif"}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			line := strings.Join(RenditionArgs(0, row.src, Rendition{Video: "1080", Audio: "aac2"}, row.encoder, "motion_adaptive", false), " ")
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
	if !strings.Contains(line, "fieldmatch,decimate") || !strings.Contains(line, "fps=24000/1001") {
		t.Fatalf("film: %s", line)
	}
	if strings.Contains(line, "bwdif") {
		t.Fatalf("film should not also bob: %s", line)
	}
	if !strings.Contains(line, "-hls_playlist_type event") {
		t.Fatalf("recording playlist: %s", line)
	}
}

func TestSmoothBlendOnlyWhenProbePassed(t *testing.T) {
	off := strings.Join(PictureArgs(Graph{VideoCodec: "H264", Mode: "smooth", Encoder: "libx264"}), " ")
	if strings.Contains(off, "minterpolate") {
		t.Fatalf("blend stays off until a realtime probe passes: %s", off)
	}
	on := strings.Join(PictureArgs(Graph{VideoCodec: "H264", Mode: "smooth", Blend: true, Encoder: "libx264"}), " ")
	if !strings.Contains(on, "minterpolate=fps=60000/1001:mi_mode=blend") {
		t.Fatalf("blend: %s", on)
	}
}

func TestVideoToolboxIsRealtime(t *testing.T) {
	line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "h264_videotoolbox", Mode: "broadcast"}), " ")
	if !strings.Contains(line, "-c:v h264_videotoolbox -realtime 1 -a53cc 0") {
		t.Fatalf("videotoolbox args: %s", line)
	}
}
