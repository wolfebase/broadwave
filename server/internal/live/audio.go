package live

import (
	"fmt"
	"strings"
)

// Audio tracks come from the PMT, not from the order ffmpeg would pick.
// ISO-639 (descriptor 0x0A) gives the language. The AC-3 descriptor (0x81)
// bsmod says whether the track is the main mix or described video.

const (
	streamAC3  = 0x81
	streamEAC3 = 0x87
	streamAAC  = 0x0f
	streamLATM = 0x11
	streamMP3  = 0x04
	streamMP2  = 0x03
	streamPriv = 0x06

	descISO639 = 0x0A
	descAC3    = 0x81
	descEAC3   = 0x7A
)

// AudioTrack is one elementary audio stream on a program.
type AudioTrack struct {
	PID      int    `json:"pid"`
	Language string `json:"language,omitempty"`
	Role     string `json:"role"` // main, language, described
	Codec    string `json:"codec"`
	Label    string `json:"label"`
}

// AudioTracks reads audio elementary streams for one program.
// An empty result means the PMT was not in the buffer yet.
func AudioTracks(data []byte, program int) []AudioTrack {
	pmtPID := 0
	for _, sec := range sections(data, 0) {
		if len(sec) < 12 || sec[0] != 0x00 {
			continue
		}
		pmtPID = patPMT(sec, program)
		if pmtPID != 0 {
			break
		}
	}
	if pmtPID == 0 {
		return nil
	}
	for _, sec := range sections(data, pmtPID) {
		if len(sec) < 12 || sec[0] != 0x02 {
			continue
		}
		if tracks := pmtAudio(sec); len(tracks) > 0 {
			return assignRoles(tracks)
		}
	}
	return nil
}

type rawAudio struct {
	pid       int
	codec     string
	lang      string
	audioType int // ISO-639 audio_type, -1 if absent
	bsmod     int // AC-3 bitstream mode, -1 if absent
}

func pmtAudio(sec []byte) []rawAudio {
	info := int(sec[10]&0x0f)<<8 | int(sec[11])
	off := 12 + info
	end := sectionEnd(sec)
	var out []rawAudio
	for off+5 <= end {
		streamType := int(sec[off])
		pid := int(sec[off+1]&0x1f)<<8 | int(sec[off+2])
		esLen := int(sec[off+3]&0x0f)<<8 | int(sec[off+4])
		es := off + 5
		if es+esLen > end {
			break
		}
		desc := sec[es : es+esLen]
		off = es + esLen
		codec, ok := audioCodecOf(streamType, desc)
		if !ok {
			continue
		}
		lang, audioType := iso639(desc)
		out = append(out, rawAudio{
			pid: pid, codec: codec, lang: lang, audioType: audioType, bsmod: ac3BSMod(desc),
		})
	}
	return out
}

func audioCodecOf(streamType int, desc []byte) (string, bool) {
	switch streamType {
	case streamAC3:
		return "ac3", true
	case streamEAC3:
		return "eac3", true
	case streamAAC, streamLATM:
		return "aac", true
	case streamMP2, streamMP3:
		return "mp2", true
	case streamPriv:
		if hasDesc(desc, descEAC3) {
			return "eac3", true
		}
		if hasDesc(desc, descAC3) {
			return "ac3", true
		}
	}
	return "", false
}

func hasDesc(b []byte, tag int) bool {
	for off := 0; off+2 <= len(b); {
		t := int(b[off])
		n := int(b[off+1])
		off += 2
		if off+n > len(b) {
			return false
		}
		if t == tag {
			return true
		}
		off += n
	}
	return false
}

func iso639(b []byte) (lang string, audioType int) {
	audioType = -1
	for off := 0; off+2 <= len(b); {
		tag := int(b[off])
		n := int(b[off+1])
		off += 2
		if off+n > len(b) {
			return "", -1
		}
		body := b[off : off+n]
		off += n
		if tag != descISO639 || len(body) < 4 {
			continue
		}
		lang = string(body[0:3])
		audioType = int(body[3])
		return lang, audioType
	}
	return "", -1
}

// ac3BSMod reads bitstream_mode from an AC-3 descriptor (tag 0x81).
// The third payload byte holds bsmod in the top three bits (A/52 Annex A).
func ac3BSMod(b []byte) int {
	for off := 0; off+2 <= len(b); {
		tag := int(b[off])
		n := int(b[off+1])
		off += 2
		if off+n > len(b) {
			return -1
		}
		body := b[off : off+n]
		off += n
		if tag == descAC3 && len(body) >= 3 {
			return int(body[2] >> 5)
		}
	}
	return -1
}

func assignRoles(in []rawAudio) []AudioTrack {
	primary := ""
	for _, t := range in {
		if completeMain(t) && t.lang == "eng" {
			primary = "eng"
			break
		}
	}
	if primary == "" {
		for _, t := range in {
			if completeMain(t) {
				primary = t.lang
				break
			}
		}
	}
	mainSet := false
	out := make([]AudioTrack, 0, len(in))
	for _, t := range in {
		role := ""
		switch {
		case t.bsmod == 2 || t.audioType == 3:
			role = "described"
		case !mainSet && completeMain(t) && (primary == "" || t.lang == primary || t.lang == ""):
			role = "main"
			mainSet = true
		case t.lang != "" && t.lang != primary:
			role = "language"
		default:
			// A second mix in the same language is the described-video track
			// when the broadcast did not set bsmod (41.1's extra English stereo).
			role = "described"
		}
		out = append(out, AudioTrack{
			PID: t.pid, Language: t.lang, Role: role, Codec: t.codec, Label: audioLabel(role, t.lang),
		})
	}
	if !mainSet && len(out) > 0 {
		for i := range out {
			if out[i].Role != "described" {
				out[i].Role = "main"
				out[i].Label = audioLabel("main", out[i].Language)
				break
			}
		}
	}
	return out
}

func completeMain(t rawAudio) bool {
	return t.bsmod <= 0
}

func audioLabel(role, lang string) string {
	name := languageName(lang)
	switch role {
	case "language":
		if name != "" {
			return name
		}
		return "Second language"
	case "described":
		return "Described video"
	default:
		if name != "" {
			return name
		}
		return "Main"
	}
}

func languageName(code string) string {
	switch code {
	case "eng":
		return "English"
	case "spa":
		return "Spanish"
	case "fra", "fre":
		return "French"
	case "deu", "ger":
		return "German"
	case "zho", "chi":
		return "Chinese"
	case "kor":
		return "Korean"
	case "jpn":
		return "Japanese"
	default:
		return ""
	}
}

// PickTrack returns the first track with the requested role.
// role is main, language, or described. An unknown role picks main.
func PickTrack(tracks []AudioTrack, role string) (AudioTrack, bool) {
	if role == "" || role == "auto" {
		role = "main"
	}
	for _, t := range tracks {
		if t.Role == role {
			return t, true
		}
	}
	if role != "main" {
		return AudioTrack{}, false
	}
	if len(tracks) > 0 {
		return tracks[0], true
	}
	return AudioTrack{}, false
}

func trackLog(tracks []AudioTrack) string {
	parts := make([]string, len(tracks))
	for i, t := range tracks {
		parts[i] = fmt.Sprintf("%s pid %d %s", t.Role, t.PID, t.Label)
	}
	return strings.Join(parts, ", ")
}

func (f *feed) sourceFor(want Rendition) Source {
	src := f.source
	role := "main"
	switch want.Track {
	case "lang":
		role = "language"
	case "vi":
		role = "described"
	}
	if t, ok := PickTrack(f.tracks, role); ok {
		src.AudioPID = t.PID
		if t.Codec != "" {
			src.AudioCodec = strings.ToUpper(t.Codec)
		}
	}
	return src
}
