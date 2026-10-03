package live

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A master playlist lists a rendition's picture view once and each of its
// sound tracks as an alternate in one AUDIO group, so AVPlayer's audio menu
// and hls.js's audioTracks switch sound without a new watch.

// trackCodec is an init segment track's RFC 6381 codec, picture size, and
// sound width.
type trackCodec struct {
	codec         string
	width, height int
	channels      int
}

// initCodecs reads each track's codec string from its sample description.
func initCodecs(init []byte) map[uint32]trackCodec {
	out := map[uint32]trackCodec{}
	for _, t := range boxes(child(init, "moov")) {
		if t.kind != "trak" {
			continue
		}
		id, _, ok := trackScale(t.body)
		if !ok {
			continue
		}
		stsd := child(child(child(child(t.body, "mdia"), "minf"), "stbl"), "stsd")
		if len(stsd) < 8 {
			continue
		}
		entries := boxes(stsd[8:])
		if len(entries) == 0 {
			continue
		}
		if c, ok := sampleCodec(entries[0].kind, entries[0].body); ok {
			out[id] = c
		}
	}
	return out
}

func sampleCodec(kind string, entry []byte) (trackCodec, bool) {
	switch kind {
	case "ac-3", "ec-3", "mp4a":
		// An audio sample entry's child boxes follow 28 bytes of fixed fields.
		if len(entry) < 28 {
			return trackCodec{}, false
		}
		c := trackCodec{codec: kind, channels: int(binary.BigEndian.Uint16(entry[16:18]))}
		if kind == "mp4a" {
			// ffmpeg's aac encoder writes low complexity.
			c.codec = "mp4a.40.2"
		}
		// The sample entry says 2 for any AC-3. dac3 has the mix.
		if dac3 := child(entry[28:], "dac3"); kind == "ac-3" && len(dac3) >= 3 {
			v := int(dac3[0])<<16 | int(dac3[1])<<8 | int(dac3[2])
			acmod, lfe := v>>11&7, v>>10&1
			c.channels = [...]int{2, 1, 2, 3, 3, 4, 4, 5}[acmod] + lfe
		}
		return c, true
	case "avc1", "avc3", "hvc1", "hev1":
	default:
		return trackCodec{}, false
	}
	// A visual sample entry's child boxes follow 78 bytes of fixed fields.
	if len(entry) < 78 {
		return trackCodec{}, false
	}
	c := trackCodec{width: int(binary.BigEndian.Uint16(entry[24:26])), height: int(binary.BigEndian.Uint16(entry[26:28]))}
	switch kind {
	case "avc1", "avc3":
		avcC := child(entry[78:], "avcC")
		if len(avcC) < 4 {
			return trackCodec{}, false
		}
		c.codec = fmt.Sprintf("%s.%02X%02X%02X", kind, avcC[1], avcC[2], avcC[3])
	default:
		hvcC := child(entry[78:], "hvcC")
		if len(hvcC) < 13 {
			return trackCodec{}, false
		}
		c.codec = hevcCodec(kind, hvcC)
	}
	return c, true
}

// hevcCodec follows ISO/IEC 14496-15 annex E: profile space and profile,
// the compatibility flags bit-reversed, tier and level, then the constraint
// bytes without trailing zeros.
func hevcCodec(kind string, hvcC []byte) string {
	space := [...]string{"", "A", "B", "C"}[hvcC[1]>>6]
	tier := "L"
	if hvcC[1]&0x20 != 0 {
		tier = "H"
	}
	compat := bits.Reverse32(binary.BigEndian.Uint32(hvcC[2:6]))
	parts := []string{kind, space + strconv.Itoa(int(hvcC[1]&0x1f)), fmt.Sprintf("%X", compat), tier + strconv.Itoa(int(hvcC[12]))}
	constraints := hvcC[6:12]
	end := len(constraints)
	for end > 0 && constraints[end-1] == 0 {
		end--
	}
	for _, b := range constraints[:end] {
		parts = append(parts, fmt.Sprintf("%X", b))
	}
	return strings.Join(parts, ".")
}

// twoLetter maps the ISO 639-2 codes heard on US and Canadian stations to
// their ISO 639-1 tags. A code with no two-letter tag is valid BCP 47 as is.
var twoLetter = map[string]string{
	"eng": "en", "spa": "es", "fra": "fr", "fre": "fr", "deu": "de", "ger": "de",
	"ita": "it", "por": "pt", "rus": "ru", "pol": "pl", "zho": "zh", "chi": "zh",
	"kor": "ko", "jpn": "ja", "vie": "vi", "ara": "ar", "hin": "hi", "tgl": "tl",
	"hat": "ht", "heb": "he", "hun": "hu", "ell": "el", "gre": "el", "nld": "nl",
	"dut": "nl", "ukr": "uk", "tha": "th", "fas": "fa", "per": "fa", "urd": "ur",
}

// bcp47 turns a PMT's ISO 639-2 code into the tag a player matches against
// the device language.
func bcp47(code string) string {
	if tag, ok := twoLetter[code]; ok {
		return tag
	}
	return code
}

// quoted is an HLS quoted string. It has no escapes, so quotes and line
// breaks are dropped.
func quoted(s string) string {
	return `"` + strings.Map(func(r rune) rune {
		if r == '"' || r < 0x20 {
			return -1
		}
		return r
	}, s) + `"`
}

// renditionBandwidth is the peak segment rate of a rendition's picture:
// twice the encoder's target. The rate holds over two seconds, so a short
// segment that opens on a keyframe runs far above it (1080 from a 1080i
// channel: median 14.2, max 26.8 Mb/s per segment). AVPlayer flags every
// segment over BANDWIDTH. A copy is the broadcast, under 20 Mb/s.
func renditionBandwidth(r Rendition) int {
	if r.normalized().Video == "copy" {
		return 20_000_000
	}
	return 2 * renditionRate(r)
}

// renditionRate is the encoder's target for a rendition's picture, at the
// field-rate size where that size differs.
func renditionRate(r Rendition) int {
	r = r.normalized()
	if r.Video == "copy" {
		return 0
	}
	_, _, rate := outputSize(Graph{Profile: renditionProfile(r.Video), FullRate: r.FullRate}, true)
	return bitsPerSecond(rate)
}

// bitsPerSecond reads an ffmpeg rate such as 14M or 1200k.
func bitsPerSecond(rate string) int {
	scale := 1
	if n, ok := strings.CutSuffix(rate, "M"); ok {
		rate, scale = n, 1_000_000
	} else if n, ok := strings.CutSuffix(rate, "k"); ok {
		rate, scale = n, 1_000
	}
	v, _ := strconv.Atoi(rate)
	return v * scale
}

// streamRates is BANDWIDTH and, for an encode, AVERAGE-BANDWIDTH.
func streamRates(r Rendition, sound int) string {
	s := fmt.Sprintf("BANDWIDTH=%d", renditionBandwidth(r)+sound)
	if avg := renditionRate(r); avg > 0 {
		s += fmt.Sprintf(",AVERAGE-BANDWIDTH=%d", avg+sound)
	}
	return s
}

// masterPlaylist lists video.m3u8 and one audio-<id>.m3u8 per sound track.
// tracks[0] is the main mix. A copied track in another codec than the main is
// left out: one variant names one sound codec.
func masterPlaylist(r Rendition, codecs map[uint32]trackCodec, tracks []AudioTrack, captions bool) ([]byte, error) {
	r = r.normalized()
	video, ok := codecs[1]
	if !ok || r.Audio == "none" || len(tracks) == 0 {
		return nil, os.ErrNotExist
	}
	sound, ok := codecs[2]
	if !ok {
		return nil, os.ErrNotExist
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	names := map[string]bool{}
	seen := map[string]bool{}
	for i, t := range tracks {
		id := uint32(2)
		if i > 0 {
			id = extraTrackID(i - 1)
		}
		if c, ok := codecs[id]; !ok || c.codec != sound.codec {
			continue
		}
		base := t.Label
		if base == "" {
			base = "Main"
		}
		name := base
		for n := 2; names[name]; n++ {
			name = fmt.Sprintf("%s %d", base, n)
		}
		names[name] = true
		attrs := []string{"TYPE=AUDIO", `GROUP-ID="aud"`, "NAME=" + quoted(name)}
		if t.Language != "" {
			attrs = append(attrs, "LANGUAGE="+quoted(bcp47(t.Language)))
		}
		def := "NO"
		if i == 0 {
			def = "YES"
		}
		// A player picks among automatic tracks by language and role, so a
		// second track with the same pair is only picked by hand.
		pair := t.Language + "/" + t.Role
		auto := "YES"
		if seen[pair] {
			auto = "NO"
		}
		seen[pair] = true
		attrs = append(attrs, "DEFAULT="+def, "AUTOSELECT="+auto)
		if ch := trackChannels(r, t, i == 0, codecs[id].channels); ch > 0 {
			attrs = append(attrs, fmt.Sprintf(`CHANNELS="%d"`, ch))
		}
		if t.Role == "described" {
			attrs = append(attrs, `CHARACTERISTICS="public.accessibility.describes-video"`)
		}
		attrs = append(attrs, fmt.Sprintf(`URI="audio-%d.m3u8"`, id))
		b.WriteString("#EXT-X-MEDIA:" + strings.Join(attrs, ",") + "\n")
	}
	// A player plays the picture and one sound track at a time.
	soundRate := 160_000
	switch r.Audio {
	case "copy", "ac3":
		soundRate = 448_000
	case "aac6":
		soundRate = 384_000
	}
	inf := []string{streamRates(r, soundRate), fmt.Sprintf(`CODECS="%s,%s"`, video.codec, sound.codec)}
	if video.width > 0 && video.height > 0 {
		inf = append(inf, fmt.Sprintf("RESOLUTION=%dx%d", video.width, video.height))
	}
	inf = append(inf, `AUDIO="aud"`)
	if captions {
		b.WriteString(captionMedia)
		inf = append(inf, `SUBTITLES="cc"`)
	}
	inf = append(inf, "CLOSED-CAPTIONS=NONE")
	b.WriteString("#EXT-X-STREAM-INF:" + strings.Join(inf, ",") + "\nvideo.m3u8\n")
	return []byte(b.String()), nil
}

// trackChannels is the width a sound track is sent at. A copied track the
// PMT did not measure takes the init's width.
func trackChannels(r Rendition, t AudioTrack, main bool, sent int) int {
	switch r.Audio {
	case "aac2":
		return 2
	case "aac6":
		if main || t.Channels == 0 || t.Channels > 2 {
			return 6
		}
	}
	if t.Channels == 0 {
		return sent
	}
	return t.Channels
}

// mainTrackLocked is the track an encode maps as its main, as sourceFor
// chooses it. Before the PMT, that is the program's first audio stream.
func mainTrackLocked(f *feed, spec Rendition) AudioTrack {
	if len(f.tracks) == 0 {
		if len(f.stored) > 0 {
			return f.stored[0]
		}
		return AudioTrack{Label: "Main"}
	}
	role := map[string]string{"lang": "language", "vi": "described"}[spec.normalized().Track]
	if t, ok := PickTrack(f.tracks, role); ok {
		return t
	}
	return f.tracks[0]
}

// MasterPath is the master playlist of a running rendition that carries
// other sound tracks. A rendition with one sound has nothing to switch.
func (h *Hub) MasterPath(channelID int64, key string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil || f.renditions[key] == nil || len(f.renditions[key].extras) == 0 {
		return "", false
	}
	return fmt.Sprintf("/media/live/%d/%s/master.m3u8", channelID, key), true
}

// MasterPlaylist is master.m3u8 for a rendition: its picture view and each
// of its sound tracks, named from the PMT. An encode started again after the
// watch answered (a late scan-type result) has no header until its first
// segment, so this waits up to masterWait for it.
func (h *Hub) MasterPlaylist(ctx context.Context, channelID int64, key string) ([]byte, error) {
	deadline := time.Now().Add(masterWait)
	for {
		body, gate, err := h.masterOnce(channelID, key)
		if !errors.Is(err, os.ErrNotExist) || gate == nil || ctx.Err() != nil || !time.Now().Before(deadline) {
			return body, err
		}
		started := time.Now()
		gate.wait(0, -1, min(250*time.Millisecond, time.Until(deadline)))
		// A gate already past segment 0 with no header yet would spin.
		if time.Since(started) < 50*time.Millisecond {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

const masterWait = 8 * time.Second

func (h *Hub) masterOnce(channelID int64, key string) ([]byte, *playlistGate, error) {
	h.mu.Lock()
	f := h.channels[channelID]
	if f == nil || f.renditions[key] == nil {
		h.mu.Unlock()
		return nil, nil, os.ErrNotExist
	}
	r := f.renditions[key]
	spec, dir, captions, gate := r.spec, r.dir, f.captions != nil, r.gate
	tracks := append([]AudioTrack{mainTrackLocked(f, spec)}, r.extras...)
	h.mu.Unlock()
	init, err := os.ReadFile(filepath.Join(dir, "init.mp4"))
	if err != nil {
		return nil, gate, err
	}
	body, err := masterPlaylist(spec, initCodecs(init), tracks, captions)
	return body, gate, err
}
