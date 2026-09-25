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
	// Channels is the mix width (6 for 5.1). Passthrough prefers the widest main.
	Channels int `json:"-"`
	// Measured is set once an AC-3 frame, not only the PMT, supplied Channels.
	Measured bool `json:"-"`
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
			measureAC3(data, tracks)
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
	channels  int // mix width, 0 if unknown
	measured  bool
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
		bsmod, channels := ac3Meta(desc)
		out = append(out, rawAudio{
			pid: pid, codec: codec, lang: lang, audioType: audioType, bsmod: bsmod, channels: channels,
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

// ac3Meta reads bitstream_mode and mix width from an AC-3 descriptor (tag 0x81).
// The third payload byte is bsmod (top 3 bits) then num_channels (A/52 Annex A).
func ac3Meta(b []byte) (bsmod, channels int) {
	bsmod = -1
	for off := 0; off+2 <= len(b); {
		tag := int(b[off])
		n := int(b[off+1])
		off += 2
		if off+n > len(b) {
			return -1, 0
		}
		body := b[off : off+n]
		off += n
		if tag == descAC3 && len(body) >= 3 {
			return int(body[2] >> 5), ac3ChannelCount(int(body[2]>>1) & 0x0f)
		}
	}
	return -1, 0
}

// ac3ChannelCount maps A/52 num_channels onto a mix width.
// 7 is 3/2 (the 5.1 bed without the LFE bit) and 13 is a service of up to 6 channels.
func ac3ChannelCount(code int) int {
	switch code {
	case 0, 2, 9:
		return 2
	case 1, 8:
		return 1
	case 3, 4, 10:
		return 3
	case 5, 6, 11:
		return 4
	case 7, 12:
		return 5
	case 13:
		return 6
	default:
		return 0
	}
}

// measureAC3 replaces descriptor mix info with the AC-3 frame header.
// A station can copy one descriptor onto every audio stream, so num_channels
// then says both mixes are 5.1. The frame's bsmod and acmod are the mix.
func measureAC3(data []byte, tracks []rawAudio) {
	index := map[int]int{}
	for i, t := range tracks {
		if t.codec == "ac3" {
			index[t.pid] = i
		}
	}
	if len(index) == 0 || len(data) < 188 {
		return
	}
	votes := map[int]map[[2]int]int{}
	for off := 0; off+188 <= len(data); {
		if !tsAligned(data, off) {
			off++
			continue
		}
		pid := int(data[off+1]&0x1f)<<8 | int(data[off+2])
		start := 4
		if data[off+3]&0x10 == 0 {
			off += 188
			continue
		}
		if data[off+3]&0x20 != 0 {
			start += 1 + int(data[off+4])
		}
		if _, ok := index[pid]; ok && start < 188 {
			pay := data[off+start : off+188]
			for i := 0; i+7 <= len(pay); i++ {
				bsmod, channels, ok := ac3Frame(pay[i:])
				if !ok {
					continue
				}
				if votes[pid] == nil {
					votes[pid] = map[[2]int]int{}
				}
				votes[pid][[2]int{bsmod, channels}]++
				i++
			}
		}
		off += 188
	}
	for pid, idx := range index {
		bsmod, channels, n, ok := majorityAC3(votes[pid])
		if !ok || n < 2 {
			continue
		}
		tracks[idx].bsmod = bsmod
		tracks[idx].channels = channels
		tracks[idx].measured = true
	}
}

func majorityAC3(votes map[[2]int]int) (bsmod, channels, n int, ok bool) {
	best, second := 0, 0
	var key [2]int
	for k, c := range votes {
		if c > best {
			second = best
			best = c
			key = k
			continue
		}
		if c > second {
			second = c
		}
	}
	if best < 2 || best <= second {
		return 0, 0, best, false
	}
	return key[0], key[1], best, true
}

func tsAligned(data []byte, off int) bool {
	if off+188 > len(data) || data[off] != 0x47 {
		return false
	}
	// A following packet that is not sync means this 0x47 was payload.
	if off+376 <= len(data) && data[off+188] != 0x47 {
		return false
	}
	return true
}

// ac3Frame reads bsmod and mix width from an AC-3 sync header at p.
// bsid above 10 is E-AC-3 and uses a different header.
func ac3Frame(p []byte) (bsmod, channels int, ok bool) {
	if len(p) < 7 || p[0] != 0x0b || p[1] != 0x77 {
		return 0, 0, false
	}
	bsid := int(p[5] >> 3)
	if bsid == 0 || bsid > 10 {
		return 0, 0, false
	}
	bsmod = int(p[5] & 0x07)
	acmod := int(p[6] >> 5)
	bit := 3
	if acmod&0x01 != 0 && acmod != 0x01 {
		bit += 2
	}
	if acmod&0x04 != 0 {
		bit += 2
	}
	if acmod == 0x02 {
		bit += 2
	}
	lfe := 0
	if bit <= 7 {
		lfe = int(p[6]>>(7-bit)) & 1
	}
	width := [...]int{2, 1, 2, 3, 3, 4, 4, 5}
	return bsmod, width[acmod] + lfe, true
}

// audioReady is false while two mains still share an unmeasured descriptor.
func audioReady(tracks []AudioTrack) bool {
	mains, measured := 0, 0
	for _, t := range tracks {
		if t.Role != "main" {
			continue
		}
		mains++
		if t.Measured {
			measured++
		}
	}
	if mains < 2 {
		return true
	}
	return measured == mains
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
	mainAt := fullestMain(in, primary)
	out := make([]AudioTrack, 0, len(in))
	for i, t := range in {
		role := ""
		switch {
		case describedAudio(t):
			role = "described"
		case i == mainAt:
			role = "main"
		case fullMix(t) && sameMix(t, primary):
			// A second complete main is still a full mix. 41.1's unlabeled
			// extra English stereo is not one of these: it has no bsmod.
			role = "main"
		case t.lang != "" && t.lang != primary:
			role = "language"
		default:
			role = "described"
		}
		out = append(out, AudioTrack{
			PID: t.pid, Language: t.lang, Role: role, Codec: t.codec,
			Label: audioLabel(role, t.lang), Channels: t.channels, Measured: t.measured,
		})
	}
	if mainAt < 0 && len(out) > 0 {
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

func sameMix(t rawAudio, primary string) bool {
	return primary == "" || t.lang == primary || t.lang == ""
}

func describedAudio(t rawAudio) bool {
	return t.bsmod == 2 || t.audioType == 3
}

// fullMix is an AC-3 complete main (bsmod 0). An absent bsmod is not one.
func fullMix(t rawAudio) bool {
	return t.bsmod == 0 && !describedAudio(t)
}

// fullestMain is the complete main passthrough should keep. Among same-language
// complete mains, the widest mix wins, so a stereo bed does not take 5.1's place.
// With no complete main, the first unlabeled main is used.
func fullestMain(in []rawAudio, primary string) int {
	best, bestCh := -1, -1
	for i, t := range in {
		if !fullMix(t) || !sameMix(t, primary) {
			continue
		}
		if best < 0 || t.channels > bestCh {
			best, bestCh = i, t.channels
		}
	}
	if best >= 0 {
		return best
	}
	for i, t := range in {
		if describedAudio(t) || !completeMain(t) || !sameMix(t, primary) {
			continue
		}
		return i
	}
	return -1
}

func completeMain(t rawAudio) bool {
	return t.bsmod <= 0 && !describedAudio(t)
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

// PickTrack returns the track with the requested role.
// role is main, language, or described. An unknown role picks main.
// Main prefers the widest mix, so passthrough keeps a 5.1 complete main
// when a stereo complete main is also present.
func PickTrack(tracks []AudioTrack, role string) (AudioTrack, bool) {
	if role == "" || role == "auto" {
		role = "main"
	}
	found := false
	var best AudioTrack
	for _, t := range tracks {
		if t.Role != role {
			continue
		}
		if !found || (role == "main" && t.Channels > best.Channels) {
			best = t
			found = true
		}
	}
	if found {
		return best, true
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
		if t.Channels > 0 {
			parts[i] = fmt.Sprintf("%s pid %d %s %dch", t.Role, t.PID, t.Label, t.Channels)
			continue
		}
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
