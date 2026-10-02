package live

import (
	"fmt"
	"slices"
	"strconv"
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
	case "copy", "aac2", "aac6", "ac3", "none":
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
	// Alternates is true when the player switches sound tracks in place from
	// a master playlist instead of asking for another watch.
	Alternates bool `json:"alternates,omitempty"`
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
	// HD is the lineup's flag. Broadcasts rarely tag their colors, and a
	// browser guesses from the picture size, so a 640 wide tile of an HD
	// channel decodes as SD colors unless the encode says BT.709.
	HD bool
	// Extras are the program's other measured sound tracks, carried in the
	// same encode after the main one so players can switch without a new
	// watch. Tiles never get them.
	Extras []AudioTrack
	// AudioChannels is the measured width of the main sound, 0 when unknown.
	AudioChannels int
	// Height is the picture height the last tune read, 0 when unknown.
	Height int
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
	// A player that can't decode the broadcast's size gets it scaled, not sent.
	tooTall := caps.MaxHeight > 0 && src.Height > caps.MaxHeight
	if tooTall {
		canCopyVideo = false
	}
	// An HEVC picture up to 1080 is already small enough for a tile on a
	// player that decodes HEVC. Sending it as is costs nothing; a new encode
	// of 10-bit HEVC costs more than two cores.
	copyTile := canCopyVideo && v == "hevc" && src.Height > 0 && src.Height <= 1080
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
		if copyTile {
			r.Video = "copy"
			why = append(why, "Original picture tile")
		} else if host.Focus == "360" {
			r.Video = "360"
			why = append(why, "360p tile")
		} else {
			r.Video = "540"
			why = append(why, "540p tile")
		}
	case "focus":
		// A measured host picks the tile. The screen cap below can still lower it.
		switch {
		case copyTile:
			r.Video = "copy"
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
			case tooTall:
				why = append(why, "Scaled to fit this device")
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
	if quality == "focus" && r.Video != "720" && r.Video != "copy" {
		r.FullRate = host.Focus == "" || host.FullRate
	}
	if quality == "focus" {
		switch {
		case r.Video == "copy":
			why = append(why, "Original picture tile")
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
		switch {
		case has(caps.Audio, a):
			r.Audio = "copy"
			why = append(why, "original surround")
		case surroundAC4(a, src, caps):
			r.Audio = "ac3"
			why = append(why, "5.1 AC-3")
		default:
			r.Audio = "aac6"
			why = append(why, "5.1 AAC")
		}
	default:
		switch {
		case has(caps.Audio, a) && quality != "saver":
			r.Audio = "copy"
			why = append(why, "original sound")
		case surroundAC4(a, src, caps) && quality != "saver":
			r.Audio = "ac3"
			why = append(why, "5.1 AC-3")
		default:
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
	// A browser that lists HEVC plays it as sent; its conversions stay H.264,
	// which every browser decodes in hardware.
	if r.Video != "copy" && has(caps.Video, "hevc") && caps.Platform != "web" {
		r.Codec = "hevc"
		why = append(why, "HEVC")
	}
	r = r.normalized()
	return Decision{Rendition: r, Reason: strings.Join(why, ", ")}
}

// surroundAC4 is an ATSC 3.0 5.1 mix for a player that plays AC-3 but not
// AC-4, which is every player today.
func surroundAC4(codec string, src Source, caps Caps) bool {
	return codec == "ac4" && src.AudioChannels >= 6 && has(caps.Audio, "ac3")
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

// sourceKeyint is only a ceiling. -g still inserts an IDR when it is shorter
// than the source group of pictures, and the copy of that broadcast does not
// have that frame. Ten seconds at 60 fps is past a broadcast group.
const sourceKeyint = 600

// RenditionArgs builds ffmpeg for one live rendition. Timestamps are kept from
// the broadcast (-copyts). fMP4 still starts each encode near zero (Pack
// moves each track's start out of the edit list and into its fragments), so
// each rendition keeps its own clock, seeded from the channel's other encodes
// (see seedClockLocked).
func RenditionArgs(program int, src Source, r Rendition, encoder, deint string) []string {
	return renditionArgs(program, src, r, encoder, deint, "pipe:0")
}

// extraTrackID is the fMP4 track id of Source.Extras[i]. ffmpeg numbers the
// tracks by output stream from 1: the picture, the main sound, then the extras.
func extraTrackID(i int) uint32 { return uint32(i) + 3 }

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
	// A tuner multiplex is tens of megabits, so a 2 MB cap ends before the
	// sequence header. The program pipe starts a program on a sequence header
	// with its tables, so half a second holds every stream's parameters; at
	// 1 s a new tune on a 1080i channel waited ~0.4 s more. A stream that can
	// start mid-group, with its sound up to a second ahead of the picture,
	// keeps the second. A remote playlist is slower to start and is not a fat mux.
	probeSize, probeFor := "8000000", "1000000"
	if program > 0 && input == "pipe:0" {
		probeFor = "500000"
	}
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
		for _, t := range src.Extras {
			args = append(args, "-map", fmt.Sprintf("0:i:%d", t.PID))
		}
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
		vf := videoFilter(g, vaapiDeintMode(g, interlaced), interlaced, field, width, height, fps)
		if src.HD {
			// Tagging the frames first keeps ffmpeg from converting an
			// untagged picture it would assume is BT.601.
			vf = "setparams=colorspace=bt709:color_primaries=bt709:color_trc=bt709," + vf
		}
		args = append(args, "-vf", vf)
		// A graph that drops frames (a film pulldown, a half-rate tile) can drop
		// the frame that carried the source keyframe, and the segment then runs
		// until one survives: 20 s on a 60p channel, a 63 s hold-back. Four
		// seconds still keeps a broadcast group whole where nothing is dropped.
		keyint := sourceKeyint
		if fps != "" && !field {
			keyint = 2 * gop
		}
		args = append(args, videoCodec(outEnc, rate, keyint)...)
		if src.HD {
			args = append(args, "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709")
		}
		args = append(args, "-force_key_frames", openingKeyframes)
	} else {
		args = append(args, "-c:v", "copy")
		// ffmpeg tags copied HEVC hev1; Apple players only take hvc1.
		if codecName(src.VideoCodec) == "hevc" {
			args = append(args, "-tag:v", "hvc1")
		}
	}
	switch r.Audio {
	case "none":
		args = append(args, "-an")
	case "copy":
		args = append(args, "-c:a", "copy")
		if len(src.Extras) == 0 {
			if strings.EqualFold(src.AudioCodec, "AAC") {
				args = append(args, "-bsf:a", "aac_adtstoasc")
			}
			break
		}
		codecs := []string{src.AudioCodec}
		for _, t := range src.Extras {
			codecs = append(codecs, t.Codec)
		}
		for i, c := range codecs {
			if strings.EqualFold(c, "AAC") {
				args = append(args, fmt.Sprintf("-bsf:a:%d", i), "aac_adtstoasc")
			}
		}
	case "aac6":
		args = append(args, "-af", audioFilter(r), "-c:a", "aac", "-ac", "6", "-b:a", "384k")
		// A stereo or mono second language stays as wide as it was sent.
		for i, t := range src.Extras {
			if t.Channels > 0 && t.Channels <= 2 {
				args = append(args, fmt.Sprintf("-ac:a:%d", i+1), strconv.Itoa(t.Channels), fmt.Sprintf("-b:a:%d", i+1), "160k")
			}
		}
	case "ac3":
		// AC-3 keeps a 5.1 mix as sent; a stereo second language stays stereo.
		args = append(args, "-af", audioFilter(r), "-c:a", "ac3", "-b:a", "448k")
		for i, t := range src.Extras {
			if t.Channels > 0 && t.Channels <= 2 {
				args = append(args, fmt.Sprintf("-b:a:%d", i+1), "192k")
			}
		}
	default:
		args = append(args, "-af", audioFilter(r), "-c:a", "aac", "-ac", "2", "-b:a", "160k")
	}
	if len(src.Extras) > 0 && r.Audio != "none" {
		// A quiet track otherwise lets the muxer hold the picture for up to
		// ten seconds while it waits for that track's next packet.
		args = append(args, "-max_interleave_delta", "1000000")
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
// min_hard_comp is in seconds and sits above a 33-bit wrap (about 95443s).
// A smaller value makes the resampler allocate the whole gap. async=1000
// still stretches a small drift.
func audioFilter(r Rendition) string {
	af := "aresample=async=1000:min_hard_comp=1000000"
	if r.Even {
		af += ",loudnorm=I=-16:LRA=11:TP=-1.5"
	}
	return af
}
