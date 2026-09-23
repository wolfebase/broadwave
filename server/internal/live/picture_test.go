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
	if !strings.Contains(line, "fps=30000/1001") {
		t.Fatalf("progressive rate: %s", line)
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
