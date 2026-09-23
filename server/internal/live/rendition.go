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
	} else {
		r.Mode = NormalizeMode(r.Mode)
	}
	return r
}

// Key names the rendition on disk and in URLs.
func (r Rendition) Key() string {
	r = r.normalized()
	if r.Mode == "" {
		return r.Video + "." + r.Audio
	}
	return r.Video + "." + r.Audio + "." + r.Mode
}

func ParseRenditionKey(key string) (Rendition, bool) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Rendition{}, false
	}
	r := Rendition{Video: parts[0], Audio: parts[1]}
	if len(parts) == 3 {
		r.Mode = parts[2]
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
	Quality string `json:"quality,omitempty"` // auto, original, high, medium, saver, tile, 360
	Audio   string `json:"audio,omitempty"`   // auto, surround, stereo, none
	Picture string `json:"picture,omitempty"` // broadcast, smooth, film
}

// Source describes the broadcast as far as the server knows it.
type Source struct {
	VideoCodec string
	AudioCodec string
	// Progressive is true only once a probe has shown the picture is not interlaced.
	Progressive bool
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

// Decide picks the rendition for one viewer. It never transcodes what the client
// can play as broadcast, unless the viewer asked for less data.
func Decide(src Source, caps Caps, p Prefs) Decision {
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
		r.Video = "540"
		why = append(why, "540p tile")
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
			case !src.Progressive && v == "h264":
				why = append(why, "Deinterlaced for smooth motion")
			default:
				why = append(why, "Converted for this device")
			}
		}
	}
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
	r.Mode = p.Picture
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

// RenditionArgs builds ffmpeg for one live rendition. Timestamps are kept from
// the broadcast (-copyts) so every rendition of a channel shares one timeline.
func RenditionArgs(program int, src Source, r Rendition, encoder, deint string, blend bool) []string {
	r = r.normalized()
	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "+genpts+discardcorrupt", "-copyts"}
	transcode := r.Video != "copy"
	if transcode && encoder == "h264_vaapi" {
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
	}
	args = append(args, "-probesize", "2000000", "-analyzeduration", "1500000", "-i", "pipe:0")
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d:v:0", program))
		if r.Audio != "none" {
			args = append(args, "-map", fmt.Sprintf("0:p:%d:a:0", program))
		}
	} else {
		args = append(args, "-map", "0:v:0")
		if r.Audio != "none" {
			args = append(args, "-map", "0:a:0")
		}
	}
	if transcode {
		g := Graph{VideoCodec: src.VideoCodec, Profile: renditionProfile(r.Video), Encoder: encoder, Mode: r.Mode, Deint: deint, Blend: blend}
		interlaced := (InterlacedCodec(src.VideoCodec) || (!src.Progressive && codecName(src.VideoCodec) == "h264")) && g.Mode != "film"
		field := interlaced && !smallPicture(g.Profile)
		width, height, rate := pictureSize(g.Profile, field)
		fps, gop := pictureRate(g, field)
		args = append(args, "-vf", videoFilter(g, vaapiDeintMode(g, interlaced), interlaced, field, width, height, fps))
		args = append(args, videoCodec(encoder, rate, gop)...)
		args = append(args, "-force_key_frames", "expr:if(isnan(prev_forced_t),1,gte(t,prev_forced_t+2))")
	} else {
		args = append(args, "-c:v", "copy")
	}
	switch r.Audio {
	case "none":
		args = append(args, "-an")
	case "copy":
		args = append(args, "-c:a", "copy")
	case "aac6":
		args = append(args, "-af", "aresample=async=1000", "-c:a", "aac", "-ac", "6", "-b:a", "384k")
	default:
		args = append(args, "-af", "aresample=async=1000", "-c:a", "aac", "-ac", "2", "-b:a", "160k")
	}
	// CMAF (fMP4) segments: players read timing straight from the boxes with no
	// transmuxing, Apple devices need it for HEVC, and it is the base for LL-HLS.
	return append(args,
		"-video_track_timescale", "90000",
		"-f", "hls", "-hls_time", "2",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", "seg%05d.m4s",
		"-hls_list_size", "2700",
		"-hls_flags", "delete_segments+independent_segments+omit_endlist",
		"index.m3u8",
	)
}
