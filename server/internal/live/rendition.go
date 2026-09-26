package live

import (
	"fmt"
	"slices"
	"strings"
)

// Rendition is one delivery form of a live channel. Viewers who need the same
// form share one ffmpeg process; a different form never disturbs them.
type Rendition struct {
	Video string `json:"video"` // copy, 1080, 720, 540
	Audio string `json:"audio"` // copy, aac2, aac6
	Mode  string `json:"mode,omitempty"`
	Codec string `json:"codec,omitempty"` // hevc, or empty for H.264
	Track string `json:"track,omitempty"` // lang, vi; empty is the main mix
	Even  bool   `json:"even,omitempty"`
	// FullRate is a 540p or 360p tile at field rate. The key gains ".60" so it
	// does not share ffmpeg with the 30 fps tile of the same size.
	FullRate bool `json:"fullRate,omitempty"`
}

func (r Rendition) normalized() Rendition {
	switch r.Video {
	case "copy", "1080", "720", "540", "360":
	default:
		r.Video = "1080"
	}
	switch r.Audio {
	case "copy", "aac2", "aac6", "none":
	default:
		r.Audio = "aac2"
	}
	if r.Video == "copy" {
		r.Mode = ""
		r.Codec = ""
		r.FullRate = false
	} else {
		r.Mode = NormalizeMode(r.Mode)
		if r.Codec != "hevc" {
			r.Codec = ""
		}
	}
	switch r.Track {
	case "lang", "vi":
	default:
		r.Track = ""
	}
	if r.Audio == "none" {
		r.Track = ""
		r.Even = false
	}
	return r
}

// Key names the rendition on disk and in URLs.
func (r Rendition) Key() string {
	r = r.normalized()
	key := r.Video + "." + r.Audio
	if r.Mode != "" {
		key += "." + r.Mode
	}
	if r.Track != "" {
		key += "." + r.Track
	}
	if r.Even {
		key += ".even"
	}
	if r.FullRate {
		key += ".60"
	}
	if r.Codec == "hevc" {
		key += ".hevc"
	}
	return key
}

func ParseRenditionKey(key string) (Rendition, bool) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 || len(parts) > 7 {
		return Rendition{}, false
	}
	r := Rendition{Video: parts[0], Audio: parts[1]}
	for _, p := range parts[2:] {
		switch p {
		case "hevc":
			r.Codec = "hevc"
		case "even":
			r.Even = true
		case "60":
			r.FullRate = true
		case "lang", "vi":
			r.Track = p
		case "broadcast", "smooth", "film":
			r.Mode = p
		default:
			return Rendition{}, false
		}
	}
	if r.normalized().Key() != key {
		return Rendition{}, false
	}
	return r.normalized(), true
}

// Caps is what a client can play, sent with every watch request.
type Caps struct {
	Platform  string   `json:"platform"`
	Video     []string `json:"video"`
	Audio     []string `json:"audio"`
	MaxHeight int      `json:"maxHeight,omitempty"`
	Network   string   `json:"network,omitempty"`
}

// Prefs are the viewer's choices. Empty fields mean automatic.
type Prefs struct {
	Quality string `json:"quality,omitempty"` // auto, original, high, medium, saver, tile, 360, focus
	Audio   string `json:"audio,omitempty"`   // auto, surround, stereo, none
	Picture string `json:"picture,omitempty"` // broadcast, smooth, film
	Track   string `json:"track,omitempty"`   // main, language, described
	Even    bool   `json:"even,omitempty"`
}

// Source describes the broadcast as far as the server knows it.
type Source struct {
	VideoCodec string
	AudioCodec string
	// Progressive is true only once a probe has shown the picture is not interlaced.
	Progressive bool
	// Film is 3:2 pulldown. Soft telecine is repeat_first_field; hard telecine
	// has no flags and is recovered with pullup when the viewer picks film.
	Film      bool
	UserAgent string
	Referrer  string
	// AudioPID is the PMT elementary stream to map. Zero keeps the first audio.
	AudioPID int
	// Lace is true once a scan has shown interlaced H.264. An empty scan does
	// not lace it: a progressive playlist keeps its own rate.
	Lace bool
}

type Decision struct {
	Rendition Rendition `json:"rendition"`
	Reason    string    `json:"reason"`
}

func codecName(raw string) string {
	c := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(raw, "-", ""), ".", ""))
	switch {
	case c == "":
		return ""
	case strings.HasPrefix(c, "mpeg2"):
		return "mpeg2"
	case c == "h264" || c == "avc" || c == "avc1":
		return "h264"
	case c == "hevc" || c == "h265":
		return "hevc"
	case c == "ac3" || c == "ac-3":
		return "ac3"
	case c == "eac3" || c == "ec3":
		return "eac3"
	case strings.HasPrefix(c, "aac"):
		return "aac"
	case c == "ac4":
		return "ac4"
	case c == "mp2" || c == "mpeg":
		return "mp2"
	}
	return c
}

func has(list []string, codec string) bool {
	return codec != "" && slices.ContainsFunc(list, func(s string) bool { return codecName(s) == codec })
}

// hardwareEncoder is the fallback when startup has not measured this host.
// A measured host uses Host.Focus instead. These encoders hold a 720p60
// focused tile. Anything else stays at 540p60.
func hardwareEncoder(encoder string) bool {
	switch encoder {
	case "h264_vaapi", "hevc_vaapi", "h264_qsv", "hevc_qsv", "h264_nvenc", "hevc_nvenc", "h264_videotoolbox", "hevc_videotoolbox":
		return true
	default:
		return false
	}
}

// Decide picks the rendition for one viewer. It never transcodes what the client
// can play as broadcast, unless the viewer asked for less data.
func Decide(src Source, caps Caps, p Prefs) Decision {
	return DecideOn(src, caps, p, "")
}

// DecideOn is Decide with the server encoder, so a focused tile can be 720p60
// on a GPU and 540p60 in software. A zero Host means startup has not measured.
func DecideOn(src Source, caps Caps, p Prefs, encoder string) Decision {
	return DecideFor(src, caps, p, encoder, Host{})
}

// DecideFor is DecideOn with the startup measurement. The host caps the
// transcode and picks the selected tile.
func DecideFor(src Source, caps Caps, p Prefs, encoder string, host Host) Decision {
	v := codecName(src.VideoCodec)
	a := codecName(src.AudioCodec)
	canCopyVideo := has(caps.Video, v) && (src.Progressive || v == "hevc")
	quality := strings.ToLower(p.Quality)
	if quality == "" || quality == "auto" {
		quality = "original"
		if caps.Network == "cellular" || caps.Network == "remote" {
			quality = "medium"
		}
	}
	var r Rendition
	var why []string
	switch quality {
	case "medium":
		r.Video = "720"
		why = append(why, "720p to save data")
	case "saver":
		r.Video = "540"
		why = append(why, "Data saver")
	case "tile":
		if host.Focus == "360" {
			r.Video = "360"
			why = append(why, "360p tile")
		} else {
			r.Video = "540"
			why = append(why, "540p tile")
		}
	case "focus":
		// A measured host picks the tile. The screen cap below can still lower it.
		switch {
		case host.Focus != "":
			r.Video = host.Focus
		case !hardwareEncoder(encoder):
			r.Video = "540"
		default:
			r.Video = "720"
		}
	case "360":
		r.Video = "360"
		why = append(why, "360p tile")
	default:
		if canCopyVideo {
			r.Video = "copy"
			why = append(why, "Original picture")
		} else {
			r.Video = "1080"
			switch {
			case v == "mpeg2":
				why = append(why, "Converted from MPEG-2")
			case src.Lace && v == "h264":
				why = append(why, "Deinterlaced for smooth motion")
			default:
				why = append(why, "Converted for this device")
			}
		}
	}
	r.Video, why = capToHost(r.Video, host, why)
	if caps.MaxHeight > 0 && r.Video != "copy" {
		switch {
		case caps.MaxHeight < 480:
			r.Video = "360"
		case caps.MaxHeight < 720:
			r.Video = "540"
		case caps.MaxHeight < 1080 && r.Video == "1080":
			r.Video = "720"
		}
	}
	if quality == "focus" && r.Video != "720" {
		r.FullRate = host.Focus == "" || host.FullRate
	}
	if quality == "focus" {
		switch {
		case r.Video == "720":
			why = append(why, "720p60 tile")
		case r.Video == "360" && r.FullRate:
			why = append(why, "360p60 tile")
		case r.Video == "360":
			why = append(why, "360p tile")
		case r.FullRate:
			why = append(why, "540p60 tile")
		default:
			why = append(why, "540p tile")
		}
	}
	audio := strings.ToLower(p.Audio)
	if (audio == "" || audio == "auto") && (quality == "tile" || quality == "360") {
		audio = "none"
	}
	switch audio {
	case "none":
		r.Audio = "none"
		why = append(why, "silent tile")
	case "stereo":
		r.Audio = "aac2"
		why = append(why, "stereo")
	case "surround":
		if has(caps.Audio, a) {
			r.Audio = "copy"
			why = append(why, "original surround")
		} else {
			r.Audio = "aac6"
			why = append(why, "5.1 AAC")
		}
	default:
		if has(caps.Audio, a) && quality != "saver" {
			r.Audio = "copy"
			why = append(why, "original sound")
		} else {
			r.Audio = "aac2"
			why = append(why, "stereo")
		}
	}
	switch strings.ToLower(p.Track) {
	case "language", "lang", "sap":
		r.Track = "lang"
		why = append(why, "second language")
	case "described", "vi":
		r.Track = "vi"
		why = append(why, "described video")
	}
	if p.Even && r.Audio != "none" {
		r.Even = true
		if r.Audio == "copy" {
			if audio == "stereo" || quality == "saver" {
				r.Audio = "aac2"
			} else {
				r.Audio = "aac6"
			}
		}
		why = append(why, "even volume")
	}
	r.Mode = p.Picture
	if r.Video != "copy" && has(caps.Video, "hevc") {
		r.Codec = "hevc"
		why = append(why, "HEVC")
	}
	r = r.normalized()
	return Decision{Rendition: r, Reason: strings.Join(why, ", ")}
}

// LegacyCaps maps the web player's profile and audio choice from before
// capability negotiation.
func LegacyCaps(profile, audio, picture string) (Caps, Prefs) {
	caps := Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	p := Prefs{Picture: picture}
	switch profile {
	case "balanced":
		p.Quality = "medium"
	case "saver":
		p.Quality = "saver"
	default:
		p.Quality = "original"
	}
	if audio == "surround" {
		p.Audio = "surround"
	} else {
		p.Audio = "stereo"
	}
	return caps, p
}

// capToHost lowers a transcode the bench cannot keep up with.
// A copied broadcast is left alone.
func capToHost(video string, host Host, why []string) (string, []string) {
	if host.Height <= 0 || host.Height >= 1080 || video == "copy" {
		return video, why
	}
	rank := map[string]int{"360": 1, "540": 2, "720": 3, "1080": 4}
	limit := "720"
	if host.Height <= 360 {
		limit = "360"
	} else if host.Height <= 540 {
		limit = "540"
	}
	if rank[video] <= rank[limit] {
		return video, why
	}
	return limit, append(why, limit+"p on this server")
}

func renditionProfile(video string) string {
	switch video {
	case "720":
		return "balanced"
	case "540":
		return "saver"
	case "360":
		return "tile"
	default:
		return "transparent"
	}
}

// openingKeyframes puts a transcode keyframe wherever the source has one.
// A fixed interval drifts off that group of pictures, so a copy and a
// transcode would close their segments at different frames. Counting
// n_forced*2 drifts off the broadcast clock the same way.
const openingKeyframes = "source"

// RenditionArgs builds ffmpeg for one live rendition. Timestamps are kept from
// the broadcast (-copyts). fMP4 still starts each encode at zero, so each
// rendition keeps its own wall clock (see rendition.clock).
func RenditionArgs(program int, src Source, r Rendition, encoder, deint string) []string {
	return renditionArgs(program, src, r, encoder, deint, "pipe:0")
}

func renditionArgs(program int, src Source, r Rendition, encoder, deint string, input string) []string {
	r = r.normalized()
	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "+genpts+discardcorrupt", "-copyts"}
	transcode := r.Video != "copy"
	outEnc := encoder
	if transcode {
		outEnc = OutputEncoder(encoder, r.Codec)
	}
	gpu := false
	if transcode {
		mode := r.Mode
		if src.Film && NormalizeMode(mode) == "broadcast" {
			mode = "film"
		}
		probe := Graph{VideoCodec: src.VideoCodec, Profile: renditionProfile(r.Video), Encoder: outEnc, Mode: mode, Deint: deint, Progressive: src.Progressive, FullRate: r.FullRate}
		inter := fieldDoubled(src.VideoCodec, probe.Mode, src.Progressive, src.Lace)
		gpu = gpuDecode(probe, inter, vaapiDeintMode(probe, inter))
	}
	if transcode && vaapiFamily(outEnc) {
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
		if gpu {
			args = append(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-hwaccel_device", "va")
		}
	}
	if input == "" {
		input = "pipe:0"
	}
	if strings.Contains(input, "://") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
	}
	args = append(args, headerArgs(src.UserAgent, src.Referrer)...)
	// Ceilings, not waits: ffmpeg returns once it has the parameters. A tuner
	// multiplex is tens of megabits, so a 2 MB cap ends before the sequence
	// header. A remote playlist is slower to start and is not a fat mux.
	probeSize, probeFor := "8000000", "1000000"
	if strings.Contains(input, "://") {
		probeSize, probeFor = "2000000", "1500000"
	}
	args = append(args, "-probesize", probeSize, "-analyzeduration", probeFor, "-i", input)
	audioMap := "0:a:0"
	if program > 0 {
		audioMap = fmt.Sprintf("0:p:%d:a:0", program)
	}
	if src.AudioPID > 0 {
		audioMap = fmt.Sprintf("0:i:%d", src.AudioPID)
	}
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d:v:0", program))
	} else {
		args = append(args, "-map", "0:v:0")
	}
	if r.Audio != "none" {
		args = append(args, "-map", audioMap)
	}
	if transcode {
		mode := r.Mode
		if src.Film && NormalizeMode(mode) == "broadcast" {
			mode = "film"
		}
		g := Graph{VideoCodec: src.VideoCodec, Profile: renditionProfile(r.Video), Encoder: outEnc, Mode: mode, Deint: deint, Progressive: src.Progressive, FullRate: r.FullRate}
		interlaced := fieldDoubled(src.VideoCodec, g.Mode, src.Progressive, src.Lace)
		field := interlaced && !smallPicture(g)
		width, height, rate := outputSize(g, field)
		fps, gop := pictureRate(g, field)
		args = append(args, "-vf", videoFilter(g, vaapiDeintMode(g, interlaced), interlaced, field, width, height, fps))
		args = append(args, videoCodec(outEnc, rate, gop)...)
		args = append(args, "-force_key_frames", openingKeyframes)
	} else {
		args = append(args, "-c:v", "copy")
	}
	switch r.Audio {
	case "none":
		args = append(args, "-an")
	case "copy":
		args = append(args, "-c:a", "copy")
		if strings.EqualFold(src.AudioCodec, "AAC") {
			args = append(args, "-bsf:a", "aac_adtstoasc")
		}
	case "aac6":
		args = append(args, "-af", audioFilter(r), "-c:a", "aac", "-ac", "6", "-b:a", "384k")
	default:
		args = append(args, "-af", audioFilter(r), "-c:a", "aac", "-ac", "2", "-b:a", "160k")
	}
	// Fragmented MP4 on stdout, one fragment per keyframe. The packager groups
	// those into segments that start on a keyframe. Copy and transcode share
	// the cut because the transcode's keyframes are the source's.
	// -hls_init_time is not used: on ffmpeg 8 it keeps cutting at the init
	// length until the playlist window fills.
	return append(args,
		"-video_track_timescale", "90000",
		// The defaults hold the first packet for most of a second.
		"-muxdelay", "0", "-muxpreload", "0",
		"-f", "mp4",
		// delay_moov: AC-3 has no frame size until the first packet, and an
		// empty moov written before that packet is rejected.
		"-movflags", "frag_keyframe+empty_moov+default_base_moof+delay_moov",
		"pipe:1",
	)
}

// audioFilter keeps broadcast timestamps and, when asked, levels the mix.
// loudnorm without measured values is one pass, so it can run live.
func audioFilter(r Rendition) string {
	af := "aresample=async=1000"
	if r.Even {
		af += ",loudnorm=I=-16:LRA=11:TP=-1.5"
	}
	return af
}
