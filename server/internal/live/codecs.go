package live

import (
	"bytes"
	"context"
	"strings"

	"broadwave/internal/store"
)

const streamHEVC = 0x24

// VideoCodecOf names the program's video from its PMT in the lineup's words:
// MPEG2, H264, or HEVC. Empty means the PMT is not in the buffer yet or the
// video is none of those.
func VideoCodecOf(data []byte, program int) string {
	if i := bytes.IndexByte(data, 0x47); i > 0 && i < 188 {
		data = data[i:]
	}
	pmtPID := 0
	for _, sec := range sections(data, 0) {
		if len(sec) < 12 || sec[0] != 0x00 {
			continue
		}
		if pmtPID = patPMT(sec, program); pmtPID != 0 {
			break
		}
	}
	if pmtPID == 0 {
		return ""
	}
	for _, sec := range sections(data, pmtPID) {
		if len(sec) < 12 || sec[0] != 0x02 {
			continue
		}
		info := int(sec[10]&0x0f)<<8 | int(sec[11])
		end := sectionEnd(sec)
		for off := 12 + info; off+5 <= end; {
			streamType := int(sec[off])
			esInfo := int(sec[off+3]&0x0f)<<8 | int(sec[off+4])
			off += 5 + esInfo
			switch streamType {
			case streamMPEG2:
				return "MPEG2"
			case streamH264:
				return "H264"
			case streamHEVC:
				return "HEVC"
			}
		}
	}
	return ""
}

// lineupCodec maps a PMT or ffprobe codec name to the HDHomeRun lineup's
// words. A codec the lineup has no word for is empty, which is unknown.
func lineupCodec(raw string) string {
	switch codecName(raw) {
	case "mpeg2":
		return "MPEG2"
	case "h264":
		return "H264"
	case "hevc":
		return "HEVC"
	case "ac3":
		return "AC3"
	case "eac3":
		return "EAC3"
	case "aac":
		return "AAC"
	case "mp2":
		return "MP2"
	case "ac4":
		return "AC4"
	}
	return ""
}

// linkChannel is a URL or playlist channel. A tuner's lineup names its codecs
// and stays the record. A link's playlist rarely does, so the stream is.
func linkChannel(ch store.SourceChannel) bool {
	return strings.HasPrefix(ch.DeviceID, "src-")
}

func mainCodec(tracks []AudioTrack) string {
	if t, ok := PickTrack(tracks, "main"); ok {
		return t.Codec
	}
	return ""
}

// learnCodecsLocked puts the codecs a tune read on a link feed and stores
// them for the next stream decision. A rendition already running keeps the
// decision it was made with. changed says the feed's source moved, so a
// transcode graph may need a rebuild.
func (h *Hub) learnCodecsLocked(f *feed, video, audio string) (changed bool) {
	if f == nil || !linkChannel(f.channel) {
		return false
	}
	video, audio = lineupCodec(video), lineupCodec(audio)
	if video != "" && video != f.source.VideoCodec {
		f.source.VideoCodec = video
		changed = true
	}
	if audio != "" && audio != f.source.AudioCodec {
		f.source.AudioCodec = audio
		changed = true
	}
	var saveVideo, saveAudio string
	if video != "" && video != f.channel.VideoCodec {
		f.channel.VideoCodec = video
		saveVideo = video
	}
	if audio != "" && audio != f.channel.AudioCodec {
		f.channel.AudioCodec = audio
		saveAudio = audio
	}
	if (saveVideo != "" || saveAudio != "") && h.Store != nil {
		id := f.channel.ID
		go func() { _ = h.Store.SetChannelCodecs(context.Background(), id, saveVideo, saveAudio) }()
	}
	return changed
}

// saveCodecs stores what a probe read after its feed was gone.
func (h *Hub) saveCodecs(ch store.SourceChannel, video, audio string) {
	if h.Store == nil || !linkChannel(ch) {
		return
	}
	video, audio = lineupCodec(video), lineupCodec(audio)
	if video == ch.VideoCodec {
		video = ""
	}
	if audio == ch.AudioCodec {
		audio = ""
	}
	if video != "" || audio != "" {
		_ = h.Store.SetChannelCodecs(context.Background(), ch.ID, video, audio)
	}
}
