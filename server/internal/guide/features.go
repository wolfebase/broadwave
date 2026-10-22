package guide

import "strings"

// Picture and sound kinds a listing carries. Empty is unknown.
const (
	VideoSD  = "sd"
	VideoHD  = "hd"
	VideoUHD = "uhd"

	AudioMono     = "mono"
	AudioStereo   = "stereo"
	AudioSurround = "surround"
)

// VideoQuality reads a listing's picture: XMLTV <quality> ("HDTV", "SDTV",
// "UHDTV", "1080i") or a Schedules Direct video property ("hdtv", "uhdtv").
func VideoQuality(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case v == "":
		return ""
	case strings.Contains(v, "uhd") || strings.Contains(v, "4k") || strings.Contains(v, "2160"):
		return VideoUHD
	case strings.Contains(v, "hd") || strings.Contains(v, "1080") || strings.Contains(v, "720"):
		return VideoHD
	case strings.Contains(v, "sd") || strings.Contains(v, "480") || strings.Contains(v, "576"):
		return VideoSD
	}
	return ""
}

// AudioLayout reads a listing's sound: XMLTV <stereo> ("mono", "stereo",
// "dolby", "dolby digital", "surround", "bilingual") or a Schedules Direct
// audio property ("stereo", "dd 5.1", "Dolby"). Dolby is surround.
func AudioLayout(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case v == "":
		return ""
	case strings.Contains(v, "5.1") || strings.Contains(v, "7.1") || strings.Contains(v, "surround") ||
		strings.Contains(v, "dolby") || strings.Contains(v, "atmos") || v == "dd":
		return AudioSurround
	case strings.Contains(v, "stereo") || v == "bilingual":
		return AudioStereo
	case strings.Contains(v, "mono"):
		return AudioMono
	}
	return ""
}

// richerAudio keeps the fuller of two layouts: a listing with stereo and
// 5.1 tracks is surround.
func richerAudio(a, b string) string {
	rank := map[string]int{"": 0, AudioMono: 1, AudioStereo: 2, AudioSurround: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// richerVideo keeps the larger of two pictures.
func richerVideo(a, b string) string {
	rank := map[string]int{"": 0, VideoSD: 1, VideoHD: 2, VideoUHD: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
