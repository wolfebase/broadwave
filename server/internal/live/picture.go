package live

import (
	"fmt"
	"strconv"
	"strings"
)

// Graph is the one playback picture used for live TV, recordings, and library channels.
// Mode broadcast reconstructs interlaced video at field rate (59.94). Smooth asks for
// motion-compensated deinterlace when Deint names that VAAPI mode. Film recovers 24p.
type Graph struct {
	Program    int
	VideoCodec string
	Profile    string
	Audio      string
	Encoder    string
	Mode       string
	Deint      string
	Blend      bool
	Input      string
	Live       bool
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "smooth":
		return "smooth"
	case "film":
		return "film"
	default:
		return "broadcast"
	}
}

func InterlacedCodec(codec string) bool {
	c := strings.ToLower(strings.ReplaceAll(codec, "-", ""))
	return c == "mpeg2" || c == "mpeg2video"
}

// PictureArgs builds the ffmpeg argv for Graph. Recordings on disk stay the original MPEG-TS.
func PictureArgs(g Graph) []string {
	g.Mode = NormalizeMode(g.Mode)
	if g.Input == "" {
		g.Input = "pipe:0"
	}
	if g.Profile == "" {
		g.Profile = "transparent"
	}
	if g.Audio == "" {
		g.Audio = "stereo"
	}
	interlaced := InterlacedCodec(g.VideoCodec) && g.Mode != "film"
	field := interlaced && !smallPicture(g.Profile)
	width, height, rate := pictureSize(g.Profile, field)
	fps, gop := pictureRate(g, field)

	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "+genpts+discardcorrupt"}
	vaapiDeint := vaapiDeintMode(g, interlaced)
	if g.Encoder == "h264_vaapi" {
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
	}
	if g.Input == "pipe:0" {
		args = append(args, "-probesize", "2000000", "-analyzeduration", "1500000")
	}
	args = append(args, "-i", g.Input)
	if g.Program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d:v:0", g.Program), "-map", fmt.Sprintf("0:p:%d:a:0", g.Program))
	} else {
		args = append(args, "-map", "0:v:0", "-map", "0:a:0")
	}
	args = append(args, "-vf", videoFilter(g, vaapiDeint, interlaced, field, width, height, fps))
	args = append(args, videoCodec(g.Encoder, rate, gop)...)
	args = append(args, "-force_key_frames", "expr:gte(t,n_forced*2)")
	if g.Audio == "surround" {
		args = append(args, "-af", "aresample=async=1000:first_pts=0", "-c:a", "aac", "-ac", "6", "-b:a", "384k")
	} else {
		args = append(args, "-af", "aresample=async=1000:first_pts=0", "-c:a", "aac", "-ac", "2", "-b:a", "160k")
	}
	args = append(args, "-f", "hls", "-hls_time", "2", "-hls_segment_filename", "seg%05d.ts")
	if g.Live {
		args = append(args,
			"-hls_list_size", "2700",
			"-hls_flags", "delete_segments+independent_segments+omit_endlist+program_date_time",
		)
	} else {
		args = append(args,
			"-hls_list_size", "0",
			"-hls_playlist_type", "event",
			"-hls_flags", "independent_segments+append_list+program_date_time",
		)
	}
	args = append(args, "index.m3u8")
	return args
}

// smallPicture is a bandwidth rendition: one frame per broadcast frame, no motion blend.
func smallPicture(profile string) bool {
	return profile == "saver" || profile == "tile"
}

func pictureSize(profile string, field bool) (int, int, string) {
	switch profile {
	case "balanced":
		if field {
			return 1280, 720, "8M"
		}
		return 1280, 720, "5M"
	case "saver":
		return 960, 540, "2500k"
	case "tile":
		return 640, 360, "1200k"
	default:
		if field {
			return 1920, 1080, "14M"
		}
		return 1920, 1080, "10M"
	}
}

func pictureRate(g Graph, field bool) (string, int) {
	if g.Mode == "film" {
		return "24000/1001", 48
	}
	if field || (g.Mode == "smooth" && g.Blend && !smallPicture(g.Profile)) {
		return "60000/1001", 120
	}
	return "30000/1001", 60
}

func vaapiDeintMode(g Graph, interlaced bool) string {
	if !interlaced || g.Encoder != "h264_vaapi" || g.Deint == "" {
		return ""
	}
	switch g.Deint {
	case "motion_compensated", "motion_adaptive":
		return g.Deint
	default:
		return ""
	}
}

func videoFilter(g Graph, vaapiDeint string, interlaced, field bool, width, height int, fps string) string {
	scale := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease,setsar=1,fps=%s", width, height, fps)
	if g.Mode == "film" {
		return "fieldmatch,decimate," + scale
	}
	if vaapiDeint != "" {
		rate := "frame"
		if field {
			rate = "field"
		}
		return fmt.Sprintf("format=nv12,hwupload,deinterlace_vaapi=mode=%s:rate=%s,scale_vaapi=w=%d:h=%d:force_original_aspect_ratio=decrease", vaapiDeint, rate, width, height)
	}
	var pre []string
	if interlaced {
		mode := "send_frame"
		if field {
			mode = "send_field"
		}
		pre = append(pre, "bwdif=mode="+mode+":parity=auto:deint=interlaced")
	} else if g.Mode == "smooth" && g.Blend && !smallPicture(g.Profile) {
		pre = append(pre, "minterpolate=fps=60000/1001:mi_mode=blend")
	}
	if len(pre) == 0 {
		pre = append(pre, scale)
	} else {
		pre = append(pre, scale)
	}
	vf := strings.Join(pre, ",")
	if g.Encoder == "h264_vaapi" || g.Encoder == "h264_qsv" {
		vf += ",format=nv12"
	}
	if g.Encoder == "h264_vaapi" {
		vf += ",hwupload"
	}
	return vf
}

func videoCodec(encoder, rate string, gop int) []string {
	buf := twice(rate)
	g := strconv.Itoa(gop)
	switch encoder {
	case "h264_nvenc":
		return []string{"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "cbr", "-b:v", rate, "-maxrate", rate, "-bufsize", buf, "-g", g, "-bf", "2", "-profile:v", "high"}
	case "h264_qsv":
		return []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-b:v", rate, "-maxrate", rate, "-bufsize", buf, "-g", g}
	case "h264_vaapi":
		return []string{"-c:v", "h264_vaapi", "-b:v", rate, "-maxrate", rate, "-bufsize", buf, "-g", g}
	case "h264_videotoolbox":
		// VideoToolbox writes broken SEI when it embeds A/53 captions; every segment then fails to decode.
		return []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-a53cc", "0", "-b:v", rate, "-maxrate", rate, "-bufsize", buf, "-g", g, "-profile:v", "high"}
	default:
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-b:v", rate, "-maxrate", rate, "-bufsize", buf, "-g", g}
	}
}

func twice(rate string) string {
	if n, ok := strings.CutSuffix(rate, "M"); ok {
		v, err := strconv.Atoi(n)
		if err == nil {
			return strconv.Itoa(v*2) + "M"
		}
	}
	if n, ok := strings.CutSuffix(rate, "k"); ok {
		v, err := strconv.Atoi(n)
		if err == nil {
			return strconv.Itoa(v*2) + "k"
		}
	}
	return rate
}
